# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Documentation

- Rewrote README and expanded architecture, API, database, configuration, and operations docs.
- Added security documentation and eight architecture decision records (ADRs).
- Clarified that BNPL business domains and HTTP endpoints are not yet implemented.
- Documented destructive nature of `init-dev`, `init-prod`, and default Docker entrypoint.

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
