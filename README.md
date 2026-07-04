SQL formatter: [pgFormatter official GitHub](https://github.com/darold/pgFormatter).
Install `pg_format` yourself following its official installation instructions.
Run `make sql-fmt` to format all regular `.sql` files, or
`make sql-fmt SQL=assets/db/migrations/000006_example.sql` to format one.
Run `make sql-fmt-check` (or pass `SQL=...`) for read-only verification.
**Important:** formatting changes file bytes and SHA-256. Before the first
Production release, the canonical `pkg/adapter/db/postgres/migrationsrp/000_schema.sql`
may be revised. Its digest is checked
in `pkg/adapter/db/postgres/migrationsrp/settler.go`; migration digests
and PL/pgSQL function fingerprints are recorded in
`pkg/adapter/db/postgres/invariants/registry.go` and
`pkg/adapter/db/postgres/invariants/privilege_registry.go`. The baseline
becomes immutable after the first Production release. Applied SQL
migration checksums are also stored in `bnpl.migrations`. Review and
synchronize checksums intentionally; formatting already-applied migration
files can block upgrades. Never silently rewrite DB migration history.

Current database/security architecture: [roles](doc/architecture/database-roles.md),
[privilege policy](doc/architecture/database-privileges.md), and
[build/deployment commands](doc/architecture/database-deployment.md).
Runtime is `cmd/bnpl` (`configs/runtime.yaml`); schema administration is the
independent `cmd/bnpl-dbadmin` (`configs/dbadmin.yaml`). Database commands below
use the admin executable; Runtime never loads administrative credentials.

# BNPL Platform

Go backend for the Buy Now Pay Later platform, with implemented checkout decisioning,
installment offers, customer credit/exposure, contract lifecycle, repayment,
Financial Core payment orchestration, collections, merchant operations, and
transactional Inbox/Outbox messaging.

## Current Status (pre-first-production-release)

The main business flows are implemented. This repository is **pre-release**:
tests passing does not by itself establish production deployment readiness.

- HTTP business endpoints under `/api/v1`, including partner terminal
  authentication, token-based hosted checkout, and Gateway-authenticated routes.
- Domain models and use cases for decisions, contracts, installments, credit
  accounts, repayment, payment reconciliation, collections, and policies.
- PostgreSQL financial invariants, idempotency, and transactional Inbox/Outbox.
- Financial Core gRPC payment-session creation and authoritative payment-status
  retrieval; Kafka payment results and reconciliation-result consumers.
- Runtime `cmd/bnpl` and separate privileged DB administration
  `cmd/bnpl-dbadmin` binaries, with development and production build profiles.
- Production deployment still requires a **verified trusted-Gateway boundary**:
  BNPL trusts injected `X-Auth-*` headers after upstream authentication. The
  runtime does not independently verify bearer JWTs.
- Required release validation: `make test-all`, `make test-e2e` in an isolated
  environment, DB invariant/privilege verification, and integration-contract
  tests with Financial Core.

See [architecture](doc/architecture.md), [API](doc/api.md),
[security](doc/security.md), and [operations](doc/operations.md).

## BNPL Platform Vision

The intended platform will support:

| Domain | Purpose |
| --- | --- |
| Contract | BNPL agreement lifecycle |
| Decision | Credit and checkout financing decisions |
| RepaymentPlan / Installment | Schedule generation and tracking |
| PaymentAttempt | Payment initiation and reconciliation |
| CreditLimit | Customer exposure management |
| CollectionsCase / LegalCase | Delinquency and escalation workflows |
| MerchantProfile / MerchantBranchProfile | Channel-neutral merchants and physical/online selling points |
| Restriction / Block | Risk controls |
| Audit | Regulatory and operational traceability |

Several listed domains are implemented; others (including legal-case escalation) remain future scope. See the current use cases and API for precise coverage.

## Architecture Summary

```
cmd/bnpl          Runtime entrypoint (HTTP, integrations, workers and policy assignment)
cmd/bnpl-dbadmin  Privileged database initialization and migration commands
cmd/bnpl/bootstrap Application wiring (repos, infrastructure, tasks, routes, shutdown)
pkg/core          Domain models, repository ports, use cases, infrastructure ports
pkg/adapter       Config, Gin HTTP, PostgreSQL, infrastructure implementations
configs/          Environment-expanded YAML configuration (config 0.3.0 / database 0.2.0)
```

Dependency flow is inward: adapters and CLI depend on use cases; use cases depend on `pkg/core` ports; PostgreSQL and infrastructure adapters implement those ports.

Details: [doc/architecture.md](doc/architecture.md) · ADRs: [doc/adr/](doc/adr/)

## Repository Structure

| Path | Role |
| --- | --- |
| `cmd/bnpl/` | Runtime process entrypoint, server, operational policy commands |
| `cmd/bnpl/bootstrap/` | Startup wiring, graceful shutdown, HTTP and tasks |
| `pkg/core/model/` | Domain aggregates and financial state machines |
| `pkg/core/repo/` | Database and repository port interfaces |
| `pkg/core/health/` | Lifecycle and dependency-check ports for probes |
| `pkg/core/usecase/` | Checkout, contract, credit, repayment, collection, reconciliation and worker use cases |
| `pkg/core/cache`, `clock`, `log`, `messaging`, `task`, `trace`, `circuitbreaker` | Infrastructure port interfaces |
| `pkg/adapter/config/` | YAML loading, version validation, provider factories |
| `pkg/adapter/infrastructure/` | Cache, clock, log, messaging, task, trace, health, circuit breaker adapters |
| `pkg/adapter/db/postgres/` | `pgx` pool, outbox, health, migration adapters |
| `pkg/adapter/restful/gin/` | Gin wrapper, middleware, healthrs, serdser, pagination |
| `pkg/core/mock/` | Mockery-generated test doubles |
| `configs/runtime.yaml` | Canonical configuration shape |
| `scripts/runtime/entrypoint.sh` | Runtime container startup; does not initialize the DB |
| `assets/local-development/envoy/development/config.yaml` | Local Envoy snapshot (host-specific; gateway proof placeholder) |
| `doc/` | Architecture, operations, security, and ADR documentation |

## Configuration Overview

Configuration is loaded from YAML with `${VAR}` placeholders expanded from the process environment. `versions.config` must be compatible with **0.3.0** and `versions.database` with **0.2.0**.

Resolution order for the config file path:

1. `--config` / `-c` CLI flag
2. `CONFIG_FILE` environment variable
3. Default: `configs/runtime.yaml`

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
make up                       # start infrastructure and bootstrap local Kafka topics
make build                    # build build/bin/bnpl
make init-dev                 # one-time (or reset): recreates schema bnpl
make run                      # start app; does not re-init the database
# or: ./build/bin/bnpl
```


Local Kafka topics are provisioned automatically by the one-shot `kafka-init`
service in `docker-compose.local.yml`, after Kafka passes its healthcheck.
The job uses topic names from `.env` (with local defaults), ignores duplicates,
and creates missing topics with **3 partitions / replication factor 1** on the
single-broker development cluster. Existing topics are not modified. To use
Kafka messaging in the host-side BNPL process, set `MESSAGING_PROVIDER=kafka`
in `.env` (the example defaults to `noop`).

```bash
docker compose -f docker-compose.local.yml logs kafka-init
docker compose -f docker-compose.local.yml ps -a kafka-init
docker exec bnpl-kafka /opt/kafka/bin/kafka-topics.sh --bootstrap-server localhost:9092 --list
```

The init service should exit successfully (code 0). It may still be running
immediately after `make up`; check its completion before starting a Kafka-
dependent process. This provisioning is **local-only**, not a production topic
management strategy.

Direct CLI usage:

```bash
./build/bin/bnpl                                            # start HTTP server (default command)
./build/bin/bnpl-dbadmin-development --config configs/dbadmin.yaml db init-dev                                # reset schema and apply embedded bootstrap SQL
./build/bin/bnpl-dbadmin-development --config configs/dbadmin.yaml db migrate                                 # apply filesystem migrations
./build/bin/bnpl policy-assignment import-trusted \
  --file assets/csv/trusted-customers/trusted-customers.csv --actor ops@example          # bulk-assign the trusted policy bundle
```

Full operational guide: [doc/operations.md](doc/operations.md)

## Database Bootstrap

`init-dev` is **destructive** and requires `APP_ENVIRONMENT=development`: it drops and recreates managed schema `bnpl`.
`init-prod` requires `APP_ENVIRONMENT=production`: it creates schema `bnpl` only when missing and refuses when it already exists (use `db migrate` instead).

Flow (see [doc/database.md](doc/database.md)):

1. DB Admin provisions/validates `bnpl_owner`, `bnpl_migrator`, and restricted `bnpl` roles and the owned schema.
2. The migrator assumes `bnpl_owner` to install the complete embedded `000_schema.sql`, including financial integrity triggers and reviewed Runtime privileges, followed by environment-specific seed SQL.

Embedded SQL lives in `pkg/adapter/db/postgres/migrationsrp/` and is compiled into the binary via `go:embed`.

**Note:** The first Production release freezes the complete initial schema. Later releases use `db migrate` for versioned changes; `init-prod` is only for a fresh schema. Old disposable Development databases should be reset rather than silently reusing obsolete migration history.

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

Container startup (`scripts/runtime/entrypoint.sh`):

1. Logs a short startup banner
2. `exec bnpl` to start the HTTP server as PID 1

Database bootstrap is separate: `make init-dev`, `make init-prod`, or `bnpl db migrate`.

| Variable | Default | Purpose |
| --- | --- | --- |
| `CONFIG_FILE` | `configs/runtime.yaml` | Application configuration path |
| `APP_EXECUTABLE` | `./bnpl` | Binary path inside the container |

## Makefile Usage

| Target | Description |
| --- | --- |
| `make build` | Compile static binary to `build/bin/bnpl` |
| `make run` | Build, start Compose, run entrypoint (starts app only) |
| `make up` / `make down` | Start/stop local Compose services |
| `make init-dev` | Destructive development schema recreate (`APP_ENVIRONMENT=development`) |
| `make init-prod` | One-time production schema create (`APP_ENVIRONMENT=production`; refuses if exists) |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests in an isolated temporary PostgreSQL 18 container |
| `make test-coverage` | Generate coverage report |
| `make lint` | Run golangci-lint |
| `make build-image` | Build Docker image |

## Release Preconditions and Known Limits

- The first production schema is installed with the separate DB Admin executable; do not put DB admin credentials in runtime deployments.
- Enforce and test the trusted Gateway boundary. All untrusted incoming `X-Auth-*` values must be stripped and replaced after JWT validation; prevent direct access to the BNPL business port from untrusted sources.
- Validate Financial Core idempotency, authoritative status lookup, payment session expiration and Kafka delivery contracts in a safe integration environment.
- Apply the first-release database invariant/privilege verifiers, run unit/integration/E2E tests, and verify Kafka handler-topic alignment.
- The outbox has at-least-once delivery semantics; downstream consumers must be idempotent.
- Further production-hardening work (for example deployment-specific network policy, secret provisioning and runbooks) is described in the operations and security documents.

## Future Work

Documented future items should be distinguished from existing business flows:
deployment manifests and rollback procedures, operational monitoring and
alerting, and additional integration-contract tests remain environment-specific
tasks. See [operations](doc/operations.md).

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
