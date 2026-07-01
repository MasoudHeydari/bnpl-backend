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

    -- Time at which the migration was successfully applied.
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

    -- Last delivery error, if any.
    last_error      text NULL,

    -- Time at which the message was created.
    created_at      timestamptz NOT NULL DEFAULT now(),

    -- Time at which the message was successfully published.
    published_at    timestamptz NULL,

    CONSTRAINT chk_integration_outbox_status
    CHECK (status IN ('pending', 'processing', 'published', 'failed')),

    CONSTRAINT chk_integration_outbox_attempt_count
    CHECK (attempt_count >= 0),

    CONSTRAINT chk_integration_outbox_max_attempts
    CHECK (max_attempts > 0)
);

-- Efficient polling for messages ready to be published.
CREATE INDEX IF NOT EXISTS idx_integration_outbox_pending
    ON integration_outbox (next_attempt_at ASC, created_at ASC)
    WHERE status = 'pending';

-- Destination-scoped lookup for operations and debugging.
CREATE INDEX IF NOT EXISTS idx_integration_outbox_destination_status
    ON integration_outbox (destination, status, created_at DESC);

-- Aggregate-scoped lookup for traceability.
CREATE INDEX IF NOT EXISTS idx_integration_outbox_aggregate
    ON integration_outbox (aggregate_type, created_at DESC)
    WHERE aggregate_type IS NOT NULL;

-- -----------------------------------------------------------------------------
-- Borrower Identity Verifications
-- -----------------------------------------------------------------------------
-- Identity verification artifacts used during BNPL onboarding.
--
-- This table stores hashed identity evidence and result metadata separately
-- from borrower_profiles so the BNPL profile never owns raw personal identity
-- data. Raw national IDs and phone numbers must not be stored here.
CREATE TABLE IF NOT EXISTS borrower_identity_verifications (
    -- Unique verification identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Borrower identity this verification belongs to.
    borrower_id uuid NOT NULL,

    -- Non-reversible hash of the national identifier.
    national_id_hash text NOT NULL,

    -- Non-reversible hash of the phone number.
    phone_number_hash text NOT NULL,

    -- Borrower date of birth.
    birthdate date NOT NULL,

    -- Current result state of this verification attempt.
    status text NOT NULL,

    -- Verification provider.
    provider text NOT NULL,

    -- Provider's own tracking identifier.
    provider_reference_id text NULL,

    -- Set when the verification reaches a verified state.
    verified_at timestamptz NULL,

    -- Set when the verification reaches a rejected state.
    rejected_at timestamptz NULL,

    -- Time this verification result becomes too old for future decisions.
    expires_at timestamptz NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_borrower_identity_verifications_status
    CHECK (status IN ('pending', 'verified', 'rejected', 'expired'))
);

-- Lookup of verification artifacts by borrower identity.
CREATE INDEX IF NOT EXISTS idx_borrower_identity_verifications_borrower_id
    ON borrower_identity_verifications (borrower_id);

-- Deduplication and lookup by hashed national identifier.
CREATE INDEX IF NOT EXISTS idx_borrower_identity_verifications_national_id_hash
    ON borrower_identity_verifications (national_id_hash);

-- -----------------------------------------------------------------------------
-- Borrower Profiles
-- -----------------------------------------------------------------------------
-- BNPL-specific borrower profiles.
--
-- A borrower profile references identity and decision artifacts but never owns
-- raw identity data such as national ID, phone number, or birthdate. Those live
-- in borrower_identity_verifications behind the identity/KYC boundary.
CREATE TABLE IF NOT EXISTS borrower_profiles (
    -- Unique profile identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Borrower identity this BNPL profile belongs to.
    borrower_id uuid NOT NULL UNIQUE,

    -- Identity verification artifact used for this BNPL profile.
    identity_verification_id uuid NULL REFERENCES borrower_identity_verifications (id),

    -- Latest profile-level eligibility decision affecting this profile.
    last_profile_eligibility_decision_id uuid NULL REFERENCES decisions (id),

    -- Latest credit limit assignment for this borrower profile.
    --
    -- Not declared as a foreign key to avoid a circular dependency with
    -- borrower_credit_limits, which references borrower_profiles.
    last_credit_limit_id uuid NULL,

    -- Regulatory jurisdiction that governs this borrower's BNPL profile.
    jurisdiction text NOT NULL,

    -- Current lifecycle state of the BNPL profile.
    status text NOT NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_borrower_profiles_status
    CHECK (status IN ('pending_review', 'active', 'deactivated', 'terminated'))
    );

-- Lookup of profiles by the identity verification artifact they reference.
CREATE INDEX IF NOT EXISTS idx_borrower_profiles_identity_verification_id
    ON borrower_profiles (identity_verification_id)
    WHERE identity_verification_id IS NOT NULL;

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

    -- Maximum BNPL transaction amount, stored as {amount_minor, currency}.
    max_amount jsonb NOT NULL,

    -- Maximum installment tenure, in months.
    max_tenure_months integer NOT NULL,

    -- Whether this merchant must configure per-branch profiles.
    requires_branch_mode boolean NOT NULL DEFAULT false,

    -- Regulatory jurisdiction governing this merchant's BNPL activity.
    jurisdiction text NOT NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_merchant_profiles_status
    CHECK (status IN ('pending_review', 'active', 'deactivated', 'suspended', 'terminated')),

    CONSTRAINT chk_merchant_profiles_risk_tier
    CHECK (risk_tier IN ('low', 'medium', 'high')),

    CONSTRAINT chk_merchant_profiles_max_tenure_months
    CHECK (max_tenure_months >= 0)
    );

-- -----------------------------------------------------------------------------
-- Merchant Branch Profiles
-- -----------------------------------------------------------------------------
-- Per-branch overrides of a merchant's BNPL configuration.
--
-- Applies when the parent merchant profile requires branch mode. Override
-- columns are nullable and fall back to the parent merchant configuration when
-- unset.
CREATE TABLE IF NOT EXISTS merchant_branch_profiles (
    -- Unique branch profile identifier assigned by the database.
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Parent merchant identity.
    merchant_id uuid NOT NULL,

    -- Branch identity this profile configures.
    branch_id uuid NOT NULL UNIQUE,

    -- Current lifecycle state of the branch's BNPL participation.
    status text NOT NULL,

    -- Funding arrangements available to this branch.
    funding_models jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Settlement arrangements available to this branch.
    settlement_models jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Currencies this branch may transact in.
    currencies jsonb NOT NULL DEFAULT '[]'::jsonb,

    -- Overrides the parent merchant risk tier when set.
    risk_tier_override text NULL,

    -- Overrides the parent merchant max amount when set, stored as
    -- {amount_minor, currency}.
    max_amount_override jsonb NULL,

    -- Overrides the parent merchant max tenure, in months, when set.
    max_tenure_months_override integer NULL,

    -- Regulatory jurisdiction governing this branch's BNPL activity.
    jurisdiction text NOT NULL,

    -- Creation and update timestamps.
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT chk_merchant_branch_profiles_status
    CHECK (status IN ('pending_review', 'active', 'deactivated', 'suspended', 'terminated')),

    CONSTRAINT chk_merchant_branch_profiles_risk_tier_override
    CHECK (risk_tier_override IS NULL OR risk_tier_override IN ('low', 'medium', 'high'))
    );

-- Lookup of branch profiles by parent merchant.
CREATE INDEX IF NOT EXISTS idx_merchant_branch_profiles_merchant_id
    ON merchant_branch_profiles (merchant_id);
