# راهنمای اتصال Frontendهای BNPL به Backend (فارسی)

> **مخاطب:** تیم‌های Frontend مشتری، پنل Merchant، Hosted Checkout و QA خود BNPL.  
> **زبان:** فارسی · [English](bnpl-frontend.en.md)  
> **آخرین به‌روزرسانی قرارداد wire:** ۲۰۲۶-۱۱-۱۰ — پیشوند `external_` برای شناسه‌های سرویس بیرونی. خلاصهٔ تغییرات: [changelog](bnpl-frontend-external-ids-changelog.fa.md).  
> **پیشوند API:** `/api/v1`؛ نام سرویس نباید به مسیر اضافه شود.  
> **مستند Merchant Backend:** [فارسی](../integration/partner-checkout.md) / [English](../integration/partner-checkout.en.md).

هدف این مستند این است که تیم Frontend بدون حدس‌زدن دربارهٔ احراز هویت، شکل Request/Response، زمان قطعی‌شدن پرداخت، State Machine و خطاها بتواند با **پیاده‌سازی موجود** کار کند. این Backend سرویس Login/Refresh ندارد؛ آن بخش بر عهدهٔ Identity/Gateway است.

## ۱. سه سطح دسترسی کاملاً متفاوت داریم

| نوع Frontend | احراز هویت | مسیرهای مجاز |
| --- | --- | --- |
| **پنل مشتری** | Login از Identity و Token معتبر در Gateway؛ Gateway هویت و Permission را به BNPL تزریق می‌کند | اعتبار، Decision، Contract، اقساط، Repayment و Payment History |
| **پنل Merchant** | همان Gateway، ولی با هویت Merchant و Permissionهای Merchant | Merchant Profile، Branch، Sales، مدیریت Terminal و API Key |
| **Hosted Checkout خود BNPL** | توکن عمومی Checkout با پیشوند `cs_` و OTP برای Confirm؛ بدون JWT Gateway | فقط `/api/v1/checkout-sessions/{token}` و زیرمسیرهای آن |
| **Backend پارتنر** | `X-Terminal-Id` + `X-API-Key` | فقط `/api/v1/partner/**`؛ این سطح مربوط به Browser نیست |

**هیچ‌وقت این مرزها را مخلوط نکنید.** Frontend نباید هدرهای `X-Auth-*` را خودش بسازد، Terminal API Key را در Browser قرار دهد، یا روی Hosted Checkout از مشتری JWT پنل را بخواهد. BNPL هویت را از Gateway می‌گیرد؛ Gateway باید توکن را تأیید، هدرهای جعلی را حذف و مقادیر معتبر را تزریق کند.

## ۲. تنظیم Client، آدرس‌ها و Headerها

برای هر محیط، مقدار Origin Gateway، آدرس Identity Service و Origin/Route رابط Hosted Checkout باید توسط تیم Deployment مشخص شود. خود مخزن، Host نهایی را تضمین نمی‌کند. درخواست‌های Backend از `/api/v1` شروع می‌شوند؛ نه `/bnpl/api/v1`.

### درخواست پنل احرازشده

~~~http
GET /api/v1/customer-credit HTTP/1.1
Host: api.example.test
Authorization: Bearer <identity-access-token>
Accept: application/json
Accept-Language: fa-IR
X-Request-Id: <optional-uuid>
X-Correlation-Id: <optional-id>
~~~

این‌ها را **از Browser ارسال نکنید**: `X-Auth-Subject-Id`، `X-Auth-Tenant-Id`، `X-Auth-Context-Id`، `X-Auth-Permissions`. Envoy آن‌ها را بعد از احراز هویت تزریق می‌کند و `middleware.AuthContext.Attach` در BNPL می‌خواند. CORS باید Origin واقعی Frontend را در Gateway مجاز کرده باشد؛ CORS جایگزین احراز هویت نیست.

### درخواست Hosted Checkout

~~~http
GET /api/v1/checkout-sessions/cs_<64-hex-chars>
Accept: application/json
Accept-Language: fa-IR
~~~

نه `Authorization` لازم است، نه Terminal Key و نه `X-Auth-*`. توکن `cs_` یک Locator حدس‌نزدنی است و نباید در Analytics، Log، Referrer، URL شخص ثالث یا LocalStorage پخش شود. Backend روی این مسیرها `Cache-Control: no-store` و `Referrer-Policy: no-referrer` اعمال می‌کند. هر سه سطح باید از HTTPS استفاده کنند.

هدرهای `X-Trace-Id`، `X-Request-Id` و `X-Correlation-Id` صرفاً برای پیگیری خطا هستند؛ آن‌ها مجوز دسترسی نیستند.

## ۳. قرارداد سراسری پاسخ‌ها

### موفقیت

ساختار عادی:

~~~json
{
  "data": {
    "status": "active"
  },
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

این فقط **نمونهٔ ساختار** است؛ `data` و `meta.code` واقعی برای هر Endpoint فرق می‌کند. `meta.correlation_id` و `meta.pagination` اختیاری‌اند. `meta.status` همیشه `success` است و `meta.status_code` کد HTTP را تکرار می‌کند. `meta.message` ممکن است ترجمه شود؛ برای Branch منطقی از متن پیام استفاده نکنید.

### خطا

خطا **داخل `data/meta` نیست**:

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

فیلد `details` و بخش‌های داخلی آن اختیاری‌اند. خطاهای رایج: `400` ورودی نامعتبر، `401` عدم احراز هویت، `403` فقدان مجوز، `404` عدم وجود یا عدم مالکیت، `409` Conflict، `422` رد درخواست یا Validation، `429` محدودیت تعداد درخواست، `502` خطای Provider، `503` عدم دسترس‌پذیری و `500` خطای داخلی.

**قواعد Client:**
- HTTP Status را بررسی کنید و Success/Error را با دو مدل جدا Parse کنید.
- `code` و `meta.code` ماشین‌خوان هستند؛ `message` برای نمایش است.
- برای POST مالی، یک `Idempotency-Key` ثابت به‌ازای هر نیت کاربر ساخته و بعد از Timeout **همان Key** را استفاده کنید. برای نیت تازه Key تازه بسازید.
- Retry خودکار و بی‌وقفهٔ POST مالی ممنوع است؛ بعد از نتیجهٔ مبهم Read مجاز و معتبر انجام دهید.
- لغو `fetch` به معنای Rollback شدن عملیات Backend نیست.
- همهٔ `*_minor`ها اعداد صحیح دقیق‌اند. از Floating Point و تبدیل ضمنی ریال/تومان استفاده نکنید. برای اعداد بزرگ و JavaScript دقت عددی را بررسی کنید.
- زمان‌ها `RFC3339` هستند. تاریخ سررسید را خودتان محاسبه نکنید؛ مقادیر Backend را نمایش دهید.

## ۴. نقشهٔ APIهای پنل مشتری

همهٔ مسیرها Gateway-authenticated هستند. نام Permission برای طراحی Route Guard مفید است اما مجوز واقعی در Backend بررسی می‌شود.

| Method | Path | Permission | HTTP |
| --- | --- | --- | --- |
| `GET` | `/api/v1/customer-credit` | `bnpl:credit-limits:read:self` | `200` |
| `POST` | `/api/v1/customer-credit/initiate` | `bnpl:credit-limits:create:self` | `201` یا `200` |
| `POST` | `/api/v1/checkout-decisions` | `bnpl:decisions:create:self` | `201` یا `200` |
| `POST` | `/api/v1/checkout-decisions/{decision_id}/contracts` | `bnpl:contracts:create:self` | `201` یا `200` |
| `POST` | `/api/v1/contracts/{contract_id}/cancel` | `bnpl:contracts:cancel:self` | `202` یا `200` |
| `GET` | `/api/v1/contracts` | `bnpl:contracts:read:self` | `200` |
| `GET` | `/api/v1/contracts/{contract_id}/installments` | `bnpl:installments:read:self` | `200` |
| `GET` | `/api/v1/installments` | `bnpl:installments:read:self` | `200` |
| `POST` | `/api/v1/contracts/{contract_id}/installments/{installment_id}/pay` | `bnpl:payments:start:self` | `202` یا `200` |
| `GET` | `/api/v1/payments` | `bnpl:payments:read:self` | `200` |

`customer_id`، `tenant_id` و `context_id` را در Body یا Query نسازید؛ Backend از Subject و Context معتبر Gateway استفاده می‌کند.

## ۵. اعتبار مشتری و شروع استفاده

### `GET /customer-credit`

`data.status` یکی از `active`، `not_assigned` و `expired` است. `credit_usage` یک Object با `status` و `reason?` است و با وضعیت Credit Limit فرق دارد.

- `active`: `available_amount_minor`، `currency` و در صورت وجود `expires_at` را نمایش دهید. ممکن است Credit Usage استفاده از اعتبار را منع کند.
- `not_assigned`: مشتری هنوز Limit ندارد؛ آن را موجودی صفر نشان ندهید. `meta.code=CUSTOMER_CREDIT_NOT_ASSIGNED` و CTA شروع ارزیابی مناسب است.
- `expired`: Limit قبلی قابل‌استفاده نیست؛ `meta.code=CUSTOMER_CREDIT_EXPIRED`، `expires_at` اختیاری و CTA بررسی مجدد.

### `POST /customer-credit/initiate`

هیچ Body ندارد. پاسخ `data.result`، `credit_limit_decision.outcome` با `limit_set` یا `denied`، `reason_codes[]` و `credit_limit?` دارد. `credit_limit` شامل `amount_minor`، `currency`، `status`، `effective_from` و `expires_at?` است.

`201` یعنی Limit جدید ایجاد شده؛ `200` می‌تواند «قبلاً وجود داشته» یا «رد شده» باشد. **بر اساس `outcome` تصمیم بگیرید، نه صرف HTTP 200.** سپس Credit Summary را Refresh کنید.

## ۶. Checkout مستقیم مشتری: Decision، Offer و Contract

این فلو متعلق به **مشتری احرازشده** است؛ Hosted Checkout پارتنر در بخش ۱۰ توضیح داده می‌شود.

### ساخت Decision

~~~http
POST /api/v1/checkout-decisions
Authorization: Bearer <token>
Idempotency-Key: checkout-intent-1001
Content-Type: application/json
~~~

برای POS:

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

برای خرید Online:

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

`purchase_context.type` الزامی است. نوع `pos` باید `external_pos_gps_id` داشته باشد و فیلدهای Online ممنوع‌اند؛ نوع `online` باید `external_online_store_id` داشته باشد و `external_pos_gps_id` ممنوع است. `merchant_order_id` برای Online اختیاری و حداکثر ۲۵۶ کاراکتر است. این شناسه‌ها متعلق به سیستم بیرونی (پذیرنده/فروشگاه) هستند؛ با PK داخلی BNPL اشتباه گرفته نشوند. Currency در DTO می‌تواند `IRR`، `EUR` یا `USD` باشد ولی پذیرش بیزینسی ممکن است محدودتر باشد. در خطای `VALIDATION_FAILED`، `details.fields` با همان کلیدهای JSON (snake_case) برمی‌گردد.

**Response واقعی** در `data` دارای `result`، `decision` (شامل `id`، `checkout_key`، `outcome`، `reason_codes[]`)، `purchase_context`، `requested_amount_minor`، `approved_financed_amount_minor`، `upfront_amount_minor`، `currency`، `available_credit_minor`، `expires_at?`، `checkout_action?` و `installment_offer?` است.

`installment_offer` وقتی موجود است شامل `installment_count`، `payable_now_amount_minor`، `total_payable_amount_minor`، `currency`، `expires_at` و `installments[]` می‌شود. ردیف هر قسط: `id`، `installment_order`، `status`، `due_at`، `principal_amount_minor`، `upfront_amount_minor`، `base_fee_amount_minor`، `total_amount_minor`.

**دو نکتهٔ بسیار مهم:**
1. Outcome روی HTTP فقط `approved` یا `denied` است؛ حالت Domain `conditional` / تأمین مالی جزئی، در Response نیز `approved` نشان داده می‌شود. `upfront_amount_minor` را نمایش دهید و Enum خیالی `partial_approved` نسازید.
2. دکمهٔ ادامه باید از `checkout_action.type` با `confirm_and_pay` یا `confirm_purchase` پیروی کند، نه از نوع فروشگاه یا تعداد اقساط. اگر Decision رد شده باشد Offer وجود ندارد. این مرحله اعتبار را رزرو/کم نمی‌کند.

### ایجاد Contract

~~~http
POST /api/v1/checkout-decisions/{decision_id}/contracts
Authorization: Bearer <token>
~~~

Body پرداخت/Provider نفرستید. عملیات بر اساس Decision تکرارپذیر است: `201` جدید و `200` Contract قبلی. پاسخ `data`:

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

`redirect_url` و `expires_at` اختیاری و وابسته به `payment_required` هستند. اگر URL وجود دارد، Navigation سطح اول مرورگر را به همان URL انجام دهید؛ نه بازسازی لینک و نه استخراج شناسهٔ Provider.

**ایجاد Contract یا Browser Return به معنی پرداخت موفق/Active شدن Contract نیست.** فقط نتیجهٔ معتبر FC که Backend اعمال کرده معتبر است. اگر `payment_required=false` (مثل Merchant Funding)، بدون Redirect ادامه دهید اما باز هم وضعیت نهایی معتبر Backend را بررسی کنید.

## ۷. داشبورد اقساط، تاریخچه و بازپرداخت

### خواندن داده‌ها

- `GET /contracts` فقط Contractهای `active` و `canceling` را بازمی‌گرداند، نه تاریخچهٔ همهٔ Contractها. `data` آرایه‌ای از `id`، `merchant_profile_id`، `merchant_branch_profile_id?`، `requested_amount_minor`، `currency`، `installment_count`، `status` و `created_at` است.
- `GET /contracts/{id}/installments` یک **Object** برمی‌گرداند: `contract_id`، `requested_amount_minor`، `currency`، `status`، `created_at`، `installments[]` و `next_payable_installment?`. هر قسط `id`، `installment_order`، `total_amount_minor`، `currency`، `due_at`، `status` و `paid_at?` دارد. نوبت پرداخت را از `next_payable_installment` بخوانید.
- `GET /installments` یک **Object** با `data.installments[]` و `data.repayment` است؛ نه آرایهٔ مستقیم. `repayment` خلاصهٔ **کل مشتری** را دارد، مستقل از صفحه: `due_installments[]`، `due_count`، `unpaid_count`، `next_payable?` و `has_pending_payment`. `next_payable` ممکن است `payable_amount_known`، `outstanding_amount_minor`، `payment_block_reason`، `has_pending_payment` و `payable_amount_minor` داشته باشد.
- `GET /payments` آرایهٔ PaymentAttemptهای **نهایی‌شده** را نشان می‌دهد، نه پرداخت‌های درحال‌انجام. وضعیت‌های قابل‌نمایش `succeeded`، `failed`، `canceled` و `purpose` برابر `first_installment` یا `installment` است. هر رکورد `id`، `contract_id`، `installment_id`، `amount_minor`، `currency`، `status`، `purpose`، `created_at` و `completed_at?` دارد.

**نبود یک ردیف در `/payments` به معنی نبود پرداخت در جریان نیست.** برای فعال/غیرفعال کردن دکمهٔ پرداخت `has_pending_payment` و `payment_block_reason` را رعایت کنید.

### پرداخت قسط

~~~http
POST /api/v1/contracts/{contract_id}/installments/{installment_id}/pay
Authorization: Bearer <token>
Idempotency-Key: repay-contract-123-installment-2-attempt-1
Content-Type: application/json

{"channel":"gateway"}
~~~

`channel` یکی از `gateway`، `bank_transfer` و `wallet` است، ولی قابل‌استفاده‌بودن کانال به Policy بستگی دارد. Backend ترتیب اقساط را enforce می‌کند: فقط اولین قسط پرداخت‌نشده طبق برنامه قابل پرداخت است. `202` درخواست جدید و `200` Replay است.

پاسخ `data.result` و `data.payment_attempt` دارد. `payment_attempt` شامل `id`، `installment_id`، `amount_minor`، `currency`، `state`، `channel` و `redirect_url?` و `expires_at?` است.

وقتی URL معتبر وجود دارد کاربر را به آن Navigate کنید. **اگر وجود ندارد URL نسازید و پرداخت را موفق حساب نکنید.** وضعیت `created`، `submitted` یا سایر حالت‌های عملیاتی باید Pending/Recoverable نمایش داده شوند.

Backend برای یک PaymentAttempt حداکثر **سه فراخوانی ساخت Session در FC به‌صورت Customer-triggered** مجاز می‌داند، با Idempotency Identity یکسان. شکست‌های اول و دوم می‌توانند Attempt را `created` نگه دارند و شکست سوم به لغو محلی می‌رسد. Frontend نباید Retry Loop خودکار بسازد و بودجه را تمام کند. بعد از وضعیت Terminal، پیش از نیت تازه و Key جدید، وضعیت قسط و سوابق را Refresh کنید.

### لغو Contract

~~~http
POST /api/v1/contracts/{contract_id}/cancel
Authorization: Bearer <token>
Idempotency-Key: cancel-contract-123-v1
Content-Type: application/json

{"reason":"customer_requested"}
~~~

`202` یعنی شروع لغو و `data.status=canceling`؛ `200` Replay همان لغو است. لغو قطعی با نتیجهٔ FC انجام می‌شود؛ `canceling` را «لغوشده» نمایش ندهید.

## ۸. Pagination

| API | Mode | پارامترها و ترتیب |
| --- | --- | --- |
| `GET /contracts` | Keyset | `limit` (پیش‌فرض ۲۰، حداکثر ۱۰۰)، `after_id` + `after_created_at`؛ نزولی |
| `GET /payments` | Keyset | همان؛ نزولی |
| `GET /installments` | Keyset | `limit`، `after_id` + `after_due_at`؛ صعودی |
| `GET /merchant/branches` | `pagination=keyset` یا `pagination=page` | Keyset: `limit` و `after_id` + `after_created_at`؛ Page: `page` و `limit` |
| `GET /merchant/sales` | همان | همان |

اطلاعات صفحه‌بندی در **`meta.pagination`** است، نه لزوماً در `data`. برای Keyset از `has_next` و `keyset.next_after_id` به همراه `next_after_created_at` یا `next_after_due_at` استفاده کنید. Cursor را از ردیف آخر حدس نزنید. در Page mode از `pagination.page.current_page`، `total_pages`، `total_items` و `page_size` استفاده کنید. فهرست خالی با HTTP 200 طبیعی است.

## ۹. پنل Merchant

این‌ها Routeهای Gateway-authenticated هستند، نه Partner API دارای Terminal Key.

| Method | Path | Permission |
| --- | --- | --- |
| `GET` | `/api/v1/merchant` | `bnpl:merchant-profile:read:merchant` |
| `GET` | `/api/v1/merchant/branches` | `bnpl:branches:read:merchant` |
| `GET` | `/api/v1/merchant/sales` | `bnpl:contracts:read:merchant` |
| `GET` | `/api/v1/merchant/terminal` | `bnpl:merchant-terminals:manage:merchant` |
| `POST` | `/api/v1/merchant/terminal` | همان؛ `Idempotency-Key` الزامی |
| `POST` | `/api/v1/merchant/terminal/credentials/rotate` | همان؛ `Idempotency-Key` الزامی؛ Body اختیاری `{"grace_period_seconds":3600}` |
| `POST` | `/api/v1/merchant/terminal/credentials/{credential_id}/retire` | همان |

`GET /merchant` اطلاعات پروفایل از جمله `external_merchant_id`، `display_name`، `status`، `currencies`، `funding_models`، `settlement_models` و محدودیت‌ها را دارد. Branchها `external_branch_id`، نوع `physical` یا `online` و وضعیت مستقل دارند. Sales فقط Contractهای `active` هستند.

هر Merchant فعلاً حداکثر یک Terminal دارد. `GET /merchant/terminal` همیشه لیست `data[]` برمی‌گرداند و می‌تواند خالی باشد؛ **کلید خام را برنمی‌گرداند**.

Provision/Rotate فقط هنگام صدور (و Replay مجاز همان درخواست در بازهٔ مربوطه) `api_key` را تحویل می‌دهد. آن را با UI نمایشی یک‌باره و Copy امن نشان دهید؛ در Redux، LocalStorage، Analytics، Log یا Crash Report ذخیره نکنید. `terminal_id` ممکن است بزرگ باشد؛ برای جلوگیری از افت دقت در JavaScript از String در State استفاده کنید.

**نکته:** کلید صادرشده باید به Backend پارتنر منتقل و آنجا نگهداری شود، نه اینکه پنل مرورگری با آن `/partner/**` را صدا بزند. برای Integration Merchant به مستند [Partner](../integration/partner-checkout.md) مراجعه کنید.

## ۱۰. Frontend میزبانی‌شدهٔ BNPL (Hosted Checkout)

Partner Backend قبلاً Checkout را ساخته و `checkout_url` را به مرورگر تحویل داده است. UI میزبانی‌شده از توکن `cs_` داخل URL خودش استفاده می‌کند؛ مقدار Route آن بسته به استقرار تعیین می‌شود، اما Backend APIها ثابت‌اند:

| Method | Path | کاربرد |
| --- | --- | --- |
| `GET` | `/api/v1/checkout-sessions/{token}` | وضعیت اولیه و Offer آماده؛ بدون OTP |
| `POST` | `.../{token}/start` | تهیه/خواندن Offer؛ **بدون ارسال SMS** |
| `POST` | `.../{token}/otp` | صدور OTP برای Generation مشخص |
| `POST` | `.../{token}/confirm` | اعتبارسنجی OTP و تأیید خرید |
| `GET` | `.../{token}/confirmation-status` | Poll فقط‌خواندنی نتیجهٔ مبهم Confirm |
| `GET` | `.../{token}/return` | هدایت `302` به Return ثبت‌شدهٔ Merchant |
| `POST` | `.../{token}/cancel` | لغو در وضعیت مجاز |

### مرحلهٔ اول: GET

پاسخ `data` دارای `checkout_session_id`، `status`، `merchant.display_name`، `amount_minor`، `currency`، `evaluation_generation`، `offer?`، `verification.type=sms_otp`، `verification.destination` ماسک‌شده و `expires_at` است.

GET هویت مشتری Resolve نمی‌کند، Decision نمی‌سازد، OTP ارسال نمی‌کند و FC را صدا نمی‌زند.

Offer ممکن است از قبل وجود داشته باشد، چون شعبه روی `on_create` تنظیم شده است؛ در `on_start` ممکن است هنوز Offer آماده نباشد. این Mode توسط Backend و Branch تعیین می‌شود، نه نام Damz/دیجی‌کالا.

Offer میزبانی‌شده `outcome` با `approved` یا `denied`، مبالغ تأمین مالی و پیش‌پرداخت، `currency`، `offer_expires_at`، `installment_count`، `payable_now_amount_minor`، `total_payable_amount_minor`، `installments[]` و `checkout_action` دارد. **این DTO عمداً `available_credit_minor` و شناسهٔ داخلی مشتری را حذف کرده است.**

### مرحلهٔ دوم: START

~~~http
POST /api/v1/checkout-sessions/{token}/start
Content-Type: application/json

{}
~~~

برای اولین START در `on_start`، Body خالی یا `{}` درست است. وقتی GET قبلاً Offer نشان داده، `{"evaluation_generation":N}` بفرستید. پاسخ شامل `checkout_session_id`، `status`، `evaluation_generation` و `offer` در صورت آماده بودن است. **START هیچ OTP یا SMS صادر نمی‌کند.**

اگر Offer `denied` باشد یا Session `unavailable`، OTP/Confirm را ادامه ندهید.

### مرحلهٔ سوم: OTP

~~~http
POST /api/v1/checkout-sessions/{token}/otp
Content-Type: application/json

{"evaluation_generation":1}
~~~

پاسخ: `checkout_session_id`، `status`، `evaluation_generation`، `otp_sent`، `otp_expires_at?`، `otp_next_issue_at?` و `otp_availability?`. در Challenge معتبر ممکن است SMS جدید ارسال نشود. Cooldown/Lockout و Rate Limit را رعایت کنید. شمارهٔ ماسک‌شده قابل تغییر از این API نیست.

### مرحلهٔ چهارم: CONFIRM

~~~http
POST /api/v1/checkout-sessions/{token}/confirm
Content-Type: application/json

{"evaluation_generation":1,"otp":"123456"}
~~~

تأیید با OTP شش‌رقمی و Generation معتبر انجام می‌شود. `data` شامل `checkout_session_id`، `status`، `reconfirm_required`، `evaluation_generation`، `offer?`، `contract?` (با `id` و `status`)، `payment_required`، `redirect_url?` و `expires_at?` است.

- `reconfirm_required=true`: شرایط Offer تغییر کرده؛ Offer تازه را نشان دهید، موافقت دوباره بگیرید و OTP نسل جدید دریافت کنید. OTP قبلی قابل استفاده نیست.
- `payment_required=true` همراه URL: Redirect به لینک Opaque. URL مدرک پرداخت موفق نیست.
- `status=confirming/processing`: Pending نشان دهید و درخواست تأیید موازی نسازید.
- `payment_required=false`: بدون لینک پرداخت ادامه دهید، اما فقط وقتی وضعیت `completed` شد موفقیت نشان دهید.
- Timeout/قطع ارتباط Confirm: به‌جای ارسال مجدد کورکورانه، `confirmation-status` را Poll کنید.

### مرحلهٔ پنجم: Recovery با Confirmation Status

`GET /checkout-sessions/{token}/confirmation-status` یک پاسخ خواندنی با `status`، `confirmation_status`، `next_action`، `retry_after_seconds?`، `otp_next_issue_at?` و `otp_availability?` می‌دهد.

`confirmation_status` ممکن است یکی از این‌ها باشد:

`not_started`، `processing`، `payment_action_required`، `payment_processing`، `reconfirm_required`، `retry_available`، `completed`، `needs_attention`، `canceled`، `expired`، `unavailable`.

Poll هیچ Saga، PaymentAttempt، Session پرداخت یا URL جدیدی ایجاد نمی‌کند. برای اقدام بعدی از `next_action` و `retry_after_seconds` استفاده کنید و در صورت لزوم به START/OTP/CONFIRM برگردید.

### مرحلهٔ ششم: RETURN

`GET /checkout-sessions/{token}/return` یک **Navigation** است و با HTTP `302` به URL ثبت‌شدهٔ Merchant می‌رود. مقصد را از Query String انتخاب نمی‌کند. پارامتر `checkout_session_id` و در صورت وجود `state` را به Return می‌برد.

این صرفاً UX است؛ Merchant باید برای Fulfillment، `status=completed` را با Partner Inquiry یا Webhook امضاشده بررسی کند. Frontend نیز نباید صرف Redirect نتیجهٔ FC را موفق اعلام کند.

### وضعیت‌های Session

مسیر معمول `open → confirming → processing → completed` است. `canceled`، `expired` و `unavailable` وضعیت‌های Terminal هستند. در مسیر Merchant Funding ممکن است بدون PSP Redirect تکمیل شود، و بعضی خطاهای قابل‌تکرار می‌توانند Session را به `open` برگردانند. در `confirming` یا `processing` همیشه امکان Cancel/Expire وجود ندارد؛ State معتبر Backend تعیین‌کننده است.

## ۱۱. جدول مدیریت خطا و State در Frontend

| شرایط | رفتار درست |
| --- | --- |
| `401` پنل | ورود مجدد از Identity؛ Header داخلی نسازید |
| `403` پنل | نبود Permission را توضیح دهید؛ Route Guard فقط UX است |
| `404` منابع غیرمتعلق | عدم دسترسی؛ دربارهٔ مالک منبع اطلاعات افشا نکنید |
| Credit `not_assigned` | CTA آغاز ارزیابی، نه موجودی صفر |
| Credit `expired` | CTA ارزیابی مجدد |
| Decision `denied` | نمایش Reason Codes؛ Contract نسازید |
| تأمین مالی جزئی | نمایش دقیق `upfront_amount_minor` و Schedule سرور |
| Browser/PSP Return | Pending و Refresh وضعیت معتبر Backend |
| `has_pending_payment=true` یا `payment_block_reason` | غیرفعال‌کردن شروع پرداخت تکراری |
| Timeout پرداخت | Retry محدود همان Key، اول بررسی وضعیت |
| Contract `canceling` | نمایش «در حال لغو»، نه لغوشده |
| START Hosted | نمایش Offer؛ درخواست OTP جداگانه |
| OTP Cooldown/Locked | استفاده از `otp_next_issue_at` و `otp_availability` |
| Offer نسل قدیمی | موافقت و OTP جدید |
| Confirm مبهم | Poll خواندنی؛ بدون موفقیت فرضی |
| Hosted `completed` | نمایش موفقیت و سپس Return معتبر |
| Hosted `expired/canceled/unavailable` | پایان فلو و جلوگیری از Mutation |
| `429` / `503` | Backoff محدود؛ استفاده از Retry Hint |
| بستن Tab/Cancel Fetch | عملیات Backend ممکن است انجام شده باشد؛ Read معتبر |

## ۱۲. زبان، ریال، زمان و RTL

- i18n REST فعلاً فقط **`fa-IR` و `en-US`** را پیاده کرده است. `Accept-Language` از هدر قدیمی `lang` اولویت بالاتری دارد و `Content-Language` زبان پاسخ را نشان می‌دهد. زبان ناشناخته به Default پیکربندی‌شده (معمولاً `en-US`) برمی‌گردد.
- `message`ها محلی‌سازی می‌شوند؛ `status`، `meta.code`، `error.code`، شناسه‌ها، کلیدهای JSON و مقدارهای ماشین‌خوان ترجمه نمی‌شوند.
- متن فارسی RTL و انگلیسی LTR باشد؛ UUID، URL، مبلغ و شناسه را در نمایش ایزوله کنید، **نه با دستکاری مقدار فنی**.
- مبالغ `minor unit` و دقیق‌اند. ریال/تومان را بدون تصمیم روشن UI تبدیل نکنید. `currency` مشخص‌کنندهٔ نمایش است.
- Timestampهای RFC3339 را به Locale موردنظر نمایش دهید و شمارش معکوس Expiration را جایگزین استعلام سرور نکنید.
- Error قابل‌فهم و دسترس‌پذیر نمایش دهید؛ کد فنی را برای لاگ و پشتیبانی نگه دارید.

## ۱۳. چک‌لیست پذیرش Frontend

- [ ] Origin از محیط پیکربندی شده و همهٔ مسیرها `/api/v1` دارند.
- [ ] پنل Customer/Merchant از Gateway Auth استفاده می‌کند؛ `X-Auth-*` ساخته نمی‌شود.
- [ ] Hosted Token/OTP مستقل از JWT و Terminal API Key است.
- [ ] Success `{data,meta}` از Error `{code,message,trace_id,details?}` جدا Parse می‌شود.
- [ ] `Idempotency-Key` درست مدیریت می‌شود؛ نتیجهٔ مبهم پرداخت Pending می‌ماند.
- [ ] `not_assigned` و `expired` با Credit صفر اشتباه نمی‌شوند.
- [ ] Partial Financing و Schedule فقط با مبلغ Backend نمایش داده می‌شوند.
- [ ] Redirect درگاه تأیید پرداخت/فعال‌سازی Contract محسوب نمی‌شود.
- [ ] دکمهٔ Repayment به `next_payable` و `payment_block_reason` وابسته است.
- [ ] Pagination از `meta.pagination` و Cursor واقعی استفاده می‌کند.
- [ ] START هیچ SMS نمی‌فرستد؛ OTP از Endpoint مستقل گرفته می‌شود.
- [ ] Generation جدید، موافقت و OTP جدید می‌خواهد.
- [ ] Timeout Confirm با Poll خواندنی مدیریت می‌شود.
- [ ] API Key ترمینال فقط در صدور نمایش داده می‌شود و در Storage مرورگر نمی‌ماند.
- [ ] RTL/LTR، `fa-IR`، `en-US`، Refresh، Double-Click و Timeout تست شده‌اند.

## ۱۴. نقشهٔ سورس و موارد وابسته به Deployment

- Route/Auth: `pkg/adapter/restful/gin/routes/register.go`، `middleware/auth_context.go`، `middleware/hosted_hardening.go`.
- Decision/Contract/Credit: `decisionrs/serdser.go`، `contractrs/serdser.go`، `customercreditrs/serdser.go`.
- Customer reads/Repayment: `customerrs/list_active_contracts.go`، `customerrs/list_unpaid_installments.go`، `customerrs/list_payments.go`، `installmentpaymentrs/serdser.go`.
- Merchant: `merchantrs/serdser.go`، `merchantterminalrs/resource.go` و `merchantterminalrs/serdser.go`.
- Hosted: `checkoutsessionrs/resource.go`، `checkoutsessionrs/hosted_external.go`، `checkoutsessionrs/request_otp.go`، `checkoutsessionrs/confirmation_status.go`.
- Envelope/Locale/Pagination: `serdser/success_response.go`، `serdser/error_response.go`، `pkg/adapter/restful/i18n/locale.go`، `pagination/parse.go`.

فایل‌های REST بالا در `pkg/adapter/restful/gin` هستند، مگر آنکه مسیر کامل نوشته شده باشد.

**مواردی که خود Backend مشخص نمی‌کند:** URL واقعی Gateway، قرارداد Login/Refresh در Identity، سیاست امن Token Storage، Originهای CORS، آدرس UI Hosted، صفحهٔ Return در Merchant، نقش‌های استقرار و طرح گرافیکی. تیم Frontend باید این موارد را از Identity/Deployment/Product بگیرد؛ مقدار ساختگی در کد نگذارد.

**قاعدهٔ نگه‌داری:** تغییر هر Route، DTO، Permission یا State Machine باید هم‌زمان در [نسخهٔ فارسی](bnpl-frontend.fa.md) و [English](bnpl-frontend.en.md) اعمال شود.
