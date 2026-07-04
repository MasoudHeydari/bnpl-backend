# Architecture

## Purpose

This document describes the **current** Go backend structure and how it supports the Buy Now Pay Later (BNPL) platform. The repository provides platform infrastructure, database bootstrap, HTTP APIs, and core BNPL domain use cases.

## Current State vs Intended Direction

| Area | Current state | Intended direction |
| --- | --- | --- |
| HTTP API | Gin server, trace middleware, `/healthz/live` and `/healthz/ready`, `/api/v1` business handlers (credit limit, checkout decision, contract, installment payment) | REST APIs for checkout, contracts, repayments, admin |
| Database | `migrations` + BNPL schema (`contracts`, `installments`, `payment_attempts`, `integration_inbox`, `integration_outbox`, etc.) | Full BNPL schema with forward migrations |
| Use cases | `migrationsuc`, `outboxpublisheruc`, `healthcheckuc`, `customercreditlimituc`, `getcustomercreditsummaryuc`, `decisionuc`, `contractuc`, `contractinstallmentsuc`, `listactivecontractsuc`, `installmentlifecycleuc`, `installmentrepaymentuc`, `listinstallmentpaymentsuc`, `financialcorepaymentresultuc`, `financialcorepaymentsessionuc`, `financialcorepaymentreconciliationuc`, `paymentattemptreconciliationuc` | Collections use cases |
| Events | Kafka adapter + noop; outbox publisher job; Financial Core payment-result consumer; payment-application outbox producer | Domain events for audit and integrations |
| Cache | Redis / noop adapters wired | Idempotency, rate limits, session state |
| Auth | None | Role-based access for admin and merchant APIs |
| Observability | slog/noop logger, trace headers, health probes, goroutine metrics task | Metrics, distributed tracing, audit persistence |
| Shutdown | SIGINT/SIGTERM graceful shutdown with lifecycle probes | Production orchestration with wired drain delay |

**BNPL aggregates (Contract, Installment, PaymentAttempt, etc.) exist in code** and are managed by dedicated use cases under `pkg/core/usecase/`.

## BNPL Platform Vision

The long-term system will handle:

- Customer credit evaluation and credit limit management
- Installment plan generation and contract lifecycle
- Checkout financing decisions
- Payment tracking and delinquency handling
- Collections and legal case escalation
- Merchant and branch management
- Auditability and regulatory compliance

Planned domain names (not yet implemented):

Contract · Decision · RepaymentPlan · Installment · PaymentAttempt · CreditLimit · CollectionsCase · LegalCase · MerchantProfile · MerchantBranchProfile · Restriction · Block · Audit

**Implemented payment ownership (ADR [0016](adr/0016-bnpl-payment-attempt-ownership.md)):**

- Financial Core owns payment intents, provider attempts, and monetary truth
- BNPL `PaymentAttempt` is local orchestration/idempotency for installments 2+ (not provider auth/capture/settlement)
- `Inbox` provides durable at-least-once event idempotency
- `integration_outbox` publishes `bnpl.payment_application_result.v1` business acknowledgements (`applied`, `already_applied`, `reconciliation_required`) — only for inbound **succeeded** Financial Core payments
- Pure business-event contracts live in `pkg/core/event/paymentapplication` (envelope + `EventData`; event type/version/producer constants)
- Fingerprint hashing is a Core port (`pkg/core/fingerprint`) with minimal `Generate` interface; SHA-256 implementation lives in `pkg/adapter/infrastructure/fingerprint/sha256`. Use cases validate `Value` before persistence — no panic-based fingerprint helpers in Core.
- Known payment statuses are canonicalized to typed values (`succeeded`/`failed`/`canceled`) before fingerprinting; unknown statuses remain distinct after trim/lowercase normalization
- Succeeded payments without `financial_payment_intent_id` are permanently rejected after inbox acquisition (`missing_payment_intent`); no payment-application outbox is created
- Installment `already_applied` requires proof from paid installment and compatible contract state, not only succeeded `PaymentAttempt`
- First-installment checkout activation marks only snapshotted customer debit charges paid (not discounts, waivers, or merchant/platform-paid charges)
- Outbox Kafka partition key = `financial_payment_intent_id` (`message_key` column); delivery is at-least-once — Financial Core deduplicates by outbound `event_id`
- Inbox/outbox orchestration lives in `financialcorepaymentresultuc` — not in generic core packages
- Kafka adapter validates minimum trusted envelope (`source_service`, `event_id`, `event_type`, `event_version`); temporary compatibility defaults live in one adapter mapper; payment business validation occurs **after** inbox acquisition
- Integration identity fields are normalized once (trim whitespace) before inbox persistence, fingerprinting, and outbound acknowledgements
- Raw Financial Core payment status is preserved for unknown values and included in canonical fingerprints (distinct unknown statuses cannot collapse to the same hash)
- Checkout and installment unsupported statuses durably fail inbox after acquisition — no payment-application acknowledgement
- Successful payments referencing missing BNPL contract/installment become `reconciliation_required` (not infinite retry)
- `contractuc.ReconciliationError` carries typed reconciliation reasons — no error-message substring classification
- Exact first-installment amount is enforced in `contractuc` (`paid == payable`, not `>=`)
- Fingerprint values are validated before persistence; development databases with legacy raw digests must be reset or rewritten before replay
- Installment concerns split: `decisionuc` (checkout installment offer creation), `installmentlifecycleuc`, `installmentrepaymentuc`
- `PaymentAttemptItem` is a child of `PaymentAttempt` (aggregate repository only — no separate port)
- Installment Financial Core payment client remains **noop** until official gRPC contract exists

Product context (non-English): [pintoday.md](pintoday.md), [base_doc.md](base_doc.md).

## Runtime Entrypoints

Single binary `bnpl` (Cobra):

| Command | Behavior |
| --- | --- |
| `bnpl` (default) | HTTP server + infrastructure bootstrap + scheduled tasks |
| `bnpl db init-dev` | Destructive recreate + dev seed SQL (`APP_ENVIRONMENT=development`) |
| `bnpl db init-prod` | Create-if-missing + prod seed SQL (`APP_ENVIRONMENT=production`) |
| `bnpl db migrate` | Filesystem SQL migrations |

See ADR [0005](adr/0005-single-binary-cobra-commands.md).

## Layers

### Core (`pkg/core`)

| Package | Role |
| --- | --- |
| `model` | `Secret`, `Migration`, `OutboxMessage` — no BNPL business models yet |
| `repo` | Port interfaces: Pool, Conn, Tx, Schema, Migrations, Outbox, Health |
| `health` | `LifecycleState`, `DependencyChecker` contracts for probes |
| `cache`, `clock`, `log`, `messaging`, `task`, `trace`, `circuitbreaker`, `httpclient`, `webhook`, `secretbox`, `fingerprint` | Infrastructure port interfaces / contracts (no concrete transport, SSRF, HMAC, or router implementations) |
| `pagination` | Cursor/page pagination models and helpers |
| `cerr` | Application error codes and HTTP status mapping |
| `event/paymentapplication` | Pure payment-application business-event contract (no repo/outbox orchestration) |
| `fingerprint` | Core fingerprint port and self-describing `algorithm:digest` value |
| `usecase/migrationsuc` | Schema bootstrap and migration orchestration |
| `usecase/contractuc` | Checkout contract creation, activation helpers, payment-pending expiry |
| `usecase/financialcorepaymentresultuc` | Financial Core payment-result inbox/outbox orchestration and domain application |
| `usecase/financialcorepaymentsessionuc` | Financial Core installment payment-session inbox orchestration |
| `usecase/financialcorepaymentreconciliationuc` | Financial Core inbound reconciliation-result processing |
| `usecase/decisionuc` | Checkout decision evaluation; approved/partial outcomes atomically persist the installment offer |
| `usecase/installmentlifecycleuc` | Offered-installment expiry and scheduled/due/overdue installment lifecycle batch jobs |
| `usecase/installmentrepaymentuc` | Installment repayment initiation (`PaymentAttempt` aggregate with item snapshots via `CreateWithItems`) |
| `usecase/paymentattemptreconciliationuc` | Outbound payment-attempt reconciliation request batch job |
| `usecase/outboxpublisheruc` | Claim and publish integration outbox rows |
| `usecase/healthcheckuc` | Liveness and readiness evaluation |
| `usecase/customercreditlimituc` | Customer initial/updated credit limit decisioning |
| `usecase/getcustomercreditsummaryuc` | Read-only customer credit summary projection |
| `usecase/contractinstallmentsuc` | Read-only contract installment listing |
| `usecase/listactivecontractsuc` | Read-only active contract listing |
| `usecase/listinstallmentpaymentsuc` | Read-only installment payment listing |

**Boundary:** No imports of `pgx`, `gin`, `cobra`, `redis`, `sarama`, or YAML.

### Application bootstrap (`cmd/bnpl/bootstrap`)

| Package | Role |
| --- | --- |
| `bootstrap` | Loads config, wires repos, infrastructure, use cases, tasks, routes; graceful shutdown |
| `bootstrap/infrastructure.go` | Trace, clock, logger, cache, messaging, task, circuit breaker, lifecycle, dependency checker |
| `bootstrap/shutdown.go` | Ordered shutdown on SIGINT/SIGTERM |
| `bootstrap/usecase.go` | Builds all application use cases (checkout, installment, financial-core integration, credit limit, migration, outbox publisher, and health check) |
| `bootstrap/repo.go` | PostgreSQL repository adapters (migrations, outbox, health) |

### Adapter (`pkg/adapter`, `cmd/bnpl/command`)

| Package | Role |
| --- | --- |
| `config` | YAML load, env expansion, provider factories |
| `infrastructure/httpclient` | `noop` / `standard` implementations of the Core outbound HTTP client port; bounded concurrency, queueing, transport reuse, logging, metrics |
| `restful/gin` | Gin wrapper, trace middleware, pagination helpers |
| `restful/gin/routes` | Route registration (`/healthz/*`, `/api/v1` group) |
| `restful/gin/healthrs` | Kubernetes-style liveness and readiness HTTP handlers |
| `restful/gin/serdser` | Binding, success/error envelopes with trace metadata |
| `db/postgres/healthrp` | Database ping repository for readiness |
| `db/postgres` | `pgx` pool, connection, transaction, pagination adapters |
| `db/postgres/outboxrp` | Integration outbox repository |
| `infrastructure/health` | Lifecycle state and composite dependency checkers |
| `infrastructure/*` | Cache, clock, log, messaging (incl. destination router), task, trace, fingerprint, circuit breaker, secretbox adapters |
| `external/merchantwebhook` | Provider root (`http` / `noop` identifiers). Concrete HTTP delivery lives in `merchantwebhook/http`; noop in `merchantwebhook/noop`. Pure destination rules live in `pkg/core/policy/webhook`. |
| `cmd/bnpl/command` | CLI wiring |

## Merchant checkout context

Merchant identity is independent of sales channel. `MerchantProfile` represents the business, while `MerchantBranchProfile` is a policy-bearing selling point that may be `physical` or `online`. This supports physical-only, online-only, and omnichannel merchants without introducing separate checkout APIs.

Checkout accepts one explicit purchase-context discriminator:

```text
purchase_context.type=pos
    pos_gps_id -> merchant_pos_terminals -> physical MerchantBranchProfile -> MerchantProfile

purchase_context.type=online
    online_store_id -> merchant_online_stores -> online MerchantBranchProfile -> MerchantProfile
```

The resolved merchant and branch are authoritative BNPL context; clients do not submit them directly. Purchase channel is orthogonal to checkout policy, so POS/online purchases may independently resolve to checkout policy v1, v2, or future versions. The Decision allocation snapshots both the resolved merchant context and original purchase-source identity for audit/idempotency.

For approved offers, `decisionuc` also derives a semantic `checkout_action` from the selected policy's contract plan. Clients render confirmation/payment UX from that action instead of knowing policy versions.

## Policy Conventions

Policies under `pkg/core/policy` fall into two families. Pick the family by **semantics**, not by folder shape.

### Versioned Business Policies

Examples: `checkout`, `initialcreditlimit`.

Characteristics:

- Encode business decisions (eligibility, financing, credit terms)
- Versioned (`v1` / `v2` / …) with registry + catalog
- May expose a `PolicyRef` that is snapshotted / persisted for historical replay
- Selected per customer / decision / assignment — not only at process startup

Keep the existing `contract.go` / `registry.go` / `catalog/` / `vN/` pattern. Do not flatten these into a Strict/Development factory.

### Runtime / Boundary Policies

Examples: `checkoutorigin`, `returndestination`, `webhook`.

Characteristics:

- Pure deterministic validation / security boundary rules
- Selected once at startup by Config/Bootstrap from the runtime environment
- Not persisted on business entities; no historical policy replay
- No registry, catalog, or `v1`/`v2` folders

Preferred public shape:

```go
policy := checkoutorigin.Strict(allowedHosts) // or Development(...)
origin, err := policy.Parse(raw)

policy := returndestination.Strict() // or Development()
err := policy.ValidateName(name)
err := policy.ValidateURL(raw)

policy := webhook.Strict() // or Development()
parsed, err := policy.ValidateURL(raw)
```

Config/Bootstrap maps `production`/`staging` → `Strict`, and `development`/`test` → `Development`. Core packages must not call `os.Getenv` or branch on environment names.

Related URL construction (`pkg/core/checkouturl`) is **not** a policy: it builds hosted checkout URLs from an already-validated `checkoutorigin.PublicOrigin`.

## Infrastructure Layer

Provider-based infrastructure is configured under `infrastructure:` in YAML. Only the selected provider is constructed at startup.

| Concern | Core port | Providers | Default (.env.example) |
| --- | --- | --- | --- |
| Trace IDs | `trace.Provider` | ULID (`trace` package) | Always ULID |
| Event IDs (payment application outbox) | `identifier.Generator` | ULID adapter (`trace.Provider` also implements this) | Same ULID provider |
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

Current flow:

1. `bootstrap.Boot` loads config (`config.Load`) and validates `versions`
2. Opens normal-role pool
3. Wires repositories, infrastructure (including dependency checker and lifecycle state), use cases
4. Registers configured scheduled tasks and starts task runtime and message subscriber in background
5. Builds Gin engine with trace-ID middleware (first), optional recovery and logger
6. `routes.Register` attaches `/healthz/live` and `/healthz/ready` when enabled, plus `/api/v1` business handlers (credit limit, checkout decision, contract, installment payment)
7. Gin listens on `gin.host:gin.port`
8. On SIGINT/SIGTERM: readiness fails (`StartDraining`), bounded graceful shutdown, optional liveness failure on timeout

The pool is used by readiness dependency checks, HTTP handlers, and background jobs.

## Database Flow

### Runtime (intended for future handlers)

1. Normal-role pool from config
2. Acquire connection via `Pool.Conn`
3. Execute SQL through `Conn` or `Tx`

### Bootstrap (`init-dev` / `init-prod`)

1. Validate `runtime.environment` (`APP_ENVIRONMENT`)
2. Admin pool → transaction:
   - `init-dev`: drop/create schema, role, privileges
   - `init-prod`: refuse if schema exists; otherwise create schema, role, privileges
3. Normal pool → transaction → embedded DDL + seed SQL

See [database.md](database.md) and ADR [0008](adr/0008-destructive-database-initialization-strategy.md).

### Migrations (`db migrate`)

1. Read files from `usecases.migration.files-dir` (wired from YAML)
2. Per file: transaction → execute SQL → insert into `migrations` table

Bootstrap and migrations are separate mechanisms.

### Outbox publishing

1. `outboxpublisheruc.PublishDue` claims pending rows (`SKIP LOCKED`)
2. Publishes to configured messaging publisher (topic = `destination`; Kafka key = `message_key`)
3. Marks rows published or schedules retry with exponential backoff

Payment-application outbox rows are inserted by `financialcorepaymentresultuc` in the same transaction as inbox and domain mutations. The publisher never regenerates event IDs or payloads on retry.

Triggered by the `outbox_publisher` scheduled task when `TASK_OUTBOX_PUBLISHER_ENABLED=true`.

## Configuration Flow

1. Resolve path: `--config` → `CONFIG_FILE` → `configs/config.yaml`
2. Read YAML, `os.ExpandEnv`, decode to `config.Config`
3. Validate `versions.config` and `versions.database` semver compatibility
4. Factories build pools, infrastructure providers, use cases, Gin engine

Known wiring gaps:

- No post-load validation for empty required env-expanded fields
- `bootstrap.InitDev` / `InitProd` still print loaded config via `fmt.Printf`
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
- [ ] Use circuit breaker at external dependency call sites

### Integrations

- [x] Financial Core payment-result Kafka consumer (`financialcorepaymentresultuc`)
- [x] Payment-application outbox producer (`bnpl.payment_application_result.v1`)
- [ ] Real Financial Core installment-payment gRPC adapter (not implemented)
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
| `pkg/adapter/infrastructure/fingerprint/sha256/` | SHA-256 fingerprint generator |
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
| `pkg/core/cache/`, `clock/`, `log/`, `messaging/`, `task/`, `trace/`, `fingerprint/` | Infrastructure ports |
| `pkg/core/circuitbreaker/` | Circuit breaker port |
| `pkg/core/pagination/` | Pagination models |
| `pkg/core/usecase/outboxpublisheruc/` | Outbox publish orchestration |
| `pkg/core/usecase/healthcheckuc/` | Health check use case |
| `pkg/core/mock/` | Mockery-generated test doubles |
