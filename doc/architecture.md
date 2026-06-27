# Architecture

## Purpose

This document describes the **current** Go backend structure and how it is intended to support a future Buy Now Pay Later (BNPL) platform. The repository today provides platform infrastructure, database bootstrap, and HTTP scaffolding — not a complete BNPL engine.

## Current State vs Intended Direction

| Area | Current state | Intended direction |
| --- | --- | --- |
| HTTP API | Gin server, trace middleware, empty `/api/v1` group | REST APIs for checkout, contracts, repayments, admin |
| Database | `migrations` + `integration_outbox` tables; destructive init | Full BNPL schema with forward migrations |
| Use cases | `migrationsuc`, `outboxpublisheruc` | Credit, installment, payment, collections use cases |
| Events | Kafka adapter + noop; outbox publisher job | Domain events for audit and integrations |
| Cache | Redis / noop adapters wired | Idempotency, rate limits, session state |
| Auth | None | Role-based access for admin and merchant APIs |
| Observability | slog/noop logger, trace headers, goroutine metrics task | Metrics, distributed tracing, audit persistence |

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
| `repo` | Port interfaces: Pool, Conn, Tx, Schema, Migrations, Outbox |
| `cache`, `clock`, `log`, `messaging`, `task`, `trace` | Infrastructure port interfaces |
| `pagination` | Cursor/page pagination models and helpers |
| `cerr` | HTTP-oriented error codes and wrappers |
| `usecase/migrationsuc` | Schema bootstrap and migration orchestration |
| `usecase/outboxpublisheruc` | Claim and publish integration outbox rows |

**Boundary:** No imports of `pgx`, `gin`, `cobra`, `redis`, `sarama`, or YAML.

### Application bootstrap (`cmd/bnpl/bootstrap`)

| Package | Role |
| --- | --- |
| `bootstrap` | Loads config, wires repos, infrastructure, use cases, tasks, routes |
| `bootstrap/infrastructure.go` | Constructs trace, clock, logger, cache, messaging, task runtime |
| `bootstrap/usecase.go` | Builds migration and outbox publisher use cases |
| `bootstrap/repo.go` | PostgreSQL repository adapters |

### Adapter (`pkg/adapter`, `cmd/bnpl/command`)

| Package | Role |
| --- | --- |
| `config` | YAML load, env expansion, provider factories |
| `restful/gin` | Gin wrapper, trace middleware, pagination helpers |
| `restful/gin/routes` | Route registration (placeholder) |
| `restful/gin/serdser` | Binding, success/error envelopes with trace metadata |
| `db/postgres` | `pgx` pool, connection, transaction, pagination adapters |
| `db/postgres/outboxrp` | Integration outbox repository |
| `infrastructure/*` | Cache, clock, log, messaging, task, trace adapters |
| `cmd/bnpl/command` | CLI wiring |

## Infrastructure Layer

Provider-based infrastructure is configured under `infrastructure:` in YAML. Only the selected provider is constructed at startup.

| Concern | Core port | Providers | Default (.env.example) |
| --- | --- | --- | --- |
| Trace IDs | `trace.Provider` | ULID (`trace` package) | Always ULID |
| Clock | `clock.Clock` | System (dual timezone) | UTC + Asia/Tehran |
| Logger | `log.Logger` | `noop`, `slog` | `slog` / `info` / `json` |
| Cache | `cache.Cache` | `noop`, `redis` | `noop` |
| Messaging | `messaging.Publisher`, `Subscriber` | `noop`, `kafka` (Sarama) | `noop` |
| Task runtime | `task.Runtime` | `noop`, `inprocess` | `inprocess` |

Additional adapter code exists but is **not** selectable via configuration:

| Package | Role |
| --- | --- |
| `infrastructure/cache/memory` | In-process cache for tests; not a config provider |

**Not implemented:** circuit breaker abstraction or adapter (no code or config yet).

See ADRs [0009](adr/0009-provider-based-infrastructure-configuration.md) through [0013](adr/0013-messaging-abstraction-kafka-noop.md).

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

1. `bootstrap.Boot` loads config (`config.Load`)
2. Opens normal-role pool
3. Wires repositories, infrastructure, use cases
4. Registers configured scheduled tasks and starts task runtime in background
5. Builds Gin engine with trace-ID middleware (first), optional recovery and logger
6. `routes.Register` creates `/api/v1` group — **no handlers attached**
7. Gin listens on `gin.host:gin.port`

The pool is opened and passed to bootstrap but unused by routes today. Outbox publisher and goroutine metrics tasks run in the background when enabled.

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
3. Factories build pools, infrastructure providers, use cases, Gin engine

Known wiring gaps:

- `versions` in YAML — ignored
- No post-load validation for empty required fields
- `bootstrap.InitDev` / `InitProd` still print loaded config via `fmt.Printf`
- Circuit breaker — not present

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

- [ ] Register health/readiness endpoints
- [ ] Implement `/api/v1` handlers
- [ ] Wire authentication middleware
- [ ] Use database pool in route handlers

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
- [ ] Circuit breaker provider and config section

### Integrations

- [ ] Kafka consumer subscription wiring in bootstrap (subscriber constructed but not started)
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
| `pkg/core/cache/`, `clock/`, `log/`, `messaging/`, `task/`, `trace/` | Infrastructure ports |
| `pkg/core/pagination/` | Pagination models |
| `pkg/core/usecase/outboxpublisheruc/` | Outbox publish orchestration |
