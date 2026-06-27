# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Added

- Provider-based `infrastructure:` configuration section (logger, cache, messaging, task, clock, trace).
- Core infrastructure ports: `cache`, `clock`, `log`, `messaging`, `task`, `trace`.
- Cache adapters: noop, Redis (`go-redis`); in-memory adapter for tests (not config-selectable).
- Logging adapters: noop, slog (JSON/text).
- Messaging adapters: noop, Kafka (IBM Sarama producer and consumer group).
- In-process task runtime with cron/interval triggers; noop task provider.
- Scheduled tasks: runtime goroutine metrics, outbox publisher.
- `integration_outbox` table, `outboxrp` repository, and `outboxpublisheruc` use case.
- `cmd/bnpl/bootstrap` package for application wiring (repos, infrastructure, use cases, tasks, routes).
- Trace-ID middleware (`X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id`) and ULID trace provider.
- HTTP `serdser` success/error envelopes and Gin pagination helpers.
- Core and PostgreSQL pagination utilities.
- Expanded `cerr` error codes and HTTP status mapping.
- Mockery configuration and generated mocks under `pkg/core/mock`.
- ADRs 0009–0013 for infrastructure, cache, tasks, outbox, and messaging decisions.

### Changed

- HTTP server startup moved from `command.startWebServer` to `bootstrap.Boot` with full infrastructure wiring.
- `gin.trusted_proxies` and `usecases.migration.files-dir` are now decoded from YAML.
- `000_schema.sql` includes `integration_outbox` DDL and indexes.
- Docker entrypoint: empty `DB_INIT_COMMAND` skips DB init; added init retry loop.
- Config path resolution: CLI flag → `CONFIG_FILE` → `configs/config.yaml`.
- README and all `doc/` guides updated to reflect current code.

### Infrastructure

- Redis cache startup validates connectivity with `PING`.
- Kafka producer configurable acks, retries, and consumer topic list.
- Dual clock timezones: system (UTC) and business (Asia/Tehran default).
- Outbox publisher exponential backoff via use case configuration.

### Tests

- Unit tests: cache noop/redis, messaging noop, config provider factories, task noop/inprocess, pagination (core, postgres, gin), log noop.
- Redis adapter tests use `miniredis`.

### Documentation

- Rewrote `doc/architecture.md`, `doc/configuration.md`, `doc/operations.md`, `doc/database.md`, `doc/api.md`.
- Updated ADRs 0001, 0003, 0006 with 2026-06-27 amendments.
- Added ADRs 0009–0013.
- Expanded `.env.example` with infrastructure and task variables.

### Known gaps

- No BNPL business domains, HTTP handlers, or outbox insert use cases.
- Circuit breaker not implemented (mentioned in planning context only).
- Kafka subscriber not started in bootstrap; no payment-result consumer handlers.
- `memory` cache not exposed as a configuration provider.
- No health/readiness endpoints, HTTP metrics, or OpenTelemetry tracing.
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
