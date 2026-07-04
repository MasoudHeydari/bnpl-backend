# BNPL Partner Checkout Integration — One Contract for All Merchants (English)

> **Audience:** Partner backend, frontend, QA and operations teams (Damz, Digikala and every other merchant).  
> **Language:** English · [فارسی](partner-checkout.md)  
> **Backend reference:** `MasoudHeydari/bnpl-backend`, branch `feat-add-decision-domain`, commit `e71c4e621a842087928dfa37dbf92571ee3f2c27`.  
> **API base path:** `/api/v1`. Ask BNPL for actual Sandbox/Production origins, terminal credentials and Hosted Checkout URL.  
> **Related BNPL-owned UI guide:** [Frontend (EN)](../frontend/bnpl-frontend.en.md) / [فارسی](../frontend/bnpl-frontend.fa.md).

This is the **single partner integration contract**. Merchant origin ("internal" Damz vs "external" Digikala) does **not** select different APIs. All partner backends use terminal authentication and the same route family. What differs is **server-side Merchant/Branch configuration**, including `offer_preparation_mode`.

## 1. Principles and responsibilities

1. **Merchant backend** makes `/api/v1/partner/**` calls with `X-Terminal-Id` and `X-API-Key`. It never exposes either secret or the merchant's webhook signing secret in JavaScript/browser code.
2. **Merchant frontend** navigates the user to the exact opaque `checkout_url` returned by BNPL; it must not construct an alternative URL.
3. **BNPL Hosted Checkout frontend** calls its own public-token routes `/api/v1/checkout-sessions/{token}/**`, uses SMS OTP for confirmation, and interacts with the payment provider only through redirect.
4. **Merchant backend** verifies completion via authenticated Partner Inquiry `status=completed` or a **cryptographically verified** `bnpl.checkout.completed.v1` webhook. Neither browser return nor PSP redirects prove payment.
5. Applying an order fulfillment transition on the merchant side must be **idempotent**.

The existing code's `InternalCheckoutSessionOrigin` is an internal domain distinction, **not** a second Partner HTTP integration. The current Partner Create uses the common external-partner hosted session origin.

## 2. Integration architecture

~~~text
Merchant Backend -- X-Terminal-Id / X-API-Key --> BNPL /api/v1/partner/
          |
          +-- POST /partner/checkout-sessions
          |    <-- checkout_session_id (public cs_ token), checkout_url, expires_at
          |        [optional offer on branch mode on_create]
          |
Merchant Frontend -- navigate to checkout_url --> BNPL Hosted Checkout
                                                   |
                                                   | GET  /checkout-sessions/{token}
                                                   | POST .../start   (prepare/read offer)
                                                   | POST .../otp     (request SMS)
                                                   | POST .../confirm (OTP + confirm/pay)
                                                   | GET  .../confirmation-status
                                                   | GET  .../return  (UX only)
                                                   |
Merchant Backend <-- signed lifecycle webhook -----+
Merchant Backend -- GET Partner Inquiry -----------> BNPL
~~~

There are three distinct auth surfaces:

| Surface | Authentication | Consumer |
| --- | --- | --- |
| `/api/v1/partner/**` | Partner terminal ID / API key and persisted terminal permissions | Merchant backend |
| `/api/v1/merchant/terminal/**` | Trusted Gateway identity and merchant permissions | Authenticated Merchant management UI |
| `/api/v1/checkout-sessions/{token}/**` | Public checkout token and OTP where required; no Gateway identity JWT | BNPL Hosted Checkout UI |

The public `checkout_session_id` is a `cs_` prefix plus 64 hexadecimal characters, **not** the checkout session's internal UUID.

## 3. Merchant onboarding and one-time configuration

### 3.1 Provisioning a terminal

An authenticated Merchant management client, with gateway permission `bnpl:merchant-terminals:manage:merchant`, may use:

| Method | Path | Behavior |
| --- | --- | --- |
| `POST` | `/api/v1/merchant/terminal` | Provision the merchant's terminal and initial API key; requires `Idempotency-Key` |
| `GET` | `/api/v1/merchant/terminal` | Return a terminal metadata array **without** plaintext keys |
| `POST` | `/api/v1/merchant/terminal/credentials/rotate` | Issue a new key, optionally grace-period the previous key |
| `POST` | `/api/v1/merchant/terminal/credentials/{credential_id}/retire` | Retire a previous credential |

Currently each merchant has at most one terminal. `terminal_id` is a positive decimal identifier; treat it as a **string in JS** to avoid losing precision. `api_key` is confidential and has the `bnpl_…` prefix. It is returned upon Provision/Rotate and eligible idempotent replay during the limited replay window. `GET` never returns it. Store it in a secret-management system immediately; a lost credential requires rotation once the replay window has passed.

Terminal permissions are assigned server-side when provisioning and checked on every request. An API key is **not** a JWT and does not encode permissions.

| Persisted terminal permission | Operations |
| --- | --- |
| `bnpl:checkout-sessions:create:merchant` | Create and Cancel sessions |
| `bnpl:checkout-sessions:read:merchant` | Partner Inquiry |
| `bnpl:checkout-return-destinations:manage:merchant` | Return Destination configuration |
| `bnpl:checkout-webhooks:manage:merchant` | Webhook Destination configuration |

An authenticated key can still return `403` if its terminal lacks an assigned permission.

### 3.2 Partner authentication headers

Every Partner API request requires:

~~~http
X-Terminal-Id: 4829174650293817
X-API-Key: bnpl_<secret>
~~~

These routes **do not** use `X-Auth-Subject-Id`, `X-Auth-Tenant-Id`, `X-Auth-Context-Id`, `X-Auth-Permissions` or an end-user JWT as merchant authorization. Merchant identity and permissions are resolved from the authenticated terminal on the backend.

`X-Trace-Id`, `X-Request-Id`, and `X-Correlation-Id` are optional correlation headers and do not grant privileges. Use HTTPS from a controlled backend, never a browser.

### 3.3 Register a Return Destination

~~~http
PUT /api/v1/partner/return-destinations/{destination_name}
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...
~~~

~~~json
{
  "return_url": "https://shop.example/checkout/bnpl/return",
  "enabled": true
}
~~~

The symbolic `destination_name` must match `^[a-z0-9]+(-[a-z0-9]+)*$`, length 1–64 (e.g., `default` or `digikala-web`). Checkout Create sends **this name**, not a free-form return URL. The URL must be absolute and valid without embedded user credentials, query strings or fragments; use HTTPS in production. The optional `enabled` field preserves its previous value when omitted (defaults to `true` on the first insert). A successful upsert returns `200` with `data.destination_name`, `return_url` and `enabled`.

### 3.4 Register a Webhook Destination

~~~http
PUT /api/v1/partner/webhook-destination
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...
~~~

~~~json
{
  "webhook_url": "https://shop.example/webhooks/bnpl",
  "signing_secret": "<random-secret-32-to-512-chars>",
  "enabled": true
}
~~~

The current model has one webhook destination **per merchant**, not per order. The signing secret is required upon first registration. On subsequent updates omission retains the existing secret, while a supplied value rotates it. The allowed length is 32–512 characters. `enabled` also retains the existing value when omitted. The response contains `webhook_url` and `enabled`, not the secret.

The webhook destination is distinct from the browser Return Destination, and is not supplied as an arbitrary URL in Checkout Create.

### 3.5 Online Store and branch configuration

Every Partner Create refers to a registered `online_store_id` belonging to the authenticated merchant. BNPL resolves the online merchant branch, its policies, and eligibility. Do **not** send free-form `merchant_id` or `branch_id`.

`merchant_branch_profiles.offer_preparation_mode` belongs to server-side branch configuration and is snapshotted onto the session when created. It cannot be selected by the Merchant via a Checkout Create field.

## 4. Partner backend endpoint index

| Method | Path | Terminal permission |
| --- | --- | --- |
| `POST` | `/api/v1/partner/checkout-sessions` | `bnpl:checkout-sessions:create:merchant` |
| `GET` | `/api/v1/partner/checkout-sessions/{checkout_session_id}` | `bnpl:checkout-sessions:read:merchant` |
| `POST` | `/api/v1/partner/checkout-sessions/{checkout_session_id}/cancel` | `bnpl:checkout-sessions:create:merchant` |
| `PUT` | `/api/v1/partner/return-destinations/{destination_name}` | `bnpl:checkout-return-destinations:manage:merchant` |
| `PUT` | `/api/v1/partner/webhook-destination` | `bnpl:checkout-webhooks:manage:merchant` |

These are the **only** public Partner checkout integration endpoints in this route family. Browser-only token endpoints are listed in section 7.

## 5. Create a Checkout Session

~~~http
POST /api/v1/partner/checkout-sessions
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...
~~~

~~~json
{
  "online_store_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
  "merchant_order_id": "ORDER-1001",
  "idempotency_key": "ORDER-1001-ATTEMPT-1",
  "customer_reference": "CUSTOMER-12345",
  "customer_phone": "+989121234567",
  "return_destination": "default",
  "merchant_state": "CART-1001",
  "requested_amount_minor": 120000000,
  "currency": "IRR"
}
~~~

| Field | Contract |
| --- | --- |
| `online_store_id` | Required; UUID registered for this merchant |
| `merchant_order_id` | Required merchant order reference; max 256 characters |
| `idempotency_key` | Optional; max 256; defaults to `merchant_order_id` when omitted. Recommend supplying it explicitly |
| `customer_reference` | Optional, max 256; merchant-only correlation, **not** customer identity |
| `customer_phone` | Required, max 32; canonicalized/validated for merchant jurisdiction; Iranian example `09…` / `+989…` |
| `return_destination` | Required pre-registered, enabled symbolic destination name |
| `merchant_state` | Optional, max 256; non-sensitive opaque merchant correlation |
| `requested_amount_minor` | Required positive integer, at most MaxInt64, in the currency's minor units |
| `currency` | Required wire enum `IRR` / `EUR` / `USD`; merchant/branch/policy eligibility can restrict it |

Do not send `merchant_id`, `branch_id`, `identity_subject_id`, `offer_preparation_mode`, `webhook_url`, or an arbitrary return URL. Convert toman to **IRR** before request if your own system stores toman; check integer overflow.

**HTTP:** `201` new checkout; `200` idempotent replay.

~~~json
{
  "data": {
    "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "status": "open",
    "checkout_url": "https://checkout.example/checkout/cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "expires_at": "2026-11-04T13:00:00Z"
  },
  "meta": {
    "status": "success",
    "code": "CHECKOUT_SESSION_CREATED",
    "message": "Checkout session created.",
    "status_code": 201,
    "timestamp": "2026-11-04T12:30:00Z",
    "trace_id": "a1b2c3d4-e5f6-4890-abcd-ef1234567890",
    "request_id": "b2c3d4e5-f6a7-4901-bcde-f12345678901"
  }
}
~~~

This is a **wire-shape example**; actual timestamps, token, localized text and optional `offer` vary. `checkout_url` is opaque: hand it to the browser unmodified. `checkout_session_id` is the public `cs_` token used by Inquiry, Cancel, Return and lifecycle webhooks; it is **not** an internal UUID.

## 6. The only meaningful integration-mode difference: `on_create` / `on_start`

Both Damz and Digikala can use either mode depending on their **branch** configuration. It is **not** an internal/external merchant type.

| Behavior | `on_create` | `on_start` (default) |
| --- | --- | --- |
| Partner Create | Resolve customer phone through Customer Reference Service and prepare Decision/Offer | Create session/handoff; defer Offer preparation |
| Customer cannot be resolved / is ineligible | Could transition to `unavailable` during Create | Could transition to `unavailable` during first START |
| `data.offer` at Partner Create | **May be present** | **Absent** |
| Hosted GET before START | Can show prepared Offer | May have no Offer yet |
| Hosted START | Revalidate/read prepared Offer | Prepare Offer and return evaluation generation |
| SMS OTP | Separate `POST .../otp` only | Same |
| Confirmation | Shared Hosted Confirm/OTP workflow | Same |

### What an optional Partner `offer` contains

For `on_create`, current backend DTOs may return `data.offer`. For approved offers:

- `outcome=approved`, `requested_amount_minor`, `approved_financed_amount_minor`, `required_down_payment_amount_minor` and `currency`.
- `available_credit_minor`, `offer_expires_at`, `installment_count`, `payable_now_amount_minor`, `total_payable_amount_minor`.
- `installments[]` with `installment_order`, `due_at` and `total_amount_minor`.
- `checkout_action` with `type`, `payment_required` and `payment_amount_minor`.

For `denied` only `outcome` is included. `unavailable` status can occur instead of a usable Offer. **Data minimization:** `available_credit_minor` can currently be returned to the *merchant backend* in this mode; treat it as confidential and do not log or display it indiscriminately. The Hosted public-token response deliberately excludes account-level credit values.

An Offer is a financing proposal, **not** financial success or order fulfillment evidence.

## 7. BNPL-owned Hosted Checkout flow — for coordination only

Merchant **backend** must **not** call these methods. The merchant **frontend** navigates to `checkout_url`; BNPL-owned Hosted UI calls:

| Method | Path | Behavior |
| --- | --- | --- |
| `GET` | `/api/v1/checkout-sessions/{token}` | Read session, masked verification destination, optional safe Offer; no side effects |
| `POST` | `/api/v1/checkout-sessions/{token}/start` | Prepare/retrieve Offer; **does not send SMS** |
| `POST` | `/api/v1/checkout-sessions/{token}/otp` | Request SMS OTP for the Offer's `evaluation_generation` |
| `POST` | `/api/v1/checkout-sessions/{token}/confirm` | Verify OTP and confirm purchase |
| `GET` | `/api/v1/checkout-sessions/{token}/confirmation-status` | Read-only pending/uncertain Confirm status |
| `GET` | `/api/v1/checkout-sessions/{token}/return` | Browser `302` to registered merchant destination; UX only |
| `POST` | `/api/v1/checkout-sessions/{token}/cancel` | Customer token cancellation if state permits |

1. GET returns `status`, `merchant.display_name`, `amount_minor`, `currency`, `evaluation_generation`, optional `offer`, `verification.type=sms_otp`, masked `verification.destination` and `expires_at`. GET never issues OTP or calls FC.
2. START accepts optional `{"evaluation_generation":N}`. For an unprepared offer, empty body/`{}` is allowed. It prepares or reuses an Offer and returns the current generation. **It does not send OTP.**
3. OTP requires `{"evaluation_generation":N}`, returns `otp_sent`, `otp_expires_at?`, `otp_next_issue_at?` and `otp_availability?`. Current valid challenges can be replayed without another SMS. Respect cooldown/lockouts.
4. CONFIRM requires `{"evaluation_generation":N,"otp":"123456"}`. A changed or stale Offer may return `reconfirm_required=true` and a new generation. The user must see/reaccept refreshed terms and obtain a new OTP.
5. `payment_required=true` with a `redirect_url` indicates further customer payment navigation, **not payment success**. `confirming`/`processing` are pending states, not safe fulfillment.
6. On ambiguous Confirm, the read-only `confirmation-status` returns `confirmation_status`, `next_action` and retry hints; it does not create another payment or return a payment URL.

The `confirmation_status` values currently include `not_started`, `processing`, `payment_action_required`, `payment_processing`, `reconfirm_required`, `retry_available`, `completed`, `needs_attention`, `canceled`, `expired` and `unavailable`.

**Browser Return:** `GET /api/v1/checkout-sessions/{token}/return` redirects to the pre-registered merchant URL with `checkout_session_id` and optional opaque `state`. Query strings must not be interpreted as payment result or used to select a new destination. The merchant page should show pending and ask its backend to confirm the order.

## 8. Partner Inquiry: the authoritative Merchant API

~~~http
GET /api/v1/partner/checkout-sessions/{checkout_session_id}
X-Terminal-Id: ...
X-API-Key: ...
~~~

`200` `data`:

~~~json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "merchant_order_id": "ORDER-1001",
  "status": "completed",
  "amount_minor": 120000000,
  "currency": "IRR",
  "completed_at": "2026-11-04T13:03:00Z"
}
~~~

`completed_at` only appears upon completion. Inquiry is **read-only**: it may project a past-deadline `open` session as `expired` without mutating the stored record. Another merchant's session is reported as `404`, protecting ownership from enumeration.

Before order fulfillment, compare **all of**: `checkout_session_id`, `merchant_order_id`, `amount_minor`, `currency` and `status=completed` to the order persisted by your backend. Fulfillment must be idempotent.

| Session `status` | Meaning | Fulfill? |
| --- | --- | --- |
| `open` | Checkout may proceed; Offer may not yet be prepared | No |
| `confirming` | Customer confirmation / setup in progress | No |
| `processing` | Authoritative payment outcome pending | No |
| `completed` | Checkout authoritatively completed in BNPL | **Yes, after matching** |
| `canceled` | Customer or merchant canceled | No |
| `expired` | Checkout deadline passed | No |
| `unavailable` | No eligible/available financing | No |

There is no merchant-facing `failed` CheckoutSession state. A retryable payment failure may return the session to `open`; neither `confirming` nor `processing` is a generally cancellable state.

## 9. Cancel by Merchant

~~~http
POST /api/v1/partner/checkout-sessions/{checkout_session_id}/cancel
X-Terminal-Id: ...
X-API-Key: ...
~~~

On permitted cancellation, `200` and `data` includes `checkout_session_id`, `status=canceled` and `return_url`. Already-canceled replay is idempotent. Cancellation of `confirming` or `processing` may return `409` because the financial outcome is still uncertain; `completed`, `expired` and `unavailable` are not cancellable. On conflict, inquire rather than assuming the payment was canceled.

## 10. Lifecycle webhooks — signature, deduplication, retries

When a registered webhook destination is enabled, terminal CheckoutSession transitions are enqueued in the transactional outbox for HTTP delivery. Delivery is **at least once**; duplicates are expected. The merchant must always have Inquiry as a fallback.

| `event_type` | `data.status` |
| --- | --- |
| `bnpl.checkout.completed.v1` | `completed` |
| `bnpl.checkout.canceled.v1` | `canceled` |
| `bnpl.checkout.expired.v1` | `expired` |
| `bnpl.checkout.unavailable.v1` | `unavailable` |

Example event envelope:

~~~json
{
  "event_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef:bnpl.checkout.completed.v1:completed",
  "event_type": "bnpl.checkout.completed.v1",
  "event_version": "1",
  "producer": "bnpl-backend",
  "occurred_at": "2026-11-04T13:03:00Z",
  "data": {
    "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "merchant_order_id": "ORDER-1001",
    "merchant_id": "22222222-2222-4222-8222-222222222222",
    "status": "completed",
    "amount_minor": 120000000,
    "currency": "IRR",
    "completed_at": "2026-11-04T13:03:00Z",
    "merchant_state": "CART-1001"
  }
}
~~~

`completed_at` is present on completion, `reason` on cancellation where available (such as `customer_canceled` or `merchant_canceled`), and `merchant_state` when specified. No PSP/FC/PaymentAttempt identifiers or customer credit details are published in this merchant event.

`event_id` is stable per logical transition:

~~~text
{checkout_session_id}:{event_type}:{status}
~~~

Webhook HTTP headers:

| Header | Value |
| --- | --- |
| `Content-Type` | `application/json` |
| `User-Agent` | `bnpl-webhook/1` |
| `X-BNPL-Webhook-Id` | `event_id` |
| `X-BNPL-Webhook-Timestamp` | Unix timestamp seconds for this **delivery attempt** |
| `X-BNPL-Webhook-Signature` | `v1=<lowercase hex HMAC-SHA256>` |
| `X-BNPL-Webhook-Event` | event type, when provided in the outbox header |

The canonical MAC input is the exact UTF-8 byte sequence:

~~~text
v1\n<event-id>\n<unix-timestamp-decimal>\n<raw-http-body>
~~~

These are **actual LF bytes** between segments, followed immediately by the **raw HTTP request body**. Do not reformat/parse/re-serialize the JSON before computing the signature.

~~~python
import hashlib
import hmac

def verify_bnpl_webhook(secret: bytes, event_id: str,
                        timestamp: str, raw_body: bytes,
                        signature: str) -> bool:
    canonical = (f"v1\n{event_id}\n{timestamp}\n").encode("utf-8") + raw_body
    expected = "v1=" + hmac.new(secret, canonical, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, signature)
~~~

Also validate that the timestamp is numeric and inside a **locally agreed freshness window**, persist `event_id` for deduplication, match order/merchant/amount/currency/token, and apply the business transition atomically. Return `2xx` **only after durable processing**, including for duplicates already processed. Signature verification alone is not enough if order binding is missing.

BNPL loads the active webhook URL/secret when dispatching, so destination disablement or rotation may affect pending deliveries. Implement key-rotation and retry procedures.

## 11. Idempotency, errors, and recovery

- `idempotency_key` for **Partner Checkout Create** is a **JSON body field**, not the `Idempotency-Key` HTTP header used to Provision/Rotate the merchant terminal.
- If Create times out, **retry the same input with the same key**; do not generate a new checkout immediately.
- Reusing a key with different order/online store, amount, currency or customer phone is a conflict. BNPL compares canonicalized customer phone.
- After terminal `canceled`/`expired`/`unavailable`, a genuinely new checkout attempt needs a **new** key. An already `completed` order must not produce a duplicate checkout.
- A newly chosen key does not authorize concurrent active CheckoutSessions for the same `merchant_order_id`. Inquire into the previous session first.
- Treat `confirmed=success` browser query parameters as untrusted input; there is no such authoritative API contract.

### Error envelope

Errors are **not** nested inside `data/meta`:

~~~json
{
  "code": "VALIDATION_FAILED",
  "message": "Localized error description.",
  "trace_id": "a1b2c3d4-e5f6-4890-abcd-ef1234567890",
  "details": {
    "fields": ["customer_phone"],
    "violations": [
      {"code": "REQUIRED", "message": "Customer phone is required", "fields": ["customer_phone"]}
    ]
  }
}
~~~

`message` and `details.violations[].message` are localized and not stable machine conditions. Inspect HTTP status, canonical `code`, and violation fields instead. Success uses `{data,meta}` with stable `meta.code` and canonical `meta.status=success`.

| HTTP | Appropriate merchant action |
| --- | --- |
| `400` | Fix malformed input; changed operation should use new idempotency identity |
| `401` | Check terminal ID / API key and credential status |
| `403` | Check merchant ownership, branch eligibility, and stored terminal permissions |
| `404` on Inquiry | Unknown or foreign checkout session |
| `409` | Idempotency/state conflict; inquire before retrying |
| `422` in Hosted UI | OTP/validation issue; hosted client should follow cooldown/reconfirm instructions |
| `429` | Respect rate limiting and retry hints |
| `502` / `503` / `5xx` / timeout | Bounded backoff; replay exact same key, inquire when outcome is ambiguous |

### Partner QA scenarios

| Scenario | Expected behavior |
| --- | --- |
| Same Create body + idempotency key twice | `201` then `200`; same checkout token |
| Same key but changed amount / store / phone | Conflict, no second checkout |
| Branch `on_create` vs `on_start` | Create may have Offer only in `on_create`; START prepares it in deferred mode |
| Hosted GET | No OTP or Decision creation |
| Hosted START | Offer only; **no SMS** |
| Hosted OTP replay during valid challenge | Controlled/idempotent; no unlimited SMS |
| Hosted Confirm with invalid/stale generation | Denied or reconfirm; no unapproved purchase |
| Ambiguous Confirm | Read-only status poll, no automatic double confirmation |
| Webhook duplicate | One merchant fulfillment |
| Browser Return before webhook | Merchant shows pending and inquires |
| Merchant Cancel races with Confirm | Conflict possible; inquire and await authoritative outcome |
| Inquiry of another merchant's token | `404` |
| New key for a completed order | No new checkout |
| Checkout deadline passes in open state | `expired`, not fulfilled |

## 12. Integration example (Merchant Backend only)

~~~bash
# BNPL_API_BASE is an origin, e.g. https://api.example
curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/return-destinations/default" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"return_url":"https://shop.example/checkout/bnpl/return","enabled":true}'

curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/webhook-destination" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"webhook_url":"https://shop.example/webhooks/bnpl","signing_secret":"<random-32-to-512-char-secret>","enabled":true}'

curl -sS -X POST "$BNPL_API_BASE/api/v1/partner/checkout-sessions" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "online_store_id":"6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
    "merchant_order_id":"ORDER-1001",
    "idempotency_key":"ORDER-1001-ATTEMPT-1",
    "customer_phone":"+989121234567",
    "return_destination":"default",
    "requested_amount_minor":120000000,
    "currency":"IRR"
  }'

curl -sS "$BNPL_API_BASE/api/v1/partner/checkout-sessions/$CHECKOUT_SESSION_ID" \
  -H "X-Terminal-Id: $TERMINAL_ID" \
  -H "X-API-Key: $API_KEY"
~~~

The merchant frontend performs only:

~~~javascript
window.location.assign(checkout_url); // never reconstruct BNPL's URL
~~~

After return it requests the result from its own backend, not from a guessed status parameter.

## 13. Release checklist for Merchant teams

**Merchant Backend**
- [ ] Terminal ID and API key stored securely; HTTPS and required permissions in place.
- [ ] `online_store_id`, Return Destination and Webhook Destination provisioned.
- [ ] Customer phone comes from the actual order; `customer_reference` is not treated as identity.
- [ ] `on_create` and `on_start` are both supported without merchant-name conditionals.
- [ ] Order ID, stable create idempotency key, checkout token, amount and currency are durably stored.
- [ ] Only verified `completed` Inquiry or signed completed webhook can fulfill an order.
- [ ] Webhook HMAC verified against raw body, timestamp checked, event ID deduplicated.
- [ ] Missing or delayed webhook falls back to Inquiry; duplicate messages do not double-fulfill.
- [ ] HTTP timeouts, conflicts, cancellation during Confirm, and session expiration are tested.

**Merchant Frontend**
- [ ] Receives and navigates opaque `checkout_url`, without terminal secrets.
- [ ] Shows pending after Browser Return until merchant backend confirms completion.

**BNPL Hosted UI**
- [ ] Knows that START does not send OTP.
- [ ] Uses `evaluation_generation` when requesting OTP and confirming.
- [ ] Requires fresh consent/OTP after `reconfirm_required`.
- [ ] Polls `confirmation-status` on ambiguous Confirm without creating additional financial requests.

## 14. Environment handoff and implementation reference

Request from BNPL per environment: API Gateway origin, Hosted Checkout origin, Terminal ID/secure API Key issuance, registered `online_store_id` and branch mode, Return/Webhook destinations, safe testing OTP/checkout data, webhook freshness/retry window, and instructions for any `available_credit_minor` confidentiality restrictions.

Backend code reference map:

| Responsibility | Source |
| --- | --- |
| Auth groups, routes | `pkg/adapter/restful/gin/routes/register.go`; `checkoutsessionrs/resource.go` |
| Partner terminal authentication and permissions | `middleware/partner_terminal_auth.go`; `merchantterminaluc/authenticate.go`; `merchantterminal/permissions.go` |
| Terminal lifecycle | `merchantterminalrs/resource.go`; `merchantterminaluc/provision.go` |
| Create DTO / status / Offer | `checkoutsessionrs/create_partner.go`; `checkoutsessionrs/serdser.go`; `checkoutsessionrs/swagger_schema.go` |
| Offer timing and phone resolution | `checkoutsessionuc/create_partner.go`; `checkoutsessionuc/external_prepare.go`; `model/merchant_branch_profile.go` |
| Hosted GET/START/OTP/CONFIRM | `checkoutsessionrs/hosted_external.go`; `checkoutsessionrs/request_otp.go`; `checkoutsessionuc/external_start.go` |
| Inquiry / cancellation | `checkoutsessionuc/get_partner_status.go`; `checkoutsessionrs/cancel.go` |
| Webhook event and HMAC | `core/event/checkoutlifecycle/event.go`; `checkoutsessionuc/lifecycle_outbox.go`; `merchantwebhook/http/signature.go`; `merchantwebhook/http/dispatcher.go` |
| Error / success envelope | `pkg/adapter/restful/gin/serdser/error_response.go`; `success_response.go` |

Most paths shown without the root directory begin under `pkg/core/` or `pkg/adapter/` according to their package name.

**Documentation maintenance:** Keep [English](partner-checkout.en.md) and [Persian](partner-checkout.md) synchronized for every change to routes, DTOs, permissions, webhook envelope or state transitions. **Do not create separate Internal vs External Partner guides.**
