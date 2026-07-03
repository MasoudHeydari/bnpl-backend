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
