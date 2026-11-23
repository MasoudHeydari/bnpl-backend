# راهنمای اتصال Frontendهای BNPL به Backend (فارسی)

> **مخاطب:** تیم‌های Frontend مشتری، پنل Merchant، Hosted Checkout و QA خود BNPL.  
> **زبان:** فارسی · [English](bnpl-frontend.en.md)  
> **به‌روزرسانی قرارداد (۲۰ نوامبر ۲۰۲۶):** دو محصول اعتباری مستقل (`pay_in_installments`، `pay_next_month`)، انتخاب credit-kind در Partner، و repayment batch. changelog شناسه‌های خارجی: [changelog](bnpl-frontend-external-ids-changelog.fa.md).
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

## ۱ـالف. دو محصول اعتباری مستقل (غیرقابل ادغام)

BNPL اکنون **دو محصول اعتباری مستقل** دارد. موجودی، واجدشرایط بودن، exposure و تصمیم checkout آن‌ها **هرگز نباید با هم جمع یا جایگزین** شود.

| Credit kind | معنا | برچسب repayment_plan | زمان‌بندی معمول |
| --- | --- | --- | --- |
| `pay_in_installments` | حساب اعتباری اقساطی | `pay_in_3` یا `pay_in_4` (از Checkout Policy حاکم) | ۳ قسط (سیاست معتمد **v2**) یا ۴ قسط (سیاست عادی **v1**) |
| `pay_next_month` | اعتبار ماهانه / معوق | **ندارد** — بدون برچسب `pay_in_N` | یک قسط در **۰۰:۰۰ Asia/Tehran روز ۱ ماه جلالی بعدی** پس از فعال‌سازی قرارداد (سیاست **v3**) |

**`credit_kind` ≠ `repayment_plan`.** Kind حساب اعتبار را انتخاب می‌کند؛ Plan فقط تصویر طول سیاست اقساطی است. اعتبار ماهانه `installment_count=1` دارد و `repayment_plan` را حذف می‌کند.

**قاعده تأمین مالی فعال (لانچ):** فقط تأمین کامل (`full-financing-only`). اگر موجودی همان محصول کافی نباشد، تصمیم/آفر **رد** می‌شود. قواعد catalog برای مابه‌التفاوت پیش‌پرداخت و excess-financing **غیرفعال**اند — UI مسیر پیش‌پرداخت جزئی را به‌عنوان رفتار production نسازید.

**Exposure:** اعتبار قابل‌استفاده = سقف محصول − اصل معوق **فعال** همان CreditAccount. Decision/Offer و قرارداد `payment_pending` موجودی را کم نمی‌کنند. مصرف اعتبار فقط با `active` شدن قرارداد است.

**محدودیت خرید جدید (`credit_usage`):** اقساط due/overdue می‌تواند همه محصولات را محدود کند؛ چرخه ماهانه پرداخت‌نشده می‌تواند فقط `pay_next_month` را محدود کند (`previous_month_unpaid`).

**سررسید ماهانه:** کامنت قدیمی v3 گاهی «پایان روز ۱» گفته؛ پیاده‌سازی واقعی **شروع روز ۱ (۰۰:۰۰ Asia/Tehran)** ماه جلالی بعدی است. همان `due_at` سرور را نمایش دهید.


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

همهٔ مسیرهای زیر با هویت Gateway و permissionهای زیر کار می‌کنند. Frontend فقط به Gateway احراز هویت می‌کند و هرگز `X-Auth-*` جعلی نمی‌سازد.

| Method | Path | Permission | Status |
| --- | --- | --- | --- |
| `GET` | `/api/v1/customer-credit` | `bnpl:credit-limits:read:self` | `200` |
| `POST` | `/api/v1/customer-credit/initiate` | `bnpl:credit-limits:create:self` | `201` اگر حداقل یک محصول جدید ساخته شود؛ وگرنه `200` |
| `POST` | `/api/v1/checkout-decisions` | `bnpl:decisions:create:self` | `201` جدید؛ `200` replay |
| `POST` | `/api/v1/checkout-decisions/{decision_id}/contracts` | `bnpl:contracts:create:self` | `201` / `200` |
| `POST` | `/api/v1/contracts/{contract_id}/cancel` | `bnpl:contracts:cancel:self` | `202` / `200` |
| `GET` | `/api/v1/contracts` | `bnpl:contracts:read:self` | `200` |
| `GET` | `/api/v1/contracts/{contract_id}/installments` | `bnpl:installments:read:self` | `200` |
| `GET` | `/api/v1/installments` | `bnpl:installments:read:self` | `200` |
| `POST` | `/api/v1/contracts/{contract_id}/installments/{installment_id}/pay` | `bnpl:payments:start:self` | `202` / `200` |
| `POST` | `/api/v1/repayment-batches` | `bnpl:payments:start:self` | `202` |
| `POST` | `/api/v1/repayment-batches/{id}/start` | `bnpl:payments:start:self` | `202` |
| `GET` | `/api/v1/repayment-batches/{id}` | `bnpl:payments:read:self` | `200` |
| `GET` | `/api/v1/payments` | `bnpl:payments:read:self` | `200` |

## ۵. اعتبار مشتری و شروع استفاده

### ۵.۱ خواندن همهٔ محصولات (پیش‌فرض)

`GET /api/v1/customer-credit` **بدون** `credit_kind` هر دو محصول را برمی‌گرداند:

~~~json
{
  "data": {
    "status": "assigned",
    "credits": [
      {
        "credit_kind": "pay_in_installments",
        "status": "active",
        "credit_usage": { "status": "allowed" },
        "limit_amount_minor": 2000000000,
        "available_amount_minor": 500000000,
        "currency": "IRR",
        "expires_at": "2027-11-19T10:41:34Z",
        "application": { "status": "assigned" }
      },
      {
        "credit_kind": "pay_next_month",
        "status": "active",
        "credit_usage": { "status": "restricted", "reason": "previous_month_unpaid" },
        "limit_amount_minor": 30000000,
        "available_amount_minor": 30000000,
        "currency": "IRR",
        "expires_at": "2027-11-19T10:41:34Z",
        "application": { "status": "assigned" }
      }
    ]
  },
  "meta": { "code": "CUSTOMER_CREDIT_RETRIEVED", "status_code": 200 }
}
~~~

**`data.status` تجمیعی:** `not_assigned` | `partially_assigned` | `assigned`.

**وضعیت هر محصول (`credits[].status`):** `active` | `not_assigned` | `expired`.

**`application.status`:** `assigned` | `not_assigned` | `reassessment_required`.

**`credit_usage.status`:** `allowed` | `restricted` | `unavailable`. برای `restricted`، `reason` عمومی می‌تواند شامل `previous_month_unpaid`، `collections_default`، `collections_due`، `credit_usage_restricted` باشد.

**حضور فیلدهای پولی:** فقط در `active`؛ برای `not_assigned`/`expired` صفر جعلی نسازید. همیشه **دو کارت محصول** بسازید و موجودی‌ها را جمع نکنید.

اگر تجمیعی `not_assigned` باشد، `meta.code` برابر `CUSTOMER_CREDIT_NOT_ASSIGNED` است.

#### فقط یکی assign شده / هیچ‌کدام / یکی restricted

همان شکل `credits[]` را حفظ کنید؛ برای محصول بدون assign فیلدهای پولی را حذف‌شده بگذارید؛ برای `restricted` CTA خرید جدید آن محصول را غیرفعال کنید.

### ۵.۲ کوئری سازگار تک‌محصولی

`GET /api/v1/customer-credit?credit_kind=pay_in_installments`  
`GET /api/v1/customer-credit?credit_kind=pay_next_month`

پاسخ **تخت قدیمی** است (بدون `credits[]`). parser دو شکل را قاطی نکنید. `credit_kind` نامعتبر → خطای validation.

### ۵.۳ آغاز تخصیص اعتبار

~~~http
POST /api/v1/customer-credit/initiate
Authorization: Bearer <access-token>
~~~

بدون body. موبایل از claim هویت (`auth_mobile_number`) و header گیت‌وی `x-auth-mobile-number` می‌آید؛ **کلاینت مرورگر نباید headerهای اعتماد را جعل کند**. CRS فقط `ACTIVE` را می‌پذیرد.

**بدون `credit_kind`:** هر دو محصول را تخصیص می‌دهد (`status` + `credits[]` با `result`/`credit_limit_decision`/`credit_limit?`). `result`: `created` | `already_assigned` | `denied`. Outcome: `limit_set` | `denied`. HTTP `201` اگر هر محصولی `created` باشد؛ وگرنه `200`. موفقیت را فقط از HTTP 200 نتیجه نگیرید.

**با `?credit_kind=`:** پاسخ تک‌محصولی legacy.

سیاست معتمد checkout **v2** (۳ قسط) و عادی **v1** (۴ قسط) و ماهانه **v3** سمت سرور انتخاب می‌شوند؛ طول طرح را hard-code نکنید.

## ۶. Checkout مستقیم مشتری: Decision → Offer → Contract

مسیر مشتری احرازشده. Hosted Merchant مسیر جدا است (بخش ۱۰).

### ساخت Decision

~~~http
POST /api/v1/checkout-decisions
Authorization: Bearer <access-token>
Idempotency-Key: checkout-intent-1001
Content-Type: application/json
~~~

**بدنهٔ عمومی `credit_kind` ندارد.** فقط `purchase_context`، `requested_amount_minor`، `currency`. Kind حذف‌شده در use case به‌معنی `pay_in_installments` است. فعلاً انتخاب عمومی `pay_next_month` روی این endpoint وجود ندارد؛ مسیر دو‌محصولی Partner Hosted است.

نمونه POS / Online مانند قبل با `external_pos_gps_id` / `external_online_store_id`.

نمونهٔ تأیید **تأمین کامل** (قاعده لانچ فعال — بدون مسیر پیش‌پرداخت جزئی):

~~~json
{
  "result": "created",
  "decision": {
    "id": "7c9e6679-7425-40de-944b-e07fc1f90ae7",
    "checkout_key": "checkout-intent-1001",
    "outcome": "approved",
    "reason_codes": []
  },
  "requested_amount_minor": 150000000,
  "approved_financed_amount_minor": 150000000,
  "upfront_amount_minor": 0,
  "currency": "IRR",
  "available_credit_minor": 500000000,
  "checkout_action": {
    "type": "confirm_purchase",
    "payment_required": false,
    "payment_amount_minor": 0
  },
  "installment_offer": {
    "installment_count": 3,
    "payable_now_amount_minor": 0,
    "total_payable_amount_minor": 150000000,
    "currency": "IRR",
    "installments": []
  }
}
~~~

با کمبود موجودی همان محصول، `outcome=denied` می‌آید نه CTA پیش‌پرداخت. CTA را از `checkout_action.type` (`confirm_and_pay` / `confirm_purchase`) بگیرید. Decision اعتبار را reserve نمی‌کند. زمان‌بندی را محلی محاسبه نکنید.

### ایجاد Contract

~~~http
POST /api/v1/checkout-decisions/{decision_id}/contracts
Authorization: Bearer <access-token>
~~~

Idempotent بر اساس Decision. اگر `payment_required=true` و `redirect_url` باشد به همان URL بروید. اگر `payment_required=false` (مسیر merchant-funding) هنوز باید وضعیت authoritative را بخوانید. `201` یا برگشت PSP به‌معنی فعال‌سازی نهایی نیست. `payment_pending` می‌تواند تا تکمیل funding در FC بماند.

## ۷. داشبورد اقساط، تاریخچه و بازپرداخت

### خواندن داده‌ها

- `GET /api/v1/contracts`: قراردادهای `active`/`canceling` با `credit_kind` و `repayment_plan` اختیاری.
- `GET /api/v1/contracts/{id}/installments`: شامل `credit_kind`، `repayment_plan?`، `installments[]`، `next_payable_installment?`. ماهانه یک قسط دارد.
- `GET /api/v1/installments`: شیء با `installments[]` و `repayment` سراسری؛ فیلدهایی مانند `customer_payable_amount_minor` (nullable)، `principal_amount_minor`، `late_fee_amount_minor`، `grace_period_days`، `next_payable`، `payment_block_reason`، `has_pending_payment`.
- `GET /api/v1/payments`: فقط attemptهای نهایی.

لیست خالی با `200` عادی است. جریمه و next-payable را سمت کلاینت حساب نکنید.

### پرداخت تک‌قسط

`POST /api/v1/contracts/{contract_id}/installments/{installment_id}/pay` با `Idempotency-Key` و `{"channel":"gateway"|"bank_transfer"|"wallet"}`. ترتیب اقساط را سرور enforce می‌کند. Redirect ≠ تسویه.

### Repayment Batch (چند قسط)

قوانین انتخاب سرور:

1. همهٔ اقساط **overdue** در **هر دو محصول و همهٔ merchantها** باید داخل batch باشند.
2. داخل یک قرارداد چندقسطی، قسط unpaid زودتر را نمی‌توان رد کرد.
3. قسط due ولی هنوز overdue‌نشده اختیاری است.

`POST /api/v1/repayment-batches` — body `{"installment_ids":[...]}` (۱..۵۰)، header `Idempotency-Key` اجباری، موفقیت **`202`** با `{id,status,total_amount_minor,currency}`.

`POST /api/v1/repayment-batches/{id}/start` — `Idempotency-Key` اجباری، **`202`** با `{id,status,redirect_url?}`. provider/return URL فقط سمت سرور است. **توجه:** `status` در Start واژگان **FC collection** است (مثلاً `requires_customer_action`، `succeeded`)، نه لزوماً همان enum BNPL در Create/Get.

`GET /api/v1/repayment-batches/{id}` — وضعیت durable با واژگان **BNPL**: `created`, `prepared`, `starting`, `requires_action`, `reconciling`, `accounting_pending`, `posted`, `failed`, `expired`, `manual_review`. تکمیل UX فقط وقتی Get برابر **`posted`** است (یا وضعیت اقساط authoritative را دوباره بخوانید). `202`/redirect/`requires_action`/FC `succeeded` به‌تنهایی تسویه قطعی نیست.

### لغو Contract

`POST /api/v1/contracts/{contract_id}/cancel` با `Idempotency-Key` و `{"reason":"customer_requested"}`. در `canceling` هنوز «لغو شده» نشان ندهید.

### لینک کوتاه پیامک بازپرداخت

یادآورهای پیامکی به `https://<SHORT_LINK_PUBLIC_HOST>/s/<CODE>` اشاره می‌کنند و BNPL با `302` به یک صفحهٔ **ثابت** ریدایرکت می‌کند (هیچ کد، شناسهٔ قسط یا شناسهٔ مشتری در URL نیست). قرارداد کامل: [لینک کوتاه پیامک بازپرداخت ← صفحهٔ اقساط](repayment-short-link-redirect.fa.md).

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

### تنظیم admin محصولات اعتباری Merchant (نه Terminal Partner)

APIهای admin احرازشده:

- `GET /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds`
- `PUT /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds/{credit_kind}` با `{"enabled":true|false}`

پیش‌فرض: اقساطی معمولاً فعال؛ `pay_next_month` تا enable صریح غیرفعال. این‌ها با credential ترمینال Partner صدا زده نمی‌شوند.

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

`GET /api/v1/checkout-sessions/{token}` فیلدهایی مثل `status`، مبلغ، `evaluation_generation`، `credit_kind` اختیاری (انتخاب Partner)، `supported_credit_kinds[]`، `offer` حریم‌خصوصی‌محور، مقصد OTP ماسک‌شده و `expires_at` برمی‌گرداند. GET فقط‌خواندنی است، Decision/OTP/FC نمی‌سازد و credit kind را نمی‌پذیرد. تا قبل از Partner `PUT .../credit-kind` ممکن است `credit_kind` غایب باشد. Hosted محصول را انتخاب نمی‌کند.

### مرحلهٔ دوم: START

~~~http
POST /api/v1/checkout-sessions/{token}/start
Content-Type: application/json

{}
~~~

START نمی‌تواند credit kind انتخاب‌شده توسط Partner را override کند؛ بدون Select قبلی ادامه نمی‌دهد. body اختیاری `{"evaluation_generation":N}` برای fencing است. START فقط Offer را آماده/برمی‌گرداند و **SMS نمی‌فرستد**.

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

`not_started`، `processing`، `payment_action_required`، `payment_processing`، `reconfirm_required`، `retry_available`، `awaiting_merchant_verification`، `cancellation_pending`، `completed`، `needs_attention`، `canceled`، `expired`، `unavailable`.

Poll هیچ Saga، PaymentAttempt، Session پرداخت یا URL جدیدی ایجاد نمی‌کند. برای اقدام بعدی از `next_action` و `retry_after_seconds` استفاده کنید و در صورت لزوم به START/OTP/CONFIRM برگردید.

وقتی `confirmation_status=awaiting_merchant_verification` و `next_action=complete_hosted_return` است، پرداخت در BNPL قطعی شده ولی پذیرش Merchant هنوز نشده. پیام کوتاه «پرداخت انجام شد» نشان دهید و سپس `GET .../return` را برای Navigation به سایت Merchant صدا بزنید. URL Merchant را از این پاسخ نسازید و خرید را نهایی اعلام نکنید.

### مرحلهٔ ششم: RETURN

`GET /checkout-sessions/{token}/return` یک **Navigation** است و با HTTP `302` به URL ثبت‌شدهٔ Merchant می‌رود. مقصد را از Query String انتخاب نمی‌کند. پارامتر `checkout_session_id` و در صورت وجود `state` را به Return می‌برد.

این صرفاً UX است؛ Merchant باید برای Fulfillment از Partner Inquiry به `verify_pending` برسد و سپس `POST .../verify` بزند تا `verified_by_merchant` شود. Frontend نیز نباید صرف Redirect نتیجهٔ FC یا پذیرش فروشنده را موفق اعلام کند.

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
- [ ] تحت قاعدهٔ لانچ full-financing-only، مسیر پیش‌پرداخت جزئی ساخته نمی‌شود؛ Schedule فقط از Backend نمایش داده می‌شود.
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
