# External Partner ↔ BNPL Checkout Integration Guide

> This document is the **merchant-facing integration contract** for Partner checkout (Digikala, Technolife, Damz, and all other merchants). All merchants use the same Partner Create API and customer token lifecycle; first-party notes live in [internal-partner-checkout.md](internal-partner-checkout.md).
>
> Financial Core, PSP, payment providers, payment intents/sessions, and other BNPL internals are **out of contract**. Do not model them in the merchant domain.

**Audience:** Merchant Backend · Merchant Frontend · Merchant QA · Cursor / AI coding agents in the merchant repo  
**Source of truth:** Latest BNPL source (`checkoutsessionrs`, `checkoutsessionuc`, `checkoutlifecycle`, `otpcode`, external `identity` / `notification` ports, `merchantwebhook/http`, `merchantterminaluc`, permission catalog, return/webhook policies)  
**Base path:** all REST paths below are under `/api/v1`  
**Auth:** Partner APIs authenticate Merchant identity with terminal credentials — `X-Terminal-Id` + `X-API-Key` on every `/api/v1/partner/**` call; see [Partner terminal authentication](#authentication--permissions) and [api.md](../api.md#partner-terminal-authentication). Partner Create is JWT-independent (no Gateway tenant/context/subject headers). Customer token routes under `/api/v1/checkout-sessions/:token` are also JWT-independent (public token + OTP). Gateway-provided SubjectID/Permissions do not control Partner Merchant identity or permissions. Checkout payment uses Financial Core v2 (`package payment.v1`; `/payment.v1.CheckoutPaymentService/CreateCheckoutPayment`, `/payment.v1.CheckoutPaymentService/GetPaymentStatus`) without FinancialScope/tenant/context on new payment RPCs. Purchase `CreateCheckoutPayment` uses `payer=customer` / `payee=platform` with an empty platform `party_id` and a required `expires_at`; no customer `financial_delegation` Bearer token is required.

---

## Quick Start (mental model)

```text
Create Checkout (Identity resolve + Decision/Offer prepared; no OTP)
    ↓
Receive checkout_url
    ↓
Redirect Customer to BNPL Checkout UI (checkout_url)
    ↓
GET /checkout-sessions/{token} → customer sees offer (per branch offer_preparation_mode)
    ↓
Continue → POST .../start → SMS OTP (when mode is on_start or refresh needed)
    ↓
POST .../confirm → OTP authorizes generation → Contract fulfillment
    ↓
Receive Authoritative Result via Signed Webhook and/or Inquiry
```

Three non-negotiable rules:

| Channel | Authority |
| --- | --- |
| Browser return to merchant | **Not** payment success |
| Signed lifecycle webhook | **Authoritative** |
| Partner Inquiry API | **Authoritative** |

Merchant may fulfill / finalize an order **only** when Inquiry or Webhook says:

```text
status == "completed"
```

and `checkout_session_id`, `merchant_order_id`, `amount_minor`, `currency` match the merchant’s expected values.

---

## Architecture

```text
External Merchant Backend
        |
        | POST /partner/checkout-sessions
        v
BNPL
        |
        | checkout_url
        v
Merchant Frontend / Browser
        |
        | Redirect (opaque URL)
        v
BNPL Checkout UI (token routes)
        |
        | GET /checkout-sessions/{token}: project persisted safe offer (read-only)
        | POST .../start: issue SMS OTP for exact evaluation_generation
        | POST .../confirm: OTP proves phone possession + authorizes that generation
        | BNPL handles contract + payment internally
        v
BNPL
        |
        +---- Signed Webhook ----> Merchant Backend
        |
        +---- Inquiry API <------ Merchant Backend
        |
        v
Browser Return to Merchant (UX only)
```

Merchant **never**:

- selects a payment provider
- calls Financial Core
- confirms the contract for the customer
- treats browser return as payment confirmation
- reconstructs `checkout_url`
- receives customer credit / offer data on partner create

---

## Unified Partner Create (all merchants)

Digikala, Technolife, Damz, and other merchants all use **`POST /api/v1/partner/checkout-sessions`** with **`X-Terminal-Id` + `X-API-Key`** and required **`customer_phone`**. There is no separate merchant create route and no `identity_subject_id` on create.

The main operational difference between merchants is **`merchant_branch_profiles.offer_preparation_mode`** for the selling branch (snapshotted on the session):

| Mode | When Decision/Offer is prepared | Customer experience |
| --- | --- | --- |
| `on_start` (default) | On customer `POST .../start` after Continue | Page load may show status without a full offer until Continue |
| `on_create` | Before successful Partner Create returns (when policy allows) | Page load can show the prepared safe offer immediately |

Partner Create response is always merchant-safe: `checkout_session_id` (public `cs_…` token), `status`, `checkout_url`, `expires_at` — **never** credit, schedules, or Decision economics.

First-party Damz notes: [internal-partner-checkout.md](internal-partner-checkout.md).

Merchant-only customer handle example:

```text
customer_reference = DAMZ-CUSTOMER-12345
```

That value is **not** a BNPL customer id.

---

## Authentication & Permissions

Partner runtime APIs under `/api/v1/partner/**` authenticate **Merchant identity** with terminal credentials. They are **JWT-independent**: Partner Create does **not** require `X-Auth-Tenant-Id` / `X-Auth-Context-Id`.

```text
Merchant Backend
        ↓
Gateway (optional edge; must strip spoofed X-Auth-* on /api/v1/partner/)
        ↓
BNPL Partner API
        ↓
X-Terminal-Id + X-API-Key
        ↓
PartnerTerminalAuth → MerchantPrincipal + terminal permissions
        ↓
RequireAny(permission)
        ↓
Partner use case
        ↓ (Create)
merchant_branch_profiles.offer_preparation_mode
        ↓ snapshot onto checkout_sessions.offer_preparation_mode
on_create: prepare before create success · on_start: prepare on customer POST .../start
        ↓
Customer POST .../confirm → contract/payment (Financial Core v2; no scope/tenant/context on RPC — delegation FIXME, fail closed)
```

**Trust on Partner Create:**

| Concern | Source |
| --- | --- |
| Merchant identity + permissions | Terminal ID + API key (`PartnerTerminalAuth`) |
| When the credit offer is prepared | Server-owned `merchant_branch_profiles.offer_preparation_mode` (`on_create` \| `on_start`, default `on_start`) snapshotted on `checkout_sessions.offer_preparation_mode` |

Gateway-provided SubjectID/Permissions/Tenant/Context do **not** control Partner Merchant identity or permissions. Checkout payment RPCs use Financial Core v2 without FinancialScope/tenant/context; purchase parties are customer→platform with an empty platform `party_id`.

**Authentication ≠ Authorization.** The API key proves *who* the terminal is. It does **not** encode permissions. Permissions are assigned server-side to the terminal at provision time and loaded on every authenticated request.

### Gateway security prerequisite

Production infrastructure MUST:

1. prevent direct public access to BNPL;
2. strip spoofed client-supplied `X-Auth-*` headers on Partner routes;
3. forward `X-Terminal-Id` and `X-API-Key` unchanged.

Partner routes do **not** require Gateway JWT injection of tenant/context. BNPL does not parse JWTs.

### Required Partner headers (every `/api/v1/partner/**` call)

| Header | Meaning |
| --- | --- |
| `X-Terminal-Id` | Public numeric terminal id (decimal string, no leading zeros / whitespace tricks) |
| `X-API-Key` | Secret API key (`bnpl_…`). Compared exactly; never logged by BNPL |

Optional tracing headers (always echoed/generated): `X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id`.

Do **not** treat as Merchant-controlled on Partner routes:

| Header | Why |
| --- | --- |
| `x-auth-subject-id` | **Not trusted** for Merchant identity — terminal credentials own SubjectID |
| `x-auth-permissions` | **Not trusted** — cannot escalate Partner access |
| `x-auth-tenant-id` / `x-auth-context-id` | **Not required and not trusted** on Partner routes |

Missing, duplicate, malformed, unknown, or wrong **terminal** credentials all return the same public `401` (`error.invalid_terminal_credentials`). Callers cannot tell which part failed.

```bash
# Reuse on every Partner curl in this guide:
PARTNER_AUTH=(
  -H "X-Terminal-Id: $TERMINAL_ID"
  -H "X-API-Key: $API_KEY"
)
```

### How merchants obtain `TERMINAL_ID` / `API_KEY`

Credential lifecycle is a **Merchant Admin Panel** concern (gateway-authenticated), not a Partner runtime call:

| Panel API | Purpose |
| --- | --- |
| `POST /api/v1/merchant/terminal` | Provision terminal + first API key (`Idempotency-Key` required) |
| `GET /api/v1/merchant/terminal` | Metadata only — **never** returns plaintext `api_key` |
| `POST /api/v1/merchant/terminal/credentials/rotate` | Issue a new key; previous key may stay valid for a grace period |
| `POST /api/v1/merchant/terminal/credentials/{credential_id}/retire` | End grace early for a retiring key |

Show-once policy for partners/operators:

- Plaintext `api_key` is returned only by the issuance response (Provision / Rotate) and by an **idempotent replay of that same issuance request** within the configured replay window.
- Read APIs never expose the secret. Store it immediately in the merchant secret manager / vault / env.
- Outside the replay window a lost key requires Rotate.

Panel routes require gateway auth including `bnpl:merchant-terminals:manage:merchant`. Details: [api.md — Partner terminal authentication](../api.md#partner-terminal-authentication).

### Terminal permissions (server-assigned)

On successful Provision, BNPL persists an initial permission set for that terminal (supported External Partner contract today). Authenticate loads those rows; there is **no** runtime hard-coded grant-all fallback.

| Permission | Used for |
| --- | --- |
| `bnpl:checkout-sessions:create:merchant` | Create + Cancel partner checkout |
| `bnpl:checkout-sessions:read:merchant` | Inquiry |
| `bnpl:checkout-return-destinations:manage:merchant` | Register return destinations |
| `bnpl:checkout-webhooks:manage:merchant` | Register / rotate webhook destination |

Notes:

- Integration-config permissions are **separate** from checkout create/read. A terminal without a manage grant receives `403` on that route even with a valid API key.
- Rotating or retiring an API key **does not** change terminal permissions.
- Partners cannot supply a permission list in the API key, request body, or headers.
- If BNPL operators later trim a terminal’s grants, affected routes return `403` without requiring a key rotation.

> “Permission: …” annotations on Partner endpoints below mean **required terminal assignment**, not a header the merchant sends.

### Customer token routes — Merchant Backend must not use

All customers (every merchant) use **unauthenticated** token routes under `/api/v1/checkout-sessions/{token}/...` where `{token}` is the public checkout token (`cs_<64 hex>`). The BNPL checkout **frontend** calls those; Merchant Backend never does.

These routes are registered **without** gateway `AuthContext.Attach`. OTP + public token are the customer credentials.

| Audience | Base path | Auth |
| --- | --- | --- |
| BNPL checkout UI (customer) | `/api/v1/checkout-sessions/{token}/...` | None (public token + OTP) |
| Merchant Partner API | `/api/v1/partner/checkout-sessions/...` | Terminal ID + API key |

Merchant-facing Partner permissions (not customer-token routes):

| Permission | Endpoint |
| --- | --- |
| `bnpl:checkout-sessions:create:merchant` | Partner create / cancel |
| `bnpl:checkout-sessions:read:merchant` | Partner inquiry |

---

## One-time Integration Setup

Do these once per environment (sandbox / production) before creating checkouts.

**Prerequisites:** store `TERMINAL_ID` and `API_KEY` from Merchant Panel Provision in Backend secrets. Confirm the terminal has the manage permissions needed for setup steps below (default Provision seeds the full supported set).

### 1. Register Return Destination

Symbolic name → fixed HTTPS return URL. Checkout create never accepts arbitrary return URLs.

```http
PUT /api/v1/partner/return-destinations/{destination_name}
Permission: bnpl:checkout-return-destinations:manage:merchant
```

**Path param `destination_name`:** lowercase kebab-case `^[a-z0-9]+(-[a-z0-9]+)*$`, length 1..64. No silent lowercase/rewrite. Examples: `damz-web`, `default`. Rejected: `Damz-Web`, `damz_web`, `damz--web`.

**Request body:**

```json
{
  "return_url": "https://merchant.example/checkout/bnpl/return",
  "enabled": true
}
```

| Field | Rules |
| --- | --- |
| `return_url` | Required, max 2048. Absolute `http`/`https`, hostname required, **no** userinfo, **no** query, **no** fragment (including empty `?` / `#`). Production/staging: HTTPS only. |
| `enabled` | Optional. Omit preserves stored value (`true` on first create). |

**Response `200`:**

```json
{
  "destination_name": "damz-web",
  "return_url": "https://merchant.example/checkout/bnpl/return",
  "enabled": true
}
```

```bash
curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/return-destinations/damz-web" \
  -H "Content-Type: application/json" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "return_url": "https://merchant.example/checkout/bnpl/return",
    "enabled": true
  }'
```

### 2. Register Webhook Destination

One webhook endpoint per merchant (not named destinations). Checkout create never carries a webhook URL.

```http
PUT /api/v1/partner/webhook-destination
Permission: bnpl:checkout-webhooks:manage:merchant
```

**Request body:**

```json
{
  "webhook_url": "https://merchant.example/webhooks/bnpl",
  "signing_secret": "at-least-32-bytes-of-high-entropy-secret!!",
  "enabled": true
}
```

| Field | Rules |
| --- | --- |
| `webhook_url` | Required, max 2048. Same structural URL rules as return URLs; production HTTPS + SSRF policy (no private/metadata targets). |
| `signing_secret` | Write-only. **Required on first registration**, length 32..512. Omit later to keep stored secret; send new value to rotate. **Never logged / never returned.** |
| `enabled` | Optional; omit preserves stored value. |

**Response `200` (secret never echoed):**

```json
{
  "webhook_url": "https://merchant.example/webhooks/bnpl",
  "enabled": true
}
```

```bash
curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/webhook-destination" \
  -H "Content-Type: application/json" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "webhook_url": "https://merchant.example/webhooks/bnpl",
    "signing_secret": "at-least-32-bytes-of-high-entropy-secret!!",
    "enabled": true
  }'
```

### 3. Customer phone (required on every Partner Create)

There is **no** Customer Binding or account-link step. Merchants assert the paying customer’s mobile on **Create Checkout**; BNPL encrypts it server-side, resolves it through Identity (`ResolveSubjectByMobile`), binds the customer when the subject is `individual` + `active`, and prepares the Decision/Offer before returning a successful `open` checkout URL. Hosted **GET** projects that offer read-only. Hosted **START** (after the customer clicks Continue) generates an OTP and submits it through the Notification Service for the exact `evaluation_generation` shown. Hosted **CONFIRM** proves phone possession and authorizes that generation before Contract fulfillment. Identity subject status is not synchronized into BNPL CustomerProfile status.

See [Customer phone + token OTP flow](#customer-phone--token-otp-flow).

> **Removed from contract:** `POST /api/v1/partner/customer-bindings`, revoke, HMAC account-link proofs, and `bnpl:checkout-customer-bindings:manage:merchant`.

---

## Customer phone + token OTP flow

### What the merchant sends at Create

| Field | Required | Meaning |
| --- | --- | --- |
| `customer_phone` | **yes** | Iranian mobile: `09XXXXXXXXX` or `+989XXXXXXXXX`. BNPL canonicalizes to E.164 (`+989…`). |
| `customer_reference` | no | Optional merchant-owned handle (max 256). Correlation only — **not** a BNPL customer id. |

Merchant **must not** send `identity_subject_id` on Partner Create. Merchant **must not** treat `customer_reference` as authority to pick a BNPL customer.

BNPL may persist **`customer_profiles.external_customer_id`** from the Customer Reference Service (`customer_id` on that service). That **ExternalCustomerID** is not BNPL internal `customer_profiles.id` and is not supplied by merchants on Partner Create.

### What happens on the customer token flow

1. Customer opens the opaque `checkout_url` (public token in the path).
2. Checkout UI `GET /api/v1/checkout-sessions/{token}` loads persisted safe offer (when prepared per branch mode), masked SMS destination, and `evaluation_generation` (read-only; no Identity resolve, Decision create, OTP send, or writes on GET).
3. Customer reviews the offer and clicks Continue; UI `POST /api/v1/checkout-sessions/{token}/start` with `{ "evaluation_generation": N }` — on `on_start` branches this prepares the offer if needed, validates generation, then sends SMS OTP. No OTP on page load alone.
4. Customer enters the OTP; UI `POST /api/v1/checkout-sessions/{token}/confirm` with `{ "evaluation_generation", "otp" }`.
5. On OTP success BNPL continues Confirm & Pay internally (contract + payment / merchant-funding UX). OTP alone is not monetary truth; Contract activates only after the policy-required authoritative Financial Core operation succeeds (v2 RPCs; purchase checkout payment payer=customer, payee=platform with empty platform party_id).

Partner APIs never expose OTP codes, full phone numbers, or customer credit data.

---

## Create Checkout (primary Partner API)

```http
POST /api/v1/partner/checkout-sessions
Caller: Merchant Backend only
Permission: bnpl:checkout-sessions:create:merchant
Success: 201 Created (new) or 200 OK (idempotent replay)
Success meta.code: CHECKOUT_SESSION_CREATED
```

### Request body (exact)

```json
{
  "online_store_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
  "merchant_order_id": "DAMZ-ORDER-1001",
  "idempotency_key": "DAMZ-ORDER-1001-ATTEMPT-1",
  "customer_reference": "DAMZ-CUSTOMER-12345",
  "customer_phone": "+989121234567",
  "return_destination": "damz-web",
  "merchant_state": "DAMZ-CART-1001",
  "requested_amount_minor": 120000000,
  "currency": "IRR"
}
```

There is **no** `identity_subject_id` on this request.

### Field reference

#### `online_store_id`

UUID of the merchant’s online store provisioned in BNPL (maps to online store → online `MerchantBranchProfile` → merchant). Must belong to the authenticated merchant; otherwise Forbidden (`error.online_store_is_not_owned_by_the_authenticated_merchant`). Provisioning is out-of-band (see Open Questions).

#### `merchant_order_id`

Merchant business order reference. Max 256. Required.

- One order may have multiple checkout **attempts** over time (after cancel/expire/unavailable).
- Only one **active** attempt (`open` | `confirming` | `processing`) at a time.
- A **completed** attempt closes the order for new keys (`IDEMPOTENCY_CONFLICT`).

#### `idempotency_key`

Optional, max 256. Identifies one create operation.

- Omitted → defaults to `merchant_order_id` (`EffectiveIdempotencyKey`).
- Timeout / retry of the **same** operation → **same** key.
- New attempt after terminal prior attempt → **new** key.
- Same key + same commercial terms → replay same session (`200`).
- Same key / active order + **different** terms → `IDEMPOTENCY_CONFLICT` (no mutation).

Compared terms on replay: `online_store_id`, `merchant_order_id`, `customer_reference`, `customer_phone` (canonical E.164), `return_destination`, `merchant_state`, `requested_amount_minor`, `currency`.

#### `customer_phone`

Required for partner create. Accepts Iranian mobile `09…` or `+989…`; BNPL stores canonical E.164. Wrong or missing phone fails validation at Create. Idempotent replay must send the **same** phone as the original create.

#### `customer_reference`

Optional for partner create, max 256. Merchant customer handle for correlation (webhooks, inquiry context). Not a customer id.

#### `return_destination`

Required symbolic name (1..64, kebab-case grammar). Must be registered and **enabled**. Not a URL.

#### `merchant_state`

Optional opaque correlation, max 256. Echoed on browser return as `state` and on webhooks when present. BNPL does not interpret it.

#### `requested_amount_minor`

Required `uint64`, minimum 1, maximum `math.MaxInt64`. Amount in **minor units** of `currency`.

Supported currencies in model: `IRR`, `EUR`, `USD`.

**IRR:** BNPL expects **Iranian Rial** minor units (1 IRR = 1 minor unit). If the merchant ledger stores **Toman**, convert before send:

```text
amount_irr_minor = amount_toman * 10
```

Example: 12,000,000 Toman → `requested_amount_minor = 120000000`, `currency = "IRR"`.

#### `currency`

Required. Must be a valid BNPL currency (`IRR` | `EUR` | `USD`).

### Response body (merchant-safe)

```json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "status": "open",
  "checkout_url": "https://bnpl.test/checkout/cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "expires_at": "2026-10-06T12:30:00Z"
}
```

Partner create does **not** return: credit balances, offers, installment schedules, Decision details, provider codes, or Financial Core ids.

`checkout_session_id` in Partner responses is the **public opaque token** (`cs_` + 64 lowercase hex). The JSON field name stays `checkout_session_id`; it is **not** the internal BNPL UUID. Use the same token for Inquiry, Cancel, webhooks, and browser return query params.

`checkout_url` shape from BNPL: `{public_origin}/checkout/{token}` where `public_origin` is BNPL deployment config (`CHECKOUT_SESSION_PUBLIC_ORIGIN`). Treat the URL as **opaque** — do not rebuild it.

```bash
curl -sS -X POST "$BNPL_API_BASE/api/v1/partner/checkout-sessions" \
  -H "Content-Type: application/json" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY" \
  -d '{
    "online_store_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
    "merchant_order_id": "DAMZ-ORDER-1001",
    "idempotency_key": "DAMZ-ORDER-1001-ATTEMPT-1",
    "customer_reference": "DAMZ-CUSTOMER-12345",
    "customer_phone": "+989121234567",
    "return_destination": "damz-web",
    "merchant_state": "DAMZ-CART-1001",
    "requested_amount_minor": 120000000,
    "currency": "IRR"
  }'
```

**When to call:** After order is ready for BNPL payment method, return destination + webhook are configured, and you have the customer’s mobile (`customer_phone`) for this checkout attempt.

**Retry:** network timeout → same `idempotency_key`. New commercial attempt → new key.

---

## Browser Redirect

Merchant Frontend must redirect the customer to the `checkout_url` from Create:

```javascript
window.location.assign(checkout_url);
```

Rules:

- Do not reconstruct the URL.
- Do not append merchant query params to BNPL’s hosted URL.
- Do not call Partner APIs from the browser with merchant credentials.

---

## Customer token OTP flow (informational — Merchant Backend does not call these)

These **unauthenticated** routes are called by the **BNPL checkout frontend** for all merchants. Path `{token}` is the public token (`cs_<64 hex>`) from Create — not an internal UUID.

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/checkout-sessions/{token}` |
| `POST` | `/api/v1/checkout-sessions/{token}/start` |
| `POST` | `/api/v1/checkout-sessions/{token}/confirm` |
| `POST` | `/api/v1/checkout-sessions/{token}/cancel` |
| `GET` | `/api/v1/checkout-sessions/{token}/return` |

### Frontend state machine

```text
PAGE LOAD
    GET checkout
    ↓
    render offer (evaluation_generation + safe purchase economics)

APPROVED
    show Continue button

CONTINUE
    POST start({ evaluation_generation })
    ↓
    show OTP input (otp_sent / otp_expires_at)

OTP ENTERED
    show Confirm & Pay (or Confirm purchase) button

CONFIRM
    POST confirm({ evaluation_generation, otp })
```

**Confirm & Pay** means: authorize the purchase + create a `payment_pending` contract + obtain payment continuation. It does **not** mean the Contract is active immediately. Activation happens only after the policy-required authoritative Financial Core operation succeeds.

For normal policy (`payment_required=true`): redirect to the returned payment URL; Contract becomes active only after authoritative FC payment success.

For special policy (`payment_required=false`): after authoritative merchant-funding success, checkout may already be `completed`; navigate through the hosted return endpoint.

For `reconfirm_required=true`: the previously accepted **material** offer can no longer be honored (for example financed/upfront terms changed, or eligibility is no longer met). A raw change in evaluation-time `available_credit` alone does **not** force reconfirmation when the same financed/upfront terms remain supportable. Show the new offer, require another explicit Continue, and request a **new** OTP for the new generation. Never reuse the old OTP.

If the refreshed evaluation is denied, the checkout becomes `unavailable` (and may emit `bnpl.checkout.unavailable.v1`). That is **not** a `reconfirm_required` result.

### Flow

1. Partner Create already resolved Identity, bound the customer, and persisted an approved Decision/Offer (no OTP).
2. Browser opens `checkout_url`.
3. Checkout UI `GET /api/v1/checkout-sessions/{token}` — **read-only** projection of persisted state:
   - merchant display name, purchase amount, status, `evaluation_generation`
   - privacy-safe `offer` (approved purchase economics; **no** `available_credit_minor`, `customer_reference`, Decision ID, full phone, customer ids)
   - masked SMS destination (`verification.type` = `sms_otp`)
   - GET never calls Identity, creates a Decision, sends OTP, or mutates lifecycle state
4. Customer reviews the offer and clicks Continue.
5. Checkout UI `POST /api/v1/checkout-sessions/{token}/start` with `{ "evaluation_generation": N }` (exact generation from GET):
   - validates open session, bound customer, persisted approved offer for generation N
   - prepares/reuses the financing Offer only — **never** issues OTP or submits Notification
   - returns the Offer and authoritative `evaluation_generation`
   - **does not** call Identity when the offer is already prepared
   - stale generation → reconfirm/stale-offer error
6. Checkout UI `POST /api/v1/checkout-sessions/{token}/otp` with `{ "evaluation_generation": N }`:
   - delegates to the reusable `otp` use case (`otp_challenges` / `otp_rate_limits`) after checkout eligibility locks
   - applies OTP rate limits, issues a challenge, and submits Notification outside the transaction
   - returns privacy-safe OTP state (`otp_sent`, `otp_expires_at`, `otp_next_issue_at`, `otp_availability`)
   - an unused active challenge is replayed without a second SMS; a consumed challenge blocks re-issue until `otp_next_issue_at`
7. Customer enters the OTP; Checkout UI `POST /api/v1/checkout-sessions/{token}/confirm` with `{ "evaluation_generation": N, "otp": "<six-digit code>" }`.
   - **Every external purchase requires OTP.** The code is bound to the active `evaluation_generation` via opaque context version.
   - OTP Notification submission is idempotent per `issue_id` while an accepted challenge is still valid (retries do not create a new Notification request).
8. If Confirm-time Decision revalidation proves the accepted material offer can no longer be honored → bump generation, invalidate OTP bound to the old generation, create a fresh Decision → `reconfirm_required: true` + new approved offer (no Contract; no Financial Core call). Frontend must Continue → START(new gen) → OTP → Confirm. A denied refresh makes the checkout `unavailable` instead.
9. If OTP invalid / expired → validation error; customer retries `POST .../otp` for a fresh code when allowed.
10. If ok → BNPL creates contract and returns customer next-step UX:
   - `payment_required: true` → follow Confirm `redirect_url` to Financial Core payment
   - `payment_required: false` (special purchase / trusted policy) → after OTP + authoritative Financial Core merchant funding, session may reach `completed` without a customer payment redirect
11. BNPL determines authoritative outcome → `completed` / terminal states.
12. Merchant receives webhook; browser may navigate to `GET .../return` (UX only — **not** payment truth).

### Browser return (`GET .../return`)

Hosted UI calls this **after** a no-payment (`payment_required: false`) path reaches `completed`, or when the product needs to send the customer back to the merchant storefront.

| Property | Rule |
| --- | --- |
| Auth | None (public token in path) |
| Destination | **Only** the HTTPS `ReturnURL` registered via `return_destination` at Create — never from request query, `Referer`, or `Origin` |
| Query built by BNPL | `checkout_session_id` = public token; optional `state` = `merchant_state` when present |
| Authority | **Not** payment or order fulfillment — merchants trust **only** Partner Inquiry (`status=completed`) and signed `bnpl.checkout.completed.v1` webhook |

Request query parameters such as `?url=` are ignored. Merchant Backend must not call this route.

### Why `/start` exists (external)

Partner Create prepares Identity resolution and the financing offer. `/start` is the **only** Hosted step that may request/send the checkout OTP, and only after the customer explicitly Continues on the exact `evaluation_generation` they saw. No OTP is sent merely because the checkout page was loaded.

### `evaluation_generation`

Offer version the customer saw on GET. START and Confirm must send the same generation. If Confirm-time material revalidation advances the generation, Confirm returns `reconfirm_required: true` and a refreshed offer — Hosted UI must show it, Continue again, and obtain a new OTP for the new generation. **External merchants ignore this field entirely** (Partner APIs never expose it).

### Checkout lifetime vs reconfirmation

`checkout_session.expires_at` is the hard hosted-checkout deadline.

- Preparing an offer may **shorten** that lifetime (never extend it) so the session cannot outlive the prepared terms.
- A refreshed / reconfirmed offer **never extends** `expires_at`.
- The usable window for generation N+1 is bounded by the existing session deadline. A new generation created near expiry may therefore have only a short remaining confirmation window.
- Once the session expires, the merchant must create a new checkout attempt under the existing idempotency / merchant-order rules.

#### Technical timing semantics

Checkout expiry is evaluated against current time after acquiring the
CheckoutSession row lock for state-changing operations. Waiting for a lock
does not preserve an earlier eligibility to continue the checkout.

### Confirm request / response (customer token UI only)

```json
{
  "evaluation_generation": 1,
  "otp": "123456"
}
```

Success shape (fields present depend on path):

```json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "status": "processing",
  "reconfirm_required": false,
  "evaluation_generation": 1,
  "payment_required": true,
  "redirect_url": "https://…",
  "expires_at": "2026-10-06T12:35:00Z",
  "contract": { "id": "…", "status": "payment_pending" }
}
```

or reconfirm:

```json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "status": "open",
  "reconfirm_required": true,
  "evaluation_generation": 2,
  "offer": { "outcome": "approved", "…": "…" },
  "payment_required": false
}
```

Merchant Backend must not call hosted start/confirm/return. Do not persist Financial Core / provider identifiers even if Hosted responses mention contract ids for the customer UI.

**Hosted frontend routing summary:**

| After Confirm | Next step |
| --- | --- |
| `payment_required: true` | Redirect customer to Financial Core using Confirm `redirect_url` |
| `payment_required: false` and session `completed` | `GET /api/v1/checkout-sessions/{token}/return` → merchant registered return URL |

---

## Inquiry (authoritative)

```http
GET /api/v1/partner/checkout-sessions/{token}
Permission: bnpl:checkout-sessions:read:merchant
Success: 200
Success meta.code: CHECKOUT_SESSION_RETRIEVED
```

**Response:**

```json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "merchant_order_id": "DAMZ-ORDER-1001",
  "status": "completed",
  "amount_minor": 120000000,
  "currency": "IRR",
  "completed_at": "2026-10-06T12:40:00Z"
}
```

Notes from source:

- Foreign merchant’s session → **404 Not Found** (not Forbidden) — anti-enumeration.
- Open session past `expires_at` may be **projected** as `expired` on read without writing durable state.
- No credit / offer / provider / FC fields.

**Before finalize, Merchant Backend must verify:**

```text
checkout_session_id  matches expected
merchant_order_id    matches expected
amount_minor         matches expected
currency             matches expected
status            == "completed"
```

Only `completed` authorizes fulfillment.

```bash
curl -sS "$BNPL_API_BASE/api/v1/partner/checkout-sessions/$CHECKOUT_SESSION_ID" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY"
```

**Retry:** safe to retry (read-only).

---

## Cancellation

```http
POST /api/v1/partner/checkout-sessions/{token}/cancel
Permission: bnpl:checkout-sessions:create:merchant
Success: 200
Success meta.code: CHECKOUT_SESSION_CANCELED
```

**Response:**

```json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "status": "canceled",
  "return_url": "https://merchant.example/checkout/bnpl/return?checkout_session_id=…&state=…"
}
```

| Current status | Result |
| --- | --- |
| `open` (not expired, no accepted contract blocking) | Canceled; reason in lifecycle event: `merchant_canceled` |
| `canceled` | Idempotent success; same return URL |
| `confirming` / `processing` | Conflict — payment outcome unresolved (`error.checkout_payment_is_in_progress_and_must_be_resolved_first`) |
| `open` with contract already accepted | Conflict (same in-progress payment gate) |
| Open but elapsed | May expire durably then reject as expired |
| `completed` / `expired` / `unavailable` | Conflict — not cancelable |

Lifecycle webhook `bnpl.checkout.canceled.v1` is enqueued in the same transaction when cancellation commits.

---

## Status State Machine (merchant-visible)

Exact status strings:

`open` | `confirming` | `processing` | `completed` | `canceled` | `expired` | `unavailable`

```text
OPEN ──confirm──▶ CONFIRMING ──▶ PROCESSING ──▶ COMPLETED
  │                   │              │
  │                   └──(init failed)──▶ OPEN
  │                                  └──(retryable payment failure)──▶ OPEN
  ├──▶ CANCELED   ├──▶ EXPIRED   └──▶ UNAVAILABLE
```

| Status | Meaning | Terminal? | Merchant may fulfill? | Recommended merchant action |
| --- | --- | --- | --- | --- |
| `open` | Handoff usable; customer may continue | No | **No** | Wait / show pending |
| `confirming` | Confirm claimed session; contract/payment init | No | **No** | Wait; do not cancel |
| `processing` | Awaiting authoritative payment outcome | No | **No** | Wait; Inquiry/Webhook |
| `completed` | Activated contract produced | Yes | **Yes** (after amount/order match) | Finalize order |
| `canceled` | Abandoned by merchant or customer | Yes | **No** | Release reservation / fail order |
| `expired` | Handoff lifetime elapsed | Yes | **No** | New attempt with new idempotency key if needed |
| `unavailable` | Financing could not be offered | Yes | **No** | New attempt / other payment method |

**Rule:** Only `completed` means Merchant may finalize/fulfill.

There is no merchant-facing `failed` checkout status. Retryable payment failure returns the session toward `open` for the customer; that is Hosted concern.

---

## Webhook (authoritative)

### Lifecycle event types (exact)

| `event_type` | When |
| --- | --- |
| `bnpl.checkout.completed.v1` | Checkout produced activated contract |
| `bnpl.checkout.canceled.v1` | Abandoned (customer or merchant) |
| `bnpl.checkout.expired.v1` | Open handoff lifetime elapsed |
| `bnpl.checkout.unavailable.v1` | Financing could not be offered |

Envelope:

```json
{
  "event_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef:bnpl.checkout.completed.v1:completed",
  "event_type": "bnpl.checkout.completed.v1",
  "event_version": "1",
  "producer": "bnpl-backend",
  "occurred_at": "2026-10-06T12:40:00Z",
  "data": {
    "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "merchant_order_id": "DAMZ-ORDER-1001",
    "merchant_id": "22222222-2222-2222-2222-222222222222",
    "status": "completed",
    "amount_minor": 120000000,
    "currency": "IRR",
    "completed_at": "2026-10-06T12:40:00Z",
    "merchant_state": "DAMZ-CART-1001"
  }
}
```

`data.reason` appears on canceled events (`customer_canceled` | `merchant_canceled`).  
`data.completed_at` only on completed.  
No credit / schedule / provider / Financial Core ids.

`event_id` construction (stable across retries): `{publicCheckoutToken}:{eventType}:{status}` (same `cs_…` value as `data.checkout_session_id`).

### Delivery headers (exact)

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `User-Agent` | `bnpl-webhook/1` |
| `X-BNPL-Webhook-Id` | Stable event id (dedupe key; same across retries) |
| `X-BNPL-Webhook-Timestamp` | Unix seconds for **this attempt** |
| `X-BNPL-Webhook-Signature` | `v1=<lowercase-hex HMAC-SHA256>` |
| `X-BNPL-Webhook-Event` | `event_type` when present on the outbox message |

### Signature algorithm (exact from source)

Canonical bytes:

```text
v1
<event-id>
<unix-timestamp-decimal>
<body>
```

i.e. `v1` + LF + eventID + LF + timestamp + LF + **raw body bytes**.

```text
X-BNPL-Webhook-Signature = "v1=" + hex(HMAC-SHA256(signing_secret, canonical))
```

Merchant receiver must:

1. Keep the **raw** request body.
2. Parse timestamp; reject outside chosen freshness window (BNPL does not prescribe the window).
3. Recompute HMAC over the canonical string; compare with **constant-time** equality to the `v1=` hex.
4. Persist `X-BNPL-Webhook-Id` / `event_id` for dedupe (at-least-once delivery).
5. **Never** re-serialize JSON before verifying.
6. On `completed`, still match amount/order/currency before fulfill.
7. Respond `2xx` only after durable processing (or after recognizing a duplicate as already processed).

### QA sample (illustrative body + headers)

```http
POST /webhooks/bnpl HTTP/1.1
Host: merchant.example
Content-Type: application/json
User-Agent: bnpl-webhook/1
X-BNPL-Webhook-Id: cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef:bnpl.checkout.completed.v1:completed
X-BNPL-Webhook-Timestamp: 1728211200
X-BNPL-Webhook-Signature: v1=<hex>
X-BNPL-Webhook-Event: bnpl.checkout.completed.v1

{"event_id":"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef:bnpl.checkout.completed.v1:completed","event_type":"bnpl.checkout.completed.v1","event_version":"1","producer":"bnpl-backend","occurred_at":"2026-10-06T12:40:00Z","data":{"checkout_session_id":"cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef","merchant_order_id":"DAMZ-ORDER-1001","merchant_id":"22222222-2222-2222-2222-222222222222","status":"completed","amount_minor":120000000,"currency":"IRR","completed_at":"2026-10-06T12:40:00Z","merchant_state":"DAMZ-CART-1001"}}
```

Duplicate deliveries with the same `X-BNPL-Webhook-Id` are expected — treat as idempotent success.

---

## Browser Return (UX only)

After Hosted flow, BNPL may send the browser to the registered `return_url` with query parameters built by BNPL:

| Query param | Source |
| --- | --- |
| `checkout_session_id` | Always |
| `state` | Present only when `merchant_state` was non-empty |

Example:

```text
https://merchant.example/checkout/bnpl/return?checkout_session_id=cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef&state=DAMZ-CART-1001
```

**Browser Return ≠ Payment Success.**

Merchant Frontend after return should ask **Merchant Backend** for order status (show “checking purchase…”). Merchant Backend uses Inquiry and/or waits for Webhook. Frontend must **not** call BNPL Partner APIs with merchant credentials.

### Race: return before webhook

Normal. Frontend polls Merchant Backend. Backend, if unresolved, may Inquiry BNPL. Do not fulfill on return alone. Do not fail the UX solely because webhook is late — Inquiry can confirm `completed`.

---

## Retry & Idempotency Summary

| Operation | Rule |
| --- | --- |
| Create Checkout timeout | Retry with **same** `idempotency_key` |
| New attempt after terminal | **New** `idempotency_key` |
| Inquiry | Safe to retry |
| Webhook | Expect duplicates; dedupe on event id |
| Cancel | Idempotent when already `canceled` |
| Hosted OTP | START is idempotent while an active/in-flight challenge exists: it does **not** resend SMS or consume another send budget. After the challenge expires and policy allows, START may issue a new challenge. |
| Create with wrong `customer_phone` | Fix phone + new idempotency key for a new attempt |

---

## Error Handling

Public REST errors use:

```json
{
  "code": "INVALID_ARGUMENT",
  "message": "…localized…",
  "trace_id": "…",
  "details": {
    "fields": ["…"],
    "violations": [{ "code": "…", "message": "…", "fields": ["…"] }]
  }
}
```

Machine-relevant `code` values from `cerr` used in partner flows (not exhaustive of entire platform):

| HTTP | BNPL `code` | Typical message key / situation | Merchant action | Retry? |
| --- | --- | --- | --- | --- |
| 400 | `INVALID_ARGUMENT` / `VALIDATION_FAILED` | Invalid input, currency, destination name, URL, amounts | Fix request | No |
| 400 | `INVALID_ARGUMENT` | `error.merchant_return_destination_is_not_registered` | Register / enable destination | No |
| 400 | `INVALID_ARGUMENT` / `VALIDATION_FAILED` | Invalid / missing `customer_phone` on Create | Send valid `09…` or `+989…` mobile | No |
| 403 | `FORBIDDEN` | Online store not owned; merchant inactive | Fix ownership / merchant status | No |
| 404 | `RESOURCE_NOT_FOUND` | Checkout not found **or** other merchant’s session | Do not probe | No |
| 409 | `IDEMPOTENCY_CONFLICT` | Same key/order with different terms or completed order | Use correct key / new order attempt | No |
| 409 | `CONFLICT` | Expired; cannot continue; cannot cancel; payment in progress; concurrent create | Branch on status / Inquiry | Depends |
| 401 | `UNAUTHORIZED` | Invalid / missing `X-Terminal-Id` or `X-API-Key` | Fix terminal credentials | No |
| 422 | `UNPROCESSABLE_ENTITY` | Hosted OTP wrong / expired / too many attempts | Customer retries Start for new code when allowed | Depends |
| 403 | `FORBIDDEN` | Terminal authenticated but missing required server-side permission | Contact BNPL ops to adjust terminal permission assignments (key rotation alone does not grant permissions) | No |
| 5xx | `INTERNAL` / `SERVICE_UNAVAILABLE` | Transient | Retry with backoff; Create uses same idempotency key | Yes (careful) |

Do not invent error codes beyond what BNPL returns.

---

## Security Requirements

External Merchant must:

- Keep `X-Terminal-Id` / `X-API-Key` only in Backend secret storage.
- Never call Partner APIs from the Browser.
- Never send gateway `x-auth-permissions` (or other `x-auth-*`) hoping to escalate Partner access — BNPL ignores them on `/partner/**`.
- Never treat `customer_reference` as BNPL customer identity.
- Collect `customer_phone` only through merchant-trusted channels; BNPL never returns the full phone on Partner APIs.
- Never trust Browser Return as payment success.
- Never treat `checkout_session_id` alone as proof of payment.
- Verify webhook HMAC, timestamp freshness, and dedupe `event_id`.
- Match `amount_minor` / `currency` / `merchant_order_id` before fulfill.
- Never log API keys, webhook signing secrets, or SMS OTP codes.
- Never reconstruct `checkout_url`.
- Never store Financial Core / PSP / provider identifiers as part of this contract.
- Never select a payment provider.

---

## Merchant Data Storage (recommended)

Persist at least:

```text
merchant_order_id
checkout_session_id
idempotency_key
customer_reference
online_store_id
expected_amount_minor
currency
merchant_state
return_destination
bnpl_checkout_status
completed_at (nullable)
processed_webhook_event_ids
created_at
updated_at
```

---

## Failure Scenarios

| Scenario | Merchant behavior |
| --- | --- |
| Create timeout | Retry Create with **same** `idempotency_key` |
| Create retry success | Use returned `checkout_session_id` / `checkout_url` |
| Invalid `customer_phone` at Create | Validation error; fix phone before retry |
| OTP wrong / expired on hosted | Customer retries on BNPL; merchant order unpaid until Inquiry/Webhook |
| Identity cannot resolve checkout phone | Partner Create terminalizes the checkout as `unavailable`; do not redirect/fulfill. A later corrected attempt uses a new idempotency key under the normal merchant-order rules. |
| Denied / `unavailable` | Do not fulfill; offer other payment method or new attempt |
| Material offer no longer honor-able (`reconfirm_required`) | Hosted only; merchant ignores. Not every credit-snapshot change. Denied refresh → `unavailable` |
| Browser closes BNPL page | Session may stay `open` until expiry/cancel; Inquiry |
| Browser closes payment page | May be `processing` / return to `open`; wait for webhook/Inquiry |
| Return before webhook | Show pending; Backend Inquiry |
| Webhook twice | Dedupe on event id; fulfill once |
| Webhook never arrives | Poll Inquiry until terminal or timeout policy |
| Inquiry `processing` / `confirming` | Wait; do not fulfill |
| Inquiry `completed` | Fulfill after field match |
| Checkout `expired` | New attempt with new idempotency key if customer retries BNPL |
| Merchant cancel races customer confirm | Cancel may Conflict if confirming/processing; Inquiry for truth |
| BNPL temporarily unavailable | Retry Create with same key; surface soft failure to UX |

---

## End-to-End Example

Constants:

```text
BNPL_API_BASE           = https://api.bnpl.example          # placeholder — see Open Questions
TERMINAL_ID             = 4242                             # from Merchant Panel Provision
API_KEY                 = bnpl_…                           # store immediately; never log
online_store_id         = 6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c
merchant_order_id       = DAMZ-ORDER-1001
idempotency_key         = DAMZ-ORDER-1001-ATTEMPT-1
customer_reference      = DAMZ-CUSTOMER-12345
customer_phone          = +989121234567
return_destination      = damz-web
merchant_state          = DAMZ-CART-1001
requested_amount_minor  = 120000000
currency                = IRR
```

0. **Obtain terminal credentials** — Merchant Panel `POST /api/v1/merchant/terminal` (gateway auth); persist `terminal_id` + `api_key`.
1. **Register return destination** — `PUT .../return-destinations/damz-web` with `X-Terminal-Id` / `X-API-Key` (curl above).
2. **Register webhook** — `PUT .../webhook-destination` with signing secret.
3. **Create Partner Checkout** — `POST .../partner/checkout-sessions` with `customer_phone` → receive `checkout_url` + public token.
4. **Persist** `checkout_session_id` (token), key, amounts.
5. **Frontend** `window.location.assign(checkout_url)`.
6. **BNPL checkout UI** — GET token session (read-only offer when prepared), Continue → `start` (SMS OTP for that generation), customer confirms with OTP.
7. BNPL handles payment internally.
8. Checkout → `completed`.
9. **Webhook** `bnpl.checkout.completed.v1` → verify signature → match fields → finalize order.
10. Optionally **Inquiry** to confirm.
11. Browser may hit return URL; Frontend asks Backend; Backend already finalized or still pending.

---

## Cursor Implementation Instructions for External Merchant

Copy this section into the merchant project’s Cursor chat.

### Goal

Implement BNPL as a **token checkout payment method** using **only** Partner APIs under `/api/v1/partner/...`. Do **not** call `/api/v1/checkout-sessions/...` (`start`, `confirm`, `cancel`, `return`) from Merchant Backend — those are for the BNPL checkout UI and public token only.

### Process

1. Inspect the merchant repo architecture (HTTP client patterns, secrets, webhooks, order state machine).
2. Add a BNPL Partner client module matching existing HTTP/auth patterns.
3. Implement config: API base URL, `TERMINAL_ID`, `API_KEY` (from Merchant Panel Provision/Rotate), webhook signing secret storage.
4. Attach `X-Terminal-Id` and `X-API-Key` on every Partner request. Do **not** send merchant-fabricated `x-auth-*` headers. Partner Create is JWT-independent.
5. Implement one-time ops endpoints or admin jobs for:
   - `PUT /partner/return-destinations/{name}`
   - `PUT /partner/webhook-destination`
6. Implement Create Checkout (`POST /partner/checkout-sessions`) with required `customer_phone`, optional `customer_reference`, and persistence of public `checkout_session_id` token, `idempotency_key`, expected amount/currency/order.
7. Expose `checkout_url` to Frontend only as an opaque redirect target.
8. Implement Inquiry client (`GET /partner/checkout-sessions/{id}`) using the public token from Create.
9. Implement Webhook receiver:
   - raw body
   - HMAC-SHA256 over `v1\n{id}\n{ts}\n{body}`
   - constant-time compare
   - timestamp window
   - `event_id` dedupe store
   - amount/order/currency match
   - finalize **only** on `status == completed` / event `bnpl.checkout.completed.v1`
10. Implement Cancel when product requires it.
11. Browser return page: pending UX → poll merchant Backend only.
12. Never put Partner credentials in Frontend.
13. Never introduce Financial Core, PSP, payment intent/session, or provider selection into merchant domain models.
14. Treat `403` on Partner routes as missing **server-side terminal permission**, not a missing Identity PermissionSeed header.

### Backend checklist

- [ ] Partner client sends `X-Terminal-Id` + `X-API-Key` on every `/partner/**` call
- [ ] No merchant-fabricated `x-auth-*` on Partner requests; Partner Create does not require JWT tenant/context headers
- [ ] Secrets stored only in Backend (terminal id + API key + webhook signing secret)
- [ ] Return destination + webhook registration
- [ ] Create with `customer_phone` + idempotency persistence
- [ ] Inquiry
- [ ] Webhook verify + dedupe + finalize gate
- [ ] Cancel
- [ ] IRR/Toman conversion if ledger is Toman
- [ ] No customer token start/confirm/cancel/return from Backend

### Frontend checklist

- [ ] Redirect to `checkout_url` only
- [ ] Return page shows pending / polls Backend
- [ ] Never calls BNPL Partner APIs with merchant secrets
- [ ] Never treats return query as success

---

## Public Contract Audit

Partner Inquiry / Create / Webhook `data` intentionally omit customer credit and FC identifiers.

Customer Confirm response may include `contract.id` / `contract.status` and `redirect_url` for the **checkout UI**. Merchant Backend must not call token routes and must not treat them as Partner contract fields.

---

## BNPL Issues Found While Producing This Guide

| Severity | Endpoint / File | Problem | Impact on External Merchant | Recommended Fix |
| --- | --- | --- | --- | --- |
| Low | Webhook event naming | Older notes sometimes say `checkout.completed` | Wrong event type in receivers | Always use `bnpl.checkout.completed.v1` |
| Low | Webhook timestamp skew | BNPL does not define receiver tolerance | Merchants must pick a window | Publish recommended skew in ops docs |
| Info | Hosted Confirm | Exposes `contract` object to customer clients | Not a Partner leak if Partner never calls Confirm | Keep Partner docs free of contract/FC fields |
| Resolved | Partner auth docs | Older drafts showed gateway `x-auth-*` on `/partner/**` | Wrong credentials / false PermissionSeed expectation | Use `X-Terminal-Id` + `X-API-Key`; permissions are server-side terminal assignments |

---

## Open Integration Questions

Do **not** invent answers. Confirm with BNPL ops / Identity before production:

1. **Sandbox API base URL** and **Production API base URL**
2. **Hosted Checkout public origin** values per environment (BNPL sets `CHECKOUT_SESSION_PUBLIC_ORIGIN`)
3. **Merchant Panel access** for terminal Provision / Rotate (who operates `POST /api/v1/merchant/terminal`, how secrets are handed to Backend)
4. **Online store UUID provisioning** for the merchant
5. **Webhook signing secret rotation runbook** with merchant
6. **Recommended webhook timestamp skew**
7. **Sandbox test identities** (terminal id + API key fixtures, store id, test mobile numbers for OTP)
8. Whether this merchant’s terminal needs a **trimmed** permission set (default Provision seeds the supported External Partner set; changes are server-side and do not require rotating the API key)

---

## Definition Checklist (for reviewers)

- [x] External Partner flow only (`/partner/...`)
- [x] Partner auth is `X-Terminal-Id` + `X-API-Key` (not gateway `x-auth-*`)
- [x] Permissions are server-side terminal assignments (not client headers / API key claims)
- [x] Source-backed fields, statuses, headers, signature
- [x] No `identity_subject_id` on Create
- [x] `customer_phone` + optional `customer_reference` documented
- [x] Public `cs_…` token in Partner + webhook fields documented
- [x] Customer token OTP `start` / `confirm` informational only
- [x] Partner Create returns merchant-safe projection
- [x] Browser Return non-authoritative
- [x] Webhook + Inquiry authoritative
- [x] Amount / currency / order validation
- [x] Retry / idempotency
- [x] Status machine
- [x] Failure scenarios
- [x] Cursor instructions
)
