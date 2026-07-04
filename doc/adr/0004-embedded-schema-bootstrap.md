# ADR 0004: Embedded Bootstrap SQL

## Status

Accepted

## Context

Fresh database environments (local Docker, CI testcontainers, new deployments) must initialize the managed `bnpl` schema without depending on external SQL files on disk. Container images should be self-contained, and bootstrap SQL should version with application code.

The initialization use case (`InitDBUseCase`) runs in two phases:

1. Admin transaction: drop/create schema, role, privileges (`initdb.go` + `schemarp`).
2. Normal-role transaction: apply base DDL and seed data via `SchemaInitializer`.

`Settler` in `pkg/adapter/db/postgres/migrationsrp/settler.go` embeds:

- `000_schema.sql` — creates `migrations` table
- `000_dev.sql` — development seed (currently empty)
- `000_prod.sql` — production seed (currently empty)

The `assets/db/init/` directory contains symlinks to these files for human reference; the runtime path uses `go:embed` only.

## Decision

Embed bootstrap SQL with `go:embed` in `migrationsrp/settler.go`. Select dev or prod seed through `InitDevSchema` vs `InitProdSchema` called from `db init-dev` and `db init-prod` respectively.

Bootstrap SQL is separate from filesystem migrations consumed by `db migrate` (`migrationuc.go` reads from `migDirPath` on disk).

## Consequences

**Positive**

- `init-dev` and `init-prod` work from a single compiled binary with no mounted SQL volume.
- Bootstrap assets are reviewed in the same PR as code that depends on them.
- CI and Docker builds always ship consistent DDL.

**Negative**

- Changing bootstrap SQL requires a rebuild and redeploy.
- Empty dev/prod seed files mean both commands produce identical schema contents today.
- Operators may confuse bootstrap (`init-*`) with incremental migrations (`migrate`); they are different code paths.

**Operational impact**

- Docker entrypoint starts the application only; bootstrap is a separate CLI/Make command.
- Bootstrap is destructive (drops schema); see ADR 0008.

## Alternatives Considered

**Filesystem SQL only (mount `assets/db/init` at runtime)**

- Rejected: couples deployment to volume mounts and risks version skew between binary and SQL files.

**Flyway/Liquibase/golang-migrate as sole mechanism**

- Deferred: incremental migration tooling is planned via `db migrate`, but bootstrap reset remains a distinct operational command for greenfield environments.

**Single combined init command without dev/prod split**

- Rejected: retaining `init-dev` and `init-prod` entrypoints preserves a hook for environment-specific seed data even though seed files are empty today.

## Relationship To Future BNPL Domains

Bootstrap SQL will eventually create base platform tables shared by all domains (audit log, event outbox, reference data). Domain-specific tables (contracts, installments, merchants) should be added through **filesystem migrations** (`db migrate`) once that path is fully wired, not through destructive bootstrap, so production schemas can evolve without full resets.

Seed data distinctions:

- `000_dev.sql` — synthetic merchants, test customers, fixture credit limits (future).
- `000_prod.sql` — minimal reference data only; no synthetic PII (future).

Until non-destructive migration workflows exist, embedded bootstrap remains appropriate for greenfield and development resets only.
