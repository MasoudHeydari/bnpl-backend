# Architecture

## Purpose

This document describes the **current** Go backend structure and how it is intended to support a future Buy Now Pay Later (BNPL) platform. The repository today provides platform infrastructure, database bootstrap, and HTTP scaffolding — not a complete BNPL engine.

## Current State vs Intended Direction

| Area | Current state | Intended direction |
| --- | --- | --- |
| HTTP API | Gin server, trace middleware, `/healthz/live` and `/healthz/ready`, empty `/api/v1` group | REST APIs for checkout, contracts, repayments, admin |
| Database | `migrations` + `integration_outbox` tables; destructive init | Full BNPL schema with forward migrations |
| Use cases | `migrationsuc`, `outboxpublisheruc`, `healthcheckuc` | Credit, installment, payment, collections use cases |
| Events | Kafka adapter + noop; outbox publisher job; subscriber started (placeholder handler) | Domain events for audit and integrations |
| Cache | Redis / noop adapters wired | Idempotency, rate limits, session state |
| Auth | None | Role-based access for admin and merchant APIs |
| Observability | slog/noop logger, trace headers, health probes, goroutine metrics task | Metrics, distributed tracing, audit persistence |
| Shutdown | SIGINT/SIGTERM graceful shutdown with lifecycle probes | Production orchestration with wired drain delay |

**Do not assume business aggregates (Contract, Installment, MerchantProfile, etc.) exist in code.** They are planned domains listed for architectural alignment only.

## BNPL Platform Vision

The long-term system will handle:

- Borrower credit evaluation and credit limit management
- Installment plan generation and contract lifecycle
- Checkout financing decisions
- Payment tracking and delinquency handling
- Collections and legal case escalation
- Merchant and branch management
- Auditability and regulatory compliance

Planned domain names (not yet implemented):

Contract · Decision · RepaymentPlan · Installment · PaymentAttempt · CreditLimit · CollectionsCase · LegalCase · MerchantProfile · MerchantBranchProfile · Restriction · Block · Audit

Product context (non-English): [pintoday.md](pintoday.md), [base_doc.md](base_doc.md).

## Runtime Entrypoints

Single binary `bnpl` (Cobra):

| Command | Behavior |
| --- | --- |
| `bnpl` (default) | HTTP server + infrastructure bootstrap + scheduled tasks |
| `bnpl db init-dev` | Destructive bootstrap + dev seed SQL |
| `bnpl db init-prod` | Destructive bootstrap + prod seed SQL |
| `bnpl db migrate` | Filesystem SQL migrations |

See ADR [0005](adr/0005-single-binary-cobra-commands.md).

## Layers

### Core (`pkg/core`)

| Package | Role |
| --- | --- |
| `model` | `Secret`, `Migration`, `OutboxMessage` — no BNPL business models yet |
| `repo` | Port interfaces: Pool, Conn, Tx, Schema, Migrations, Outbox, Health |
| `health` | `LifecycleState`, `DependencyChecker` contracts for probes |
| `cache`, `clock`, `log`, `messaging`, `task`, `trace`, `circuitbreaker` | Infrastructure port interfaces |
| `pagination` | Cursor/page pagination models and helpers |
| `cerr` | Application error codes and HTTP status mapping |
| `usecase/migrationsuc` | Schema bootstrap and migration orchestration |
| `usecase/outboxpublisheruc` | Claim and publish integration outbox rows |
| `usecase/healthcheckuc` | Liveness and readiness evaluation |

**Boundary:** No imports of `pgx`, `gin`, `cobra`, `redis`, `sarama`, or YAML.

### Application bootstrap (`cmd/bnpl/bootstrap`)

| Package | Role |
| --- | --- |
| `bootstrap` | Loads config, wires repos, infrastructure, use cases, tasks, routes; graceful shutdown |
| `bootstrap/infrastructure.go` | Trace, clock, logger, cache, messaging, task, circuit breaker, lifecycle, dependency checker |
| `bootstrap/shutdown.go` | Ordered shutdown on SIGINT/SIGTERM |
| `bootstrap/usecase.go` | Builds migration, outbox publisher, and health check use cases |
| `bootstrap/repo.go` | PostgreSQL repository adapters (migrations, outbox, health) |

### Adapter (`pkg/adapter`, `cmd/bnpl/command`)

| Package | Role |
| --- | --- |
| `config` | YAML load, env expansion, provider factories |
| `restful/gin` | Gin wrapper, trace middleware, pagination helpers |
| `restful/gin/routes` | Route registration (`/healthz/*`, `/api/v1` group) |
| `restful/gin/healthrs` | Kubernetes-style liveness and readiness HTTP handlers |
| `restful/gin/serdser` | Binding, success/error envelopes with trace metadata |
| `db/postgres/healthrp` | Database ping repository for readiness |
| `db/postgres` | `pgx` pool, connection, transaction, pagination adapters |
| `db/postgres/outboxrp` | Integration outbox repository |
| `infrastructure/health` | Lifecycle state and composite dependency checkers |
| `infrastructure/*` | Cache, clock, log, messaging, task, trace, circuit breaker adapters |
| `cmd/bnpl/command` | CLI wiring |

## Infrastructure Layer

Provider-based infrastructure is configured under `infrastructure:` in YAML. Only the selected provider is constructed at startup.

| Concern | Core port | Providers | Default (.env.example) |
| --- | --- | --- | --- |
| Trace IDs | `trace.Provider` | ULID (`trace` package) | Always ULID |
| Clock | `clock.Clock` | System (dual timezone) | UTC + Asia/Tehran |
| Logger | `log.Logger` | `noop`, `slog` | `slog` / `info` / `json` |
| Cache | `cache.Cache` | `noop`, `redis` | `noop` |
| Messaging | `messaging.Publisher`, `Subscriber`, `Health` | `noop`, `kafka` (Sarama) | `noop` |
| Task runtime | `task.Runtime` | `noop`, `inprocess` | `inprocess` |
| Circuit breaker | `circuitbreaker.CircuitBreaker` | `noop`, `gobreaker` | `gobreaker` |
| Health probes | `health.LifecycleState`, `health.DependencyChecker` | `infrastructure/health` adapters | Checks: database, cache, messaging |

Additional adapter code exists but is **not** selectable via configuration:

| Package | Role |
| --- | --- |
| `infrastructure/cache/memory` | In-process cache for tests; not a config provider |

Circuit breaker is constructed at startup but **not yet used** by domain call sites.

See ADRs [0009](adr/0009-provider-based-infrastructure-configuration.md) through [0014](adr/0014-health-probes-and-graceful-shutdown.md).

## Dependency Direction

```
cmd/bnpl/command  →  cmd/bnpl/bootstrap  →  pkg/adapter/config
                                                    ↓
                              pkg/core/usecase  →  pkg/core/repo
                                                    ↑
                              pkg/adapter/db/postgres
                              pkg/adapter/infrastructure/*
```

No reverse dependencies from core into adapters.

## Request Flow (HTTP)

Current flow — no business handlers:

1. `bootstrap.Boot` loads config (`config.Load`) and validates `versions`
2. Opens normal-role pool
3. Wires repositories, infrastructure (including dependency checker and lifecycle state), use cases
4. Registers configured scheduled tasks and starts task runtime and message subscriber in background
5. Builds Gin engine with trace-ID middleware (first), optional recovery and logger
6. `routes.Register` attaches `/healthz/live` and `/healthz/ready` when enabled, plus `/api/v1` group (no business handlers)
7. Gin listens on `gin.host:gin.port`
8. On SIGINT/SIGTERM: readiness fails (`StartDraining`), bounded graceful shutdown, optional liveness failure on timeout

The pool is used by readiness dependency checks and background jobs. `/api/v1` has no business handlers yet.

## Database Flow

### Runtime (intended for future handlers)

1. Normal-role pool from config
2. Acquire connection via `Pool.Conn`
3. Execute SQL through `Conn` or `Tx`

### Bootstrap (`init-dev` / `init-prod`)

1. Admin pool → transaction → drop/create schema, role, privileges
2. Normal pool → transaction → embedded DDL + seed SQL

Both paths are **destructive**. See [database.md](database.md) and ADR [0008](adr/0008-destructive-database-initialization-strategy.md).

### Migrations (`db migrate`)

1. Read files from `usecases.migration.files-dir` (wired from YAML)
2. Per file: transaction → execute SQL → insert into `migrations` table

Bootstrap and migrations are separate mechanisms.

### Outbox publishing

1. `outboxpublisheruc.PublishDue` claims pending rows (`SKIP LOCKED`)
2. Publishes to configured messaging publisher (topic = `destination` column)
3. Marks rows published or schedules retry with exponential backoff

Triggered by the `outbox_publisher` scheduled task when `TASK_OUTBOX_PUBLISHER_ENABLED=true`.

## Configuration Flow

1. Resolve path: `--config` → `CONFIG_FILE` → `configs/config.yaml`
2. Read YAML, `os.ExpandEnv`, decode to `config.Config`
3. Validate `versions.config` and `versions.database` semver compatibility
4. Factories build pools, infrastructure providers, use cases, Gin engine

Known wiring gaps:

- No post-load validation for empty required env-expanded fields
- `bootstrap.InitDev` / `InitProd` still print loaded config via `fmt.Printf`
- `SHUTDOWN_DRAIN_DELAY` decoded but not applied in bootstrap shutdown timing
- Circuit breaker constructed but unused by call sites

Details: [configuration.md](configuration.md)

## How Infrastructure Supports Future BNPL Domains

| Today's decision | Future benefit |
| --- | --- |
| Repository ports | Swap PostgreSQL, mock repos in unit tests for credit rules |
| Transaction callbacks | Atomic contract + installment + outbox insert |
| Outbox + messaging | Reliable domain event delivery without dual-write |
| Cache abstraction | Idempotency keys, OTP state, rate limits |
| Provider config | Disable Redis/Kafka locally with noop providers |
| Layered packages | Add `usecase/contractuc` without touching `pgx` |
| `cerr` + `serdser` | Consistent HTTP errors and envelopes for merchant APIs |
| Trace headers | Correlate HTTP, jobs, and future audit events |
| Single binary | Ops commands for collections batch jobs alongside HTTP |

## Open Gaps

### HTTP

- [x] Register health/readiness endpoints (`GET /healthz/live`, `GET /healthz/ready`)
- [ ] Implement `/api/v1` handlers
- [ ] Wire authentication middleware
- [ ] Use database pool in business route handlers

### Business domains

- [ ] Define core models and repository ports for Contract, Decision, CreditLimit
- [ ] Implement use cases and PostgreSQL repositories
- [ ] Add API schemas and validation

### Database

- [ ] Create `assets/db/migrations` with initial forward migrations
- [ ] Populate dev/prod seed SQL or document intentional emptiness
- [ ] Production-safe, non-destructive migration workflow
- [ ] Reduce admin credential use to dedicated jobs
- [ ] Outbox insert API for domain use cases (no producer use case yet)

### Configuration

- [ ] Post-load validation for required fields
- [ ] Remove or gate config debug printing in CLI init commands
- [ ] Wire `SHUTDOWN_DRAIN_DELAY` into bootstrap shutdown sequence
- [ ] Use circuit breaker at external dependency call sites

### Integrations

- [ ] Domain Kafka consumer handlers (subscriber runs with placeholder handler today)
- [ ] Redis connectivity check skipped when provider is noop (by design)
- [ ] `memory` cache as optional provider for single-node dev

### Observability

- [ ] HTTP request metrics (latency, status codes)
- [ ] Pool stats and migration duration metrics
- [ ] Distributed tracing export (OpenTelemetry)
- [ ] Audit event persistence beyond outbox scaffolding

### Security

- [ ] AuthN/AuthZ for APIs
- [ ] Secret manager integration
- [ ] Audit log for sensitive operations

See [security.md](security.md) for current secret handling limits.

## Architecture Decision Records

| ADR | Topic |
| --- | --- |
| [0001](adr/0001-layered-architecture.md) | Layered architecture |
| [0002](adr/0002-repository-ports.md) | Repository ports |
| [0003](adr/0003-env-expanded-yaml-config.md) | Env-expanded YAML |
| [0004](adr/0004-embedded-schema-bootstrap.md) | Embedded bootstrap SQL |
| [0005](adr/0005-single-binary-cobra-commands.md) | Single binary / Cobra |
| [0006](adr/0006-docker-entrypoint-startup-flow.md) | Docker entrypoint |
| [0007](adr/0007-secret-handling-strategy.md) | Secret handling |
| [0008](adr/0008-destructive-database-initialization-strategy.md) | Destructive init |
| [0009](adr/0009-provider-based-infrastructure-configuration.md) | Provider-based infrastructure |
| [0010](adr/0010-cache-abstraction-and-adapters.md) | Cache abstraction |
| [0011](adr/0011-in-process-task-runtime.md) | In-process task runtime |
| [0012](adr/0012-transactional-outbox-publisher.md) | Transactional outbox |
| [0013](adr/0013-messaging-abstraction-kafka-noop.md) | Messaging abstraction |
| [0014](adr/0014-health-probes-and-graceful-shutdown.md) | Health probes and graceful shutdown |

## Package Map

| Path | Responsibility |
| --- | --- |
| `cmd/bnpl/main.go` | Delegates to `command.Execute()` |
| `cmd/bnpl/command/` | Cobra tree, server and DB commands |
| `cmd/bnpl/bootstrap/` | Application wiring and startup |
| `pkg/adapter/config/` | Config load and provider factories |
| `pkg/adapter/infrastructure/cache/` | Cache provider selection |
| `pkg/adapter/infrastructure/cache/noop/` | No-op cache |
| `pkg/adapter/infrastructure/cache/redis/` | Redis cache |
| `pkg/adapter/infrastructure/cache/memory/` | In-memory cache (tests only) |
| `pkg/adapter/infrastructure/clock/` | System and fixed clocks |
| `pkg/adapter/infrastructure/log/` | slog and noop loggers |
| `pkg/adapter/infrastructure/messaging/` | Kafka (Sarama) and noop messaging |
| `pkg/adapter/infrastructure/task/` | In-process scheduler and noop runtime |
| `pkg/adapter/infrastructure/task/jobs/` | Scheduled job registry (outbox, metrics) |
| `pkg/adapter/infrastructure/trace/` | ULID trace provider |
| `pkg/adapter/restful/gin/` | Gin adapter and middleware |
| `pkg/adapter/restful/gin/pagination/` | HTTP pagination query parsing |
| `pkg/adapter/restful/gin/serdser/` | Request/response envelopes |
| `pkg/adapter/db/postgres/` | pgx adapters |
| `pkg/adapter/db/postgres/outboxrp/` | Outbox repository |
| `pkg/adapter/db/postgres/migrationsrp/` | Bootstrap embed + migration repo |
| `pkg/adapter/infrastructure/health/` | Lifecycle state and dependency checkers |
| `pkg/adapter/infrastructure/circuitbreaker/` | noop and gobreaker circuit breakers |
| `pkg/adapter/restful/gin/healthrs/` | Health probe HTTP handlers |
| `pkg/adapter/db/postgres/healthrp/` | Health repository (database ping) |
| `pkg/core/health/` | Lifecycle and dependency-check ports |
| `pkg/core/cache/`, `clock/`, `log/`, `messaging/`, `task/`, `trace/` | Infrastructure ports |
| `pkg/core/circuitbreaker/` | Circuit breaker port |
| `pkg/core/pagination/` | Pagination models |
| `pkg/core/usecase/outboxpublisheruc/` | Outbox publish orchestration |
| `pkg/core/usecase/healthcheckuc/` | Health check use case |
| `pkg/core/mock/` | Mockery-generated test doubles |
