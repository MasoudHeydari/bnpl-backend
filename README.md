# BNPL Platform

Go backend for a Buy Now Pay Later (BNPL) financial platform. The repository is being built incrementally: today it provides platform foundations (configuration, PostgreSQL bootstrap, migrations scaffolding, CLI, HTTP server shell, Docker assets, and CI/CD). The long-term goal is a regulated BNPL engine for credit decisions, installment plans, repayments, collections, and more.

## Current Status

**This repository does not yet implement BNPL business workflows.**

What exists today:

- Single-binary CLI (`bnpl`) with Cobra subcommands
- YAML configuration with `${ENV}` expansion
- PostgreSQL connection pooling and transaction adapters (`pgx`)
- Destructive schema bootstrap (`db init-dev`, `db init-prod`)
- Filesystem migration runner (`db migrate`) — partially wired; see [Known limitations](#known-limitations)
- Gin HTTP server bootstrap with an empty `/api/v1` route group
- Docker image build and local Compose infrastructure (PostgreSQL, Kafka)
- GitHub Actions CI/CD and GitLab CI pipelines

What is **not** implemented:

- Borrower credit evaluation, installment plans, credit limits, checkout decisions
- Contract, repayment, payment, collections, or legal-case domains
- Concrete HTTP API handlers or business aggregates
- Production-safe, non-destructive schema evolution
- Kafka, Redis, or structured application logging integration in Go code

See [doc/architecture.md](doc/architecture.md) for layer details and [doc/api.md](doc/api.md) for HTTP status.

## BNPL Platform Vision

The intended platform will support:

| Domain | Purpose |
| --- | --- |
| Contract | BNPL agreement lifecycle |
| Decision | Credit and checkout financing decisions |
| RepaymentPlan / Installment | Schedule generation and tracking |
| PaymentAttempt | Payment initiation and reconciliation |
| CreditLimit | Borrower exposure management |
| CollectionsCase / LegalCase | Delinquency and escalation workflows |
| MerchantProfile / MerchantBranchProfile | Merchant onboarding and branches |
| Restriction / Block | Risk controls |
| Audit | Regulatory and operational traceability |

These domains are **planned**, not present in the current codebase.

## Architecture Summary

```
cmd/bnpl          CLI entrypoint (default: HTTP server; subcommands: db *)
pkg/core          Domain models, repository ports, use cases, shared errors
pkg/adapter       Config, Gin HTTP, PostgreSQL implementations
assets/db/init    Symlinks to embedded bootstrap SQL (reference only)
configs/          Environment-expanded YAML configuration
```

Dependency flow is inward: adapters and CLI depend on use cases; use cases depend on `pkg/core/repo` interfaces; PostgreSQL adapters implement those interfaces.

Details: [doc/architecture.md](doc/architecture.md) · ADRs: [doc/adr/](doc/adr/)

## Repository Structure

| Path | Role |
| --- | --- |
| `cmd/bnpl/` | Process entrypoint and Cobra commands |
| `pkg/core/model/` | Shared data structures (`Secret`, `Migration`) |
| `pkg/core/repo/` | Database and repository port interfaces |
| `pkg/core/usecase/migrationsuc/` | Schema bootstrap and migration orchestration |
| `pkg/core/cerr/` | HTTP-oriented error wrappers |
| `pkg/adapter/config/` | YAML loading and dependency wiring |
| `pkg/adapter/db/postgres/` | `pgx` pool, connection, transaction adapters |
| `pkg/adapter/restful/gin/` | Gin wrapper, routes, serialization helpers |
| `pkg/adapter/db/postgres/migrationsrp/` | Embedded bootstrap SQL and migration repository |
| `pkg/adapter/db/postgres/schemarp/` | Schema and role administration SQL |
| `configs/config.yaml` | Canonical configuration shape |
| `scripts/docker-entrypoint.sh` | Container startup: DB init then HTTP server |
| `doc/` | Architecture, operations, security, and ADR documentation |

## Configuration Overview

Configuration is loaded from YAML with `${VAR}` placeholders expanded from the process environment.

Resolution order for the config file path:

1. `--config` / `-c` CLI flag
2. `CONFIG_FILE` environment variable
3. Default: `configs/config.yaml`

Key environment variables are documented in [.env.example](.env.example) and [doc/configuration.md](doc/configuration.md).

Database credentials use pgpass-format lines in configuration (`POSTGRES_ADMIN_PGPASS`, `POSTGRES_NORMAL_PGPASS`), not a `.pgpass` file read by the application. See [doc/security.md](doc/security.md).

## Local Development

**Requirements:** Go 1.26.2, Docker with Compose (for local PostgreSQL/Kafka).

```bash
cp .env.example .env          # adjust values for your workstation
make up                       # start PostgreSQL and Kafka
make build                    # build build/bin/bnpl
make init-dev                 # destructive schema bootstrap (development)
make run                      # start HTTP server via docker-entrypoint.sh
```

Direct CLI usage:

```bash
./build/bin/bnpl              # start HTTP server (default command)
./build/bin/bnpl db init-dev  # reset schema and apply embedded bootstrap SQL
./build/bin/bnpl db migrate   # apply filesystem migrations (see limitations)
```

Full operational guide: [doc/operations.md](doc/operations.md)

## Database Bootstrap

Schema initialization is **destructive**: both `init-dev` and `init-prod` drop and recreate the managed `bnpl` schema.

Flow (see [doc/database.md](doc/database.md)):

1. Admin role: drop schema → create schema → ensure `bnpl` role → grant privileges
2. Application role: execute embedded `000_schema.sql` and environment seed SQL in one transaction

Embedded SQL lives in `pkg/adapter/db/postgres/migrationsrp/` and is compiled into the binary via `go:embed`. The `assets/db/init/` directory contains symlinks to those files for reference only.

**Warning:** `init-prod` is not safe for live production databases. It performs the same destructive reset as `init-dev`.

## Migrations

Two separate mechanisms exist:

| Mechanism | Command | Source | Status |
| --- | --- | --- | --- |
| Embedded bootstrap | `db init-dev` / `db init-prod` | `go:embed` SQL in binary | Implemented |
| Filesystem migrations | `db migrate` | `MIGRATIONS_FILES_DIR` (default `assets/db/migrations`) | Partially wired — directory missing; YAML path not decoded |

Bootstrap and migrations are independent concerns. Bootstrap recreates the schema; migrations apply incremental SQL files and record them in the `migrations` table.

## Docker Usage

Build and run:

```bash
make build-image              # docker build -t bnpl-backend:v0.1.0 .
make run-image                # runs container with default entrypoint
```

Container startup (`scripts/docker-entrypoint.sh`):

1. Runs `bnpl db ${DB_INIT_COMMAND:-init-dev}` (destructive by default)
2. `exec bnpl` to start the HTTP server as PID 1

Environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `CONFIG_FILE` | `configs/config.yaml` | Application configuration path |
| `APP_EXECUTABLE` | `./bnpl` | Binary path inside the container |
| `DB_INIT_COMMAND` | `init-dev` | Bootstrap subcommand (`init-dev` or `init-prod`) |

See [doc/operations.md](doc/operations.md) and ADR [0006](doc/adr/0006-docker-entrypoint-startup-flow.md).

## Makefile Usage

| Target | Description |
| --- | --- |
| `make build` | Compile static binary to `build/bin/bnpl` |
| `make run` | Build and start via `scripts/docker-entrypoint.sh` |
| `make up` / `make down` | Start/stop local Compose services |
| `make init-dev` / `make init-prod` | Destructive database bootstrap |
| `make test` | Run unit tests |
| `make lint` | Run golangci-lint |
| `make build-image` | Build Docker image |
| `make publish-image-to-hamravesh` | Tag and push to Hamravesh registry |

The Makefile auto-loads `.env` via `-include .env`.

## Deployment Notes

- CI (`.github/workflows/ci.yml`): gofmt, vet, lint, tests, security scans, Docker build validation
- CD (`.github/workflows/cd.yml`): builds and pushes images to GHCR and Hamravesh; deploy job references `scripts/deploy.sh`, which is **not present** in this repository
- GitLab CI (`.gitlab-ci.yml`): test, build, Docker push on `main`; Helm/deploy stages are commented out
- Deploy production images by immutable SHA tag, not `latest`

## Known Limitations

- No business API endpoints or domain aggregates
- `init-dev` and `init-prod` are destructive; unsuitable for evolving production schemas
- Development and production seed SQL files (`000_dev.sql`, `000_prod.sql`) are empty
- `MIGRATIONS_FILES_DIR` is not decoded from YAML into the config struct
- `assets/db/migrations` directory does not exist
- `GIN_TRUSTED_PROXY` in YAML is ignored; Gin adapter hardcodes `127.0.0.1`
- Kafka env vars in `.env.example` are unused by Go code
- `make run` prints loaded config including redacted secret hashes via `fmt.Printf`
- `scripts/deploy.sh` referenced by CD workflow is missing
- No unit or integration test files in the repository yet

## Roadmap

Near-term platform work (not yet implemented):

1. Wire `usecases.migration.files-dir` and add initial migration files
2. Implement HTTP health and readiness endpoints
3. Introduce first BNPL domain aggregates (Contract, Decision, CreditLimit)
4. Non-destructive production migration workflow
5. Structured logging, metrics, and tracing
6. Kafka event publishing for audit and downstream consumers
7. Reduce admin credential exposure during routine operations

Business roadmap context (Persian product docs): [doc/pintoday.md](doc/pintoday.md), [doc/base_doc.md](doc/base_doc.md).

## Documentation Index

| Document | Contents |
| --- | --- |
| [doc/architecture.md](doc/architecture.md) | Layers, flows, current vs planned |
| [doc/api.md](doc/api.md) | HTTP server status |
| [doc/database.md](doc/database.md) | Bootstrap, migrations, roles, risks |
| [doc/configuration.md](doc/configuration.md) | YAML, env vars, wiring gaps |
| [doc/operations.md](doc/operations.md) | Makefile, Docker, CI/CD |
| [doc/security.md](doc/security.md) | Secrets, pgpass, operational risks |
| [doc/adr/](doc/adr/) | Architecture decision records |
| [CHANGELOG.md](CHANGELOG.md) | Release history |

## Contributing

1. Fork or branch from `main`
2. Copy `.env.example` to `.env` for local development
3. Run `make build`, `make test`, `make lint` before opening a merge request
4. Keep documentation aligned with code — do not describe unimplemented features as complete
5. Follow existing package layout: core interfaces inward, adapters outward
6. ADRs are required for significant architectural changes under `doc/adr/`

Go version, lint, and CI expectations are pinned in `go.mod`, `Makefile`, and `.github/workflows/ci.yml`.
