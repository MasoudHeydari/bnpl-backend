# API

## Status

The repository contains **HTTP server scaffolding and operational health probes**. No BNPL business endpoints are implemented.

| Category | Status |
| --- | --- |
| Gin engine and middleware | Implemented |
| Trace / request / correlation headers | Implemented |
| `/healthz/live` and `/healthz/ready` | Implemented (when `HEALTH_ENABLED=true`) |
| `/api/v1` route group | Implemented (empty) |
| Response envelopes (`serdser`) | Implemented (health routes use success + `SerErr`) |
| Business REST handlers | **Not implemented** |
| Authentication / authorization | **Not implemented** |
| OpenAPI / protobuf API definitions | **Not present** |

## Implemented

### Server bootstrap

1. Load config (`config.Load`) and validate versions
2. Open normal-role PostgreSQL pool
3. Wire infrastructure, use cases, and scheduled tasks
4. Create Gin engine with trace-ID middleware (always), optional recovery and logger
5. Register routes via `routes.Register`
6. Listen on `{GIN_HOST}:{GIN_PORT}`
7. Graceful shutdown on `SIGINT` / `SIGTERM`

Entry: `cmd/bnpl/command/root.go` → `bootstrap.Boot`.

### Registered routes

| Path | Methods | Handler | Notes |
| --- | --- | --- | --- |
| `/healthz/live` | `GET` | `healthcheckuc.Liveness` | Process alive; no dependency checks |
| `/healthz/ready` | `GET` | `healthcheckuc.Readiness` | Draining state + configured dependency checks |
| `/api/v1` | — | Route group only | No child routes registered |

Health routes are registered only when `usecases.health.enabled` is true (`HEALTH_ENABLED`).

Unmatched paths receive Gin's default 404 behavior.

### Health probe semantics

| Probe | Fails when | Does not check |
| --- | --- | --- |
| Liveness (`/healthz/live`) | Shutdown exceeded deadline (`cerr.ServiceUnavailable`) | Database, Redis, Kafka |
| Readiness (`/healthz/ready`) | Application is draining; any configured dependency check fails | — |

Dependency checks are configured in `infrastructure.health.checks` (default: `database`, `cache`, `messaging`). See [operations.md](operations.md).

**Success response** (both probes):

```json
{"status":"UP"}
```

**Failure response** uses the standard `serdser` error envelope (`SerErr`). `cerr.ServiceUnavailable` maps to HTTP 503.

### Middleware

From `config.Gin` (order matters):

| Order | Middleware | Env / config | Effect |
| --- | --- | --- | --- |
| 1 | Trace ID | Always on | Propagates `X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id` |
| 2 | Recovery | `GIN_RECOVERY` | Panic → HTTP 500 |
| 3 | Logger | `GIN_LOGGER` | Request/response logging via slog |

`GIN_TRUSTED_PROXY` is decoded from `gin.trusted_proxies` in YAML and passed to Gin.

### Trace and correlation headers

Middleware: `pkg/adapter/restful/gin/middleware/traceid.go`

| Header | Constant | Behavior |
| --- | --- | --- |
| `X-Trace-Id` | `header.TraceID` | Reused from request or generated (ULID); echoed in response |
| `X-Request-Id` | `header.RequestID` | Reused from request or generated; echoed in response |
| `X-Correlation-Id` | `header.CorrelationID` | Reused from request or generated; echoed in response |

Values are stored in `context.Context` via `pkg/core/trace` and included in `serdser` error/success envelopes when handlers use those helpers.

### Shared response helpers

`pkg/adapter/restful/gin/serdser`:

- `Bind` — request binding; validation errors → standardized JSON error envelope
- `SerErr` — `cerr.Error` → mapped HTTP status and machine-readable code
- `Ser`, `SerOK`, `SerCreated`, … — success envelopes with trace metadata and optional pagination

Pagination query parsing: `pkg/adapter/restful/gin/pagination` (ready for list endpoints).

### Status codes today

| Code | When |
| --- | --- |
| 200 | Health probes succeed |
| 404 | Unknown paths (Gin default) |
| 500 | Panic in middleware chain if recovery enabled; unclassified errors from `SerErr` |
| 503 | Health probe failure wrapping `cerr.ServiceUnavailable` |

No application-defined business 2xx/4xx responses are emitted by `/api/v1` routes.

## Planned

Future BNPL APIs (not implemented — do not treat as available):

### Credit and checkout (Decision, CreditLimit)

- Pre-qualification / credit check at checkout
- Credit limit inquiry and reservation

### Contracts and installments (Contract, RepaymentPlan, Installment)

- Create and activate BNPL contract from order
- Query installment schedule and status

### Payments (PaymentAttempt)

- Initiate installment payment
- Payment gateway webhook/callback

### Merchant (MerchantProfile, MerchantBranchProfile)

- Merchant onboarding and configuration
- Branch management

### Admin and audit (Audit, CollectionsCase, LegalCase)

- Search customers, contracts, installments
- Manual operational actions with audit trail
- Collections case management

Exact paths, payloads, and versioning are **not defined** in this repository yet.

## Request / Response Flow (future handlers)

When handlers are added, the intended pattern is:

```
HTTP request
  → TraceID middleware (context + response headers)
  → Gin middleware (logger, recovery)
  → route handler
  → serdser.Bind (validation)
  → use case method
  → repository port
  → PostgreSQL adapter
  → serdser.Ser* or serdser.SerErr
```

Handlers should depend on use cases, not `pgx` or raw SQL.

## TODO

- [ ] Define API versioning strategy under `/api/v1`
- [ ] Document request/response schemas when first business endpoints land
- [ ] Add authentication middleware before any sensitive routes
- [ ] Wire business handlers to use the database pool

## Related documentation

- [architecture.md](architecture.md) — HTTP request flow
- [operations.md](operations.md) — health probes and graceful shutdown
- [security.md](security.md) — future AuthN/AuthZ requirements
