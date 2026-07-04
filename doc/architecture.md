# Architecture

## Purpose

This document describes the **current** Go backend structure and how it supports the Buy Now Pay Later (BNPL) platform. The repository provides platform infrastructure, database bootstrap, HTTP APIs, and core BNPL domain use cases.

## Current Implementation

The code contains executable BNPL business flows, not merely platform
scaffolding. Domain models and use cases cover decisions and offers, checkout
session/OTP confirmation, contracts, installment repayment, customer credit and
exposure, collections, merchant terminal operations, and policy assignment.

- **Runtime:** `cmd/bnpl` serves Gin APIs under `/api/v1`, consumes Financial
  Core payment-result and payment-reconciliation-result events, runs scheduled
  tasks, and publishes transactional outbox events.
- **Database administration:** `cmd/bnpl-dbadmin` is separate from Runtime,
  owns bootstrap/migrations, and uses elevated DB credentials only in admin jobs.
- **Auth surfaces:** hosted checkout uses a customer token and OTP; partner
  routes authenticate a terminal ID/API key; other business routes consume
  Gateway-authenticated `X-Auth-*` context with permissions.
- **Payment ownership:** Financial Core is authoritative for monetary payment
  status. BNPL keeps local PaymentAttempt orchestration and only applies
  authoritative results.
- **First release:** the initial PostgreSQL schema may be rebuilt before
  production, but runtime concurrency, crash consistency, and financial
  integrity still require verification.

See [API](api.md), [security](security.md), and
[database deployment](architecture/database-deployment.md).

## Domains and Runtime Entrypoints

Implemented business areas include Decision, installments and offers,
CheckoutSession, Contract, PaymentAttempt, credit accounts and exposure,
merchant and partner terminal context, reconciliation, collections and
repayment reminders. Future legal-case processing and additional financial
integrations are not implied by those implementations.

The Runtime binary `bnpl` serves APIs and scheduled work. The separate DB
Admin binary `bnpl-dbadmin` owns `db init-dev`, `db init-prod`, and `db
migrate`. `init-dev` is destructive and development-only. Production schema
initialization refuses an existing managed schema. See
[database deployment](architecture/database-deployment.md).

## Layers

### Core (`pkg/core`)

The Core contains domain models, repository/external-service ports,
versioned business policies, business events, semantic errors, and use cases.
Key use cases include `decisionuc`, `checkoutsessionuc`, `contractuc`,
`customercredituc`, `installmentrepaymentuc`,
`financialcorepaymentresultuc`, `financialcorepaymentreconciliationuc`,
`paymentattemptreconciliationuc`, `collectionsuc`,
`sagaexecutoruc`, and `outboxpublisheruc`.

Core must not import Gin, pgx, Sarama, Redis, or transport implementations.
The `cerr` errors are transport-independent; REST maps and localizes them.

### Application bootstrap (`cmd/bnpl/bootstrap`)

Bootstrap wires ports, PostgreSQL repositories, external gRPC clients, Kafka
consumers, scheduled workers, HTTP routes, health checks and graceful
shutdown. Database management is not part of the Runtime binary.

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

## Request and Database Flows

1. Startup validates runtime configuration, opens restricted PostgreSQL
   connections, wires providers, registers task handlers and Kafka consumers,
   then serves the API and health endpoints.
2. The `/api/v1` HTTP surface separates partner terminal authentication,
   customer token checkout, and trusted-Gateway-authenticated requests.
3. Business use cases enter transactions through repository ports. Financial
   Core network requests run outside database transactions.
4. Authoritative Financial Core payment-result events are processed through
   `financialcorepaymentresultuc` with durable inbox acquisition and
   transactional domain/outbox updates. Submitted-payment reconciliation
   remains active even though automatic installment session creation
   rediscovery has been removed.
5. The outbox publisher claims pending rows and publishes at least once.
   Downstream consumers must deduplicate by stable event identities.

The initial schema is embedded in the separate DB Admin binary. First release
uses non-destructive `init-prod` against an empty managed schema; later schema
evolution uses `db migrate`. Runtime never installs DDL.

See [database invariants](architecture/database-invariants.md) and
[database deployment](architecture/database-deployment.md).

## Configuration Flow

Runtime loads `configs/runtime.yaml`, DB Admin loads `configs/dbadmin.yaml`.
Both use environment expansion and configuration validation. Financial Core
payment sessions are created synchronously via gRPC with stable idempotency
identities; Kafka consumer bindings cover payment results and payment
reconciliation results, not session-created events.

See [configuration](configuration.md) for provider and security constraints.

## Release and Operational Boundaries

- Verify NetworkPolicy/ingress/firewall rules prevent direct untrusted access
  to the BNPL business port. Envoy must validate authentication, strip
  client-controlled `X-Auth-*` headers and inject trusted values. BNPL does not
  independently validate bearer JWTs.
- Preserve PostgreSQL financial triggers, partial unique indexes, and limited
  Runtime database privileges; verify actual deployed schema after init.
- Enable and verify Kafka payment-result and reconciliation-result consumers,
  the outbox publisher and business lifecycle workers.
- Keep `make test-all` and the separate `make test-e2e` release checks.
  Safe FC integration-contract and disaster/failure tests also need to run.
- Any environment-specific controls not in this repository (e.g. Kubernetes
  NetworkPolicy or externally managed Envoy) must be verified as deployment
  evidence; code configuration alone does not establish them.

Remaining enhancements such as distributed tracing export, security response
playbooks and additional operational instrumentation are not equivalent to
missing business implementations.

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
| [0016](adr/0016-bnpl-payment-attempt-ownership.md) | PaymentAttempt and Financial Core ownership |

## Package Map

| Path | Responsibility |
| --- | --- |
| `cmd/bnpl/main.go` | Delegates to `command.Execute()` |
| `cmd/bnpl/command/` | Runtime server and operational commands |
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
| `pkg/adapter/infrastructure/task/jobs/` | Scheduled job registry (outbox, lifecycle, reconciliation, metrics) |
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
