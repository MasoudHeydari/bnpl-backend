SET search_path TO bnpl;

-- -----------------------------------------------------------------------------
-- Migrations
-- -----------------------------------------------------------------------------
-- Tracks applied database schema migrations.
--
-- Each successfully executed migration is recorded exactly once and used to
-- determine which migrations still need to be applied.
CREATE TABLE IF NOT EXISTS migrations (
    -- Monotonically increasing migration record identifier.
    id BIGSERIAL PRIMARY KEY,

    -- Unique migration name.
    --
    -- Example:
    --   000001_create_contracts_table.sql
    name TEXT NOT NULL UNIQUE,

    -- Immutable digest of the applied migration contents.
    checksum TEXT NOT NULL CHECK (checksum ~ '^sha256:[0-9a-f]{64}$'),

    -- True only while a non-transactional migration is unresolved.
    -- A dirty row blocks automatic re-execution until an operator verifies the
    -- actual database state and resolves the migration deliberately.
    dirty BOOLEAN NOT NULL DEFAULT FALSE,

    -- Time at which the migration was successfully applied or started.
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- -----------------------------------------------------------------------------
-- Integration Outbox
-- -----------------------------------------------------------------------------
-- Stores integration messages that must be delivered to external systems.
--
-- Producers insert rows into this table in the same database transaction as the
-- business change. A background publisher later reads pending rows and delivers
-- them to the configured destination, such as Kafka.
--
-- This table is used for reliable delivery. It does not guarantee exactly-once
-- consumption; downstream consumers must handle duplicate messages idempotently.
CREATE TABLE IF NOT EXISTS integration_outbox (
    -- Unique message identifier assigned by the database.
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Logical destination for this message.
    -- Examples: kafka.audit_events, kafka.notifications, kafka.payments.
    destination     text NOT NULL,

    -- Kafka partition key for ordered delivery of related messages.
    message_key     text NOT NULL,

    -- Machine-readable event type.
    -- Examples: audit.logged, installment.overdue_marked, contract.created.
    event_type      text NOT NULL,

    -- Logical aggregate/entity type related to this message.
    -- Examples: contract, installment, payment_attempt, audit_event.
    aggregate_type  text NULL,

    -- Serialized event payload.
    payload         jsonb NOT NULL,

    -- Delivery status managed by the outbox publisher.
    status          text NOT NULL DEFAULT 'pending',

    -- Number of delivery attempts already performed.
    attempt_count   integer NOT NULL DEFAULT 0,

    -- Maximum number of attempts before the message is marked as failed.
    max_attempts    integer NOT NULL DEFAULT 10,

    -- Earliest time at which this message may be retried.
    next_attempt_at timestamptz NOT NULL DEFAULT now(),

    -- Timestamp set when a publisher worker claims this message.
    locked_at       timestamptz NULL,

    -- Identifier of the publisher worker that claimed this message.
    locked_by       text NULL,

    -- Monotonic lease fence incremented on every claim or reclaim.
    lock_version    bigint NOT NULL DEFAULT 0,

    -- Last delivery error, if any.
    last_error      text NULL,

    -- Time at which the message was created.
    created_at      timestamptz NOT NULL DEFAULT now(),

    -- Time at which the message was successfully published.
    published_at    timestamptz NULL,

    CONSTRAINT chk_integration_outbox_status
    CHECK (status IN ('pending', 'processing', 'published', 'skipped', 'failed')),

    CONSTRAINT chk_integration_outbox_attempt_count
    CHECK (attempt_count >= 0),

    CONSTRAINT chk_integration_outbox_max_attempts
    CHECK (max_attempts > 0),

    CONSTRAINT chk_integration_outbox_lock_version
    CHECK (lock_version >= 0),

    CONSTRAINT chk_integration_outbox_text_lengths
    CHECK (
        char_length(destination) BETWEEN 1 AND 256
        AND char_length(message_key) BETWEEN 1 AND 512
        AND char_length(event_type) BETWEEN 1 AND 256
        AND (aggregate_type IS NULL OR char_length(aggregate_type) BETWEEN 1 AND 128)
        AND (locked_by IS NULL OR char_length(locked_by) BETWEEN 1 AND 128)
        AND (last_error IS NULL OR char_length(last_error) <= 4096)
    )
);

-- Efficient polling for messages ready to be published.
CREATE INDEX IF NOT EXISTS idx_integration_outbox_pending
    ON integration_outbox (next_attempt_at ASC, created_at ASC)
    WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_integration_outbox_processing_locked_at
    ON integration_outbox (locked_at)
    WHERE status = 'processing';

CREATE INDEX IF NOT EXISTS idx_integration_outbox_status_locked_next_attempt
    ON integration_outbox (status, locked_at, next_attempt_at);

-- Destination-scoped lookup for operations and debugging.
CREATE INDEX IF NOT EXISTS idx_integration_outbox_destination_status
    ON integration_outbox (destination, status, created_at DESC);

-- Aggregate-scoped lookup for traceability.
CREATE INDEX IF NOT EXISTS idx_integration_outbox_aggregate
    ON integration_outbox (aggregate_type, created_at DESC)
    WHERE aggregate_type IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Integration Inbox
-- -----------------------------------------------------------------------------
-- Durable inbound integration events from external systems such as Financial Core.
--
-- Producers insert rows when an event is first received. Business handlers update
-- processing status in the same transaction as domain changes.
CREATE TABLE IF NOT EXISTS integration_inbox (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    event_id text NOT NULL,
    event_type text NOT NULL,
    event_version text NOT NULL,
    source_service text NOT NULL,

    correlation_id text NULL,
    causation_id text NULL,

    payload jsonb NOT NULL,
    -- Self-describing fingerprint in algorithm:digest form (for example sha256:<64-hex>).
    payload_fingerprint text NOT NULL CHECK (payload_fingerprint ~ '^sha256:[0-9a-f]{64}$'),

    consumer_name text NOT NULL,
    subject_type text NULL,
    subject_id text NULL,
    external_reference_type text NULL,
    external_reference text NULL,

    status text NOT NULL,

    received_at timestamptz NOT NULL,
    processed_at timestamptz NULL,
    failed_at timestamptz NULL,

    failure_code text NULL,
    failure_message text NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_integration_inbox_source_event UNIQUE (source_service, event_id),

    CONSTRAINT chk_integration_inbox_status
        CHECK (status IN (
            'received',
            'processed',
            'reconciliation_required',
            'failed'
        )),

    CONSTRAINT chk_integration_inbox_text_lengths
        CHECK (
            char_length(event_id) BETWEEN 1 AND 256
            AND char_length(event_type) BETWEEN 1 AND 256
            AND char_length(event_version) BETWEEN 1 AND 64
            AND char_length(source_service) BETWEEN 1 AND 128
            AND (correlation_id IS NULL OR char_length(correlation_id) BETWEEN 1 AND 256)
            AND (causation_id IS NULL OR char_length(causation_id) BETWEEN 1 AND 256)
            AND char_length(consumer_name) BETWEEN 1 AND 128
            AND (subject_type IS NULL OR char_length(subject_type) BETWEEN 1 AND 128)
            AND (subject_id IS NULL OR char_length(subject_id) BETWEEN 1 AND 256)
            AND (external_reference_type IS NULL OR char_length(external_reference_type) BETWEEN 1 AND 128)
            AND (external_reference IS NULL OR char_length(external_reference) BETWEEN 1 AND 256)
            AND (failure_code IS NULL OR char_length(failure_code) BETWEEN 1 AND 256)
            AND (failure_message IS NULL OR char_length(failure_message) <= 4096)
        )
);

CREATE INDEX IF NOT EXISTS idx_integration_inbox_status_received_at
    ON integration_inbox (status, received_at ASC);

CREATE INDEX IF NOT EXISTS idx_integration_inbox_consumer_subject
    ON integration_inbox (consumer_name, subject_type, subject_id)
    WHERE subject_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_integration_inbox_external_reference
    ON integration_inbox (external_reference_type, external_reference)
    WHERE external_reference IS NOT NULL;

-- Records payload identity conflicts for reused event ids without mutating terminal inbox rows.
CREATE TABLE IF NOT EXISTS integration_inbox_conflicts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    inbox_id uuid NOT NULL
        REFERENCES integration_inbox (id),

    source_service text NOT NULL,
    event_id text NOT NULL,

    existing_payload_fingerprint text NOT NULL CHECK (existing_payload_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    incoming_payload_fingerprint text NOT NULL CHECK (incoming_payload_fingerprint ~ '^sha256:[0-9a-f]{64}$'),
    incoming_payload jsonb NOT NULL,

    failure_code text NOT NULL,
    detected_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_integration_inbox_conflicts_event_fingerprint
        UNIQUE (source_service, event_id, incoming_payload_fingerprint),

    CONSTRAINT chk_integration_inbox_conflicts_text_lengths
        CHECK (
            char_length(source_service) BETWEEN 1 AND 128
            AND char_length(event_id) BETWEEN 1 AND 256
            AND char_length(failure_code) BETWEEN 1 AND 256
        )
);

CREATE INDEX IF NOT EXISTS idx_integration_inbox_conflicts_inbox_id
    ON integration_inbox_conflicts (inbox_id);

-- -----------------------------------------------------------------------------
-- Durable Sagas
-- -----------------------------------------------------------------------------
-- Stores durable orchestration state for long-running Saga instances.
--
-- Business state remains in its owning tables. These rows contain only process
-- coordination metadata required for recovery, retries, leasing, fencing, and
-- compensation progress.
CREATE TABLE IF NOT EXISTS saga_instances (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    saga_type text NOT NULL,
    definition_version integer NOT NULL,
    business_key text NOT NULL,

    subject_type text NULL,
    subject_id text NULL,

    phase text NOT NULL,
    status text NOT NULL,
    current_step text NOT NULL,

    step_attempt_count integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NULL,

    lease_until timestamptz NULL,
    locked_by text NULL,

    -- Monotonic fencing token incremented on every claim and durable state
    -- mutation. Stale workers must update with the exact version they claimed.
    version bigint NOT NULL DEFAULT 0,

    last_error text NULL,
    correlation_id text NULL,
    causation_id text NULL,

    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    finished_at timestamptz NULL,

    CONSTRAINT uq_saga_instances_type_business_key
        UNIQUE (saga_type, business_key),

    CONSTRAINT chk_saga_instances_definition_version
        CHECK (definition_version > 0),

    CONSTRAINT chk_saga_instances_phase
        CHECK (phase IN ('forward', 'compensating')),

    CONSTRAINT chk_saga_instances_status
        CHECK (status IN (
            'running',
            'waiting',
            'reconciliation_required',
            'manual_review_required',
            'completed',
            'compensated',
            'failed'
        )),

    CONSTRAINT chk_saga_instances_counters
        CHECK (step_attempt_count >= 0 AND version >= 0),

    CONSTRAINT chk_saga_instances_subject_pair
        CHECK ((subject_type IS NULL) = (subject_id IS NULL)),

    CONSTRAINT chk_saga_instances_lease_pair
        CHECK ((lease_until IS NULL) = (locked_by IS NULL)),

    CONSTRAINT chk_saga_instances_running_metadata
        CHECK (
            status = 'running'
            OR (next_attempt_at IS NULL AND lease_until IS NULL AND locked_by IS NULL)
        ),

    CONSTRAINT chk_saga_instances_terminal_time
        CHECK (
            (status IN ('completed', 'compensated', 'failed') AND finished_at IS NOT NULL)
            OR
            (status NOT IN ('completed', 'compensated', 'failed') AND finished_at IS NULL)
        ),

    CONSTRAINT chk_saga_instances_terminal_phase
        CHECK (
            (status <> 'completed' OR phase = 'forward')
            AND
            (status <> 'compensated' OR phase = 'compensating')
        ),

    CONSTRAINT chk_saga_instances_timestamps
        CHECK (
            updated_at >= created_at
            AND (finished_at IS NULL OR finished_at >= created_at)
        ),

    CONSTRAINT chk_saga_instances_text_lengths
        CHECK (
            char_length(saga_type) BETWEEN 1 AND 128
            AND char_length(business_key) BETWEEN 1 AND 512
            AND char_length(current_step) BETWEEN 1 AND 128
            AND (subject_type IS NULL OR char_length(subject_type) BETWEEN 1 AND 128)
            AND (subject_id IS NULL OR char_length(subject_id) BETWEEN 1 AND 256)
            AND (locked_by IS NULL OR char_length(locked_by) BETWEEN 1 AND 128)
            AND (last_error IS NULL OR char_length(last_error) <= 4096)
            AND (correlation_id IS NULL OR char_length(correlation_id) BETWEEN 1 AND 256)
            AND (causation_id IS NULL OR char_length(causation_id) BETWEEN 1 AND 256)
        )
);

-- Supports worker polling for runnable Saga instances. next_attempt_at is NULL
-- for immediately runnable work and non-NULL for delayed retries.
CREATE INDEX IF NOT EXISTS idx_saga_instances_running_due
    ON saga_instances (next_attempt_at ASC, created_at ASC, id ASC)
    WHERE status = 'running';

-- Supports stale-lease recovery and operational inspection of active claims.
CREATE INDEX IF NOT EXISTS idx_saga_instances_running_lease
    ON saga_instances (lease_until ASC)
    WHERE status = 'running' AND lease_until IS NOT NULL;

-- Optional subject lookup for operational tooling and event-driven resumption.
CREATE INDEX IF NOT EXISTS idx_saga_instances_subject
    ON saga_instances (subject_type, subject_id, created_at DESC)
    WHERE subject_id IS NOT NULL;

-- Records committed forward-step completions in durable execution order. The
-- history is used to determine compensation order and must not be inferred from
-- the current Saga snapshot.
CREATE TABLE IF NOT EXISTS saga_forward_step_completions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    saga_id uuid NOT NULL
        REFERENCES saga_instances (id),

    step text NOT NULL,
    sequence bigint NOT NULL,
    attempt_count integer NOT NULL,

    compensation_state text NOT NULL DEFAULT 'not_compensated',

    completed_at timestamptz NOT NULL,
    compensation_resolved_at timestamptz NULL,

    CONSTRAINT uq_saga_forward_step_completions_saga_sequence
        UNIQUE (saga_id, sequence),

    CONSTRAINT chk_saga_forward_step_completions_sequence
        CHECK (sequence > 0),

    CONSTRAINT chk_saga_forward_step_completions_attempt_count
        CHECK (attempt_count > 0),

    CONSTRAINT chk_saga_forward_step_completions_compensation_state
        CHECK (compensation_state IN ('not_compensated', 'compensated', 'not_required')),

    CONSTRAINT chk_saga_forward_step_completions_resolution
        CHECK (
            (compensation_state = 'not_compensated' AND compensation_resolved_at IS NULL)
            OR
            (compensation_state IN ('compensated', 'not_required') AND compensation_resolved_at IS NOT NULL)
        ),

    CONSTRAINT chk_saga_forward_step_completions_timestamps
        CHECK (
            compensation_resolved_at IS NULL
            OR compensation_resolved_at >= completed_at
        ),

    CONSTRAINT chk_saga_forward_step_completions_step_length
        CHECK (char_length(step) BETWEEN 1 AND 128)
);

-- Compensation walks unresolved forward completions in reverse completion order.
CREATE INDEX IF NOT EXISTS idx_saga_forward_step_completions_unresolved
    ON saga_forward_step_completions (saga_id, sequence DESC)
    WHERE compensation_state = 'not_compensated';

-- -----------------------------------------------------------------------------
-- Decisions
-- -----------------------------------------------------------------------------
-- Immutable audit envelope for governed BNPL decisions.
CREATE TABLE IF NOT EXISTS decisions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    type text NOT NULL,
    target_type text NOT NULL,
    target_id uuid NOT NULL,
    outcome text NOT NULL,
    reason_codes text[] NOT NULL DEFAULT '{}',
    policy_id text NOT NULL,
    policy_version text NOT NULL,
    jurisdiction text NOT NULL,
    actor_type text NOT NULL,
    actor_id uuid NULL,
    prior_decision_id uuid NULL REFERENCES decisions (id),
    reviewable_until timestamptz NULL,
    expires_at timestamptz NULL,
    decided_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    idempotency_key text NOT NULL DEFAULT '',
    -- Self-describing idempotency fingerprint in algorithm:digest form (for example sha256:<64-hex>).
    idempotency_fingerprint text NOT NULL DEFAULT '' CHECK (
        idempotency_fingerprint = '' OR idempotency_fingerprint ~ '^sha256:[0-9a-f]{64}$'
    ),

    CONSTRAINT chk_decisions_idempotency
    CHECK (
        (idempotency_key = '' AND idempotency_fingerprint = '')
        OR
        (idempotency_key <> '' AND idempotency_fingerprint <> '')
    ),

    CONSTRAINT chk_decisions_type
    CHECK (type IN ('eligibility', 'credit_limit', 'restriction', 'contract_approval', 'manual_override')),

    CONSTRAINT chk_decisions_target_type
    CHECK (target_type IN ('customer_profile', 'merchant_profile', 'merchant_branch_profile', 'contract', 'checkout_key')),

    CONSTRAINT chk_decisions_outcome
    CHECK (outcome IN ('approved', 'denied', 'conditional', 'limit_set', 'restricted', 'unrestricted')),

    CONSTRAINT chk_decisions_actor_type
    CHECK (actor_type IN ('system', 'admin')),

    CONSTRAINT chk_decisions_jurisdiction
    CHECK (jurisdiction IN ('IR')),

    CONSTRAINT chk_decisions_text_lengths
    CHECK (
        char_length(policy_id) BETWEEN 1 AND 128
        AND char_length(policy_version) BETWEEN 1 AND 64
        AND char_length(idempotency_key) <= 256
    )
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_decisions_idempotency_key
    ON decisions (idempotency_key)
    WHERE idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_decisions_target
    ON decisions (target_type, target_id);

CREATE INDEX IF NOT EXISTS idx_decisions_prior_decision_id
    ON decisions (prior_decision_id)
    WHERE prior_decision_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Customer Profiles
-- -----------------------------------------------------------------------------
-- BNPL-specific customer profiles.
--
-- A customer profile never owns raw identity data such as national ID,
-- phone number, or birthdate, and never performs or persists identity
-- verification. Customer identity verification is owned by the upstream
-- Identity Service; BNPL receives an already authenticated customer identity
-- (identity_subject_id) through the trusted gateway boundary.
CREATE TABLE IF NOT EXISTS customer_profiles (
    -- Unique profile identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Customer identity this BNPL profile belongs to.
    identity_subject_id uuid NOT NULL UNIQUE,


    -- Regulatory jurisdiction that governs this customer's BNPL profile.
    jurisdiction text NOT NULL,

    -- Current lifecycle state of the BNPL profile.
    status text NOT NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_customer_profiles_status
    CHECK (status IN ('active', 'deactivated', 'terminated')),

    CONSTRAINT chk_customer_profiles_jurisdiction
    CHECK (jurisdiction IN ('IR'))
);

-- -----------------------------------------------------------------------------
-- Policy Assignments
-- -----------------------------------------------------------------------------
-- Runtime business data mapping a generic subject to one versioned policy
-- family. Policy definitions/limits live in code; this table stores only who
-- receives which policy and preserves assignment history over time.
CREATE TABLE IF NOT EXISTS policy_assignments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    subject_type text NOT NULL,
    subject_id text NOT NULL,

    policy_family text NOT NULL,
    policy_id text NOT NULL,
    policy_version text NOT NULL,

    reason text NOT NULL DEFAULT '',
    source text NOT NULL DEFAULT '',

    valid_from timestamptz NOT NULL,
    valid_until timestamptz NULL,

    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_policy_assignments_text_lengths
        CHECK (
            char_length(subject_id) BETWEEN 1 AND 256
            AND char_length(policy_id) BETWEEN 1 AND 128
            AND char_length(policy_version) BETWEEN 1 AND 64
            AND char_length(reason) <= 512
            AND char_length(source) <= 128
            AND char_length(created_by) BETWEEN 1 AND 256
        ),
    CONSTRAINT chk_policy_assignments_interval
        CHECK (valid_until IS NULL OR valid_until >= valid_from)
);

-- At most one open-ended assignment may exist for one subject/policy family.
-- Historical rows remain queryable after they are closed.
CREATE UNIQUE INDEX IF NOT EXISTS uq_policy_assignments_open
    ON policy_assignments (subject_type, subject_id, policy_family)
    WHERE valid_until IS NULL;

CREATE INDEX IF NOT EXISTS idx_policy_assignments_active_lookup
    ON policy_assignments (subject_type, subject_id, policy_family, valid_from DESC);

-- Durable evidence for operational/bulk assignment imports. Item failures are
-- intentionally independent so one rejected Identity Service subject does not
-- roll back successful subjects in the same file.
CREATE TABLE IF NOT EXISTS policy_assignment_imports (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    import_type text NOT NULL,
    source_file text NOT NULL,
    status text NOT NULL,
    total_count integer NOT NULL,
    success_count integer NOT NULL DEFAULT 0,
    failure_count integer NOT NULL DEFAULT 0,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz NULL,

    CONSTRAINT chk_policy_assignment_imports_status
        CHECK (status IN ('processing', 'completed', 'completed_with_failures')),
    CONSTRAINT chk_policy_assignment_imports_counts
        CHECK (total_count >= 0 AND success_count >= 0 AND failure_count >= 0),
    CONSTRAINT chk_policy_assignment_imports_text_lengths
        CHECK (
            char_length(import_type) BETWEEN 1 AND 128
            AND char_length(source_file) BETWEEN 1 AND 1024
            AND char_length(created_by) BETWEEN 1 AND 256
        )
);

CREATE TABLE IF NOT EXISTS policy_assignment_import_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    import_id uuid NOT NULL REFERENCES policy_assignment_imports(id) ON DELETE RESTRICT,
    row_number integer NOT NULL,
    subject_id text NOT NULL,
    status text NOT NULL,
    error_code text NULL,
    error_message text NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_policy_assignment_import_items_row UNIQUE (import_id, row_number),
    CONSTRAINT chk_policy_assignment_import_items_row_number CHECK (row_number > 0),
    CONSTRAINT chk_policy_assignment_import_items_status
        CHECK (status IN ('pending', 'succeeded', 'failed')),
    CONSTRAINT chk_policy_assignment_import_items_text_lengths
        CHECK (
            char_length(subject_id) BETWEEN 1 AND 256
            AND (error_code IS NULL OR char_length(error_code) <= 256)
            AND (error_message IS NULL OR char_length(error_message) <= 4096)
        )
);

CREATE INDEX IF NOT EXISTS idx_policy_assignment_import_items_import_status
    ON policy_assignment_import_items (import_id, status, row_number);

-- -----------------------------------------------------------------------------
-- Merchant Profiles
-- -----------------------------------------------------------------------------
-- Merchant BNPL participation configuration.
--
-- Defines whether and how a merchant offers BNPL, including funding and
-- settlement arrangements, exposure limits, and supported currencies. Composite
-- and list-valued fields are stored as jsonb to match the domain model exactly.
CREATE TABLE IF NOT EXISTS merchant_profiles (
    -- Unique profile identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Merchant identity this BNPL profile belongs to.
    merchant_id uuid NOT NULL UNIQUE,

    -- Public/trade name shown on merchant-facing surfaces.
    -- Temporarily stored in BNPL; later owned by Merchant Service.
    display_name text NOT NULL,

    -- Registered legal entity name when known.
    legal_name text NOT NULL DEFAULT '',

    -- Optional absolute URL for the merchant logo.
    logo_url text NOT NULL DEFAULT '',

    -- Optional customer-support phone number.
    support_phone text NOT NULL DEFAULT '',

    -- Optional customer-support email address.
    support_email text NOT NULL DEFAULT '',

    -- Current lifecycle state of the merchant's BNPL participation.
    status text NOT NULL,

    -- Funding arrangements available to this merchant.
    funding_models jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Settlement arrangements available to this merchant.
    settlement_models jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Currencies this merchant may transact in.
    currencies jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Credit risk classification used to bound BNPL exposure.
    risk_tier text NOT NULL,

    -- Maximum BNPL transaction amount in the smallest currency unit.
    max_amount_minor bigint NOT NULL DEFAULT 0,

    -- Currency of the maximum BNPL transaction amount.
    max_amount_currency text NOT NULL DEFAULT '',

    -- Maximum installment tenure, in months.
    max_tenure_months integer NOT NULL,

    -- Whether this merchant must configure per-branch profiles.
    requires_branch_mode boolean NOT NULL DEFAULT false,

    -- Regulatory jurisdiction governing this merchant's BNPL activity.
    jurisdiction text NOT NULL,

    -- Server-owned Financial Core payment scope for Partner/hosted checkouts.
    -- Assigned only by trusted BNPL provisioning; never partner- or JWT-supplied.
    -- Partner Create snapshots these onto checkout_sessions.auth_* at create time.
    payment_tenant_id uuid NULL,
    payment_context_id uuid NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_merchant_profiles_status
    CHECK (status IN ('pending_review', 'active', 'deactivated', 'suspended', 'terminated')),

    CONSTRAINT chk_merchant_profiles_risk_tier
    CHECK (risk_tier IN ('low', 'medium', 'high')),

    CONSTRAINT chk_merchant_profiles_max_tenure_months
    CHECK (max_tenure_months >= 0),

    CONSTRAINT chk_merchant_profiles_max_amount_minor
    CHECK (max_amount_minor >= 0),

    CONSTRAINT chk_merchant_profiles_jurisdiction
    CHECK (jurisdiction IN ('IR')),

    CONSTRAINT chk_merchant_profiles_currencies
    CHECK (
        jsonb_typeof(currencies) = 'array'
        AND currencies <@ '["IRR", "EUR", "USD"]'::jsonb
    ),

    CONSTRAINT chk_merchant_profiles_max_amount_currency
    CHECK (max_amount_currency = '' OR max_amount_currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT chk_merchant_profiles_payment_scope
    CHECK (
        (payment_tenant_id IS NULL AND payment_context_id IS NULL)
        OR (payment_tenant_id IS NOT NULL AND payment_context_id IS NOT NULL)
    )
);

COMMENT ON COLUMN merchant_profiles.payment_tenant_id IS
    'Server-owned Financial Core tenant UUID for Partner/hosted checkouts; assigned by trusted BNPL provisioning only.';
COMMENT ON COLUMN merchant_profiles.payment_context_id IS
    'Server-owned Financial Core context UUID for Partner/hosted checkouts; assigned by trusted BNPL provisioning only.';

-- -----------------------------------------------------------------------------
-- Merchant Branch Profiles
-- -----------------------------------------------------------------------------
-- Merchant selling points that may be physical locations or virtual online outlets.
-- Per-selling-point override columns fall back to the parent merchant configuration when unset.
CREATE TABLE IF NOT EXISTS merchant_branch_profiles (
    -- Unique selling-point profile identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent merchant identity.
    merchant_id uuid NOT NULL,

    -- External identity of this physical or online selling point.
    branch_id uuid NOT NULL UNIQUE,

    -- Sales-surface kind used to validate POS and online source mappings.
    branch_type text NOT NULL,

    -- Current lifecycle state of the selling point's BNPL participation.
    status text NOT NULL,

    -- Funding arrangements available to this selling point.
    funding_models jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Settlement arrangements available to this selling point.
    settlement_models jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Currencies this selling point may transact in.
    currencies jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Overrides the parent merchant risk tier when set.
    risk_tier_override text NULL,

    -- Overrides the parent merchant max amount minor unit when set.
    max_amount_override_minor bigint NULL,

    -- Overrides the parent merchant max amount currency when set.
    max_amount_override_currency text NULL,

    -- Overrides the parent merchant max tenure, in months, when set.
    max_tenure_months_override integer NULL,

    -- Regulatory jurisdiction governing this selling point's BNPL activity.
    jurisdiction text NOT NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Enables source-mapping foreign keys to enforce the selling-point kind.
    CONSTRAINT uq_merchant_branch_profiles_id_type
    UNIQUE (id, branch_type),

    CONSTRAINT chk_merchant_branch_profiles_type
    CHECK (branch_type IN ('physical', 'online')),

    CONSTRAINT chk_merchant_branch_profiles_status
    CHECK (status IN ('pending_review', 'active', 'deactivated', 'suspended', 'terminated')),

    CONSTRAINT chk_merchant_branch_profiles_risk_tier_override
    CHECK (risk_tier_override IS NULL OR risk_tier_override IN ('low', 'medium', 'high')),

    CONSTRAINT chk_merchant_branch_profiles_max_amount_override
    CHECK (
        (max_amount_override_minor IS NULL AND max_amount_override_currency IS NULL)
        OR
        (
            max_amount_override_minor IS NOT NULL
            AND max_amount_override_currency IN ('IRR', 'EUR', 'USD')
            AND max_amount_override_minor >= 0
        )
    ),

    CONSTRAINT chk_merchant_branch_profiles_jurisdiction
    CHECK (jurisdiction IN ('IR')),

    CONSTRAINT chk_merchant_branch_profiles_currencies
    CHECK (
        jsonb_typeof(currencies) = 'array'
        AND currencies <@ '["IRR", "EUR", "USD"]'::jsonb
    )
);

-- Lookup of selling-point profiles by parent merchant.
CREATE INDEX IF NOT EXISTS idx_merchant_branch_profiles_merchant_id
    ON merchant_branch_profiles (merchant_id);

-- -----------------------------------------------------------------------------
-- Merchant POS Terminals
-- -----------------------------------------------------------------------------
-- Maps external POS/GPS terminal identities to physical merchant selling points.
CREATE TABLE IF NOT EXISTS merchant_pos_terminals (
    -- Internal mapping identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Physical selling point that owns this terminal.
    merchant_branch_profile_id uuid NOT NULL,

    -- Fixed discriminator used by the composite FK to reject non-physical owners.
    branch_type text NOT NULL DEFAULT 'physical',

    -- External POS/GPS terminal identity supplied by physical checkout clients.
    pos_gps_id uuid NOT NULL UNIQUE,

    -- Disabled mappings cannot be used to resolve a checkout source.
    enabled boolean NOT NULL DEFAULT true,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_merchant_pos_terminals_branch_type
    CHECK (branch_type = 'physical'),

    CONSTRAINT fk_merchant_pos_terminals_physical_branch
    FOREIGN KEY (merchant_branch_profile_id, branch_type)
    REFERENCES merchant_branch_profiles (id, branch_type)
);

-- Supports listing or managing all POS terminals owned by one physical selling point.
CREATE INDEX IF NOT EXISTS idx_merchant_pos_terminals_branch_profile_id
    ON merchant_pos_terminals (merchant_branch_profile_id);

-- -----------------------------------------------------------------------------
-- Merchant Online Stores
-- -----------------------------------------------------------------------------
-- Maps external online-store identities to virtual merchant selling points.
CREATE TABLE IF NOT EXISTS merchant_online_stores (
    -- Internal mapping identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Online selling point represented by this storefront mapping.
    merchant_branch_profile_id uuid NOT NULL,

    -- Fixed discriminator used by the composite FK to reject non-online owners.
    branch_type text NOT NULL DEFAULT 'online',

    -- External online-store identity supplied by online checkout clients.
    online_store_id uuid NOT NULL UNIQUE,

    -- Disabled mappings cannot be used to resolve a checkout source.
    enabled boolean NOT NULL DEFAULT true,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_merchant_online_stores_branch_type
    CHECK (branch_type = 'online'),

    CONSTRAINT fk_merchant_online_stores_online_branch
    FOREIGN KEY (merchant_branch_profile_id, branch_type)
    REFERENCES merchant_branch_profiles (id, branch_type)
);

-- Supports listing or managing all online storefronts owned by one virtual selling point.
CREATE INDEX IF NOT EXISTS idx_merchant_online_stores_branch_profile_id
    ON merchant_online_stores (merchant_branch_profile_id);

-- -----------------------------------------------------------------------------
-- Credit Accounts
-- -----------------------------------------------------------------------------
-- One operational BNPL credit account exists per customer/currency. It stores
-- current governed capacity plus committed outstanding principal exposure.
-- It is not a cash balance or financial ledger; Financial Core remains the
-- authority for payment and money-movement facts.
CREATE TABLE IF NOT EXISTS credit_accounts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    customer_id uuid NOT NULL REFERENCES customer_profiles (id),
    currency text NOT NULL,

    limit_amount_minor bigint NOT NULL DEFAULT 0,
    exposure_amount_minor bigint NOT NULL DEFAULT 0,

    status text NOT NULL,

    -- Decision that established the current limit state. Historical changes
    -- are retained in credit_transactions rather than versioned limit rows.
    last_limit_decision_id uuid NULL REFERENCES decisions (id),

    effective_from timestamptz NOT NULL,
    expires_at timestamptz NULL,
    revoked_at timestamptz NULL,

    version bigint NOT NULL DEFAULT 1,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_credit_accounts_customer_currency
        UNIQUE (customer_id, currency),

    CONSTRAINT chk_credit_accounts_status
        CHECK (status IN ('active', 'expired', 'revoked')),

    CONSTRAINT chk_credit_accounts_amounts
        CHECK (limit_amount_minor >= 0 AND exposure_amount_minor >= 0),

    CONSTRAINT chk_credit_accounts_currency
        CHECK (currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT chk_credit_accounts_version
        CHECK (version > 0)
);

CREATE INDEX IF NOT EXISTS idx_credit_accounts_last_limit_decision_id
    ON credit_accounts (last_limit_decision_id)
    WHERE last_limit_decision_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Credit Transactions
-- -----------------------------------------------------------------------------
-- Immutable business journal of BNPL credit-capacity changes. This is not a
-- payment or accounting ledger. amount_minor is the absolute value for limit_set
-- and a positive delta for all other types. Idempotency is tied to the causal business fact,
-- not to an arbitrary retry token.
CREATE TABLE IF NOT EXISTS credit_transactions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    credit_account_id uuid NOT NULL REFERENCES credit_accounts (id),

    type text NOT NULL,
    amount_minor bigint NOT NULL,
    currency text NOT NULL,
    reason_code text NOT NULL,

    reference_type text NOT NULL,
    reference_id uuid NOT NULL,

    occurred_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_credit_transactions_business_fact
        UNIQUE (credit_account_id, type, reference_type, reference_id),

    CONSTRAINT chk_credit_transactions_type
        CHECK (type IN (
            'limit_set',
            'limit_increased',
            'limit_decreased',
            'exposure_increased',
            'exposure_decreased'
        )),

    CONSTRAINT chk_credit_transactions_amount
        CHECK (amount_minor > 0),

    CONSTRAINT chk_credit_transactions_currency
        CHECK (currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT chk_credit_transactions_reason
        CHECK (reason_code IN (
            'credit_assessment',
            'contract_activated',
            'installment_paid',
            'contract_canceled'
        )),

    CONSTRAINT chk_credit_transactions_reference_type
        CHECK (reference_type IN ('decision', 'contract', 'installment'))
);

CREATE INDEX IF NOT EXISTS idx_credit_transactions_account_occurred
    ON credit_transactions (credit_account_id, occurred_at DESC, id DESC);

-- -----------------------------------------------------------------------------
-- Contracts
-- -----------------------------------------------------------------------------
-- Checkout BNPL contracts enter payment_pending before becoming active.
-- payment_pending waits either for the customer's initial payment (a
-- PaymentAttempt exists) or for Financial Core merchant funding / economic
-- recognition (no PaymentAttempt). No credit exposure is consumed until active.
CREATE TABLE IF NOT EXISTS contracts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    decision_id uuid NOT NULL
        REFERENCES decisions (id),

    checkout_key text NOT NULL,

    customer_id uuid NOT NULL
        REFERENCES customer_profiles (id),

    -- Credit account whose capacity this contract consumes. A payment_pending
    -- contract acts as the durable single-in-flight payment gate; no credit
    -- amount is reserved while it is pending.
    credit_account_id uuid NOT NULL
        REFERENCES credit_accounts (id),

    merchant_profile_id uuid NOT NULL
        REFERENCES merchant_profiles (id),

    merchant_branch_profile_id uuid NULL
        REFERENCES merchant_branch_profiles (id),

    currency text NOT NULL,

    requested_amount_minor bigint NOT NULL,
    approved_financed_amount_minor bigint NOT NULL,
    upfront_amount_minor bigint NOT NULL DEFAULT 0,

    installment_count integer NOT NULL,

    payable_now_amount_minor bigint NOT NULL,
    remaining_principal_amount_minor bigint NOT NULL,

    status text NOT NULL,

    payment_reference text NULL,
    payment_id uuid NULL,

    financial_invoice_id text NULL,
    financial_merchant_entitlement_id text NULL,
    financial_receivable_id text NULL,
    financial_collection_obligation_id text NULL,
    financial_payment_intent_id text NULL,
    financial_payment_attempt_id text NULL,
    successful_financial_payment_attempt_id text NULL,
    financial_payment_session_id text NULL,
    financial_payment_link text NULL,
    financial_payment_expires_at timestamptz NULL,
    financial_payment_reused_idempotent_result boolean NOT NULL DEFAULT false,

    expires_at timestamptz NOT NULL,
    activated_at timestamptz NULL,
    completed_at timestamptz NULL,
    canceled_at timestamptz NULL,

    -- Customer-initiated cancellation metadata (active → canceling).
    -- Financial Core remains authoritative for canceling → canceled.
    cancellation_idempotency_key text NULL,
    cancellation_reason text NULL,
    cancellation_request_fingerprint text NULL,
    cancellation_requested_at timestamptz NULL,
    cancellation_financial_core_idempotency_key text NULL,
    cancellation_tenant_id uuid NULL,
    cancellation_context_id uuid NULL,
    cancellation_financial_core_accepted_at timestamptz NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_contracts_amounts
        CHECK (
            requested_amount_minor >= 0
            AND approved_financed_amount_minor >= 0
            AND upfront_amount_minor >= 0
            AND payable_now_amount_minor >= 0
            AND remaining_principal_amount_minor >= 0
        ),

    CONSTRAINT chk_contracts_installment_count
        CHECK (installment_count > 0),

    CONSTRAINT chk_contracts_status
        CHECK (status IN (
            'payment_pending',
            'active',
            'canceling',
            'completed',
            'canceled',
            'expired',
            'defaulted'
        )),

    CONSTRAINT chk_contracts_activated_at
        CHECK (
            status <> 'active'
            OR activated_at IS NOT NULL
        ),

    CONSTRAINT chk_contracts_completed_at
        CHECK (
            status <> 'completed'
            OR completed_at IS NOT NULL
        ),

    CONSTRAINT chk_contracts_canceled_at
        CHECK (
            status <> 'canceled'
            OR canceled_at IS NOT NULL
        ),

    CONSTRAINT chk_contracts_cancellation_metadata
        CHECK (
            status <> 'canceling'
            OR (
                cancellation_idempotency_key IS NOT NULL
                AND char_length(cancellation_idempotency_key) BETWEEN 1 AND 256
                AND cancellation_reason IS NOT NULL
                AND cancellation_reason IN ('customer_requested')
                AND cancellation_request_fingerprint IS NOT NULL
                AND char_length(cancellation_request_fingerprint) BETWEEN 1 AND 128
                AND cancellation_requested_at IS NOT NULL
                AND cancellation_financial_core_idempotency_key IS NOT NULL
                AND char_length(cancellation_financial_core_idempotency_key) BETWEEN 1 AND 256
                AND cancellation_tenant_id IS NOT NULL
                AND cancellation_context_id IS NOT NULL
            )
        ),

    CONSTRAINT chk_contracts_payment_session_metadata
        CHECK (
            financial_payment_link IS NULL
            OR (
                financial_invoice_id IS NOT NULL
                AND financial_payment_intent_id IS NOT NULL
                AND financial_payment_session_id IS NOT NULL
                AND financial_payment_expires_at IS NOT NULL
            )
        ),

    CONSTRAINT chk_contracts_integration_text_lengths
        CHECK (
            char_length(checkout_key) BETWEEN 1 AND 256
            AND (financial_invoice_id IS NULL OR char_length(financial_invoice_id) BETWEEN 1 AND 256)
            AND (financial_payment_intent_id IS NULL OR char_length(financial_payment_intent_id) BETWEEN 1 AND 256)
            AND (financial_payment_attempt_id IS NULL OR char_length(financial_payment_attempt_id) BETWEEN 1 AND 256)
            AND (successful_financial_payment_attempt_id IS NULL OR char_length(successful_financial_payment_attempt_id) BETWEEN 1 AND 256)
            AND (financial_payment_session_id IS NULL OR char_length(financial_payment_session_id) BETWEEN 1 AND 256)
            AND (financial_payment_link IS NULL OR char_length(financial_payment_link) BETWEEN 1 AND 2048)
        ),

    CONSTRAINT chk_contracts_currency
        CHECK (currency IN ('IRR', 'EUR', 'USD'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_decision_id
    ON contracts (decision_id);

CREATE INDEX IF NOT EXISTS idx_contracts_customer_status
    ON contracts (customer_id, status);

-- At most one checkout for the same credit account may be in a pre-activation
-- state at a time. This is a concurrency gate, not a monetary reservation:
-- credit_accounts.exposure_amount_minor is unchanged until local activation.
CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_one_pre_activation_per_credit_account
    ON contracts (credit_account_id)
    WHERE status = 'payment_pending';

COMMENT ON COLUMN contracts.status IS
    'Contract lifecycle status. payment_pending is the single pre-activation state: waiting for Financial Core initial-payment success or merchant-funding success (a PaymentAttempt exists only for the former). No credit exposure is consumed until active.';

CREATE INDEX IF NOT EXISTS idx_contracts_customer_active_created_id
    ON contracts (customer_id, created_at DESC, id DESC)
    WHERE status IN ('active', 'canceling');

CREATE UNIQUE INDEX IF NOT EXISTS idx_contracts_checkout_key
    ON contracts (checkout_key);

CREATE INDEX IF NOT EXISTS idx_contracts_status_expires_at
    ON contracts (status, expires_at);

-- -----------------------------------------------------------------------------
-- Installments
-- -----------------------------------------------------------------------------
-- Per-installment offer rows for checkout decisions. Offered installments
-- become contract installments after successful first payment and activation.
CREATE TABLE IF NOT EXISTS installments (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    decision_id uuid NOT NULL
        REFERENCES decisions (id),

    contract_id uuid NULL
        REFERENCES contracts (id),

    customer_id uuid NOT NULL
        REFERENCES customer_profiles (id),

    merchant_profile_id uuid NOT NULL
        REFERENCES merchant_profiles (id),

    merchant_branch_profile_id uuid NULL
        REFERENCES merchant_branch_profiles (id),

    installment_order integer NOT NULL,

    currency text NOT NULL,

    principal_amount_minor bigint NOT NULL,
    upfront_amount_minor bigint NOT NULL DEFAULT 0,
    base_fee_amount_minor bigint NOT NULL DEFAULT 0,
    total_amount_minor bigint NOT NULL,

    due_at timestamptz NOT NULL,

    grace_period_days integer NOT NULL DEFAULT 0,

    status text NOT NULL,

    paid_amount_minor bigint NOT NULL DEFAULT 0,
    paid_at timestamptz NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_installments_order
        CHECK (installment_order > 0),

    CONSTRAINT chk_installments_amounts
        CHECK (
            principal_amount_minor > 0
            AND upfront_amount_minor >= 0
            AND base_fee_amount_minor >= 0
            AND total_amount_minor > 0
            AND paid_amount_minor >= 0
        ),

    CONSTRAINT chk_installments_grace_period_days
        CHECK (grace_period_days >= 0),

    CONSTRAINT chk_installments_status
        CHECK (status IN ('offered', 'scheduled', 'due', 'paid', 'overdue', 'canceled', 'expired')),

    CONSTRAINT chk_installments_contract_required_after_offer
        CHECK (
            status IN ('offered', 'expired', 'canceled')
            OR contract_id IS NOT NULL
        ),

    CONSTRAINT chk_installments_currency
        CHECK (currency IN ('IRR', 'EUR', 'USD'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_installments_decision_order
    ON installments (decision_id, installment_order);

CREATE INDEX IF NOT EXISTS idx_installments_contract_id
    ON installments (contract_id);

CREATE INDEX IF NOT EXISTS idx_installments_customer_status_due_at
    ON installments (customer_id, status, due_at);

CREATE INDEX IF NOT EXISTS idx_installments_status_due_at
    ON installments (status, due_at);

CREATE INDEX IF NOT EXISTS idx_installments_offered_decision_id
    ON installments (decision_id)
    WHERE status = 'offered';

-- -----------------------------------------------------------------------------
-- Installment Charges
-- -----------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS installment_charges (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    installment_id uuid NOT NULL
        REFERENCES installments (id),

    charge_type text NOT NULL,
    charge_status text NOT NULL,

    amount_minor bigint NOT NULL,
    currency text NOT NULL,

    payer_type text NOT NULL,
    recipient_type text NOT NULL,

    policy_id text NOT NULL,
    policy_version text NOT NULL,

    assessed_at timestamptz NOT NULL,
    due_at timestamptz NULL,
    paid_at timestamptz NULL,

    metadata jsonb NOT NULL DEFAULT '{}'::jsonb,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_installment_charges_amount
        CHECK (amount_minor >= 0),

    CONSTRAINT chk_installment_charges_status
        CHECK (charge_status IN ('offered', 'applied', 'paid', 'waived', 'canceled')),

    CONSTRAINT chk_installment_charges_currency
        CHECK (currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT chk_installment_charges_text_lengths
        CHECK (
            char_length(charge_type) BETWEEN 1 AND 128
            AND char_length(payer_type) BETWEEN 1 AND 128
            AND char_length(recipient_type) BETWEEN 1 AND 128
            AND char_length(policy_id) BETWEEN 1 AND 128
            AND char_length(policy_version) BETWEEN 1 AND 64
        )
);

CREATE INDEX IF NOT EXISTS idx_installment_charges_installment_id
    ON installment_charges (installment_id);

CREATE INDEX IF NOT EXISTS idx_installment_charges_type_status
    ON installment_charges (charge_type, charge_status);

CREATE INDEX IF NOT EXISTS idx_installment_charges_offered_installment_id
    ON installment_charges (installment_id)
    WHERE charge_status = 'offered';

-- -----------------------------------------------------------------------------
-- Payment Attempts
-- -----------------------------------------------------------------------------
-- Local BNPL repayment orchestration records for installment payments.
--
-- Financial Core remains authoritative for monetary truth and provider attempts.
CREATE TABLE IF NOT EXISTS payment_attempts (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    contract_id uuid NOT NULL
        REFERENCES contracts (id),

    installment_id uuid NOT NULL
        REFERENCES installments (id),

    amount_minor bigint NOT NULL,
    currency text NOT NULL,
    purpose text NOT NULL DEFAULT 'installment_repayment',

    tenant_id text NULL,
    context_id text NULL,
    provider_code text NULL,
    success_return_url text NULL,
    failure_return_url text NULL,
    cancel_return_url text NULL,

    state text NOT NULL,
    channel text NOT NULL,

    idempotency_key text NOT NULL,
    -- Self-describing request fingerprint in algorithm:digest form (for example sha256:<64-hex>).
    request_fingerprint text NOT NULL CHECK (request_fingerprint ~ '^sha256:[0-9a-f]{64}$'),

    financial_collection_obligation_id text NULL,
    financial_payment_intent_id text NULL,
    financial_payment_attempt_id text NULL,
    financial_payment_session_id text NULL,

    payment_url text NULL,
    financial_payment_expires_at timestamptz NULL,

    failure_code text NULL,
    reconciliation_reason text NULL,

    reconciliation_attempt_count integer NOT NULL DEFAULT 0,
    reconciliation_requested_at timestamptz NULL,
    next_reconciliation_at timestamptz NULL,
    reconciliation_resolved_at timestamptz NULL,
    last_reconciliation_error text NULL,
    manual_review_required_at timestamptz NULL,

    submitted_at timestamptz NULL,
    financial_paid_at timestamptz NULL,
    applied_at timestamptz NULL,
    failed_at timestamptz NULL,
    canceled_at timestamptz NULL,

    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_payment_attempts_amount
        CHECK (amount_minor > 0),

    CONSTRAINT chk_payment_attempts_purpose
        CHECK (purpose IN ('checkout_first_installment', 'installment_repayment')),

    CONSTRAINT chk_payment_attempts_checkout_context
        CHECK (
            purpose <> 'checkout_first_installment'
            OR (
                tenant_id IS NOT NULL
                AND context_id IS NOT NULL
                AND success_return_url IS NOT NULL
                AND failure_return_url IS NOT NULL
                AND cancel_return_url IS NOT NULL
            )
        ),

    CONSTRAINT chk_payment_attempts_state
        CHECK (state IN (
            'created',
            'submitted',
            'succeeded',
            'failed',
            'canceled',
            'reconciliation_required',
            'manual_review_required'
        )),

    CONSTRAINT chk_payment_attempts_channel
        CHECK (channel IN ('gateway', 'bank_transfer', 'wallet')),

    CONSTRAINT chk_payment_attempts_reconciliation_attempt_count
        CHECK (reconciliation_attempt_count >= 0),

    CONSTRAINT chk_payment_attempts_reconciliation_required_metadata
        CHECK (
            state <> 'reconciliation_required'
            OR (
                reconciliation_reason IS NOT NULL
                AND reconciliation_requested_at IS NOT NULL
            )
        ),

    CONSTRAINT chk_payment_attempts_reconciliation_resolved_metadata
        CHECK (
            reconciliation_resolved_at IS NULL
            OR state IN ('submitted', 'succeeded', 'failed', 'canceled')
        ),

    CONSTRAINT chk_payment_attempts_manual_review_metadata
        CHECK (
            state <> 'manual_review_required'
            OR (
                manual_review_required_at IS NOT NULL
                AND last_reconciliation_error IS NOT NULL
                AND next_reconciliation_at IS NULL
            )
        ),

    CONSTRAINT chk_payment_attempts_state_metadata
        CHECK (
            (state <> 'submitted' OR submitted_at IS NOT NULL)
            AND (state <> 'succeeded' OR (financial_paid_at IS NOT NULL AND applied_at IS NOT NULL))
            AND (state <> 'failed' OR (failed_at IS NOT NULL AND failure_code IS NOT NULL))
            AND (state <> 'canceled' OR canceled_at IS NOT NULL)
        ),

    CONSTRAINT chk_payment_attempts_text_lengths
        CHECK (
            char_length(idempotency_key) BETWEEN 1 AND 256
            AND char_length(request_fingerprint) = 71
            AND (tenant_id IS NULL OR char_length(tenant_id) BETWEEN 1 AND 128)
            AND (context_id IS NULL OR char_length(context_id) BETWEEN 1 AND 256)
            AND (provider_code IS NULL OR char_length(provider_code) BETWEEN 1 AND 128)
            AND (success_return_url IS NULL OR char_length(success_return_url) BETWEEN 1 AND 2048)
            AND (failure_return_url IS NULL OR char_length(failure_return_url) BETWEEN 1 AND 2048)
            AND (cancel_return_url IS NULL OR char_length(cancel_return_url) BETWEEN 1 AND 2048)
            AND (financial_payment_intent_id IS NULL OR char_length(financial_payment_intent_id) BETWEEN 1 AND 256)
            AND (financial_payment_attempt_id IS NULL OR char_length(financial_payment_attempt_id) BETWEEN 1 AND 256)
            AND (financial_payment_session_id IS NULL OR char_length(financial_payment_session_id) BETWEEN 1 AND 256)
            AND (payment_url IS NULL OR char_length(payment_url) BETWEEN 1 AND 2048)
            AND (failure_code IS NULL OR char_length(failure_code) BETWEEN 1 AND 256)
            AND (last_reconciliation_error IS NULL OR char_length(last_reconciliation_error) BETWEEN 1 AND 4096)
        ),

    CONSTRAINT chk_payment_attempts_currency
        CHECK (currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT uq_payment_attempts_installment_idempotency_key
        UNIQUE (installment_id, idempotency_key)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_financial_payment_intent_id
    ON payment_attempts (financial_payment_intent_id)
    WHERE financial_payment_intent_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_financial_payment_attempt_id
    ON payment_attempts (financial_payment_attempt_id)
    WHERE financial_payment_attempt_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_succeeded_installment
    ON payment_attempts (installment_id)
    WHERE state = 'succeeded';

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempts_active_installment
    ON payment_attempts (installment_id)
    WHERE state IN ('created', 'submitted', 'reconciliation_required', 'manual_review_required');

CREATE INDEX IF NOT EXISTS idx_payment_attempts_contract_id
    ON payment_attempts (contract_id);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_installment_id
    ON payment_attempts (installment_id);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_state
    ON payment_attempts (state);

CREATE INDEX IF NOT EXISTS idx_payment_attempts_reconciliation_due
    ON payment_attempts (next_reconciliation_at ASC, created_at ASC)
    WHERE state IN ('created', 'submitted', 'reconciliation_required');

CREATE INDEX IF NOT EXISTS idx_payment_attempts_submitted_expires_at
    ON payment_attempts (financial_payment_expires_at)
    WHERE state = 'submitted';

-- -----------------------------------------------------------------------------
-- Payment Attempt Items
-- -----------------------------------------------------------------------------
-- Snapshotted payable obligation items captured when a repayment attempt starts.
CREATE TABLE IF NOT EXISTS payment_attempt_items (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    payment_attempt_id uuid NOT NULL
        REFERENCES payment_attempts (id),

    item_type text NOT NULL,
    source_id uuid NOT NULL,
    amount_minor bigint NOT NULL,
    effect text NOT NULL,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_payment_attempt_items_amount
        CHECK (amount_minor > 0),

    CONSTRAINT chk_payment_attempt_items_item_type
        CHECK (item_type IN ('principal', 'upfront', 'charge')),

    CONSTRAINT chk_payment_attempt_items_effect
        CHECK (effect IN ('debit', 'credit'))
);

CREATE INDEX IF NOT EXISTS idx_payment_attempt_items_payment_attempt_id
    ON payment_attempt_items (payment_attempt_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempt_items_source
    ON payment_attempt_items (payment_attempt_id, item_type, source_id);

CREATE UNIQUE INDEX IF NOT EXISTS uq_payment_attempt_items_principal
    ON payment_attempt_items (payment_attempt_id)
    WHERE item_type = 'principal';

-- -----------------------------------------------------------------------------
-- Checkout Decision Allocations
-- -----------------------------------------------------------------------------
-- Persisted material financial result of a checkout decision inquiry.
CREATE TABLE IF NOT EXISTS checkout_decision_allocations (
    decision_id uuid PRIMARY KEY REFERENCES decisions (id),
    checkout_key text NOT NULL,
    customer_id uuid NOT NULL REFERENCES customer_profiles (id),
    merchant_profile_id uuid NOT NULL REFERENCES merchant_profiles (id),
    merchant_branch_profile_id uuid NULL REFERENCES merchant_branch_profiles (id),

    -- Sales channel resolved when the Decision was created.
    purchase_channel text NOT NULL,

    -- External POS/GPS or online-store identity used to resolve the merchant selling point.
    purchase_source_id uuid NOT NULL,

    -- Optional merchant-owned order reference for online purchases.
    merchant_order_id text NULL,

    requested_amount_minor bigint NOT NULL,
    approved_financed_amount_minor bigint NOT NULL,
    upfront_amount_minor bigint NOT NULL DEFAULT 0,
    available_credit_minor bigint NOT NULL DEFAULT 0,
    currency text NOT NULL,
    installment_count integer NOT NULL,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_checkout_decision_allocations_amounts
    CHECK (
        requested_amount_minor >= 0
        AND approved_financed_amount_minor >= 0
        AND upfront_amount_minor >= 0
        AND available_credit_minor >= 0
    ),

    CONSTRAINT chk_checkout_decision_allocations_installment_count
    CHECK (installment_count > 0),

    CONSTRAINT chk_checkout_decision_allocations_purchase_channel
    CHECK (purchase_channel IN ('pos', 'online')),

    CONSTRAINT chk_checkout_decision_allocations_merchant_order_id
    CHECK (merchant_order_id IS NULL OR char_length(merchant_order_id) BETWEEN 1 AND 256),

    CONSTRAINT chk_checkout_decision_allocations_currency
    CHECK (currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT chk_checkout_decision_allocations_checkout_key
    CHECK (char_length(checkout_key) BETWEEN 1 AND 256)
);

CREATE INDEX IF NOT EXISTS idx_checkout_decision_allocations_customer_id
    ON checkout_decision_allocations (customer_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_checkout_decision_allocations_checkout_key
    ON checkout_decision_allocations (checkout_key);

CREATE INDEX IF NOT EXISTS idx_checkout_decision_allocations_expires_at
    ON checkout_decision_allocations (expires_at);

-- -----------------------------------------------------------------------------
-- Merchant Return Destinations
-- -----------------------------------------------------------------------------
-- Return URLs registered for a merchant during onboarding or configuration.
--
-- Checkout requests select a destination by symbolic name only. BNPL never
-- accepts an arbitrary return URL from a checkout request, which is what keeps
-- the hosted handoff free of open-redirect exposure.
CREATE TABLE IF NOT EXISTS merchant_return_destinations (
    -- Internal destination identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Merchant BNPL profile that owns this destination.
    merchant_id uuid NOT NULL,

    -- Symbolic name merchants reference per checkout.
    destination_name text NOT NULL,

    -- Absolute URL the browser is sent to when checkout finishes.
    return_url text NOT NULL,

    -- Disabled destinations cannot be selected by a new checkout.
    enabled boolean NOT NULL DEFAULT true,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT fk_merchant_return_destinations_merchant_profile
    FOREIGN KEY (merchant_id) REFERENCES merchant_profiles (merchant_id),

    CONSTRAINT chk_merchant_return_destinations_text_lengths
    CHECK (
        char_length(destination_name) BETWEEN 1 AND 64
        AND char_length(return_url) BETWEEN 1 AND 2048
    ),

    -- Lowercase kebab-case: letters, digits, single hyphens; no leading/trailing hyphen.
    CONSTRAINT chk_merchant_return_destinations_destination_name_format
    CHECK (destination_name ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),

    -- Application policy may further require HTTPS in production; DB allows http
    -- so development localhost fixtures remain valid.
    CONSTRAINT chk_merchant_return_destinations_return_url_scheme
    CHECK (return_url ~ '^https?://')
);

-- One destination name per merchant; checkout resolves the URL by this key.
CREATE UNIQUE INDEX IF NOT EXISTS uq_merchant_return_destinations_name
    ON merchant_return_destinations (merchant_id, destination_name);

-- -----------------------------------------------------------------------------
-- Merchant Webhook Destinations
-- -----------------------------------------------------------------------------
-- Pre-registered HTTP endpoint for merchant checkout lifecycle notifications.
-- Checkout requests never supply a webhook URL; BNPL resolves the destination
-- from this table so merchants cannot redirect notifications to arbitrary hosts.
--
-- Each merchant has at most one destination. destination_name is retained for
-- compatibility and always defaults to 'default'.
CREATE TABLE IF NOT EXISTS merchant_webhook_destinations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id uuid NOT NULL,
    -- Compatibility label; uniqueness is enforced on merchant_id alone.
    destination_name text NOT NULL DEFAULT 'default',
    webhook_url text NOT NULL,
    enabled boolean NOT NULL DEFAULT true,
    -- Envelope-encrypted shared secret for HMAC request signing, stored as an
    -- opaque encoded string. The plaintext secret is never stored. Empty
    -- ciphertext means signing is not configured.
    signing_secret_ciphertext text NOT NULL DEFAULT '',
    -- Identifier of the key that encrypted signing_secret_ciphertext.
    signing_secret_key_id text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT fk_merchant_webhook_destinations_merchant
    FOREIGN KEY (merchant_id) REFERENCES merchant_profiles (merchant_id),

    CONSTRAINT chk_merchant_webhook_destinations_text_lengths
    CHECK (
        char_length(destination_name) BETWEEN 1 AND 128
        AND char_length(webhook_url) BETWEEN 1 AND 2048
        AND char_length(signing_secret_ciphertext) <= 4096
        AND char_length(signing_secret_key_id) <= 128
    ),

    CONSTRAINT chk_merchant_webhook_destinations_signing_secret
    CHECK (
        (signing_secret_ciphertext = '' AND signing_secret_key_id = '')
        OR (signing_secret_ciphertext <> '' AND signing_secret_key_id <> '')
    ),

    CONSTRAINT chk_merchant_webhook_destinations_webhook_url_scheme
    CHECK (webhook_url ~ '^https?://')
);

-- One webhook destination per merchant.
CREATE UNIQUE INDEX IF NOT EXISTS uq_merchant_webhook_destinations_merchant_id
    ON merchant_webhook_destinations (merchant_id);

-- -----------------------------------------------------------------------------
-- Checkout Sessions
-- -----------------------------------------------------------------------------
-- Merchant integration boundary for one BNPL checkout.
--
-- Merchants and browsers address a session by the unguessable public_token
-- (cs_<64 hex>). id is internal-only. External partner sessions carry an
-- encrypted customer phone and prove possession via OTP on the hosted page;
-- internal sessions are created already bound to a trusted customer.
--
-- status describes the checkout conversation, not payment authority:
--   open        customer may continue checkout
--   confirming  a confirm request is preparing the contract / payment
--   processing  an external payment outcome is pending; never canceled or
--               expired by merchant/sweeper because the outcome is ambiguous
--   completed / canceled / expired / unavailable are terminal
-- A failed Financial Core payment attempt does not fail the session; a
-- retryable failure returns processing back to open.
CREATE TABLE IF NOT EXISTS checkout_sessions (
    -- Internal checkout session primary key; never exposed to merchants or browsers.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Unguessable public checkout handle (cs_<64 hex>); the only identifier
    -- shared with merchants and browsers.
    public_token text NOT NULL,

    -- Create path: external_partner (phone OTP on hosted page) or internal
    -- (pre-bound trusted customer).
    origin text NOT NULL,

    -- Owning merchant BNPL profile.
    merchant_profile_id uuid NOT NULL,

    -- Online selling point resolved from online_store_id at creation time.
    merchant_branch_profile_id uuid NOT NULL,

    -- External storefront identity supplied by the merchant.
    online_store_id uuid NOT NULL,

    -- Merchant-supplied key that makes the create request idempotent.
    idempotency_key text NOT NULL,

    -- Merchant-owned business order reference. Not unique across all sessions:
    -- terminal sessions for the same order may coexist with one active session.
    merchant_order_id text NOT NULL,

    -- Optional merchant correlation handle; never used to resolve a customer identity.
    customer_reference text NOT NULL DEFAULT '',

    -- Envelope-encrypted canonical E.164 customer phone for OTP verification.
    customer_phone_ciphertext text NULL,

    -- Identifier of the key that encrypted customer_phone_ciphertext.
    customer_phone_key_id text NULL,

    -- Opaque identity of the currently issued OTP challenge (oi_<32 hex>).
    otp_issue_id text NULL,

    -- Keyed hash of the currently issued OTP; plaintext is never stored.
    otp_verifier text NULL,

    -- When the currently issued OTP stops being accepted.
    otp_expires_at timestamptz NULL,

    -- When Notification durably accepted submission of the currently issued OTP.
    otp_notification_accepted_at timestamptz NULL,

    -- When the customer proved possession of the checkout phone.
    otp_verified_at timestamptz NULL,

    -- Failed verification attempts against the currently issued OTP.
    otp_attempt_count integer NOT NULL DEFAULT 0,

    -- Notification Service request identifier for the accepted OTP submission.
    otp_notification_request_id text NULL,

    -- Number of one-time codes issued for this checkout; never reset.
    otp_send_count integer NOT NULL DEFAULT 0,

    -- evaluation_generation the issued or verified OTP is bound to.
    otp_evaluation_generation integer NULL,

    -- Trusted gateway tenant scope snapshotted at Partner Create for External
    -- Partner checkouts. Hosted Confirm uses this snapshot; browsers never supply it.
    auth_tenant_id uuid NULL,

    -- Trusted gateway business context snapshotted at Partner Create for External
    -- Partner checkouts. Hosted Confirm uses this snapshot; browsers never supply it.
    auth_context_id uuid NULL,

    -- Internal customer profile primary key, set once BNPL trusts the identity.
    customer_id uuid NULL,

    -- Symbolic name of the pre-registered return destination.
    return_destination text NOT NULL,

    -- Absolute return URL resolved from return_destination at creation time.
    return_url text NOT NULL,

    -- Opaque merchant correlation value echoed back on browser return.
    merchant_state text NOT NULL DEFAULT '',

    -- Checkout amount requested by the merchant, in minor currency units.
    requested_amount_minor bigint NOT NULL,

    -- Currency of requested_amount_minor.
    currency text NOT NULL,

    -- Current checkout session lifecycle state.
    status text NOT NULL,

    -- Personalized checkout Decision created for the bound customer.
    decision_id uuid NULL,

    -- BNPL contract created from the accepted offer.
    contract_id uuid NULL,

    -- When the handoff stops being usable.
    expires_at timestamptz NOT NULL,

    -- When the checkout completed; set exactly when status is 'completed'.
    completed_at timestamptz NULL,

    -- When the checkout was canceled; set exactly when status is 'canceled'.
    canceled_at timestamptz NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- Who canceled the checkout when status becomes canceled.
    cancellation_reason text NULL,

    -- Increments when a new Decision evaluation is required (e.g. expired offer).
    evaluation_generation integer NOT NULL DEFAULT 1,

    CONSTRAINT fk_checkout_sessions_merchant_profile
    FOREIGN KEY (merchant_profile_id) REFERENCES merchant_profiles (id),

    CONSTRAINT fk_checkout_sessions_merchant_branch_profile
    FOREIGN KEY (merchant_branch_profile_id) REFERENCES merchant_branch_profiles (id),

    CONSTRAINT fk_checkout_sessions_customer
    FOREIGN KEY (customer_id) REFERENCES customer_profiles (id),

    CONSTRAINT fk_checkout_sessions_decision
    FOREIGN KEY (decision_id) REFERENCES decisions (id),

    CONSTRAINT fk_checkout_sessions_contract
    FOREIGN KEY (contract_id) REFERENCES contracts (id),

    CONSTRAINT uq_checkout_sessions_public_token
    UNIQUE (public_token),

    CONSTRAINT chk_checkout_sessions_public_token_format
    CHECK (public_token ~ '^cs_[0-9a-f]{64}$'),

    CONSTRAINT chk_checkout_sessions_origin
    CHECK (origin IN ('external_partner', 'internal')),

    CONSTRAINT chk_checkout_sessions_internal_customer
    CHECK (origin <> 'internal' OR customer_id IS NOT NULL),

    CONSTRAINT chk_checkout_sessions_auth_scope
    CHECK (
        (
            origin = 'external_partner'
            AND auth_tenant_id IS NOT NULL
            AND auth_context_id IS NOT NULL
        )
        OR (
            origin = 'internal'
            AND auth_tenant_id IS NULL
            AND auth_context_id IS NULL
        )
    ),

    CONSTRAINT chk_checkout_sessions_customer_phone_pair
    CHECK ((customer_phone_ciphertext IS NULL) = (customer_phone_key_id IS NULL)),

    CONSTRAINT chk_checkout_sessions_customer_phone_lengths
    CHECK (
        (customer_phone_ciphertext IS NULL OR char_length(customer_phone_ciphertext) BETWEEN 1 AND 1024)
        AND (customer_phone_key_id IS NULL OR char_length(customer_phone_key_id) BETWEEN 1 AND 128)
    ),

    CONSTRAINT chk_checkout_sessions_otp_attempt_count
    CHECK (otp_attempt_count >= 0),

    CONSTRAINT chk_checkout_sessions_otp_issued
    CHECK (
        (otp_verifier IS NULL) = (otp_expires_at IS NULL)
        AND (otp_verifier IS NULL) = (otp_issue_id IS NULL)
    ),

    CONSTRAINT chk_checkout_sessions_otp_issue_id_format
    CHECK (otp_issue_id IS NULL OR otp_issue_id ~ '^oi_[0-9a-f]{32}$'),

    CONSTRAINT chk_checkout_sessions_otp_lengths
    CHECK (
        (otp_verifier IS NULL OR char_length(otp_verifier) BETWEEN 1 AND 256)
        AND (
            otp_notification_request_id IS NULL
            OR char_length(otp_notification_request_id) BETWEEN 1 AND 1024
        )
    ),

    CONSTRAINT chk_checkout_sessions_otp_send_count
    CHECK (otp_send_count >= 0),

    CONSTRAINT chk_checkout_sessions_otp_evaluation_generation
    CHECK (otp_evaluation_generation IS NULL OR otp_evaluation_generation >= 1),

    CONSTRAINT chk_checkout_sessions_status
    CHECK (status IN (
        'open',
        'confirming',
        'processing',
        'completed',
        'canceled',
        'expired',
        'unavailable'
    )),

    CONSTRAINT chk_checkout_sessions_currency
    CHECK (currency IN ('IRR', 'EUR', 'USD')),

    CONSTRAINT chk_checkout_sessions_requested_amount_minor
    CHECK (requested_amount_minor > 0),

    CONSTRAINT chk_checkout_sessions_text_lengths
    CHECK (
        char_length(idempotency_key) BETWEEN 1 AND 256
        AND char_length(merchant_order_id) BETWEEN 1 AND 256
        AND char_length(customer_reference) <= 256
        AND char_length(return_destination) BETWEEN 1 AND 64
        AND char_length(return_url) BETWEEN 1 AND 2048
        AND char_length(merchant_state) <= 256
    ),

    CONSTRAINT chk_checkout_sessions_return_destination_format
    CHECK (return_destination ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),

    CONSTRAINT chk_checkout_sessions_cancellation_reason
    CHECK (
        cancellation_reason IS NULL
        OR cancellation_reason IN ('customer_canceled', 'merchant_canceled')
    ),

    CONSTRAINT chk_checkout_sessions_evaluation_generation
    CHECK (evaluation_generation >= 1),

    CONSTRAINT chk_checkout_sessions_contract_link
    CHECK (
        (status NOT IN ('processing', 'completed') OR contract_id IS NOT NULL)
        AND (status <> 'unavailable' OR contract_id IS NULL)
    ),

    CONSTRAINT chk_checkout_sessions_completed_at
    CHECK (
        (status = 'completed') = (completed_at IS NOT NULL)
        AND (completed_at IS NULL OR completed_at >= created_at)
    ),

    CONSTRAINT chk_checkout_sessions_canceled
    CHECK (
        (
            status = 'canceled'
            AND canceled_at IS NOT NULL
            AND cancellation_reason IS NOT NULL
            AND canceled_at >= created_at
        )
        OR (
            status <> 'canceled'
            AND canceled_at IS NULL
            AND cancellation_reason IS NULL
        )
    )
);

COMMENT ON COLUMN checkout_sessions.id IS
    'Internal checkout session primary key; never exposed to merchants or browsers.';
COMMENT ON COLUMN checkout_sessions.public_token IS
    'Unguessable public checkout handle (cs_<64 hex>); the only identifier shared with merchants and browsers.';
COMMENT ON COLUMN checkout_sessions.origin IS
    'Create path: external_partner (phone OTP on hosted page) or internal (pre-bound trusted customer).';
COMMENT ON COLUMN checkout_sessions.auth_tenant_id IS
    'Server-owned Financial Core tenant UUID snapshotted at Partner Create from merchant_profiles.payment_tenant_id; used by Hosted Confirm. Never client- or JWT-supplied.';
COMMENT ON COLUMN checkout_sessions.auth_context_id IS
    'Server-owned Financial Core context UUID snapshotted at Partner Create from merchant_profiles.payment_context_id; used by Hosted Confirm. Never client- or JWT-supplied.';
COMMENT ON COLUMN checkout_sessions.customer_reference IS
    'Optional merchant correlation handle; never used to resolve a customer identity.';
COMMENT ON COLUMN checkout_sessions.customer_phone_ciphertext IS
    'Envelope-encrypted canonical E.164 customer phone supplied by the merchant for OTP verification.';
COMMENT ON COLUMN checkout_sessions.customer_phone_key_id IS
    'Identifier of the key that encrypted customer_phone_ciphertext.';
COMMENT ON COLUMN checkout_sessions.otp_issue_id IS
    'Opaque identity of the currently issued OTP challenge (oi_<32 hex>); MarkOTPNotificationAccepted and failure cleanup are CAS-scoped to it.';
COMMENT ON COLUMN checkout_sessions.otp_verifier IS
    'Keyed hash of the currently issued OTP; the plaintext code is never stored.';
COMMENT ON COLUMN checkout_sessions.otp_expires_at IS
    'When the currently issued OTP stops being accepted.';
COMMENT ON COLUMN checkout_sessions.otp_notification_accepted_at IS
    'When Notification durably accepted submission of the currently issued OTP.';
COMMENT ON COLUMN checkout_sessions.otp_verified_at IS
    'When the customer proved possession of the checkout phone.';
COMMENT ON COLUMN checkout_sessions.otp_attempt_count IS
    'Failed verification attempts against the currently issued OTP.';
COMMENT ON COLUMN checkout_sessions.otp_notification_request_id IS
    'Notification Service request identifier for the accepted OTP submission.';
COMMENT ON COLUMN checkout_sessions.otp_send_count IS
    'Number of one-time codes issued for this checkout; capped by the OTP security policy and never reset.';
COMMENT ON COLUMN checkout_sessions.otp_evaluation_generation IS
    'evaluation_generation the issued or verified one-time code is bound to; kept after verification so Confirm can reject a verification made for a superseded offer.';

-- The create idempotency key is scoped per merchant.
CREATE UNIQUE INDEX IF NOT EXISTS uq_checkout_sessions_idempotency_key
    ON checkout_sessions (merchant_profile_id, idempotency_key);

-- At most one non-terminal session per merchant order. Canceled, expired, and
-- unavailable attempts may be followed by a new attempt for the same order.
-- Completed orders are refused at the application layer (a second paid attempt
-- must not start); the partial unique index alone does not encode that rule.
CREATE UNIQUE INDEX IF NOT EXISTS uq_checkout_sessions_active_merchant_order
    ON checkout_sessions (merchant_profile_id, merchant_order_id)
    WHERE status IN ('open', 'confirming', 'processing');

-- Supports resolving the most recent session for a merchant order.
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_merchant_order_created
    ON checkout_sessions (merchant_profile_id, merchant_order_id, created_at DESC, id DESC);

-- Supports merchant-scoped listing and status dashboards.
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_merchant_status_created
    ON checkout_sessions (merchant_profile_id, status, created_at DESC, id DESC);

-- Supports resolving the customer's own hosted checkouts.
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_customer_status
    ON checkout_sessions (customer_id, status)
    WHERE customer_id IS NOT NULL;

-- Supports expiring open handoffs whose lifetime elapsed.
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_open_expires_at
    ON checkout_sessions (expires_at)
    WHERE status = 'open';

-- Supports resolving a checkout from its Decision or Contract.
CREATE INDEX IF NOT EXISTS idx_checkout_sessions_decision_id
    ON checkout_sessions (decision_id)
    WHERE decision_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_checkout_sessions_contract_id
    ON checkout_sessions (contract_id)
    WHERE contract_id IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Checkout OTP Rate Limits
-- -----------------------------------------------------------------------------
-- Distributed OTP issuance budgets shared across CheckoutSessions. One durable
-- counter row per (scope, scope_key) uses a resettable fixed window so replicas
-- share the same budget without an in-memory map and without unbounded history.
--
-- scope = phone    → scope_key is a keyed HMAC fingerprint of the E.164 phone
--                    (never the raw phone; domain-separated material).
-- scope = merchant → scope_key is the merchant_profiles.id UUID text.
--
-- There is deliberately no IP- or device-based scope.
CREATE TABLE IF NOT EXISTS checkout_otp_rate_limits (
    scope text NOT NULL,
    scope_key text NOT NULL,
    window_started_at timestamptz NOT NULL,
    send_count integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (scope, scope_key),

    CONSTRAINT chk_checkout_otp_rate_limits_scope
    CHECK (scope IN ('phone', 'merchant')),

    CONSTRAINT chk_checkout_otp_rate_limits_scope_key_length
    CHECK (char_length(scope_key) BETWEEN 1 AND 256),

    CONSTRAINT chk_checkout_otp_rate_limits_send_count
    CHECK (send_count >= 0)
);

COMMENT ON TABLE checkout_otp_rate_limits IS
    'Cross-checkout OTP send budgets (phone fingerprint and merchant). Fixed-window counters shared by all BNPL replicas; no IP scope.';
COMMENT ON COLUMN checkout_otp_rate_limits.scope_key IS
    'Opaque bucket key: keyed phone fingerprint or merchant profile UUID text. Never a raw phone number.';

-- -----------------------------------------------------------------------------
-- Merchant Terminals
-- -----------------------------------------------------------------------------
-- Persistent integration identity belonging to a Merchant. One terminal per
-- merchant. Used by runtime /api/v1/partner/** authentication
-- (X-Terminal-Id + X-API-Key). Terminal lifecycle APIs
-- (provision/rotate/retire/get) are Merchant Admin Panel operations and do NOT
-- use partner API-key authentication.
CREATE TABLE IF NOT EXISTS merchant_terminals (
    -- Internal primary key; never presented to partners.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Owning merchant (merchant_profiles.merchant_id / Identity subject).
    merchant_id uuid NOT NULL
        REFERENCES merchant_profiles (merchant_id),

    -- Public numeric terminal identifier. Immutable once issued, random,
    -- NOT a secret, and never rotated with the API key. Presented as
    -- X-Terminal-Id on partner runtime requests.
    terminal_id bigint NOT NULL,

    -- Whether the terminal may authenticate partner requests
    -- (active | suspended | revoked). Suspend/revoke APIs are deferred;
    -- status still gates authentication.
    status text NOT NULL,

    -- Row lifecycle timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_merchant_terminals_merchant_id
    UNIQUE (merchant_id),

    CONSTRAINT uq_merchant_terminals_terminal_id
    UNIQUE (terminal_id),

    CONSTRAINT chk_merchant_terminals_terminal_id
    CHECK (terminal_id > 0),

    CONSTRAINT chk_merchant_terminals_status
    CHECK (status IN ('active', 'suspended', 'revoked'))
);

COMMENT ON TABLE merchant_terminals IS
    'Persistent partner integration identity for a merchant; one terminal per merchant.';
COMMENT ON COLUMN merchant_terminals.terminal_id IS
    'Public immutable non-secret terminal identifier used as X-Terminal-Id; never rotated with API keys.';
COMMENT ON COLUMN merchant_terminals.status IS
    'Lifecycle gate for partner authentication (active, suspended, revoked).';

-- -----------------------------------------------------------------------------
-- Merchant Terminal Credentials
-- -----------------------------------------------------------------------------
-- Credential lifecycle/history for a Merchant Terminal. Stores an irreversible
-- verifier of the API key only; plaintext API keys must never appear here.
CREATE TABLE IF NOT EXISTS merchant_terminal_credentials (
    -- Unique credential identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Terminal this credential authenticates.
    merchant_terminal_id uuid NOT NULL
        REFERENCES merchant_terminals (id),

    -- Monotonic credential generation within the terminal (starts at 1).
    version bigint NOT NULL,

    -- Irreversible verifier of the API key (self-describing peppered HMAC).
    -- Format: hmac-sha256:<pepper_key_id>:<hex_digest>. Never plaintext.
    api_key_hash text NOT NULL,

    -- Lifecycle: active | retiring | retired | revoked.
    status text NOT NULL,

    -- When the credential was created and became usable.
    created_at timestamptz NOT NULL DEFAULT now(),
    activated_at timestamptz NOT NULL,

    -- Deadline until which a RETIRING credential may still authenticate.
    -- Required when status = retiring; NULL for active.
    retire_at timestamptz,

    -- When the credential became permanently unusable (retired).
    retired_at timestamptz,

    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_merchant_terminal_credentials_version
    UNIQUE (merchant_terminal_id, version),

    CONSTRAINT chk_merchant_terminal_credentials_version
    CHECK (version > 0),

    CONSTRAINT chk_merchant_terminal_credentials_status
    CHECK (status IN ('active', 'retiring', 'retired', 'revoked')),

    CONSTRAINT chk_merchant_terminal_credentials_retiring
    CHECK (status <> 'retiring' OR retire_at IS NOT NULL),

    CONSTRAINT chk_merchant_terminal_credentials_retired
    CHECK (status <> 'retired' OR retired_at IS NOT NULL),

    CONSTRAINT chk_merchant_terminal_credentials_active
    CHECK (status <> 'active' OR (retire_at IS NULL AND retired_at IS NULL))
);

COMMENT ON TABLE merchant_terminal_credentials IS
    'Versioned API-key verifier history for a merchant terminal; plaintext keys are never stored.';
COMMENT ON COLUMN merchant_terminal_credentials.api_key_hash IS
    'Irreversible peppered HMAC verifier (hmac-sha256:<pepper_key_id>:<digest>); never plaintext.';
COMMENT ON COLUMN merchant_terminal_credentials.version IS
    'Monotonically increasing credential generation within the terminal.';
COMMENT ON COLUMN merchant_terminal_credentials.status IS
    'active | retiring | retired | revoked credential lifecycle state.';
COMMENT ON COLUMN merchant_terminal_credentials.retire_at IS
    'Deadline until which a retiring credential may still authenticate.';
COMMENT ON COLUMN merchant_terminal_credentials.retired_at IS
    'Time the credential became permanently unusable.';

-- Supports auth lookup of acceptable credentials for a terminal.
CREATE INDEX IF NOT EXISTS idx_merchant_terminal_credentials_terminal_status
    ON merchant_terminal_credentials (merchant_terminal_id, status);

-- At most one active credential per terminal.
CREATE UNIQUE INDEX IF NOT EXISTS uq_merchant_terminal_credentials_active
    ON merchant_terminal_credentials (merchant_terminal_id)
    WHERE status = 'active';

-- At most one retiring credential per terminal (defense-in-depth with use case).
CREATE UNIQUE INDEX IF NOT EXISTS uq_merchant_terminal_credentials_retiring
    ON merchant_terminal_credentials (merchant_terminal_id)
    WHERE status = 'retiring';

-- -----------------------------------------------------------------------------
-- Merchant Terminal Issuance Deliveries
-- -----------------------------------------------------------------------------
-- Temporary recoverable delivery record used ONLY to safely replay the same
-- API key after an idempotent Provision/Rotate HTTP response is lost.
-- This table is NOT an authentication source of truth. Ciphertext is temporary;
-- after the replay window the secret material is scrubbed (tombstone) while
-- idempotency history remains so the same Idempotency-Key cannot mint a new
-- credential.
CREATE TABLE IF NOT EXISTS merchant_terminal_issuance_deliveries (
    -- Unique delivery identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Terminal the issuance belongs to.
    merchant_terminal_id uuid NOT NULL
        REFERENCES merchant_terminals (id),

    -- Credential created by the issuance.
    credential_id uuid NOT NULL
        REFERENCES merchant_terminal_credentials (id),

    -- Caller-supplied idempotency key scoped to the terminal (namespaced by
    -- operation prefix in application code).
    idempotency_key text NOT NULL,

    -- Canonical request fingerprint of the provisioning/rotation semantics.
    -- Same key + different fingerprint => idempotency conflict.
    request_fingerprint text NOT NULL,

    -- Encrypted API key held only for the replay window. NULL after purge.
    api_key_ciphertext text,

    -- Encryption key id that produced api_key_ciphertext. NULL after purge.
    api_key_encryption_key_id text,

    -- Deadline after which the ciphertext must never be replayed (even if
    -- still physically present before the cleanup worker runs).
    expires_at timestamptz NOT NULL,

    -- First time BNPL extracted the secret to build an issuance response.
    -- Does not prove the merchant received the HTTP response. Does not block
    -- further replays inside the window.
    consumed_at timestamptz,

    -- When secret-bearing columns were scrubbed. Non-NULL marks a tombstone.
    purged_at timestamptz,

    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_merchant_terminal_issuance_deliveries_idempotency
    UNIQUE (merchant_terminal_id, idempotency_key),

    -- Unpurged deliveries must carry ciphertext material; purged tombstones
    -- must have secret columns cleared and purged_at set.
    CONSTRAINT chk_merchant_terminal_issuance_deliveries_purge
    CHECK (
        (
            purged_at IS NULL
            AND api_key_ciphertext IS NOT NULL
            AND api_key_encryption_key_id IS NOT NULL
        )
        OR (
            purged_at IS NOT NULL
            AND api_key_ciphertext IS NULL
            AND api_key_encryption_key_id IS NULL
        )
    )
);

COMMENT ON TABLE merchant_terminal_issuance_deliveries IS
    'Temporary encrypted API-key delivery for idempotent Provision/Rotate replay; not an auth source of truth.';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.idempotency_key IS
    'Caller Idempotency-Key scoped to the terminal; retained on tombstones to block re-issuance.';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.request_fingerprint IS
    'Canonical fingerprint of issuance request semantics; mismatch with same key is an idempotency conflict.';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.api_key_ciphertext IS
    'Temporary encrypted API key for replay; scrubbed to NULL after expiry (tombstone).';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.api_key_encryption_key_id IS
    'Secret-box key id for api_key_ciphertext; scrubbed with the ciphertext.';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.expires_at IS
    'Replay deadline; expired deliveries must never return a secret even before physical scrub.';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.consumed_at IS
    'First time the secret was used to build a response; does not block in-window replay.';
COMMENT ON COLUMN merchant_terminal_issuance_deliveries.purged_at IS
    'When ciphertext was scrubbed; non-NULL row is an idempotency tombstone.';

-- Supports scrubbing expired, still-unpurged deliveries.
CREATE INDEX IF NOT EXISTS idx_merchant_terminal_issuance_deliveries_expires_unpurged
    ON merchant_terminal_issuance_deliveries (expires_at)
    WHERE purged_at IS NULL;

-- -----------------------------------------------------------------------------
-- Merchant Terminal Permissions
-- -----------------------------------------------------------------------------
-- Server-side authorization grants for a partner terminal. The API key is only
-- an authentication secret: it does not encode permissions, and credential
-- rotation must not change these rows. Assignments are written by BNPL
-- (provision / future admin flows), never by partner request headers.
CREATE TABLE IF NOT EXISTS merchant_terminal_permissions (
    -- Unique assignment identifier.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Terminal that receives the permission (not a credential version).
    merchant_terminal_id uuid NOT NULL
        REFERENCES merchant_terminals (id),

    -- Catalog wire permission code (for example bnpl:checkout-sessions:create:merchant).
    -- Unknown codes must not be treated as grants by application code.
    permission text NOT NULL,

    -- When the assignment was persisted.
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT uq_merchant_terminal_permissions_terminal_permission
    UNIQUE (merchant_terminal_id, permission),

    CONSTRAINT chk_merchant_terminal_permissions_permission
    CHECK (char_length(permission) BETWEEN 1 AND 128)
);

COMMENT ON TABLE merchant_terminal_permissions IS
    'Server-controlled partner terminal permission grants; independent of API-key credential versions.';
COMMENT ON COLUMN merchant_terminal_permissions.merchant_terminal_id IS
    'Owning terminal; permissions survive credential rotation.';
COMMENT ON COLUMN merchant_terminal_permissions.permission IS
    'BNPL catalog wire permission code assigned to the terminal.';
COMMENT ON COLUMN merchant_terminal_permissions.created_at IS
    'When BNPL persisted this assignment.';

-- Auth lookup of all grants for one terminal.
CREATE INDEX IF NOT EXISTS idx_merchant_terminal_permissions_terminal
    ON merchant_terminal_permissions (merchant_terminal_id);
