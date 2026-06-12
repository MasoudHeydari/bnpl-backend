# API

## Status

The repository contains **HTTP server scaffolding only**. No BNPL business endpoints are implemented.

| Category | Status |
| --- | --- |
| Gin engine and middleware | Implemented |
| `/api/v1` route group | Implemented (empty) |
| Business REST handlers | **Not implemented** |
| Authentication / authorization | **Not implemented** |
| OpenAPI / protobuf API definitions | **Not present** |

## Implemented

### Server bootstrap

1. Load config (`config.Load`)
2. Open normal-role PostgreSQL pool
3. Create Gin engine with optional logger and recovery middleware
4. Register routes via `routes.Register`
5. Listen on `{GIN_HOST}:{GIN_PORT}`

Entry: `cmd/bnpl/command/root.go` → `startWebServer`.

### Registered routes

| Path prefix | Methods | Handler | Notes |
| --- | --- | --- | --- |
| `/api/v1` | — | Route group only | No child routes registered |

Unmatched paths receive Gin's default 404 behavior.

### Middleware

From `config.Gin`:

| Setting | Env var | Effect |
| --- | --- | --- |
| Logger | `GIN_LOGGER` | Request/response logging via slog |
| Recovery | `GIN_RECOVERY` | Panic → HTTP 500 |
| Mode | `GIN_MODE` | `debug`, `release`, or `test` |

Trusted proxies are hardcoded to `127.0.0.1` in `pkg/adapter/restful/gin/gin.go`. The `GIN_TRUSTED_PROXY` YAML value is **not applied**.

### Shared response helpers (unused by routes today)

`pkg/adapter/restful/gin/serdser`:

- `Bind` — request binding; validation errors → HTTP 400 JSON
- `SerErr` — `cerr.Error` → mapped status; other errors → HTTP 500

These are ready for future handlers but have no callers in `routes` yet.

### Status codes today

| Code | When |
| --- | --- |
| 404 | Unknown paths (Gin default) |
| 500 | Panic in middleware chain if recovery enabled |

No application-defined 2xx, 4xx, or domain-specific error responses exist.

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
  → Gin middleware (logger, recovery)
  → route handler
  → serdser.Bind (validation)
  → use case method
  → repository port
  → PostgreSQL adapter
  → serdser.SerErr or JSON success response
```

Handlers should depend on use cases, not `pgx` or raw SQL.

## TODO

- [ ] Add health/readiness routes (no DB dependency or optional pool ping)
- [ ] Define API versioning strategy under `/api/v1`
- [ ] Document request/response schemas when first endpoints land
- [ ] Add authentication middleware before any sensitive routes
- [ ] Wire `routes.Register` to use the database pool passed in today
- [ ] Apply `GIN_TRUSTED_PROXY` from configuration

## Related documentation

- [architecture.md](architecture.md) — HTTP request flow
- [security.md](security.md) — future AuthN/AuthZ requirements
- [operations.md](operations.md) — running the server locally
