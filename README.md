# BNPL Platform

Go backend for a Buy Now Pay Later (BNPL) financial platform. The repository is being built incrementally: today it provides platform foundations (configuration, PostgreSQL bootstrap, infrastructure adapters, health probes, graceful shutdown, migrations scaffolding, CLI, HTTP server shell, Docker assets, and CI/CD). The long-term goal is a regulated BNPL engine for credit decisions, installment plans, repayments, collections, and more.

## Current Status

**This repository does not yet implement BNPL business workflows.**

What exists today:

- Single-binary CLI (`bnpl`) with Cobra subcommands
- YAML configuration with `${ENV}` expansion, semver version validation (0.2.0), and provider-based `infrastructure:` section
- PostgreSQL connection pooling, outbox and health repositories, and transaction adapters (`pgx`)
- Destructive schema bootstrap (`db init-dev`, `db init-prod`) including `integration_outbox` table
- Filesystem migration runner (`db migrate`) — wired; migration directory may be empty
- Gin HTTP server with trace-ID middleware, Kubernetes-style `/healthz/live` and `/healthz/ready` probes, and an empty `/api/v1` route group
- Graceful shutdown on `SIGINT` / `SIGTERM` with lifecycle-aware health probes
- Infrastructure adapters: slog/noop logging, noop/redis cache, noop/kafka messaging, noop/gobreaker circuit breaker, in-process task runtime
- Transactional outbox publisher use case and scheduled publishing task
- Docker image build and local Compose infrastructure (PostgreSQL, Kafka)
- GitHub Actions CI/CD and GitLab CI pipelines
- Unit and integration tests across bootstrap, config, infrastructure, HTTP, and core packages

What is **not** implemented:

- Borrower credit evaluation, installment plans, credit limits, checkout decisions
- Contract, repayment, payment, collections, or legal-case domains
- Concrete HTTP business API handlers or business aggregates
- Production-safe, non-destructive schema evolution as default deploy path
- Circuit breaker usage at call sites, distributed tracing export, or HTTP metrics
- Domain Kafka consumer handlers or outbox row producers
- Production readiness

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
cmd/bnpl/bootstrap Application wiring (repos, infrastructure, tasks, routes, shutdown)
pkg/core          Domain models, repository ports, use cases, infrastructure ports
pkg/adapter       Config, Gin HTTP, PostgreSQL, infrastructure implementations
configs/          Environment-expanded YAML configuration (versions 0.2.0)
```

Dependency flow is inward: adapters and CLI depend on use cases; use cases depend on `pkg/core` ports; PostgreSQL and infrastructure adapters implement those ports.

Details: [doc/architecture.md](doc/architecture.md) · ADRs: [doc/adr/](doc/adr/)

## Repository Structure

| Path | Role |
| --- | --- |
| `cmd/bnpl/` | Process entrypoint and Cobra commands |
| `cmd/bnpl/bootstrap/` | Startup wiring, graceful shutdown, HTTP and tasks |
| `pkg/core/model/` | Shared data structures (`Secret`, `Migration`, `OutboxMessage`, `SemVer`) |
| `pkg/core/repo/` | Database and repository port interfaces |
| `pkg/core/health/` | Lifecycle and dependency-check ports for probes |
| `pkg/core/usecase/` | `migrationsuc`, `outboxpublisheruc`, `healthcheckuc` |
| `pkg/core/cache`, `clock`, `log`, `messaging`, `task`, `trace`, `circuitbreaker` | Infrastructure port interfaces |
| `pkg/adapter/config/` | YAML loading, version validation, provider factories |
| `pkg/adapter/infrastructure/` | Cache, clock, log, messaging, task, trace, health, circuit breaker adapters |
| `pkg/adapter/db/postgres/` | `pgx` pool, outbox, health, migration adapters |
| `pkg/adapter/restful/gin/` | Gin wrapper, middleware, healthrs, serdser, pagination |
| `pkg/core/mock/` | Mockery-generated test doubles |
| `configs/config.yaml` | Canonical configuration shape |
| `scripts/docker-entrypoint.sh` | Container startup: optional DB init then HTTP server |
| `doc/` | Architecture, operations, security, and ADR documentation |

## Configuration Overview

Configuration is loaded from YAML with `${VAR}` placeholders expanded from the process environment. `versions.config` and `versions.database` must match binary-supported semver **0.2.0**.

Resolution order for the config file path:

1. `--config` / `-c` CLI flag
2. `CONFIG_FILE` environment variable
3. Default: `configs/config.yaml`

Key environment variables are documented in [.env.example](.env.example) and [doc/configuration.md](doc/configuration.md).

**Local development with minimal dependencies** — defaults in `.env.example` already use:

```bash
CACHE_PROVIDER=noop
MESSAGING_PROVIDER=noop
```

PostgreSQL is still required for the HTTP server. Redis is only required when `CACHE_PROVIDER=redis`.

## Local Development

**Requirements:** Go 1.26.2, Docker with Compose (for local PostgreSQL; Kafka optional with noop messaging).

```bash
cp .env.example .env          # adjust values for your workstation
make up                       # start PostgreSQL (and Kafka)
make build                    # build build/bin/bnpl
make init-dev                 # destructive schema bootstrap (development)
DB_INIT_COMMAND= make run     # skip re-init on subsequent runs
# or: ./build/bin/bnpl
```

Direct CLI usage:

```bash
./build/bin/bnpl              # start HTTP server (default command)
./build/bin/bnpl db init-dev  # reset schema and apply embedded bootstrap SQL
./build/bin/bnpl db migrate   # apply filesystem migrations
```

Full operational guide: [doc/operations.md](doc/operations.md)

## Database Bootstrap

Schema initialization is **destructive**: both `init-dev` and `init-prod` drop and recreate the managed `bnpl` schema.

Flow (see [doc/database.md](doc/database.md)):

1. Admin role: drop schema → create schema → ensure `bnpl` role → grant privileges
2. Application role: execute embedded `000_schema.sql` (migrations + integration outbox) and environment seed SQL

Embedded SQL lives in `pkg/adapter/db/postgres/migrationsrp/` and is compiled into the binary via `go:embed`.

**Warning:** `init-prod` is not safe for live production databases.

## Migrations

| Mechanism | Command | Source | Status |
| --- | --- | --- | --- |
| Embedded bootstrap | `db init-dev` / `db init-prod` | `go:embed` SQL in binary | Implemented |
| Filesystem migrations | `db migrate` | `MIGRATIONS_FILES_DIR` | Wired — add SQL files under `assets/db/migrations` |

## Docker Usage

```bash
make build-image              # docker build -t bnpl-backend:v0.2.0 .
make run-image                # runs container with default entrypoint
```

Container startup (`scripts/docker-entrypoint.sh`):

1. Runs `bnpl db ${DB_INIT_COMMAND}` when `DB_INIT_COMMAND` is set (retries on failure)
2. Skips DB init when `DB_INIT_COMMAND` is empty
3. `exec bnpl` to start the HTTP server as PID 1

| Variable | Default | Purpose |
| --- | --- | --- |
| `CONFIG_FILE` | `configs/config.yaml` | Application configuration path |
| `APP_EXECUTABLE` | `./bnpl` | Binary path inside the container |
| `DB_INIT_COMMAND` | `init-dev` in `.env.example` | Bootstrap subcommand; empty skips init |
| `DB_INIT_MAX_ATTEMPTS` | `10` | Init retry count |
| `DB_INIT_RETRY_INTERVAL_SECONDS` | `3` | Seconds between retries |

## Makefile Usage

| Target | Description |
| --- | --- |
| `make build` | Compile static binary to `build/bin/bnpl` |
| `make run` | Build, start Compose, run entrypoint |
| `make up` / `make down` | Start/stop local Compose services |
| `make init-dev` / `make init-prod` | Destructive database bootstrap |
| `make test` | Run unit tests |
| `make test-coverage` | Generate coverage report |
| `make lint` | Run golangci-lint |
| `make build-image` | Build Docker image |

## Known Limitations

- No business API endpoints or domain aggregates
- `init-dev` and `init-prod` are destructive; unsuitable for evolving production schemas
- Development and production seed SQL files are empty
- `assets/db/migrations` may not exist until first migration is added
- No domain use case inserts outbox rows yet
- Kafka subscriber runs with placeholder handler only
- Circuit breaker constructed but not used by call sites
- `SHUTDOWN_DRAIN_DELAY` configured but not applied in bootstrap shutdown
- `scripts/deploy.sh` referenced by CD workflow is missing
- **Not production-ready** — platform foundations only

## Roadmap

Near-term platform work:

1. Add initial filesystem migration files
2. Wire `SHUTDOWN_DRAIN_DELAY` into bootstrap shutdown sequence
3. Implement domain Kafka consumer handlers for payment results
4. Use circuit breaker at external dependency call sites
5. Introduce first BNPL domain aggregates (Contract, Decision, CreditLimit)
6. Non-destructive production migration workflow
7. Metrics, tracing export, and audit persistence
8. Reduce admin credential exposure during routine operations

## Documentation Index

| Document | Contents |
| --- | --- |
| [doc/architecture.md](doc/architecture.md) | Layers, infrastructure, flows, gaps |
| [doc/api.md](doc/api.md) | HTTP server, health probes, trace headers |
| [doc/database.md](doc/database.md) | Bootstrap, migrations, outbox |
| [doc/configuration.md](doc/configuration.md) | YAML, env vars, providers |
| [doc/operations.md](doc/operations.md) | Makefile, Docker, shutdown, probes |
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
