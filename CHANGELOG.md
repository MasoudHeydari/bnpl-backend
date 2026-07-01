# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.2.0] - 2026-06-30

### Added

- Provider-based `infrastructure:` configuration section (logger, cache, messaging, task, clock, trace, circuit breaker, shutdown, health checks).
- Core infrastructure ports: `cache`, `clock`, `log`, `messaging`, `task`, `trace`, `circuitbreaker`, `health`.
- Cache adapters: noop, Redis (`go-redis`); in-memory adapter for tests (not config-selectable).
- Logging adapters: noop, slog (JSON/text).
- Messaging adapters: noop, Kafka (IBM Sarama producer and consumer group) with `messaging.Health` for broker checks.
- Circuit breaker adapters: noop, gobreaker (`sony/gobreaker/v2`).
- In-process task runtime with cron/interval triggers; noop task provider.
- Scheduled tasks: runtime goroutine metrics, outbox publisher.
- `integration_outbox` table, `outboxrp` repository, and `outboxpublisheruc` use case.
- `healthrp` repository, `healthcheckuc` use case, and `GET /healthz/live` / `GET /healthz/ready` HTTP probes.
- Composite readiness dependency checks (database, cache, messaging) via `infrastructure.health.checks`.
- `cmd/bnpl/bootstrap` package: application wiring, graceful shutdown on SIGINT/SIGTERM, background workers.
- Config and database schema semver validation at `config.Load` (`versions.config` / `versions.database` = 0.2.0).
- Trace-ID middleware (`X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id`) and ULID trace provider.
- HTTP `serdser` success/error envelopes and Gin pagination helpers.
- Core and PostgreSQL pagination utilities.
- Expanded `cerr` error codes (including `SERVICE_UNAVAILABLE`) and HTTP status mapping.
- Mockery configuration (`.mockery.yaml`) and generated mocks under `pkg/core/mock`.
- ADRs 0009–0014 for infrastructure, cache, tasks, outbox, messaging, and health/shutdown decisions.

### Changed

- HTTP server startup moved from `command.startWebServer` to `bootstrap.Boot` with full infrastructure wiring.
- `gin.trusted_proxies` and `usecases.migration.files-dir` are decoded from YAML.
- `000_schema.sql` includes `integration_outbox` DDL and indexes.
- Docker entrypoint: empty `DB_INIT_COMMAND` skips DB init; added init retry loop.
- Config path resolution: CLI flag → `CONFIG_FILE` → `configs/config.yaml`.
- Kafka message subscriber starts with bootstrap (placeholder handler until domain handlers exist).
- README and all `doc/` guides updated to reflect 0.2.0 platform state.

### Infrastructure

- Redis cache startup validates connectivity with `PING`.
- Kafka producer configurable acks, retries, and consumer topic list.
- Dual clock timezones: system (UTC) and business (Asia/Tehran default).
- Outbox publisher exponential backoff via use case configuration.
- Graceful shutdown: HTTP → tasks/consumer → drain → publisher/subscriber/cache/DB; lifecycle probes during drain.

### Tests

- **Bootstrap:** `app_test`, `infrastructure_test`, `repo_test`, `route_test`, `shutdown_test`, `usecase_test`.
- **Config:** `database_test`, `gin_test`, `infrastructure_test`, `load_test`, `shutdown_test`, `usecase_test`, `vers_test`.
- **PostgreSQL:** `error_test`, `pagination_test`; `healthrp` package (no dedicated tests).
- **Infrastructure:** cache memory/noop/redis, circuit breaker noop/gobreaker, clock fixed, log noop, messaging noop, task noop/inprocess.
- **HTTP:** Gin pagination, serdser bind/success, healthrs resource, trace-ID middleware.
- **Core:** `cerr`, `model`, `pagination`, `trace`, `healthcheckuc`, `outboxpublisheruc`.
- Redis adapter tests use `miniredis`.

### Documentation

- Rewrote `doc/architecture.md`, `doc/configuration.md`, `doc/operations.md`, `doc/database.md`, `doc/api.md`.
- Updated ADRs 0001, 0003, 0006, 0009, 0013 with amendments; added ADRs 0009–0014.
- Expanded `.env.example` with infrastructure, shutdown, health, and circuit breaker variables.

### Known gaps

- No BNPL business domains, HTTP business handlers, or outbox insert use cases.
- Circuit breaker constructed but not used at domain call sites.
- Kafka consumer has placeholder handler only (no payment-result processing).
- `SHUTDOWN_DRAIN_DELAY` decoded but not applied in bootstrap shutdown timing.
- `memory` cache not exposed as a configuration provider.
- No HTTP metrics or OpenTelemetry tracing export.
- `assets/db/migrations` may be empty; dev/prod seed SQL still empty.
- `scripts/deploy.sh` missing; CD deploy incomplete.
- Destructive `init-dev`/`init-prod` remain unsuitable for production schema evolution.
- **Not production-ready.**

## [0.1.0] - 2026-06-12

### Added

- Cobra-based CLI: default HTTP server startup; `db init-dev`, `db init-prod`, and `db migrate` subcommands.
- YAML configuration loading with `${ENV}` expansion via `configs/config.yaml`.
- PostgreSQL connection pooling and transaction adapters on `pgx` / `pgxpool`.
- Destructive schema initialization: admin-role schema recreation and application-role privilege setup.
- Embedded SQL bootstrap assets (`000_schema.sql`; dev/prod seed files empty).
- Migration use case for sequential filesystem SQL execution with `migrations` table registration.
- Gin HTTP server bootstrap with optional logger and recovery middleware; empty `/api/v1` route group.
- Dockerfile, Docker Compose (PostgreSQL, Kafka), and `scripts/docker-entrypoint.sh`.
- GitHub Actions CI/CD and GitLab CI test/build/docker pipelines.

### Infrastructure

- PostgreSQL repository implementations under `pkg/adapter/db/postgres`.
- Schema repository (`schemarp`) and migration repository (`migrationsrp`).
- `model.Secret` for redacted configuration credentials.

### Known gaps (0.1.0)

- No concrete HTTP API handlers or BNPL business logic.
- `000_dev.sql` and `000_prod.sql` are empty; `init-dev` and `init-prod` yield the same schema.
- Filesystem migration directory not present; YAML migration path not decoded into config struct.
- Kafka, Redis, and structured logging env vars not wired in Go code.
- `scripts/deploy.sh` referenced by CD workflow but not included in repository.
