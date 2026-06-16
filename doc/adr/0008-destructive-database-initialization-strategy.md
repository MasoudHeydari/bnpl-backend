# ADR 0008: Destructive Database Initialization Strategy

## Status

Accepted

## Context

Greenfield environments need a reliable way to create the managed PostgreSQL schema, application role, and base tables. The `InitDBUseCase` (`pkg/core/usecase/migrationsuc/initdb.go`) implements a **reset-style** bootstrap:

**Admin phase** (single transaction):

1. `DropIfExists` on schema `bnpl` — **CASCADE deletes all objects**
2. `CreateSchema`
3. `CreateRoleIfNotExists` for role `bnpl`
4. `GrantPrivileges` and `SetSearchPath`

**Application phase** (single transaction):

1. Execute embedded `000_schema.sql`
2. Execute `000_dev.sql` or `000_prod.sql`

Both `bnpl db init-dev` and `bnpl db init-prod` invoke this same destructive admin phase; only the seed SQL file differs (both empty today).

## Decision

Provide explicit CLI commands for destructive initialization:

- `init-dev` — calls `InitDBUseCase.InitDev`
- `init-prod` — calls `InitDBUseCase.InitProd`

Name `init-prod` refers to **production-style seed SQL**, not **production-safe operation**. Both commands destroy existing data in schema `bnpl`.

Do not run these commands against live production databases holding borrower or merchant data.

Separate incremental evolution is intended for `db migrate` (filesystem SQL, migration table tracking), which is not yet fully wired.

## Consequences

**Positive**

- Predictable greenfield setup for developers and CI.
- Admin and app phases each wrapped in transactions; partial bootstrap failures roll back.
- Clear operator intent: command names signal reset, not patch.

**Negative**

- **Total data loss** in schema `bnpl` on every run.
- Docker entrypoint defaults to `init-dev` on every container start (ADR 0006).
- No confirmation prompt or `--dry-run` guard.
- `init-prod` naming misleads operators into assuming safety; documentation must emphasize destructiveness.
- Admin credentials required for routine dev resets — broad privilege exposure during bootstrap.

**Operational impact**

- Production deployments using this entrypoint will **wipe BNPL data** on pod restart until entrypoint and init strategy change.
- Backups and point-in-time recovery are the only recovery path after accidental `init-*`.
- Role `bnpl` is recreated if missing but existing role password is set only on first creation (`CreateRoleIfNotExists`); password rotation requires manual DDL.

## Alternatives Considered

**Non-destructive `CREATE IF NOT EXISTS` bootstrap only**

- Rejected for current phase: does not handle schema drift or clean dev resets; planned via migration tooling instead.

**Separate admin tool outside the application binary**

- Rejected: duplicates config and connection logic; single binary preferred (ADR 0005).

**Flyway baseline + migrate without drop**

- Target state for production; not implemented. Would replace destructive init for evolved environments.

## Relationship To Future BNPL Domains

BNPL data (contracts, installments, payment history, audit logs) is **legally and financially sensitive**. Destructive init must be confined to:

- Local developer machines
- Ephemeral CI databases
- One-time greenfield staging setup

Production schema evolution must transition to:

1. Versioned forward migrations (`db migrate`)
2. Backward-compatible DDL with expand-contract patterns
3. Admin role usage limited to migration jobs, not runtime pods
4. Audit records preserved across upgrades

Future domains requiring strong auditability (Audit, LegalCase, CollectionsCase) cannot coexist with routine `DROP SCHEMA CASCADE` in production.

**Open gap:** No implemented production-safe migration path yet. Until then, treat `init-dev` and `init-prod` as **development reset tools only**.
