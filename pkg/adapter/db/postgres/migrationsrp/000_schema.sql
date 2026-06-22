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
