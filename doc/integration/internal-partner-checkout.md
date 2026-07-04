# Internal Partner ↔ BNPL Checkout Integration Guide

> این سند قرارداد integration برای **پارتنرهای داخلی / first-party** BNPL است (مثل Damz).  
> از نظر API، Damz **همان** مسیر Partner Create را مثل Digikala استفاده می‌کند؛ تفاوت محصولی اصلی معمولاً **`offer_preparation_mode`** روی شعبه فروش (`merchant_branch_profiles`) است.  
> جزئیات Financial Core، PSP، payment provider و سایر سرویس‌های داخلی BNPL بخشی از این قرارداد نیستند.
>
> قرارداد کامل فیلدها، idempotency، webhook، OTP و state machine: [external-partner-checkout.md](external-partner-checkout.md).

**مخاطب:** مهندسان Backend / Frontend / QA پارتنر داخلی · Cursor / AI Coding Agent در ریپوی پارتنر  
**مثال مرجع:** Damz — `POST /api/v1/partner/checkout-sessions` + `customer_phone`  
**Source of truth:** آخرین سورس BNPL (`checkoutsessionrs`, `checkoutsessionuc`, `checkoutlifecycle`, `otpcode`, permission catalog)

**Base path:** `/api/v1`  
**Partner runtime auth:** `X-Terminal-Id` + `X-API-Key` روی همه `/api/v1/partner/**` (بدون JWT Tenant/Context/Subject).  
**Customer auth:** هیچ JWT — فقط public token (`cs_…`) + OTP روی `/api/v1/checkout-sessions/:token/...`.

---

## Quick Start

1. یک‌بار (Partner terminal): Return Destination و Webhook را با همان credential ترمینال ثبت کنید.
2. برای هر سفارش: `POST /api/v1/partner/checkout-sessions` با **`customer_phone`** (نه `identity_subject_id`، نه `verified_phone_number`).
3. Frontend Damz فقط `checkout_url` را باز کند (UI checkout BNPL).
4. نتیجه authoritative فقط از Signed Webhook یا Partner Inquiry.
5. Browser Return را هرگز به‌عنوان پرداخت موفق تلقی نکنید.
6. Damz Backend هرگز `GET/POST .../checkout-sessions/{token}/...` را صدا نزند.

ثابت‌های مثال (public token):

```text
merchant_order_id      = DAMZ-ORDER-20261005-1001
idempotency_key        = damz-attempt-20261005-1001-a
checkout_session_id    = cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
online_store_id        = 6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c
customer_phone         = +989121234567
return_destination     = damz-web
merchant_state         = damz-cart-1001
requested_amount_minor = 120000000
currency               = IRR
```

---

## Damz vs Digikala (تفاوت واقعی)

| موضوع | همه merchants (شامل Damz) |
| --- | --- |
| Create | `POST /api/v1/partner/checkout-sessions` |
| Auth create/inquiry/cancel/setup | `X-Terminal-Id` + `X-API-Key` |
| Customer در body create | **`customer_phone`** (الزامی)؛ `customer_reference` اختیاری |
| ممنوع در create | `identity_subject_id`، Gateway JWT headers |
| پاسخ create | merchant-safe: token، `checkout_url`، status، expiry — **بدون offer/credit** |
| Customer flow | `/api/v1/checkout-sessions/{token}/...` بدون JWT |

تفاوت پیکربندی Damz (عملیاتی):

| `merchant_branch_profiles.offer_preparation_mode` | رفتار |
| --- | --- |
| `on_start` (پیش‌فرض) | ارزیابی/offer روی `POST .../start` بعد از Continue مشتری |
| `on_create` | آماده‌سازی offer قبل از موفقیت Partner Create (وقتی policy اجازه دهد) — مشتری در GET زودتر offer امن را می‌بیند |

Mode روی session snapshot می‌شود (`checkout_sessions.offer_preparation_mode`). مالکیت mode روی **شعبه** است، نه `merchant_profiles`.

`ExternalCustomerID` = `customer_id` سرویس Customer Reference در `customer_profiles.external_customer_id` — جدا از PK داخلی BNPL.

پرداخت checkout: Financial Core v2 با namespace رسمی `payment.v1` (`/payment.v1.CheckoutPaymentService/CreateCheckoutPayment` و `GetPaymentStatus`) بدون FinancialScope/tenant/context روی RPCهای payment جدید؛ parties خرید BNPL برابر `payer=customer` و `payee=platform` با `party_id` خالی پلتفرم و `expires_at` الزامی (قرارداد FC؛ بدون Bearer مالی مشتری).

---

## Architecture

```text
Damz Backend
    |
    | POST /partner/checkout-sessions (terminal auth, customer_phone)
    v
BNPL
    |
    | checkout_url (cs_… token)
    v
Damz Frontend → redirect browser
    v
BNPL Checkout UI
    |
    | GET/POST /checkout-sessions/{token}/... (no JWT)
    v
BNPL (contract + payment داخلی)
    |
    +---- Signed Webhook ----> Damz Backend
    +---- Inquiry <----------- Damz Backend
    v
Browser Return (UX only)
```

قواعد:

- Damz قرارداد را برای Customer تأیید نمی‌کند و `start`/`confirm` را از Backend صدا نمی‌زند.
- Damz Payment Provider یا Financial Core انتخاب/فراخوانی نمی‌کند.
- Signed Webhook و Partner Inquiry authoritative هستند.
- `checkout_url` opaque است — reconstruct نکنید.

---

## Authentication

### Partner runtime (Create, Inquiry, Cancel, Return/Webhook setup)

همان [external-partner-checkout.md — Authentication](external-partner-checkout.md#authentication--permissions):

```bash
-H "X-Terminal-Id: $TERMINAL_ID"
-H "X-API-Key: $API_KEY"
```

- JWT / `x-auth-*` روی `/partner/**` برای identity merchant استفاده **نمی‌شود**.
- Permissionها server-side روی terminal assign می‌شوند (`bnpl:checkout-sessions:create:merchant`, `read:merchant`, …).

### Merchant Panel (یک‌بار — credential ترمینال)

`POST /api/v1/merchant/terminal` و rotate/retire با **Gateway auth** (panel login) انجام می‌شود؛ plaintext `api_key` فقط show-once. Damz Backend credential ترمینال را در secret store نگه می‌دارد.

Damz **نباید** `x-auth-subject-id` / `x-auth-permissions` را برای Partner routes بسازد.

---

## API Inventory (Damz Backend)

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `PUT` | `/api/v1/partner/return-destinations/{name}` | Terminal | ثبت return URL |
| `PUT` | `/api/v1/partner/webhook-destination` | Terminal | ثبت webhook |
| `POST` | `/api/v1/partner/checkout-sessions` | Terminal | **Create** — `customer_phone` |
| `GET` | `/api/v1/partner/checkout-sessions/{token}` | Terminal | Inquiry |
| `POST` | `/api/v1/partner/checkout-sessions/{token}/cancel` | Terminal | Cancel merchant |

`{token}` = public `cs_…` از create (نه UUID داخلی).

### Customer token routes — فقط UI BNPL (Damz صدا نزند)

| Method | Path |
| --- | --- |
| `GET` | `/api/v1/checkout-sessions/{token}` |
| `POST` | `/api/v1/checkout-sessions/{token}/start` |
| `POST` | `/api/v1/checkout-sessions/{token}/confirm` |
| `POST` | `/api/v1/checkout-sessions/{token}/cancel` |
| `GET` | `/api/v1/checkout-sessions/{token}/return` |

بدون JWT؛ OTP + token عمومی.

> **حذف شده از قرارداد:** `POST /api/v1/internal/checkout-sessions`، `identity_subject_id` در create، `bnpl:decisions:create:on-behalf` برای checkout merchant.

---

## Create Checkout (همان API Digikala)

جزئیات فیلد، idempotency، IRR/Toman، خطاها: [Create Checkout](external-partner-checkout.md#create-checkout-primary-partner-api).

خلاصه برای Damz:

```http
POST /api/v1/partner/checkout-sessions
X-Terminal-Id: …
X-API-Key: …
```

```json
{
  "online_store_id": "6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
  "merchant_order_id": "DAMZ-ORDER-20261005-1001",
  "idempotency_key": "damz-attempt-20261005-1001-a",
  "customer_phone": "+989121234567",
  "customer_reference": "damz-user-42",
  "return_destination": "damz-web",
  "merchant_state": "damz-cart-1001",
  "requested_amount_minor": 120000000,
  "currency": "IRR"
}
```

پاسخ merchant-safe (بدون `offer` / `evaluation_generation`):

```json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "status": "open",
  "checkout_url": "https://bnpl.example/checkout/cs_0123456789abcdef…",
  "expires_at": "2026-10-06T12:30:00Z"
}
```

Damz Backend **نباید** از create انتظار preview اعتبار/اقساط داشته باشد — حتی با branch `on_create`. preview فقط روی UI مشتری (GET token) است.

---

## Damz Frontend

- CTA مثل **Continue with BNPL** — نه Confirm Contract / Pay از طرف Damz.
- `window.location.assign(checkout_url)` — بدون reconstruct.
- اگر UX محصولی «پیش‌نمایش BNPL» می‌خواهد، از **داده داخلی Damz** یا copy عمومی استفاده کنید؛ از Partner create offer نمی‌گیرید.
- بعد از return: polling Backend Damz؛ موفقیت از URL حدس زده نشود.

---

## Customer token flow (فقط برای درک — Damz Backend دخالت ندارد)

هم‌تراز [Customer token OTP flow](external-partner-checkout.md#customer-token-otp-flow-informational--merchant-backend-does-not-call-these):

```text
GET {token}     → projection read-only (offer when prepared per mode)
POST .../start  → OTP (و prepare در on_start)
POST .../confirm→ OTP verify → contract/payment
GET .../return  → redirect به return_url ثبت‌شده (UX)
```

Damz فقط webhook/inquiry را برای `completed` می‌بیند.

---

## Inquiry, Cancel, Webhook, Browser Return

همان الگوریتم‌ها و state machine در [external-partner-checkout.md](external-partner-checkout.md):

- Inquiry: `GET /api/v1/partner/checkout-sessions/{token}` — فقط `status=completed` + match amount/order/currency → fulfill.
- Cancel: `POST .../cancel` — idempotency و conflict rules همان external.
- Webhook: `bnpl.checkout.completed.v1` و siblings — HMAC روی raw body، dedupe `event_id`.
- Browser return: **غیر authoritative**.

---

## Retry & Idempotency

| Operation | Rule |
| --- | --- |
| Create timeout | همان `idempotency_key` |
| Attempt جدید بعد از terminal state | `idempotency_key` جدید |
| Inquiry | Safe retry |
| Webhook duplicate | Dedupe on `event_id` → 2xx |

---

## Security (Damz)

- Terminal id + API key فقط server-side.
- Partner APIs فقط از Damz Backend.
- `customer_phone` را از کانال trusted Damz بگیرید؛ BNPL full phone را روی Partner API برنمی‌گرداند.
- Webhook verify + amount/order/currency match قبل از complete.
- به شناسه‌های Financial Core / PSP وابسته نشوید.

---

## Implementation checklists

### Damz Backend

```text
[ ] Partner client: X-Terminal-Id + X-API-Key on every /partner/** call
[ ] Panel/onboarding: provision terminal (gateway) → store api_key once
[ ] UpsertReturnDestination + UpsertWebhookDestination (terminal auth)
[ ] Create with customer_phone + idempotency persistence
[ ] Inquiry + Cancel
[ ] Webhook: raw-body HMAC v1, event_id dedupe, completed gate
[ ] Persist checkout_session_id (cs_ token), amount, currency, status
[ ] Never call /checkout-sessions/{token}/start|confirm|cancel|return
[ ] Never use POST /internal/checkout-sessions or identity_subject_id on create
```

### Damz Frontend

```text
[ ] Redirect to checkout_url only
[ ] Return page → poll Backend (not BNPL Partner API)
[ ] No merchant credentials in browser
```

---

## Cursor instructions (Damz repo)

Implement BNPL using **only** `/api/v1/partner/...` from Backend with terminal credentials and **`customer_phone` on create**. Copy webhook/idempotency patterns from [external-partner-checkout.md — Cursor Implementation Instructions](external-partner-checkout.md#cursor-implementation-instructions-for-external-merchant) with these Damz-specific notes:

- Branch **`on_create` vs `on_start`** affects customer UI timing only — Partner create response shape unchanged.
- Do **not** implement internal checkout or gateway `x-auth-*` on Partner calls.
- Do **not** call customer token routes from Backend.

---

## Related

- [api.md](../api.md) — route table and partner terminal authentication
- [configuration.md](../configuration.md) — OTP template, checkout public origin env keys
