# BNPL Frontend ↔ Backend Integration Guide (English)

> **Audience:** BNPL customer app, merchant administration UI, and BNPL-owned Hosted Checkout UI developers and QA.  
> **Language:** English · [فارسی](bnpl-frontend.fa.md)  
> **Wire contract update:** 2026-11-10 — `external_` prefix for IDs owned by external systems. Delta: [changelog](bnpl-frontend-external-ids-changelog.en.md).  
> **API prefix:** `/api/v1`, without a service-name prefix. The origin and frontend route for Hosted Checkout are deployment configuration, **not** hard-coded by this document.  
> **Related merchant-server documentation:** [Partner integration (EN)](../integration/partner-checkout.en.md) / [فارسی](../integration/partner-checkout.md).

This guide specifies **what the current Go backend actually exposes**. It distinguishes APIs for three trust surfaces. Not every backend feature is available through a browser API, and a redirect or optimistic UI state is never proof of a financial operation.

## 1. Identify your frontend surface first

| Surface | Who uses it | Authentication | Allowed APIs |
| --- | --- | --- | --- |
| **Customer dashboard / POS-oriented customer UI** | Signed-in customer | Identity-provider login → trusted Envoy gateway; gateway validates identity and injects authorization context | Customer decision, credit, contract, installment, payment read/write endpoints |
| **Merchant management UI** | Signed-in merchant staff | The **same trusted-gateway authentication boundary**, with merchant subject and merchant permissions | Merchant profile, branches, sales, terminal provisioning/rotation/retirement |
| **BNPL Hosted Checkout UI** | Customer redirected from a merchant checkout | Unguessable public `cs_…` token; SMS OTP for confirmation; **no gateway JWT auth context** | Only `/api/v1/checkout-sessions/{token}` and its hosted child routes |
| **Merchant server integration (not browser UI)** | Partner backend (Damz, Digikala, etc.) | `X-Terminal-Id` + `X-API-Key`, kept on merchant backend | `/api/v1/partner/**`; covered by the separate Partner guide |

**Never mix the surfaces.** In particular, do not put merchant terminal API keys in a browser, do not manufacture `X-Auth-*` headers in a client, and do not require an authenticated dashboard session to open a public Hosted Checkout URL.

The backend does **not** offer a frontend login/logout/token-refresh endpoint. Authentication and access-token lifecycle belong to the Identity Service and gateway.

## 2. Deployment and HTTP-client setup

Configure, per environment:

- `BNPL_API_ORIGIN` — gateway origin such as `https://api.example.test`.
- `IDENTITY_ORIGIN` / authentication-client settings — issued by the Identity team.
- `HOSTED_CHECKOUT_ORIGIN` and hosted-app route — supplied by the deployment team. Merchant backends provide an opaque `checkout_url`; the frontend should not reconstruct it.
- `APP_LOCALE` — optional initial UI locale; backend supports `en-US` and `fa-IR` via `Accept-Language`.
- No frontend environment variable may contain `X-API-Key`, FC credentials, a webhook signing secret, or database secrets.

Use HTTPS; apply the gateway's CORS origin allowlist. CORS is a browser boundary, **not** the application's authentication mechanism. The backend Gin HTTP service is not intended to be reached directly by untrusted clients.

### Dashboard client headers

A normal authenticated request goes to the gateway with the identity token in the mechanism agreed with the Identity/gateway team, commonly:

~~~http
GET /api/v1/customer-credit HTTP/1.1
Host: api.example.test
Authorization: Bearer <identity-access-token>
Accept: application/json
Accept-Language: en-US
X-Request-Id: <optional-uuid>
X-Correlation-Id: <optional-correlation-id>
~~~

**Do not send these from frontend code:** `X-Auth-Subject-Id`, `X-Auth-Tenant-Id`, `X-Auth-Context-Id`, `X-Auth-Permissions`. Envoy must strip untrusted copies and inject verified values. Gin's `AuthContext.Attach` then reads these exact four headers. A failed gateway login is different from a `403` caused by insufficient permissions.

`X-Trace-Id`, `X-Request-Id` and `X-Correlation-Id` are for observability only; keep the IDs from responses when filing support tickets. Do not place tokens in URLs or log headers containing secrets.

### Hosted client headers

Hosted token APIs use only their path token (and OTP where applicable):

~~~http
GET /api/v1/checkout-sessions/cs_<64-hex-chars>
Accept: application/json
Accept-Language: fa-IR
~~~

They do **not** require a Bearer token, merchant terminal key, or `X-Auth-*` headers. Hosted responses use `Cache-Control: no-store` and `Referrer-Policy: no-referrer`. The Hosted UI must similarly avoid leaking its token to analytics URLs, referrers, third-party scripts, logs, and persistent local storage.

## 3. Response envelopes and typed HTTP handling

### Success — standard non-list response

~~~json
{
  "data": { "status": "active" },
  "meta": {
    "status": "success",
    "code": "CUSTOMER_CREDIT_RETRIEVED",
    "message": "Customer credit summary returned.",
    "status_code": 200,
    "timestamp": "2026-11-04T12:00:00Z",
    "trace_id": "a1b2c3d4-e5f6-4890-abcd-ef1234567890",
    "request_id": "b2c3d4e5-f6a7-4901-bcde-f12345678901"
  }
}
~~~

The example illustrates envelope shape, not a promise that every operation returns the shown `meta.code` or `data`. Read the **actual HTTP status**, `data` and stable `meta.code`. `meta.message` is localized, and its text must never drive business logic. Optional `meta.correlation_id` and `meta.pagination` appear when applicable. Successful list endpoints are also `{data, meta}` responses; the `data` shape varies by endpoint.

### Error — **not** wrapped in `data` / `meta`

~~~json
{
  "code": "VALIDATION_FAILED",
  "message": "Request validation failed.",
  "trace_id": "a1b2c3d4-e5f6-4890-abcd-ef1234567890",
  "details": {
    "fields": ["Idempotency-Key"],
    "violations": [
      {
        "code": "REQUIRED",
        "message": "Required header is missing.",
        "fields": ["Idempotency-Key"]
      }
    ],
    "retry_after_seconds": 30
  }
}
~~~

`details`, `violations` and `retry_after_seconds` are optional. Relevant HTTP statuses include `400` (invalid argument), `401` (authentication), `403` (permission), `404` (not found/not owned), `409` (business/idempotency conflict), `422` (validation/decision rejected/expired), `429` (rate limit), `502` (payment provider), `503` (unavailable), and `500`. The exact code depends on the failed operation; never assume every conflict is retryable.

### API client implementation rules

1. Preserve the **HTTP status** and parse the appropriate success/error envelope.
2. Prefer `meta.code` / `error.code` / field violation codes over localized English/Persian messages.
3. For unsafe POST actions, create one stable `Idempotency-Key` per user intention and **reuse it after network timeout**. Generate a new key only for a genuinely new intention.
4. Avoid automatic blanket retries for financial POSTs. After an ambiguous result, re-fetch a safe authoritative read endpoint.
5. Use `AbortController` (or equivalent) for obsolete UI requests; canceling a browser fetch **does not reverse** a backend transaction.
6. Treat all amounts as exact integer **minor units**; do not use floating-point arithmetic for installments or convert IRR to toman silently. JavaScript `Number` can lose precision for large integers; agree on bigint/string serialization if the app supports very large values.
7. Parse `RFC3339` timestamps as instants and display them in the selected locale/timezone. Do not recalculate contract schedule or due dates locally.

## 4. Authenticated customer API map

Every route below uses the gateway identity context and the listed backend permission. Frontend code supplies authentication to the **gateway**, never the trusted `X-Auth-*` values.

| Method | Path | Permission | Status |
| --- | --- | --- | --- |
| `GET` | `/api/v1/customer-credit` | `bnpl:credit-limits:read:self` | `200` |
| `POST` | `/api/v1/customer-credit/initiate` | `bnpl:credit-limits:create:self` | `201` new; `200` existing/denied |
| `POST` | `/api/v1/checkout-decisions` | `bnpl:decisions:create:self` | `201` new; `200` replay |
| `POST` | `/api/v1/checkout-decisions/{decision_id}/contracts` | `bnpl:contracts:create:self` | `201` new; `200` existing |
| `POST` | `/api/v1/contracts/{contract_id}/cancel` | `bnpl:contracts:cancel:self` | `202` started; `200` replay |
| `GET` | `/api/v1/contracts` | `bnpl:contracts:read:self` | `200` |
| `GET` | `/api/v1/contracts/{contract_id}/installments` | `bnpl:installments:read:self` | `200` |
| `GET` | `/api/v1/installments` | `bnpl:installments:read:self` | `200` |
| `POST` | `/api/v1/contracts/{contract_id}/installments/{installment_id}/pay` | `bnpl:payments:start:self` | `202` new; `200` replay |
| `GET` | `/api/v1/payments` | `bnpl:payments:read:self` | `200` |

No customer ID or tenant ID is accepted from the browser for these operations. The authenticated subject is resolved by the backend. Showing or hiding UI buttons based on client permissions is UX only; the API enforces access.

## 5. Customer credit and first-time onboarding

### Read credit

`GET /api/v1/customer-credit` has no body. `data` contains:

- `status`: `active` | `not_assigned` | `expired`
- `credit_usage`: `{status, reason?}`; this is distinct from credit limit lifecycle.
- When active: `limit_amount_minor` (total governed capacity), `available_amount_minor` (remaining usable), and `currency`, plus optional `expires_at`.
- When not assigned: **do not** interpret missing amount as zero; invite the user to initiate assessment.
- When expired: `expires_at` may be present; explain that reassessment is needed.

Backend `meta.code` distinguishes `CUSTOMER_CREDIT_NOT_ASSIGNED` and `CUSTOMER_CREDIT_EXPIRED`. Eligibility and `credit_usage` restrictions can still prevent a payment even with an active limit.

### Initiate credit

~~~http
POST /api/v1/customer-credit/initiate
Authorization: Bearer <access-token>
~~~

No request body. The access token must include Identity claim `auth_mobile_number` (canonical E.164); Envoy injects it as `x-auth-mobile-number`. BNPL resolves that mobile through Customer Reference Service and assigns credit only when CRS status is `ACTIVE` (trusted-customer policy v2 applies when that CRS customer has an assignment). Response `data` contains `result`, `credit_limit_decision.outcome` (`limit_set` | `denied`), `reason_codes[]`, and optional `credit_limit` containing `amount_minor`, `currency`, `status`, `effective_from` and `expires_at`. `201` means a newly created credit assignment; `200` also covers already-assigned or denied. The frontend must branch on `credit_limit_decision.outcome` and then refresh credit, not infer success from HTTP 200 alone.

## 6. Customer direct checkout: Decision → Offer → Contract

This is the **authenticated customer** checkout path. Merchant-initiated Hosted Checkout is a separate route flow (section 10).

### Create checkout Decision

~~~http
POST /api/v1/checkout-decisions
Authorization: Bearer <access-token>
Idempotency-Key: checkout-intent-1001
Content-Type: application/json
~~~

**POS purchase:**

~~~json
{
  "purchase_context": {
    "type": "pos",
    "external_pos_gps_id": "6ba7b811-9dad-11d1-80b4-00c04fd430c8"
  },
  "requested_amount_minor": 150000000,
  "currency": "IRR"
}
~~~

**Online purchase:**

~~~json
{
  "purchase_context": {
    "type": "online",
    "external_online_store_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
    "merchant_order_id": "ORDER-1001"
  },
  "requested_amount_minor": 150000000,
  "currency": "IRR"
}
~~~

`purchase_context.type` is mandatory. `pos` requires `external_pos_gps_id` and forbids online fields; `online` requires `external_online_store_id`, forbids `external_pos_gps_id`, and accepts optional `merchant_order_id` (max 256). These IDs belong to external merchant systems; they are not BNPL internal primary keys. Currency on the wire is one of `IRR`, `EUR` or `USD`; business eligibility can be narrower. On `VALIDATION_FAILED`, `details.fields` uses the same JSON snake_case keys.

Key `data` fields on a successful decision:

~~~json
{
  "result": "created",
  "decision": {
    "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "checkout_key": "checkout-intent-1001",
    "outcome": "approved",
    "reason_codes": []
  },
  "purchase_context": {
    "type": "online",
    "external_online_store_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
    "merchant_order_id": "ORDER-1001"
  },
  "requested_amount_minor": 150000000,
  "approved_financed_amount_minor": 100000000,
  "upfront_amount_minor": 50000000,
  "currency": "IRR",
  "available_credit_minor": 100000000,
  "expires_at": "2026-11-04T12:30:00Z",
  "checkout_action": {
    "type": "confirm_and_pay",
    "payment_required": true,
    "payment_amount_minor": 75000000
  },
  "installment_offer": {
    "installment_count": 4,
    "payable_now_amount_minor": 75000000,
    "total_payable_amount_minor": 150000000,
    "currency": "IRR",
    "expires_at": "2026-11-04T12:30:00Z",
    "installments": []
  }
}
~~~

This is a **shape example**, not a calculated schedule. Actual installment arrays have `id`, `installment_order`, `status`, `due_at`, `principal_amount_minor`, `upfront_amount_minor`, `base_fee_amount_minor` and `total_amount_minor`. Always display returned terms; never calculate an alternative schedule. Conditional/partial financing is mapped to HTTP `decision.outcome=approved`; determine upfront contribution from `upfront_amount_minor`, not from an invented `partial_approved` enum.

`decision.outcome=denied` has reason codes and **no** `installment_offer`. For an approved decision use `checkout_action.type` (`confirm_and_pay` or `confirm_purchase`) to drive the CTA, not the POS/online distinction. This operation **does not reserve/deduct available credit**; eligibility is rechecked later.

### Create contract from approved decision

~~~http
POST /api/v1/checkout-decisions/{decision_id}/contracts
Authorization: Bearer <access-token>
~~~

No payment-provider selection or business request body. The operation is idempotent by Decision ID. `data` includes:

~~~json
{
  "result": "created",
  "contract": {
    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "status": "payment_pending"
  },
  "payment_required": true,
  "redirect_url": "https://payment.example/session/opaque",
  "expires_at": "2026-11-04T12:40:00Z"
}
~~~

If `payment_required=true` and `redirect_url` is present, perform a **top-level browser navigation** to exactly that URL. Do not process PSP/FC identifiers, do not build the URL yourself, and do not embed the PSP in an unsafe frame. If `payment_required=false`, no payment redirect is required; **still verify subsequent authoritative backend contract status** before reporting final activation.

A `201` contract creation response or browser payment return is **not** contract activation. Financial Core payment/funding is authoritative; BNPL activates only after its backend processing.

## 7. Customer dashboard, installments, and repayment

### Dashboard reads

- `GET /api/v1/contracts` returns **active/canceling** contracts (not all historical contracts). `data` is an array. Item fields include `id`, `merchant_profile_id`, optional `merchant_branch_profile_id`, `requested_amount_minor`, `currency`, `installment_count`, `status` and `created_at`.
- `GET /api/v1/contracts/{contract_id}/installments` returns a **single object**: `contract_id`, `requested_amount_minor`, `currency`, `status`, `created_at`, `installments[]` and nullable `next_payable_installment`. Each installment has `id`, `installment_order`, `total_amount_minor`, `currency`, `due_at`, `status` and optional `paid_at`. Do not derive current next-payable installment client-side if the backend provides it.
- `GET /api/v1/installments` returns **an object, not a bare array**: `data.installments[]` and `data.repayment`. The latter is a customer-wide summary, not just current page. It contains `due_installments[]`, `due_count`, `unpaid_count`, nullable `next_payable` and `has_pending_payment`. The `next_payable` can expose `payable_amount_known`, `outstanding_amount_minor`, `payment_block_reason`, `has_pending_payment` and `payable_amount_minor`.
- `GET /api/v1/payments` returns finalized customer payment attempts only: `succeeded`, `failed` and `canceled`; purpose `first_installment` or `installment`. In-progress attempts are **not** included. Each item provides `id`, `contract_id`, `installment_id`, `amount_minor`, `currency`, `status`, `purpose`, `created_at` and optional `completed_at`.

A `200` with empty list is normal. Do not use absence in payment history as proof that no payment is in flight. The frontend should respect `has_pending_payment` and `payment_block_reason` to disable premature payment attempts.

### Start an installment repayment

~~~http
POST /api/v1/contracts/{contract_id}/installments/{installment_id}/pay
Authorization: Bearer <access-token>
Idempotency-Key: repay-contract-123-installment-2-attempt-1
Content-Type: application/json
~~~

~~~json
{"channel":"gateway"}
~~~

`channel` must be one of `gateway`, `bank_transfer` or `wallet`; availability is subject to actual backend policy. The backend enforces earliest-payable-installment order: an arbitrary later installment ID is not necessarily payable.

`202` for a new attempt and `200` for replay. Read `data.result` and `data.payment_attempt`:

~~~json
{
  "result": "queued",
  "payment_attempt": {
    "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
    "installment_id": "0a1b2c3d-4e5f-6789-abcd-ef0123456789",
    "amount_minor": 25000000,
    "currency": "IRR",
    "state": "submitted",
    "channel": "gateway",
    "redirect_url": "https://payment.example/session/opaque",
    "expires_at": "2026-11-04T12:40:00Z"
  }
}
~~~

`redirect_url` and `expires_at` are optional. If a valid `redirect_url` is returned, navigate to it. If not, show an operation in progress or meaningful retry/recovery UX based on state/error; **do not fabricate a URL**.

For a single PaymentAttempt, the backend permits at most three **customer-triggered** synchronous Financial Core session-creation calls using one stable idempotency identity. The first two failures can retain local `created`; the third failed call cancels locally. Do not implement an automatic rapid retry loop to spend the budget. A fresh user intention after a terminal state needs a fresh key; first refresh unpaid installments / payment history to ensure the previous payment did not settle. Final payment success is determined asynchronously by FC and BNPL, not by redirect.

### Cancel a contract (separate from checkout cancellation)

~~~http
POST /api/v1/contracts/{contract_id}/cancel
Authorization: Bearer <access-token>
Idempotency-Key: cancel-contract-123-v1
Content-Type: application/json

{"reason":"customer_requested"}
~~~

`202`: cancellation started and `data.status=canceling`; `200`: exact replay. FC remains authoritative for the final `canceled` transition. Do **not** display "canceled" when merely `canceling`.

## 8. Query pagination

| Route | Mode | Parameters and order |
| --- | --- | --- |
| `GET /contracts` | Keyset | `limit` (default 20, max 100), `after_id` + `after_created_at`; descending created time |
| `GET /payments` | Keyset | Same pair; descending created time |
| `GET /installments` | Keyset | `limit`, `after_id` + `after_due_at`; ascending due time |
| `GET /merchant/branches` | `pagination=keyset` default or `pagination=page` | Keyset: `limit` + `after_id` + `after_created_at`; page: `page` + `limit` |
| `GET /merchant/sales` | Same as branches | Same options |

For keyset lists inspect `meta.pagination.keyset.next_after_id` and `next_after_created_at` or `next_after_due_at`. Continue only when `meta.pagination.has_next=true`. Do not assume `meta.pagination` is embedded inside `data`; do not synthesize timestamps from the last row. For page mode use `meta.pagination.page` and its `current_page`, `total_pages`, `total_items` and `page_size`.

## 9. Merchant management frontend

These are **gateway-authenticated merchant dashboard routes**, **not** the terminal-authenticated `/partner` APIs.

| Method | Path | Permission | Purpose |
| --- | --- | --- | --- |
| `GET` | `/api/v1/merchant` | `bnpl:merchant-profile:read:merchant` | Merchant profile (`external_merchant_id`, currencies/limits/status) |
| `GET` | `/api/v1/merchant/branches` | `bnpl:branches:read:merchant` | Physical and online branches (`external_branch_id`) |
| `GET` | `/api/v1/merchant/sales` | `bnpl:contracts:read:merchant` | Active BNPL sales |
| `GET` | `/api/v1/merchant/terminal` | `bnpl:merchant-terminals:manage:merchant` | Terminal list and credential metadata; **never plaintext key** |
| `POST` | `/api/v1/merchant/terminal` | Same | Provision one terminal; `Idempotency-Key` required; `201`/`200` |
| `POST` | `/api/v1/merchant/terminal/credentials/rotate` | Same | Rotate key; `Idempotency-Key` required; optional `{"grace_period_seconds":3600}` |
| `POST` | `/api/v1/merchant/terminal/credentials/{credential_id}/retire` | Same | Retire a key according to backend policy |

One merchant currently has at most one terminal. The terminal list is an array in `data` and may be empty.

**Sensitive-key UX:** Provision and Rotate can return `api_key` **only on issue and permitted replay during the replay window**. Show a one-time reveal/copy UI, warn that it cannot be retrieved through GET, and never persist the key in Redux, localStorage, analytics, crash reports or unencrypted browser storage. Prefer a secure server-side credential-transfer procedure; do not treat browser rendering as secure long-term storage. `terminal_id` is a potentially large numeric ID; represent it as a string in UI state to avoid JS precision loss.

Merchant pages must not invoke `/api/v1/partner/checkout-sessions` using this newly provisioned key from the browser. The merchant backend owns that integration. See the separate Partner guide.

## 10. BNPL Hosted Checkout frontend — complete flow

The merchant backend creates checkout and supplies `checkout_url`. The BNPL Hosted Checkout app receives the public `cs_` token through its own deployment-specific URL; that UI then uses only:

| Method | Endpoint | Role |
| --- | --- | --- |
| `GET` | `/api/v1/checkout-sessions/{token}` | Safe first render; no OTP sent |
| `POST` | `.../{token}/start` | Prepare/reuse offer; **never sends OTP** |
| `POST` | `.../{token}/otp` | Request SMS OTP for specific evaluation generation |
| `POST` | `.../{token}/confirm` | Verify OTP, confirm purchase, potentially create payment redirect |
| `GET` | `.../{token}/confirmation-status` | Read-only progress when Confirm outcome is ambiguous |
| `GET` | `.../{token}/return` | `302` to the merchant's pre-registered return URL; never payment authority |
| `POST` | `.../{token}/cancel` | Customer token cancellation if state permits |

### Step A: GET (initial render)

`GET /api/v1/checkout-sessions/{token}` returns `data.checkout_session_id`, `status`, `merchant.display_name`, `amount_minor`, `currency`, `evaluation_generation`, optional `offer`, `verification.type=sms_otp`, masked `verification.destination` and `expires_at`.

The token is a random public `cs_` followed by 64 hex characters, not an internal UUID. GET **does not** resolve a customer identity, generate a financing decision, request OTP, or call Financial Core.

An already prepared Offer may be visible because the merchant's branch is configured `on_create`. A deferred `on_start` branch may have no Offer until START. This mode is a merchant-branch server setting, not something the UI chooses or guesses based on merchant name.

Hosted Offer fields include `outcome` (`approved` / `denied`), optional requested/financed/down-payment amounts, `currency`, `offer_expires_at`, `installment_count`, `payable_now_amount_minor`, `total_payable_amount_minor`, `installments[]` and `checkout_action`. **Hosted DTOs deliberately exclude `available_credit_minor` and internal customer identifiers** even when partner backend Create had such data.

### Step B: START (prepare/re-read Offer)

~~~http
POST /api/v1/checkout-sessions/{token}/start
Content-Type: application/json

{}
~~~

On first deferred START, `{}` or empty body is valid. If GET already presented an Offer, send `{"evaluation_generation":N}` from that response. START returns current `checkout_session_id`, `status`, `evaluation_generation` and `offer` when prepared; it **does not issue or send OTP**.

If Offer is denied/unavailable, show its unavailable state; do not progress to OTP/Confirm for an invalid Offer.

### Step C: request OTP

~~~http
POST /api/v1/checkout-sessions/{token}/otp
Content-Type: application/json

{"evaluation_generation":1}
~~~

Response `data`: `checkout_session_id`, `status`, `evaluation_generation`, `otp_sent`, optional `otp_expires_at`, `otp_next_issue_at` and `otp_availability`. The server may reuse an existing challenge without sending another SMS. Respect cooldown, rate limits and lockouts. The masked phone is informational only; UI cannot choose a new phone via this API.

### Step D: confirm

~~~http
POST /api/v1/checkout-sessions/{token}/confirm
Content-Type: application/json

{"evaluation_generation":1,"otp":"123456"}
~~~

This verifies a six-digit OTP and runs durable purchase confirmation. Response `data` contains `checkout_session_id`, `status`, `reconfirm_required`, `evaluation_generation`, optional `offer`, optional `contract` (`id`/`status`), `payment_required`, optional `redirect_url` and `expires_at`.

- If `reconfirm_required=true`, show the refreshed Offer and obtain a **new** OTP for the new generation. Do not silently accept changed financing terms.
- If `payment_required=true` and `redirect_url` exists, navigate to the opaque URL. Its presence is **not** proof of payment.
- If `status=confirming` or `processing`, show pending; do not declare success or mint another confirmation.
- If a merchant-funding policy yields no payment redirect, show completion only when server reports `completed`, not merely because the operation returned 200.
- If Confirm times out, use `confirmation-status`; do not automatically send another OTP/Confirm in a tight loop.

### Step E: safely recover Confirm uncertainty

`GET /api/v1/checkout-sessions/{token}/confirmation-status` returns `status`, `confirmation_status`, `next_action`, optional `retry_after_seconds`, `otp_next_issue_at` and `otp_availability`. Possible `confirmation_status` values:

`not_started`, `processing`, `payment_action_required`, `payment_processing`, `reconfirm_required`, `retry_available`, `completed`, `needs_attention`, `canceled`, `expired`, `unavailable`.

This endpoint **never** returns a payment URL, starts a Saga or creates financial operations. Use server `next_action` and `retry_after_seconds` as UX hints, then re-enter the permitted START/OTP/CONFIRM workflow as needed.

### Step F: browser return

`GET /api/v1/checkout-sessions/{token}/return` is a **navigation** endpoint, returning `302` to the merchant's registered destination. It does not accept arbitrary return destinations from query parameters. It appends `checkout_session_id` and optional `state` (opaque merchant state).

The receiving merchant webpage must treat the return as UX only and ask its own backend to verify `status=completed` through Partner Inquiry or a signed lifecycle Webhook. It must **not** fulfill orders from browser query strings. The Hosted UI should likewise avoid claiming FC payment success on redirect alone.

### Hosted session status state machine

`open → confirming → processing → completed` is a common path. Terminal statuses include `canceled`, `expired` and `unavailable`. Successful merchant-funded checkouts may complete without a PSP redirect; failed payment initiation/repayment continuation may return to `open`. Cancellation/expiration is **not** permitted in every in-progress state. Always use the server state and current generation instead of assuming all transitions are legal.

## 11. UI state management and error/recovery table

| Situation | Required frontend behavior |
| --- | --- |
| 401 on authenticated dashboard | Reauthenticate through Identity/gateway; do not fake `X-Auth-*` |
| 403 on authenticated dashboard | Explain missing permission; hide unauthorized controls as UX, but do not replace server checks |
| 404 on someone else's contract/session | Treat as inaccessible; never reveal resource existence |
| Credit `not_assigned` | Offer `POST /customer-credit/initiate`; not a zero-balance state |
| Credit `expired` | Offer reassessment; do not show old limit as active |
| Decision denied | Show reason codes/message; do not attempt contract creation |
| Decision approved with upfront requirement | Show exact backend amounts and installment schedule; do not reprice |
| PSP/browser returns | Show pending; re-read authoritative BNPL state |
| Installment `has_pending_payment=true` or `payment_block_reason` | Disable duplicate payment CTA; do not assume failed |
| POST payment timeout | Reuse same idempotency key only for deliberate retry; refresh dashboard first |
| Contract `canceling` | Display "cancellation in progress", not final cancellation |
| Hosted START succeeds | Show Offer, then separately request OTP |
| Hosted OTP cooldown/locked | Respect `otp_next_issue_at` / `otp_availability`, no aggressive re-request |
| Hosted stale Offer generation | Show refreshed terms, require fresh user consent and OTP |
| Hosted Confirm uncertain | Poll `confirmation-status` using retry hint; do not manufacture financial success |
| Hosted `completed` | Show completion, then navigate via backend Return endpoint |
| Hosted `expired/canceled/unavailable` | Show terminal UI, avoid mutating actions |
| Any 429/503 | Use bounded backoff and server retry hint if returned |
| Aborted request / tab close | Treat remote outcome as unknown; recover from safe reads |

## 12. Localization, money, and accessibility

- Current REST localization supports **`en-US` and `fa-IR` only**. `Accept-Language` has precedence over the legacy `lang` request header. Response `Content-Language` identifies the selected locale; unsupported values fall back to the configured default (usually `en-US`).
- Human-readable messages are localized, but technical values such as statuses, error codes, `meta.code`, pagination, and `UP` health labels remain canonical English.
- Render Persian in RTL and English in LTR. Keep UUIDs, money identifiers, URLs and JSON values visually isolated with appropriate bidirectional styles; **never modify the wire strings to solve RTL formatting**.
- All minor-unit monetary values are exact integers. Let `currency` drive formatting, not a merchant-name guess. Do not multiply/divide by 10 without an explicit business/UI conversion rule.
- Display due dates and expiries from backend ISO timestamps. If a checkout/Offer has expired, refetch rather than trusting a stale countdown.
- Provide accessible, non-technical error descriptions, but preserve canonical codes for telemetry and support.

## 13. Frontend integration acceptance checklist

- [ ] API URLs use the gateway origin and exact `/api/v1` paths, without `/bnpl` service prefix.
- [ ] Customer and Merchant dashboards use authenticated gateway requests; frontend does not set trusted identity headers.
- [ ] Hosted Checkout uses public token and OTP only, not merchant terminal credentials or dashboard JWT.
- [ ] Response parser distinguishes success `{data,meta}` from top-level error `{code,message,trace_id,details?}`.
- [ ] All POSTs use correct idempotency policy and bounded retries; ambiguous financial operations remain pending until backend verification.
- [ ] Credit statuses `not_assigned` / `expired` / `active` are distinct UI states.
- [ ] Partial financing displays server-provided upfront/instalment terms without local recalculation.
- [ ] Contract activation/repayment/cancellation is never inferred from redirects.
- [ ] Repayment CTA uses server-selected `next_payable` / `payment_block_reason`.
- [ ] History pagination uses `meta.pagination` and the exact cursor pair.
- [ ] Hosted START does not send OTP; `/otp` is separate and bound to `evaluation_generation`.
- [ ] Stale Hosted Offer requires reconsent and OTP; uncertain Confirm uses read-only polling.
- [ ] Merchant credentials are shown only at issuance, never persisted to browser storage.
- [ ] `en-US` / `fa-IR` and RTL/LTR interaction are covered.
- [ ] QA covers duplicate taps, page reload, navigation interruptions, network timeouts, out-of-order responses, and delayed FC results.

## 14. Source-of-truth index and coordination gaps

Implementation entrypoints:

- Route surfaces and authentication: `pkg/adapter/restful/gin/routes/register.go`, `middleware/auth_context.go`, `middleware/partner_terminal_auth.go`, `middleware/hosted_hardening.go`.
- Customer Decision and Offer: `decisionrs/serdser.go`, `decisionrs/create.go`, `decisionuc`.
- Contract and cancellation: `contractrs/serdser.go`, `contractrs/create_contract.go`, `contractrs/cancel.go`.
- Credit: `customercreditrs/serdser.go`, `customercreditrs/assign_initial_limit_serdser.go`.
- Account reads and repayment: `customerrs/list_active_contracts.go`, `customerrs/list_unpaid_installments.go`, `customerrs/list_payments.go`, `installmentpaymentrs/serdser.go`.
- Merchant management and terminals: `merchantrs/resource.go`, `merchantrs/serdser.go`, `merchantterminalrs/resource.go`, `merchantterminalrs/serdser.go`.
- Hosted UI: `checkoutsessionrs/resource.go`, `checkoutsessionrs/hosted_external.go`, `checkoutsessionrs/request_otp.go`, `checkoutsessionrs/confirmation_status.go`.
- Envelope, locale, pagination: `serdser/success_response.go`, `serdser/error_response.go`, `restful/i18n/locale.go`, `pagination/parse.go`.

These relative paths are under `pkg/adapter/restful/gin/` unless a Core package is named.

**Environment-specific items not specified by this Go repository:** actual gateway URL, Identity login/refresh/token storage approach, allowed browser origins/CORS configuration, hosted app URL scheme, PSP callback page UI, feature flags/launch roles, and error copy/design assets. Obtain these from deployment/Identity teams; do not invent them.

**Document maintenance:** when routes, wire DTOs, permission strings, or the Checkout state machine change, update **both** [English](bnpl-frontend.en.md) and [Persian](bnpl-frontend.fa.md) versions in the same change.
