# API

## Status

The repository contains **HTTP server scaffolding only**. No BNPL business endpoints are implemented.

| Category | Status |
| --- | --- |
| Gin engine and middleware | Implemented |
| Trace / request / correlation headers | Implemented |
| `/api/v1` route group | Implemented (empty) |
| Response envelopes (`serdser`) | Implemented (unused by routes) |
| Business REST handlers | **Not implemented** |
| Authentication / authorization | **Not implemented** |
| OpenAPI / protobuf API definitions | **Not present** |

## Implemented

### Server bootstrap

1. Load config (`config.Load`)
2. Open normal-role PostgreSQL pool
3. Wire infrastructure, use cases, and scheduled tasks
4. Create Gin engine with trace-ID middleware (always), optional recovery and logger
5. Register routes via `routes.Register`
6. Listen on `{GIN_HOST}:{GIN_PORT}`

Entry: `cmd/bnpl/command/root.go` → `bootstrap.Boot`.

### Registered routes

| Path prefix | Methods | Handler | Notes |
| --- | --- | --- | --- |
| `/api/v1` | — | Route group only | No child routes registered |

Unmatched paths receive Gin's default 404 behavior.

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

### Shared response helpers (unused by routes today)

`pkg/adapter/restful/gin/serdser`:

- `Bind` — request binding; validation errors → standardized JSON error envelope
- `SerErr` — `cerr.Error` → mapped HTTP status and machine-readable code
- `Ser`, `SerOK`, `SerCreated`, … — success envelopes with trace metadata and optional pagination

Pagination query parsing: `pkg/adapter/restful/gin/pagination` (ready for list endpoints).

### Status codes today

| Code | When |
| --- | --- |
| 404 | Unknown paths (Gin default) |
| 500 | Panic in middleware chain if recovery enabled |

No application-defined 2xx, 4xx, or domain-specific error responses are emitted by registered routes.

## Planned

Future BNPL APIs (not implemented — do not treat as available):

### Platform / operations

- `GET /health` or `GET /ready` — liveness and dependency checks
- Version or build info endpoint for deployments

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

- [ ] Add health/readiness routes (optional pool ping)
- [ ] Define API versioning strategy under `/api/v1`
- [ ] Document request/response schemas when first endpoints land
- [ ] Add authentication middleware before any sensitive routes
- [ ] Wire `routes.Register` to use the database pool passed in today

## Related documentation

- [architecture.md](architecture.md) — HTTP request flow
- [security.md](security.md) — future AuthN/AuthZ requirements
- [operations.md](operations.md) — running the server locally
