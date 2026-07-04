# ADR 0008: Environment-Gated Database Initialization

## Status

Accepted (amended)

## Context

Greenfield environments need a reliable way to create the managed PostgreSQL schema, application role, and base tables. Operators also need a hard guard so production data cannot be wiped by bootstrap commands.

## Decision

Gate bootstrap by `runtime.environment` (`APP_ENVIRONMENT`) and split behavior:

### `init-dev` (`InitDBUseCase.InitDev`)

- Requires `APP_ENVIRONMENT=development`.
- Always **drops and recreates** schema `bnpl` (`DropIfExists` CASCADE), whether empty or populated.
- Applies embedded `000_schema.sql` + `000_dev.sql`.

### `init-prod` (`InitDBUseCase.InitProd`)

- Requires `APP_ENVIRONMENT=production`.
- If schema `bnpl` **exists** → conflict error; no drop/mutation; operators must use `db migrate`.
- If schema `bnpl` is **missing** → create schema, role, privileges, then apply embedded `000_schema.sql` + `000_prod.sql`.

Incremental evolution uses `db migrate` (filesystem SQL + `migrations` table checksum tracking).

## Consequences

**Positive**

- Development resets remain predictable and complete.
- Production bootstrap cannot wipe an existing managed schema.
- Environment mismatch fails fast before DDL.

**Negative**

- `init-dev` still destroys all objects in schema `bnpl` on every successful run.
- Empty or partially created schema `bnpl` blocks `init-prod` and requires migrate/manual repair.

**Operational impact**

- Steady-state production: run `bnpl db migrate` (or an init Job); do not bootstrap from the HTTP entrypoint.
- One-time production greenfield: `APP_ENVIRONMENT=production` + `init-prod` once.
- Local/CI: `APP_ENVIRONMENT=development` + `init-dev`.

## Alternatives Considered

**Make both commands non-destructive `CREATE IF NOT EXISTS`**

- Rejected for development: prevents clean resets and hides schema drift.

**Keep destructive `init-prod`**

- Rejected: naming encouraged accidental production wipes.

## Relationship To Future BNPL Domains

BNPL data is legally and financially sensitive. Destructive recreate remains confined to development. Production evolution stays on versioned forward migrations with expand/contract DDL.
