# Database

## Overview

PostgreSQL is accessed through `pgx` / `pgxpool`. Responsibilities are split across:

| Component | Location |
| --- | --- |
| Pool, connection, transaction adapters | `pkg/adapter/db/postgres` |
| Schema administration | `pkg/adapter/db/postgres/schemarp` |
| Bootstrap embed + migration SQL | `pkg/adapter/db/postgres/migrationsrp` |
| Orchestration | `pkg/core/usecase/migrationsuc` |

## Managed Schema

Fixed schema name: `bnpl` (`migrationsuc.SchemaName()`).

All application objects are expected to live in this schema. Bootstrap sets `search_path` for role `bnpl`.

## PostgreSQL Roles

| Role | Constant | Purpose |
| --- | --- | --- |
| `admin` | `repo.AdminRole` | Drop/create schema, role DDL, grants |
| `bnpl` | `repo.NormalRole` | Runtime queries, bootstrap SQL, migrations |

### Admin role usage

Used exclusively during **destructive initialization**:

- `DROP SCHEMA IF EXISTS bnpl CASCADE`
- `CREATE SCHEMA bnpl`
- `CREATE ROLE bnpl` (if missing)
- `GRANT ALL ON SCHEMA bnpl TO bnpl`
- `ALTER ROLE bnpl SET search_path = bnpl`

**Operational risk:** Admin credentials are powerful. Production runtime pods should not require admin access once schema exists.

### Application role usage

- HTTP server connection pool (when handlers use DB)
- Embedded bootstrap SQL execution
- Filesystem migration execution (`db migrate`)

## pgpass Usage

The application does **not** read a `.pgpass` file. Credentials are pgpass-**format lines** in configuration:

- `POSTGRES_ADMIN_PGPASS`
- `POSTGRES_NORMAL_PGPASS`

Format: `host:port:database:user:password`

Matching logic in `Database.GetRolePassword` requires exact alignment with `database.host`, `database.port`, `database.name`, and role name.

See [security.md](security.md) and ADR [0007](adr/0007-secret-handling-strategy.md).

## Bootstrap vs Migrations

These are **separate concerns**:

| Aspect | Bootstrap (`init-dev` / `init-prod`) | Migrations (`db migrate`) |
| --- | --- | --- |
| Trigger | CLI or Docker entrypoint | CLI |
| SQL source | `go:embed` in binary | Files on disk |
| Destructive | **Yes** — drops schema | **No** — applies incremental SQL |
| Tracks versions | No (recreates from scratch) | Yes — `migrations` table |
| Production safe | **No** | Intended future path (not fully wired) |

## Schema Initialization Process

Implemented by `InitDBUseCase` (`initdb.go`).

### Phase 1: Administrative bootstrap (destructive)

1. Open admin-role pool
2. Transaction:
   - Drop schema `bnpl` if exists (**CASCADE**)
   - Create schema `bnpl`
   - Ensure role `bnpl` exists with configured password
   - Grant privileges; set default `search_path`
3. Commit

### Phase 2: Schema content

1. Open normal-role pool
2. Transaction:
   - Execute embedded `000_schema.sql`
   - Execute `000_dev.sql` or `000_prod.sql`
3. Commit

## Embedded Bootstrap SQL

Embedded in `pkg/adapter/db/postgres/migrationsrp/settler.go` via `go:embed`.

| File | Current contents |
| --- | --- |
| `000_schema.sql` | `SET search_path TO bnpl`; `CREATE TABLE migrations` |
| `000_dev.sql` | Empty |
| `000_prod.sql` | Empty |

`assets/db/init/` contains symlinks to these files for reference; runtime uses embedded copies only.

**Implication:** `init-dev` and `init-prod` produce identical schema contents today.

## init-dev Behavior

Command: `bnpl db init-dev`

1. **Destroys** managed schema and all contained objects
2. Recreates schema and role privileges
3. Applies embedded DDL and dev seed SQL

**Use for:** local development resets, CI, greenfield environments.

**Do not use for:** production databases with data to preserve.

## init-prod Behavior

Command: `bnpl db init-prod`

Same destructive admin phase as `init-dev`. Differs only in seed file (`000_prod.sql`, currently empty).

**Critical:** `init-prod` is **NOT safe for live production databases**. The name indicates production-**style** seed data, not a non-destructive production migration.

See ADR [0008](adr/0008-destructive-database-initialization-strategy.md).

## Migration Lifecycle

Separate from bootstrap. Implemented in `migrationsuc.UseCase`.

Per migration file:

1. Read file from disk (`migDirPath`)
2. Normal-role connection → transaction
3. Execute SQL
4. `INSERT INTO migrations(name, created_at)`
5. Commit

Execution and registration share one transaction — avoids applied-but-untracked migrations.

### Current migration directory state

- Default path in `.env.example`: `assets/db/migrations`
- Directory **does not exist** in repository
- YAML key `usecases.migration.files-dir` is **not decoded** into `config.Migrations.migDirPath` (unexported field, no yaml tag)

`db migrate` therefore receives an empty directory path and fails or no-ops incorrectly.

## Schema Versioning

| Mechanism | Status |
| --- | --- |
| `migrations` table | Created by bootstrap DDL |
| `versions.database` in YAML | Not read by Go code |
| Semantic migration file naming | Convention only; not enforced |

## Connection Management

Pool settings from config map to `pgxpool.Config`:

- `MaxConns`, `MinConns`
- `MaxConnLifetime`, `MaxConnIdleTime`
- `HealthCheckPeriod`, `ConnectTimeout`

`postgres.NewPool` acquires a connection immediately after creation (fail-fast).

Commands defer `pool.Close()` after use.

## Transaction Boundaries

Callback pattern: `Conn.Tx(ctx, handler)`

- `nil` return → commit
- non-`nil` error → rollback
- panic → rollback, error returned

Used for both bootstrap phases and per-migration execution.

## Operational Risks

| Risk | Mitigation (current) | Target state |
| --- | --- | --- |
| Data loss on `init-*` | Documentation, ADR 0008 | Non-destructive `db migrate` only in prod |
| Docker entrypoint runs `init-dev` by default | Override `DB_INIT_COMMAND` or entrypoint | Migrate-on-deploy job |
| Admin creds in app config | Role separation at runtime | Admin only in migration job |
| Password in DDL logs | Restrict PostgreSQL `log_statement` | Parameterized role management alternative |
| Empty migration wiring | Manual bootstrap only | Fix YAML decode + add migration files |
| No backup/restore docs in repo | Operator responsibility | Document RPO/RTO with BNPL domains |

## TODO

- [ ] Wire `files-dir` YAML into `config.Migrations`
- [ ] Add `assets/db/migrations` with first forward migration
- [ ] Add `db migrate` dry-run or status command
- [ ] Document PostgreSQL backup requirements for production
- [ ] Replace destructive entrypoint default for production deployments
- [ ] Add dev/prod seed content or document intentional absence

## Related documentation

- [architecture.md](architecture.md) — database flow
- [operations.md](operations.md) — Makefile and Docker commands
- [security.md](security.md) — credential handling
