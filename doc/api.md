# API

## Status

The repository contains **HTTP server scaffolding, operational health probes, and checkout APIs**.

Customer identity verification is owned by the upstream Identity Service. BNPL
receives an already authenticated customer identity and does not persist or
perform identity verification.

| Category | Status |
| --- | --- |
| Gin engine and middleware | Implemented |
| Trace / request / correlation headers | Implemented |
| `/healthz/live` and `/healthz/ready` | Implemented (when `HEALTH_ENABLED=true`) |
| `/api/v1` checkout decision (with installment offer) | Implemented |
| Response envelopes (`serdser`) | Implemented |
| Initial credit limit | Implemented |
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
| `/api/v1/partner/checkout-sessions` | `POST` | `checkoutsessionuc.CreatePartner` | Partner checkout create (all merchants); `customer_phone` + terminal auth; merchant-safe response (no offer/credit) |
| `/api/v1/partner/checkout-sessions/:checkout_session_id` | `GET` | `checkoutsessionuc.GetPartnerStatus` | Merchant-safe checkout status for the owning merchant |
| `/api/v1/partner/checkout-sessions/:checkout_session_id/cancel` | `POST` | `checkoutsessionuc.CancelByMerchant` | Idempotent partner cancellation while no submitted/ambiguous payment exists |
| `/api/v1/partner/return-destinations/:destination_name` | `PUT` | `checkoutsessionuc.UpsertReturnDestination` | Partner-managed return destination; requires `bnpl:checkout-return-destinations:manage:merchant` |
| `/api/v1/partner/webhook-destination` | `PUT` | `checkoutsessionuc.UpsertWebhookDestination` | Partner-managed lifecycle webhook; requires `bnpl:checkout-webhooks:manage:merchant` |
| `/api/v1/checkout-sessions/:token` | `GET` | `checkoutsessionuc.GetExternal` | Unauthenticated customer read-only projection; `:token` is the public `cs_…` token |
| `/api/v1/checkout-sessions/:token/start` | `POST` | `checkoutsessionuc.StartExternal` | Deferred offer preparation (when configured) + OTP for the displayed `evaluation_generation` |
| `/api/v1/checkout-sessions/:token/confirm` | `POST` | `checkoutsessionuc.ConfirmExternal` | OTP confirm without JWT tenant/context headers |
| `/api/v1/checkout-sessions/:token/cancel` | `POST` | `checkoutsessionuc.CancelByToken` | Customer cancellation via public token |
| `/api/v1/checkout-sessions/:token/return` | `GET` | `checkoutsessionuc.ReturnExternal` | Browser redirect to the pre-registered merchant return URL |
| `/api/v1/merchant/terminal` | `GET` | `merchantterminaluc.Get` | Lists the authenticated merchant's partner terminals and credential metadata; empty list is `200` (not `404`); never returns plaintext `api_key` (show-once); requires `bnpl:merchant-terminals:manage:merchant` |
| `/api/v1/merchant/terminal` | `POST` | `merchantterminaluc.Provision` | Issues the partner terminal and first API key (plaintext `api_key` shown once — store it immediately); requires `bnpl:merchant-terminals:manage:merchant` and `Idempotency-Key` |
| `/api/v1/merchant/terminal/credentials/rotate` | `POST` | `merchantterminaluc.Rotate` | Issues a new partner API key (plaintext shown once — store it immediately); requires `bnpl:merchant-terminals:manage:merchant` and `Idempotency-Key`; previous key stays valid for the grace period |
| `/api/v1/merchant/terminal/credentials/:credential_id/retire` | `POST` | `merchantterminaluc.Retire` | Ends the grace period of a retiring credential at once; requires `bnpl:merchant-terminals:manage:merchant` |
| `/api/v1/checkout-decisions` | `POST` | `decisionuc.Create` | Unified POS/online checkout; requires `Idempotency-Key`; approved/partial outcomes atomically persist and return the installment offer and next checkout action |
| `/api/v1/customer-initial-credit-limits` | `POST` | `customercreditlimituc.AssignInitial` | Initial credit limit assignment |
| `/api/v1/contracts/:contract_id/installments/:installment_id/payment-attempts` | `POST` | `installmentrepaymentuc.CreatePaymentAttempt` | Creates a payment attempt for the next payable scheduled installment; requires `Idempotency-Key` |

Health routes are registered only when `usecases.health.enabled` is true (`HEALTH_ENABLED`).

Unmatched paths receive Gin's default 404 behavior.

### Partner terminal authentication

Everything under `/api/v1/partner` is authenticated by `middleware.PartnerTerminalAuth` (terminal credentials), **not** by the full gateway `AuthContext.Attach` middleware. Customer checkout under `/api/v1/checkout-sessions/:token` is a sibling of the Attach group (no JWT). Partner Create accepts `customer_phone` (not `identity_subject_id`). `OfferPreparationMode` lives on `merchant_branch_profiles.offer_preparation_mode`, is snapshotted on the session (`on_create` \| `on_start`, default `on_start`), and controls when Decision/Offer preparation runs relative to create vs customer `start`. Gateway tenant/context headers are not used on token routes.

| Header | Constant | Meaning |
| --- | --- | --- |
| `X-Terminal-Id` | `header.TerminalID` | Public numeric terminal identifier (decimal string) |
| `X-API-Key` | `header.APIKey` | Secret API key; stored only as an HMAC-SHA256 hash keyed with `MERCHANT_TERMINAL_API_KEY_PEPPER`; never logged |

- A missing, malformed, unknown, or wrong credential returns the same `401` (`error.invalid_terminal_credentials`), so a caller cannot tell which part was rejected.
- On success the merchant principal and its **server-side terminal permission assignments** (persisted at provision time under `merchant_terminal_permissions`) are placed on the request context; per-route `RequireAny` checks keep working unchanged. Partner clients cannot grant permissions via `x-auth-permissions` or the API key.
- **API key show-once:** plaintext `api_key` is returned only from `POST /api/v1/merchant/terminal` (Provision), `POST /api/v1/merchant/terminal/credentials/rotate`, and an idempotent replay of that same issuance request within the configured replay window. BNPL persists only an irreversible keyed HMAC verifier for authentication; read APIs never expose the secret. Merchants must copy the key into their own secure store (secret manager, vault, env) when it is issued. `GET /api/v1/merchant/terminal` returns metadata only (`terminal_id`, status, credential id/version, retiring grace) — never `api_key`. Outside the replay window a lost key requires Rotate (use `grace_period_seconds=0` to retire the previous key immediately).
- A merchant provisions its terminal with `POST /api/v1/merchant/terminal` after panel login (gateway `x-auth-subject-id` / `x-auth-tenant-id` / `x-auth-context-id` / `x-auth-permissions` including `bnpl:merchant-terminals:manage:merchant`, plus `Idempotency-Key`). The key is encrypted at rest only for the idempotent replay window (`MERCHANT_TERMINAL_ISSUANCE_DELIVERY_TTL`); responses that carry a key send `Cache-Control: no-store`.
- `GET /api/v1/merchant/terminal` returns a list: empty `data: []` when no terminal exists yet, or one entry with credential metadata and no plaintext API key.
- The same gateway auth rotates and retires credentials: `POST /api/v1/merchant/terminal/credentials/rotate` (`Idempotency-Key` required, optional `grace_period_seconds`) and `POST /api/v1/merchant/terminal/credentials/:credential_id/retire`. Rotation is refused with `409` while an earlier rotation is still inside its grace period; retire ends that grace period early.
- Production infrastructure must forward `X-Terminal-Id` and `X-API-Key` unchanged, prevent direct public access to BNPL, and strip client-supplied `x-auth-*` headers on `/api/v1/partner/`. Partner routes do not require Gateway JWT identity headers. Do not log request headers on that prefix.

Configuration: `usecases.merchant_terminal` (`MERCHANT_TERMINAL_*` variables in `.env.example`). API-key verifiers use a versioned pepper key ring (`api_key_active_pepper_key_id` / `api_key_pepper_keys`): new credentials hash with the active key id; existing credentials keep verifying with the key id embedded in their stored verifier. An old pepper must remain configured while any dependent credential exists; removing it fails authentication closed for those credentials. Pepper key ids must match `[A-Za-z0-9_-]{1,64}` (no `:`). Issuance encryption keys are separate from the webhook secret keys. Production requires `merchant_terminal_issuance_scrub` enabled so expired issuance ciphertext is scrubbed into idempotency tombstones.

### Checkout flow

### Partner checkout + token customer lifecycle

All merchant integrations (Digikala, Technolife, Damz, …) share the same boundary:

1. **Merchant Backend** — `POST /api/v1/partner/checkout-sessions` with `X-Terminal-Id` + `X-API-Key` only (no JWT tenant/context). Body includes required `customer_phone` (Iranian mobile; not `verified_phone_number`). BNPL resolves the phone through Customer Reference Service when preparing an Offer (`on_create`), binds the BNPL customer profile, and returns a merchant-safe payload: public token (`cs_<64 hex>`), `checkout_url`, status, expiry. When the branch snapshotted mode is `on_create`, Create also returns the full financing Offer; when `on_start`, Offer is omitted until customer START. Create never sends OTP.
2. **Customer browser** — BNPL checkout UI calls unauthenticated token routes under `/api/v1/checkout-sessions/:token` (`GET`, `POST .../start`, `.../confirm`, `.../cancel`, `GET .../return`). No gateway JWT.
3. **Offer timing** — Branch `offer_preparation_mode` (`merchant_branch_profiles`, snapshotted on the session): `on_create` prepares Decision/Offer before a successful create response when policy allows; `on_start` (default) defers preparation to customer `POST .../start` after they continue on the displayed `evaluation_generation`. Customer `GET` projects persisted safe offer state read-only; `start` issues SMS OTP; `confirm` verifies OTP and enters contract/payment fulfillment.

`ExternalCustomerID` is the Customer Reference Service `customer_id` stored as `customer_profiles.external_customer_id`. It is distinct from BNPL internal `CustomerID` / `customer_profiles.id`.

Checkout customer payment uses Financial Core v2 RPCs without FinancialScope/tenant/context on the request. `financial_delegation` is unresolved (FIXME) — customer payment paths fail closed until delegation is defined.

Return destinations are pre-registered during merchant onboarding/configuration and selected by symbolic name per checkout; arbitrary return URLs are not accepted. `merchant_state` is an opaque, non-sensitive correlation value echoed on browser return and webhooks.

Integration contracts:

- External-style partners: [external-partner-checkout.md](integration/external-partner-checkout.md)
- First-party partners (Damz): [internal-partner-checkout.md](integration/internal-partner-checkout.md) — same Partner Create API; branch `offer_preparation_mode` is the main product difference

**Create checkout decision** — `POST /api/v1/checkout-decisions`

The same endpoint handles physical and online purchases. The request declares the purchase channel explicitly; BNPL never infers it from missing fields and never accepts merchant/branch identifiers directly from the checkout client.

Physical POS request:

```json
{
  "purchase_context": {
    "type": "pos",
    "pos_gps_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c"
  },
  "requested_amount_minor": 150000000,
  "currency": "IRR"
}
```

Online request:

```json
{
  "purchase_context": {
    "type": "online",
    "online_store_id": "8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
    "merchant_order_id": "ORD-2026-847291"
  },
  "requested_amount_minor": 150000000,
  "currency": "IRR"
}
```

`merchant_order_id` is optional and is only a correlation reference owned by the merchant. `online_store_id` identifies the registered BNPL storefront and is used to resolve the online selling point and parent merchant. For POS purchases, `pos_gps_id` resolves the registered terminal, physical selling point, and merchant.

```text
Create Checkout Decision
→ Validate explicit purchase context
→ Resolve POS terminal or online storefront to MerchantBranchProfile + MerchantProfile
→ Resolve customer checkout policy
→ Evaluate financing
→ Persist Decision + immutable purchase-source allocation
→ If approved or partially approved:
   persist installments and charges in the same transaction
→ Return persisted installment offer + checkout_action
```

- Denied decisions return reason codes and omit `installment_offer` and `checkout_action`; no installment records are created.
- `purchase_context` is included in the response from the persisted source snapshot; later contract creation does not accept merchant/source data again.
- `checkout_action.type=confirm_and_pay` means the client should offer a confirmation/payment interaction and use `payment_amount_minor` as the policy-defined initial payment.
- `checkout_action.type=confirm_purchase` means confirmation creates the contract without an initial payment. The frontend should not branch on checkout policy version.
- `installment_offer.payable_now_amount_minor` is policy-derived. It is the initial payment for policies requiring payment before activation and `0` for direct-activation policies.
- Does **not** reserve or reduce available credit; capacity is re-checked at contract creation/activation.
- Idempotent via `Idempotency-Key`. The purchase channel, source ID, optional merchant order ID, amount, currency, customer, and selected policy are material idempotency inputs.

#### Checkout decision response — initial payment required

```json
{
  "result": "created",
  "decision": {
    "id": "...",
    "checkout_key": "...",
    "outcome": "approved",
    "reason_codes": []
  },
  "purchase_context": {
    "type": "pos",
    "pos_gps_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c"
  },
  "checkout_action": {
    "type": "confirm_and_pay",
    "payment_required": true,
    "payment_amount_minor": 75000000
  },
  "requested_amount_minor": 150000000,
  "approved_financed_amount_minor": 100000000,
  "upfront_amount_minor": 50000000,
  "currency": "IRR",
  "available_credit_minor": 0,
  "expires_at": "2026-07-07T12:00:00Z",
  "installment_offer": {
    "installment_count": 4,
    "payable_now_amount_minor": 75000000,
    "total_payable_amount_minor": 150000000,
    "currency": "IRR",
    "expires_at": "2026-07-07T12:00:00Z",
    "installments": [
      {
        "id": "...",
        "installment_order": 1,
        "status": "offered",
        "due_at": "2026-07-07T12:00:00Z",
        "principal_amount_minor": 25000000,
        "upfront_amount_minor": 50000000,
        "base_fee_amount_minor": 0,
        "total_amount_minor": 75000000
      }
    ]
  }
}
```

For a direct-activation checkout, `checkout_action.type` is `confirm_purchase`, `payment_required` is `false`, `payment_amount_minor` and `payable_now_amount_minor` are both `0`, while installments remain visible in the offer.

Idempotent replay returns `result: "already_exists"` with the same Decision, purchase-source snapshot, checkout action, and persisted installments.

### Installment payment attempt

`POST /api/v1/contracts/:contract_id/installments/:installment_id/payment-attempts`

Headers:

- `Idempotency-Key` (required)

Request body:

```json
{
  "customer_id": "...",
  "channel": "gateway"
}
```

The payment return URL is configured server-side (`external.financial_core.payment_return_url`), not supplied by the client.

Amount and currency are resolved from the persisted installment principal plus **customer-payable** charges — not accepted from the client. The attempt persists an immutable snapshot (`payment_attempt_items`) atomically with the parent `PaymentAttempt`: principal uses `PrincipalAmountMinor` only; fees/penalties are debit charge items; discounts/waivers are credit items. Merchant/platform-paid charges are excluded.

Responses:

- `201` with `"result": "created"` when a new attempt is created and submitted to Financial Core
- `200` with `"result": "already_exists"` on idempotent replay

The response includes local `PaymentAttempt` state (`submitted` after Financial Core session is persisted) and optional `payment_url` / `expires_at` from Financial Core. This is BNPL orchestration state, not Financial Core provider-attempt lifecycle.

See ADR [0016](adr/0016-bnpl-payment-attempt-ownership.md).

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
| 1 | Panic recovery | Always on | Recovers request panics, reports them through the configured error reporter, and returns HTTP 500 without re-panicking |
| 2 | Trace ID | Always on | Propagates `X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id` |
| 3 | Language | Always on | Resolves the REST response locale |
| 4 | HTTPDebugLogger | `GIN_HTTP_DEBUG_LOGGER_ENABLED` | Development-only unredacted HTTP exchange diagnostics (disabled by default) |
| 5 | AccessLogger | `GIN_LOGGER` | Structured access logging; optional allowlisted response projection via `GIN_RESPONSE_LOGGING_ENABLED` |
| 6 | Admission control | Always on | Body size and in-flight limits |
| 7 | Error handler | Always on | Serializes application errors and reports server-side failures |

See [configuration.md](configuration.md) for HTTPDebugLogger security warnings and Production response-logging allowlist rules.

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
| 200 | Health probes succeed; idempotent replays (`already_exists`) |
| 201 | New checkout decision created |
| 404 | Unknown paths (Gin default) |
| 409 | Idempotency conflict |
| 500 | Panic in middleware chain if recovery enabled; unclassified errors from `SerErr` |
| 503 | Health probe failure wrapping `cerr.ServiceUnavailable` |

## Planned

Future BNPL APIs (not implemented — do not treat as available):

### Contracts and installments (Contract, RepaymentPlan, Installment)

- Create and activate BNPL contract from order
- Query installment schedule and status

### Payments (PaymentAttempt)

- ~~Initiate installment payment~~ — **Implemented** for payable scheduled installments via payment-attempts endpoint
- Payment gateway webhook/callback — Financial Core responsibility; BNPL consumes payment-result events via Kafka + integration inbox
- Payment-result processing: inbound **succeeded** → BNPL emits `bnpl.payment_application_result.v1` (`applied` / `already_applied` / `reconciliation_required`); inbound **failed/canceled** → local terminal state only, no outbound application event
- Kafka adapter validates minimum trusted envelope (`source_service`, `event_id`, `event_type`, `event_version`); compatibility defaults are isolated in `envelope_compatibility.go`; business fields validated after inbox acquisition
- Unknown Financial Core payment statuses preserve raw normalized value in fingerprint and durable inbox failure (after acquisition)
- Missing local contract/installment on succeeded payment → `reconciliation_required` with `contract_not_found` / `installment_not_found`
- Process results: `event_already_processed` (exact inbox replay) is distinct from payment-application `already_applied`; `terminal_result_unmatched` when failed/canceled events have no matching local attempt

### Merchant (MerchantProfile, MerchantBranchProfile)

- Merchant onboarding and configuration
- Physical/online selling-point management and POS/online-store source mappings

### Admin and audit (Audit, CollectionsCase, LegalCase)

- Search customers, contracts, installments
- Manual operational actions with audit trail
- Collections case management

## Request / Response Flow

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

Handlers depend on use cases, not `pgx` or raw SQL.

## TODO

- [ ] Add authentication middleware before any sensitive routes
- [ ] Publish OpenAPI schema for implemented endpoints

## Related documentation

- [architecture.md](architecture.md) — HTTP request flow
- [operations.md](operations.md) — health probes and graceful shutdown
- [security.md](security.md) — future AuthN/AuthZ requirements
- [collections/apis.http](collections/apis.http) — HTTP client examples
