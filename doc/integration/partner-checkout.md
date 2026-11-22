# راهنمای یکپارچه اتصال Partner به BNPL

**زبان:** فارسی · [English](partner-checkout.en.md)  
**راهنمای Frontend خود BNPL:** [فارسی](../frontend/bnpl-frontend.fa.md) / [English](../frontend/bnpl-frontend.en.md)  
> **به‌روزرسانی قرارداد (۲۳ نوامبر ۲۰۲۶):** پس از موفقیت پایدار Financial Core، Session به `verify_pending` می‌رود. Fulfill فقط بعد از Partner `POST .../verify` (یا Inquiry با `status=verified_by_merchant`). وب‌هوک lifecycle برای Merchant حذف شده است. دو محصول اعتباری + `PUT .../credit-kind` از ۲۰ نوامبر پابرجاست. changelog شناسه: [changelog](../frontend/bnpl-frontend-external-ids-changelog.fa.md).

**مخاطب:** تیم Backend، Frontend، QA و عملیات همهٔ Merchantها؛ از جمله Damz، دیجی‌کالا و سایر فروشگاه‌ها.  
**وضعیت:** قرارداد اجرایی فعلی.  
**Base path:** `/api/v1`. آدرس کامل Sandbox و Production را از تیم BNPL بگیرید.  
**مرجع نهایی اختلاف:** Route Registration، DTO و Use Caseهای فعلی BNPL؛ این سند جایگزین دو راهنمای قدیمی Internal و External است.

## ۱. اصل قرارداد: یک مسیر برای همهٔ Partnerها

در API تجاری **هیچ تقسیم‌بندی Internal/External برای Damz و Digikala وجود ندارد**. Backend همهٔ Partnerها از همان `/api/v1/partner/**` با `X-Terminal-Id` و `X-API-Key` استفاده می‌کند. مسیر قدیمی `/api/v1/internal/checkout-sessions` بخشی از قرارداد فعلی نیست. در Core ممکن است `InternalCheckoutSessionOrigin` یا Use Caseهای داخلی وجود داشته باشد؛ آن‌ها API مستقلی برای Merchantهای «داخلی» تعریف نمی‌کنند. Session ساخته‌شده با `CreatePartner` از origin مربوط به Partner Hosted Checkout استفاده می‌کند.

آنچه میان فروشگاه‌ها فرق می‌کند **تنظیمات سروری Merchant/Branch و مجوزهای ترمینال** است؛ نه تشخیص داخلی یا خارجی بودن نام فروشگاه. تفاوت مهم `merchant_branch_profiles.offer_preparation_mode` در بخش ۶ توضیح داده شده است.

### اصول مسئولیت‌ها

1. **Merchant Backend** فقط `/api/v1/partner/**` را با `X-Terminal-Id` و `X-API-Key` صدا می‌زند و هرگز این اسرار را در Browser قرار نمی‌دهد.
2. **فروشگاه Merchant** لیست `credit_kinds[]` را به مشتری نشان می‌دهد و انتخاب را از طریق Backend با `PUT .../credit-kind` ثبت می‌کند.
3. **Frontend Merchant** کاربر را فقط به `checkout_url` شفاف/opaque برمی‌گرداند؛ محصول را داخل Hosted انتخاب نمی‌کند.
4. **Hosted Checkout BNPL** مسیرهای توکن عمومی را اجرا می‌کند و نمی‌تواند `credit_kind` انتخاب‌شده توسط Partner را عوض کند.
5. **تکمیل سفارش** با Partner `POST .../verify` در وضعیت `verify_pending` و سپس Inquiry با `status=verified_by_merchant` قطعی است؛ Redirect مرورگر یا PSP کافی نیست و جایگزین Verify نیست.
6. اعمال fulfillment سمت Merchant باید **idempotent** باشد.

## ۲. نقشهٔ بازیگران و مسیرها

~~~text
Merchant Backend -- X-Terminal-Id / X-API-Key --> BNPL /api/v1/partner/
          |
          +-- POST /partner/checkout-sessions
          |    <-- cs_ token, checkout_url, evaluation_generation, credit_kinds[]
          |        (Create محصول را انتخاب و Offer را آماده نمی‌کند)
          |
فروشگاه Merchant -- نمایش credit_kinds[] --> مشتری محصول را انتخاب می‌کند
          |
          +-- PUT /partner/checkout-sessions/{id}/credit-kind
          |    body: {credit_kind, expected_evaluation_generation}
          |    <-- kind؛ Offer فقط اگر mode=on_create
          |
Merchant Frontend -- checkout_url --> BNPL Hosted Checkout
                                       |
                                       | GET/START/OTP/CONFIRM/...
                                       | GET .../return  (فقط UX)
                                       |
Merchant Backend -- GET Inquiry ------> BNPL  (تا verify_pending)
Merchant Backend -- POST .../verify --> BNPL  (merchant_order_id؛ سپس Fulfill)
~~~

| سطح | مسیر | احراز هویت | مصرف‌کننده |
| --- | --- | --- | --- |
| Merchant Partner | `/api/v1/partner/**` | `X-Terminal-Id` + `X-API-Key` و Permission ذخیره‌شدهٔ ترمینال | فقط Merchant Backend |
| Merchant Panel | `/api/v1/merchant/terminal/**` | هویت و Permission معتبر از Gateway | پنل/عملیات Merchant |
| Hosted Customer | `/api/v1/checkout-sessions/{token}/**` | توکن عمومی Checkout و OTP در مراحل لازم؛ بدون JWT Gateway | فقط BNPL Hosted UI |

`{token}` در سطح Merchant و Hosted همان `cs_` به‌علاوهٔ ۶۴ رقم hex است؛ **UUID داخلی CheckoutSession نیست**.

## ۳. راه‌اندازی یک‌بارهٔ Merchant

### ۳.۱. دریافت Terminal Credentials

پنل Merchant با احراز هویت Gateway و Permission `bnpl:merchant-terminals:manage:merchant` این APIها را دارد:

| Method | Endpoint | کاربرد |
| --- | --- | --- |
| `POST` | `/api/v1/merchant/terminal` | Provision ترمینال و نخستین API Key؛ نیازمند Header `Idempotency-Key` |
| `GET` | `/api/v1/merchant/terminal` | خواندن metadata؛ **بدون API Key خام** |
| `POST` | `/api/v1/merchant/terminal/credentials/rotate` | صدور کلید جدید و دورهٔ Grace برای کلید قبلی |
| `POST` | `/api/v1/merchant/terminal/credentials/{credential_id}/retire` | پایان‌دادن به اعتبار کلید درحال بازنشستگی |

هر Merchant در مدل فعلی حداکثر یک Terminal دارد. `terminal_id` یک عدد مثبت با نمایش اعشاری استاندارد است؛ در سامانهٔ Merchant بهتر است آن را **رشته** نگه دارید تا JavaScript باعث از دست رفتن دقت نشود. `api_key` محرمانه و `bnpl_…` است. Secret خام در پاسخ Provision/Rotate و Replay مجاز همان عملیات در بازهٔ تعریف‌شده تحویل می‌شود؛ از `GET` قابل بازیابی نیست. آن را فوراً در Secret Manager ذخیره کنید.

مجوزهای اولیه هنگام Provision در جدول `merchant_terminal_permissions` ذخیره می‌شوند و روی هر درخواست دوباره خوانده می‌شوند. API Key خودش حامل Permission نیست.

| Permission روی Terminal | عملیات |
| --- | --- |
| `bnpl:checkout-sessions:create:merchant` (`CheckoutSessionsCreateMerchant`) | Create، Select credit kind، Verify (`POST .../verify`) و Cancel |
| `bnpl:checkout-sessions:read:merchant` | Inquiry |
| `bnpl:checkout-return-destinations:manage:merchant` | پیکربندی Return Destination |

تغییر/Rotate کلید به‌معنی اعطای Permission جدید نیست. `403` با کلید صحیح می‌تواند نشان‌دهندهٔ نبود Permission باشد.

### ۳.۲. Authentication در Partner Runtime

روی **تمام** درخواست‌های `/api/v1/partner/**`:

~~~http
X-Terminal-Id: 4829174650293817
X-API-Key: bnpl_<secret>
~~~

این Routeها به `X-Auth-Subject-Id`، `X-Auth-Tenant-Id`، `X-Auth-Context-Id`، `X-Auth-Permissions` و JWT درخواست Merchant متکی نیستند. MerchantID و Permission از Terminal معتبر در سمت BNPL استخراج می‌شود. مقادیر Terminal ID و API Key باید دقیق و غیرتکراری باشند؛ credential نامعتبر پاسخ Unauthorized دارد.

برای مانیتورینگ می‌توان `X-Trace-Id`، `X-Request-Id` و `X-Correlation-Id` را فرستاد. این هدرها مجوز دسترسی نیستند.

**شرط عملیاتی:** Gateway باید از جعل هدرهای هویتی جلوگیری کند؛ با این حال Partner API مستقل از هدرهای هویت Gateway است. ترافیک و کلیدهای ترمینال را روی HTTPS و فقط از Backend امن ارسال کنید.

### ۳.۳. ثبت Return Destination

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

`destination_name` نام نمادین با الگوی `^[a-z0-9]+(-[a-z0-9]+)*$` و طول ۱ تا ۶۴ است؛ مثل `default` یا `damz-web`. در Create فقط **نام** این Destination ارسال می‌شود؛ URL دلخواه در Checkout Create مجاز نیست.

`enabled` اختیاری است و با حذف آن، مقدار ذخیره‌شده حفظ می‌شود (بار اول `true`). URL باید معتبر و مطلق باشد، بدون Userinfo/Query/Fragment و whitespace غیرمجاز؛ در Production از HTTPS استفاده کنید. پاسخ `200` در `data` شامل `destination_name`، `return_url` و `enabled` است.

### ۳.۴. Online Store و تنظیم شعبه

برای Create، Merchant به `external_online_store_id` معتبر و متعلق به همان Merchant نیاز دارد. BNPL از این شناسه، Merchant Branch آنلاین و سیاست‌های آن را پیدا می‌کند؛ Merchant حق ندارد به‌جای آن `external_merchant_id` یا `external_branch_id` دلخواه بفرستد.

`offer_preparation_mode` روی **MerchantBranchProfile** است؛ Merchant آن را در بدنهٔ Create یا هدر Terminal انتخاب نمی‌کند. مقدار در زمان ساخت Session Snapshot می‌شود.

## ۴. فهرست API دقیق

### Partner Backend API

| Method | Path | Permission |
| --- | --- | --- |
| `POST` | `/api/v1/partner/checkout-sessions` | `bnpl:checkout-sessions:create:merchant` |
| `PUT` | `/api/v1/partner/checkout-sessions/{checkout_session_id}/credit-kind` | `bnpl:checkout-sessions:create:merchant` |
| `GET` | `/api/v1/partner/checkout-sessions/{checkout_session_id}` | `bnpl:checkout-sessions:read:merchant` |
| `POST` | `/api/v1/partner/checkout-sessions/{checkout_session_id}/verify` | `bnpl:checkout-sessions:create:merchant` |
| `POST` | `/api/v1/partner/checkout-sessions/{checkout_session_id}/cancel` | `bnpl:checkout-sessions:create:merchant` |
| `PUT` | `/api/v1/partner/return-destinations/{destination_name}` | `bnpl:checkout-return-destinations:manage:merchant` |

API پیکربندی Webhook Destination برای Partner وجود ندارد.

### Hosted BNPL Customer UI (نباید از Merchant Backend فراخوانی شود)

| Method | Path | رفتار |
| --- | --- | --- |
| `GET` | `/api/v1/checkout-sessions/{token}` | خواندن وضعیت/Offer امن و masked destination، بدون SMS و Side Effect |
| `POST` | `/api/v1/checkout-sessions/{token}/start` | آماده‌کردن یا خواندن Offer؛ **بدون ارسال OTP** |
| `POST` | `/api/v1/checkout-sessions/{token}/otp` | درخواست/بازپخش SMS OTP برای `evaluation_generation` مشخص |
| `POST` | `/api/v1/checkout-sessions/{token}/confirm` | اعتبارسنجی OTP و Confirm & Pay |
| `GET` | `/api/v1/checkout-sessions/{token}/confirmation-status` | Poll خواندنی برای Confirm مبهم؛ **بدون ایجاد عملیات مالی** |
| `GET` | `/api/v1/checkout-sessions/{token}/return` | Redirect `302` به Return URL ثبت‌شده؛ صرفاً UX |
| `POST` | `/api/v1/checkout-sessions/{token}/cancel` | لغو مشتری در صورت مجاز بودن وضعیت |

## ۵. ساخت Checkout توسط Merchant

~~~http
POST /api/v1/partner/checkout-sessions
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...
~~~

فیلدهای درخواست مانند قبل (`external_online_store_id`, `merchant_order_id`, `idempotency_key?`, `customer_phone`, `return_destination`, مبلغ، ارز، ...). **`credit_kind` در Create پذیرفته یا انتخاب نمی‌شود.**

HTTP: `201` جدید؛ `200` replay.

پاسخ نمونه:

~~~json
{
  "data": {
    "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "status": "open",
    "checkout_url": "https://checkout.example/checkout/cs_…",
    "evaluation_generation": 1,
    "expires_at": "2026-11-04T13:00:00Z",
    "credit_kinds": [
      {
        "kind": "pay_in_installments",
        "available": true,
        "repayment_plan": "pay_in_3",
        "installment_count": 3
      },
      {
        "kind": "pay_next_month",
        "available": false,
        "reason": {
          "code": "INSUFFICIENT_AVAILABLE_CREDIT",
          "message": "Insufficient available credit for this purchase."
        },
        "installment_count": 1
      }
    ]
  }
}
~~~

### پیش‌نمایش `credit_kinds[]` (حریم‌خصوصی)

| فیلد | معنا |
| --- | --- |
| `kind` | `pay_in_installments` یا `pay_next_month` |
| `available` | آیا UI باید این گزینه را برای این مبلغ/مشتری فعال کند |
| `reason.code` / `reason.message` | علت غیرفعال بودن (پیام محلی‌سازی‌شده) |
| `repayment_plan` | فقط برای اقساطی (`pay_in_3` / `pay_in_4`) |
| `installment_count` | طول سیاست (۱ برای ماهانه) |

کدهای رایج (مقادیر واقعی runtime): `MERCHANT_CREDIT_KIND_DISABLED`, `CUSTOMER_NOT_RESOLVED`, `MERCHANT_BRANCH_NOT_ACTIVE`, `CREDIT_NOT_ASSIGNED`, `CREDIT_ACCOUNT_UNUSABLE`, `INSUFFICIENT_AVAILABLE_CREDIT`, `NEW_CREDIT_USAGE_RESTRICTED`, `MONTHLY_PREVIOUS_CYCLE_UNPAID`, `CUSTOMER_NOT_ACTIVE`, `CHECKOUT_UPFRONT_PAYMENT_UNAVAILABLE`. (`reason.message` برای نمایش محلی‌سازی می‌شود.)

پیش‌نمایش فیلدهای پولی سطح حساب (`available_credit_minor` و سقف) را افشا نمی‌کند. Create **هرگز Offer آماده نمی‌کند**؛ `offer` روی Create غایب است. `evaluation_generation` از ۱ شروع می‌شود.

## ۵.۵ انتخاب credit kind (الزامی قبل از Hosted START)

~~~http
PUT /api/v1/partner/checkout-sessions/{checkout_session_id}/credit-kind
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...

{
  "credit_kind": "pay_next_month",
  "expected_evaluation_generation": 1
}
~~~

**متد PUT است، نه POST.** Permission ساخت Session. `expected_evaluation_generation` حتی در اولین انتخاب اجباری است.

رفتار:

| حالت | نتیجه |
| --- | --- |
| اولین انتخاب | kind ثبت می‌شود؛ generation افزایش نمی‌یابد (معمولاً روی `1` می‌ماند) |
| Retry همان kind | بدون افزایش generation؛ حتی اگر expected generation کهنه باشد |
| تعویض kind روی session قابل‌ویرایش `open` | Decision فعلی قطع، generation دقیقاً یک‌بار جلو می‌رود، Offer قبلی باطل می‌شود |
| تعویض kind با generation کهنه | Conflict/stale — generation تازه را بخوانید و دوباره بزنید |
| kind غیرفعال merchant / session منقضی / confirming | خطا |

پس از موفقیت، مرورگر را به همان `checkout_url` ببرید. Hosted محصول را دوباره انتخاب نمی‌کند. `offer` فقط وقتی mode شعبه `on_create` باشد ممکن است برگردد.

## ۶. حالت‌های آماده‌سازی Offer بعد از dual-credit: `on_create` / `on_start`

| رفتار | `on_create` | `on_start` |
| --- | --- | --- |
| Partner Create | فقط Session + `credit_kinds[]` | همان |
| Partner SelectCreditKind | ممکن است Offer همان kind را بسازد | فقط kind را ثبت می‌کند |
| Hosted START | Offer انتخاب‌شده را بازخوانی/اعتبارسنجی می‌کند | Offer را برای kind انتخاب‌شده می‌سازد |
| START بدون Select | شکست | شکست |

### تنظیم admin محصولات (نه Terminal)

- `GET /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds`
- `PUT .../credit-kinds/{credit_kind}` با `{"enabled":true|false}`

پیش‌فرض: اقساطی فعال مگر disable؛ `pay_next_month` تا enable صریح خاموش. Partner Terminal این APIها را صدا نمی‌زند.

## ۷. فلو Hosted UI: پس از انتخاب Partner، `GET → START → OTP → CONFIRM`

این بخش برای تیم Frontend میزبانی‌شدهٔ BNPL و برای آگاهی QA/Backend Merchant است؛ Merchant Backend این Endpointها را صدا نمی‌زند.

1. **GET** `/checkout-sessions/{token}`: وضعیت، `merchant.display_name`، مبلغ، `evaluation_generation`، `credit_kind` اختیاری (انتخاب Partner)، `supported_credit_kinds[]` (فعال‌بودن merchant ≠ واجدشرایطی مشتری)، `offer` حریم‌خصوصی‌محور در صورت آماده‌بودن، OTP مقصد ماسک‌شده. GET فقط‌خواندنی است؛ Decision/OTP/FC نمی‌سازد و credit kind را عوض نمی‌کند. `available_credit_minor` و هویت کامل مشتری در Hosted نیست.
2. **START** فقط بعد از Partner `PUT .../credit-kind`. body اختیاری `{"evaluation_generation":N}`؛ بدون انتخاب قبلی ادامه نمی‌دهد و kind را override نمی‌کند. Offer را برای kind انتخاب‌شده آماده/برمی‌گرداند و **SMS نمی‌فرستد**.
3. **OTP**: `POST .../otp` با `{"evaluation_generation":N}`؛ پاسخ شامل `otp_sent` و محدودیت‌های صدور مجدد است.
4. **CONFIRM**: `POST .../confirm` با `{"evaluation_generation":N,"otp":"123456"}`. `reconfirm_required=true` یعنی Offer تازه + OTP نسل جدید.
5. **تکمیل پرداخت**: `payment_required=true` + `redirect_url` فقط ادامهٔ UX است، نه تسویه. موفقیت پایدار FC Session را به `verify_pending` می‌برد تا Merchant Verify کند؛ Hosted `completed` جایگزین Verify نیست.
6. **ابهام Confirm**: `GET .../confirmation-status` فقط‌خواندنی است؛ Payment URL جدید نمی‌سازد. وضعیت‌هایی مثل `payment_action_required` / `retry_available` / `needs_attention` را از `next_action` دنبال کنید.

برای `confirmation-status`، حالت `payment_action_required` به این معنی نیست که Poll خودش لینک جدید می‌سازد؛ ادامهٔ مجاز پرداخت باید از فلو احراز هویت و Confirm پیروی کند. محدودیت‌های صدور مجدد OTP با `otp_next_issue_at` و `otp_availability` هدایت می‌شوند.

### Browser Return

`GET /api/v1/checkout-sessions/{token}/return` پاسخ **HTTP 302** به Return URL ثبت‌شده و ذخیره‌شده در Checkout است. این دست‌به‌دست‌شدن **فقط UX** است. پارامترهای خروجی صرفاً `checkout_session_id` و در صورت وجود `state=merchant_state` هستند. ورودی مثل `?url=...` مقصد را عوض نمی‌کند. صفحهٔ Merchant بعد از بازگشت باید Pending نشان دهد و از Backend بخواهد Inquiry کند و در صورت `verify_pending`، Verify بزند؛ از Query String موفقیت، Verify یا Fulfill را استنتاج نکند.

## ۸. Inquiry و نهایی‌کردن سفارش

~~~http
GET /api/v1/partner/checkout-sessions/{checkout_session_id}
X-Terminal-Id: ...
X-API-Key: ...
~~~

`200` در `data` (بسته به وضعیت):

~~~json
{
  "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "merchant_order_id": "ORDER-1001",
  "status": "verify_pending",
  "amount_minor": 120000000,
  "currency": "IRR",
  "verification_deadline_at": "2026-11-04T13:33:00Z"
}
~~~

وقتی `status=verified_by_merchant` باشد، Inquiry به‌جای `verification_deadline_at` فیلد `verified_at` را برمی‌گرداند. Inquiry وضعیت را **نمی‌نویسد**؛ ممکن است Session بازِ گذشته از Deadline را در پاسخ `expired` نشان دهد، بدون اینکه الزاماً همان لحظه State در DB تغییر کند. دسترسی به Session Merchant دیگر `404` است تا امکان Enumeration ایجاد نشود.

پیش از Fulfill، دقیقاً این پنج مورد را با سفارش ذخیره‌شدهٔ Merchant تطبیق دهید: `checkout_session_id`، `merchant_order_id`، `amount_minor`، `currency` و `status == verified_by_merchant`. وقتی Inquiry هنوز `verify_pending` است، ابتدا Verify را صدا بزنید. عملیات Fulfill باید در DB خود Merchant Idempotent و اتمیک باشد.

### وضعیت‌های Checkout برای Merchant

| `status` | معنی | Fulfill؟ |
| --- | --- | --- |
| `open` | Checkout قابل ادامه؛ Offer ممکن است هنوز تهیه نشده باشد | خیر |
| `confirming` | تأیید مشتری/آغاز عملیات در حال انجام | خیر |
| `processing` | نتیجهٔ معتبر عملیات مالی هنوز در راه است | خیر |
| `verify_pending` | موفقیت پایدار FC ثبت شده؛ Merchant باید قبل از Deadline Verify کند | خیر — Verify کنید |
| `verified_by_merchant` | Merchant در پنجرهٔ Verify پذیرش کرده است | **بله، پس از تطبیق** |
| `cancellation_pending` | پنجرهٔ Verify تمام شده (یا لغو در جریان است)؛ قابل Fulfill نیست | خیر |
| `canceled` | لغوشده توسط مشتری یا Merchant | خیر |
| `expired` | مهلت Checkout پایان یافته | خیر |
| `unavailable` | امکان ارائهٔ اعتبار/تکمیل پیشنهاد وجود ندارد | خیر |

هیچ وضعیت Merchant-facing به نام `failed` یا `completed` برای Fulfill در این State Machine وجود ندارد؛ خطای پرداخت قابل تکرار ممکن است Checkout را دوباره به `open` ببرد. `confirming` / `processing` / `verify_pending` را پرداخت قابل Fulfill فرض نکنید.

## ۹. لغو Checkout توسط Merchant

~~~http
POST /api/v1/partner/checkout-sessions/{checkout_session_id}/cancel
X-Terminal-Id: ...
X-API-Key: ...
~~~

در صورت مجاز بودن، `200` و `data` شامل `checkout_session_id`، `status=canceled` و `return_url` است. لغو Session از پیش `canceled` Idempotent است. فقط Checkout باز و قابل لغو را می‌توان لغو کرد؛ اگر Confirm/Payment شروع شده، نتیجه مبهم است یا Session در `verify_pending` است، درخواست می‌تواند `409 CONFLICT` بدهد. `verified_by_merchant`، `cancellation_pending`، `expired` و `unavailable` از این مسیر Partner قابل لغو نیستند. در Conflict، Inquiry کنید و پرداخت نامعلوم را «ناموفق» فرض نکنید.

## ۱۰. Verify توسط Merchant — پذیرش صریح پس از موفقیت FC

پس از ثبت موفقیت پایدار Financial Core، Session به `verify_pending` می‌رود و `verification_deadline_at` ذخیره می‌شود. Merchant Backend باید در همان پنجره Verify را صدا بزند. Browser Return فقط UX است و جایگزین این فراخوانی نیست.

~~~http
POST /api/v1/partner/checkout-sessions/{checkout_session_id}/verify
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...
~~~

Permission: `bnpl:checkout-sessions:create:merchant` (`CheckoutSessionsCreateMerchant`).

~~~json
{
  "merchant_order_id": "ORDER-1001"
}
~~~

پاسخ موفق `200`:

~~~json
{
  "data": {
    "checkout_session_id": "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
    "merchant_order_id": "ORDER-1001",
    "status": "verified_by_merchant",
    "verified_at": "2026-11-04T13:05:12Z"
  }
}
~~~

| قاعده | قرارداد |
| --- | --- |
| پنجره | **۳۰ دقیقه** از موفقیت پایدار FC؛ سیاست `checkout_merchant_verification/v1` |
| منبع Deadline | `verification_deadline_at` ذخیره‌شده روی Session (Inquiry در `verify_pending` آن را برمی‌گرداند) |
| Body | `merchant_order_id` الزامی و باید با Checkout مطابقت داشته باشد؛ حداکثر ۲۵۶ کاراکتر |
| Idempotency | تکرار Verify برای Session از پیش `verified_by_merchant` با همان مالک و همان `merchant_order_id` رسید ذخیره‌شده را برمی‌گرداند |
| سفارش/وضعیت نادرست | Conflict — Inquiry کنید و Fulfill نکنید |
| بعد از Deadline | Conflict (`verification_deadline_passed`) — Session در (یا به) `cancellation_pending` است |

### Job انقضای Verify

Task زمان‌بندی‌شدهٔ `checkout_verification_expiry` (بازهٔ تقریبی `1m`، envهای `TASK_CHECKOUT_VERIFICATION_EXPIRY_*`) ردیف‌های `verify_pending` سررسیدشده را با **`verification_deadline_at` ذخیره‌شده** ادعا می‌کند و بدون Refund مشتری و بدون آزادسازی exposure اعتبار، آن‌ها را به `cancellation_pending` می‌برد. جبران مالی authoritative مسیر جداگانه‌ای دارد؛ پس از پایان پنجرهٔ Verify نباید Fulfill شود.

## ۱۱. Idempotency، Error و سناریوهای شکست

### Idempotency

- اگر Create Timeout/قطع ارتباط داشت، **همان `idempotency_key` و همان مفاد تجاری** را دوباره بفرستید.
- `idempotency_key` در **Body Create** است، نه Header `Idempotency-Key` مربوط به Provision ترمینال.
- استفادهٔ دوباره از یک Key با شرایط تجاری یا `customer_phone` متفاوت، Conflict است. شماره Canonical توسط BNPL مقایسه می‌شود.
- برای تلاش تازه پس از وضعیت Terminal ناموفق (`canceled`/`expired`/`unavailable`/`cancellation_pending`)، **Key جدید** بدهید. یک سفارش `verified_by_merchant` نباید Checkout تازه بگیرد.
- اگر یک Checkout فعال برای همان `merchant_order_id` وجود دارد، با Key جدید نباید عملیات پرداخت موازی برای همان سفارش بسازید؛ ابتدا وضعیت قبلی را تعیین تکلیف کنید.
- Merchant باید Verify و Fulfill را Idempotent پردازش کند؛ پارامترهای Browser Return را سیگنال authoritative نداند.

### پاسخ خطا

خطاها با موفقیت `{data,meta}` متفاوت‌اند و قالب کلی آن‌ها:

~~~json
{
  "code": "VALIDATION_FAILED",
  "message": "Localized validation message",
  "trace_id": "a1b2c3d4-e5f6-4890-abcd-ef1234567890",
  "details": {
    "fields": ["customer_phone"],
    "violations": [
      {
        "code": "REQUIRED",
        "message": "Customer phone is required",
        "fields": ["customer_phone"]
      }
    ]
  }
}
~~~

`message` و `violations[].message` می‌توانند Localized باشند؛ منطق Merchant را به متن انسانی وابسته نکنید. در پاسخ موفق نیز مقدار ماشین‌خوان `meta.code` و `meta.status` ثابت‌اند. `status_code` در `meta`، HTTP Status را تکرار می‌کند.

| وضعیت | رفتار پیشنهادی Merchant |
| --- | --- |
| `400` و Validation | Field اشتباه را اصلاح کنید؛ از Key قبلی برای درخواست تغییرکرده استفاده نکنید |
| `401` | Terminal ID/API Key را اصلاح و از درستی Secret مطمئن شوید |
| `403` | مالکیت Store/وضعیت Merchant/Permission ذخیره‌شدهٔ Terminal را بررسی کنید |
| `404` در Inquiry | Checkout پیدا نشده یا متعلق به Merchant دیگری است |
| `409` | Conflictِ Idempotency/State؛ قبل از تکرار وضعیت فعلی را بررسی کنید |
| `422` در Hosted Confirm | مسئلهٔ OTP؛ فقط BNPL UI بر اساس Policy دوباره OTP بخواهد |
| `5xx` / Timeout | Retry محدود با Backoff، **همان Key**؛ در نتیجهٔ مبهم Inquiry را هم استفاده کنید |

### سناریوهای تست مشترک Damz و Digikala

| سناریو | انتظار |
| --- | --- |
| یک Create دوبار با Key و Body یکسان | `201` سپس `200`؛ همان توکن |
| همان Key با مبلغ/شماره/Store متفاوت | Conflict، بدون ایجاد Checkout جدید |
| `on_create` و `on_start` | Create هرگز Offer ندارد؛ Select ممکن است در `on_create` Offer بسازد؛ START در `on_start` |
| SelectCreditKind قبل از START | بدون PUT موفق، Hosted START ادامه نمی‌دهد |
| Retry همان kind با generation کهنه | موفق بدون افزایش generation |
| GET Hosted | بدون ارسال SMS یا ایجاد Decision |
| START Hosted | آماده‌سازی/بازیابی Offer، **بدون OTP** |
| OTP دوباره در Challenge معتبر | رفتار محدود و Idempotent، نه SMS بی‌قیدوشرط |
| Confirm با OTP/Generation نادرست | رد یا درخواست Reconfirm، بدون نهایی‌سازی غیرمجاز |
| Confirm Timeout | `confirmation-status` برای وضعیت؛ نه فرض Success |
| Inquiry با `verify_pending` | فراخوانی Verify با `merchant_order_id` منطبق |
| Verify دوبار با همان order id | رسید Idempotent با `verified_by_merchant` |
| Verify بعد از Deadline | Conflict؛ `cancellation_pending`؛ بدون Fulfill |
| فقط Browser Return | Pending، سپس Inquiry و Verify — هرگز Fulfill از Redirect |
| Merchant Cancel همزمان با Confirm | ممکن است Conflict؛ Inquiry، بدون فرض لغو مالی |
| Inquiry برای Session Merchant دیگر | `404` |
| Session `verified_by_merchant` و Key جدید برای همان سفارش | عدم ایجاد Checkout دوم |
| انقضای Session در حالی که Open است | `expired`، عدم Fulfillment |

## ۱۲. نمونهٔ عملی Backend Merchant

نمونهٔ دستورات؛ آدرس‌ها و Credentialها باید برای محیط خودتان تنظیم شوند:

~~~bash
# BNPL_API_BASE is the origin only, e.g. https://api.example
curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/return-destinations/default" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"return_url":"https://shop.example/checkout/bnpl/return","enabled":true}'

CREATE=$(curl -sS -X POST "$BNPL_API_BASE/api/v1/partner/checkout-sessions" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "external_online_store_id":"6e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
    "merchant_order_id":"ORDER-1001",
    "idempotency_key":"ORDER-1001-ATTEMPT-1",
    "customer_phone":"+989121234567",
    "return_destination":"default",
    "requested_amount_minor":120000000,
    "currency":"IRR"
  }')

CHECKOUT_SESSION_ID=$(jq -r '.data.checkout_session_id' <<<"$CREATE")
EVAL_GEN=$(jq -r '.data.evaluation_generation' <<<"$CREATE")
CHECKOUT_URL=$(jq -r '.data.checkout_url' <<<"$CREATE")

# فروشگاه گزینه‌های data.credit_kinds[] را نشان می‌دهد؛ سپس:
curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/checkout-sessions/$CHECKOUT_SESSION_ID/credit-kind" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"credit_kind\":\"pay_in_installments\",\"expected_evaluation_generation\":$EVAL_GEN}"

# پس از پرداخت Hosted و Inquiry با status=verify_pending:
curl -sS -X POST "$BNPL_API_BASE/api/v1/partner/checkout-sessions/$CHECKOUT_SESSION_ID/verify" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"merchant_order_id":"ORDER-1001"}'

curl -sS "$BNPL_API_BASE/api/v1/partner/checkout-sessions/$CHECKOUT_SESSION_ID" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY"
~~~

Backend Merchant باید پس از Create توکن، generation، Key، شناسهٔ سفارش، مبلغ، ارز، و پس از Select نوع انتخاب‌شده را ذخیره کند. Frontend Merchant فقط:

~~~javascript
window.location.assign(checkout_url); // returned by BNPL; do not reconstruct
~~~

در صفحهٔ Return از Backend خودش Inquiry و در صورت نیاز Verify را بگیرد. Redirect مرورگر به‌تنهایی Fulfill نیست. **Merchant Backend هرگز `/start`، `/otp`، `/confirm` یا `/return` توکنی را صدا نمی‌زند.**

## ۱۳. چک‌لیست راه‌اندازی و مالکیت مسئولیت‌ها

**Merchant Backend**

- [ ] Terminal Credential را از پنل گرفته و در Secret Manager ذخیره کرده است.
- [ ] تمام Partner APIها را با دو Header ترمینال، در Backend و روی HTTPS صدا می‌زند.
- [ ] Permissionهای ترمینال برای Create/Read/Verify/Return Destination روشن‌اند.
- [ ] `external_online_store_id` و `return_destination` درست Provision شده‌اند.
- [ ] `customer_phone` در Create از منبع قابل اعتماد سفارش می‌آید؛ `customer_reference` هویت مشتری نیست.
- [ ] حالت `on_create` و `on_start` را بدون hardcode کردن نوع Merchant تحمل می‌کند.
- [ ] Idempotency Key/توکن/مبلغ/ارز سفارش را پایدار ذخیره می‌کند.
- [ ] فقط با Partner Verify و Inquiry `verified_by_merchant` (با تطبیق کامل) سفارش را Fulfill می‌کند؛ Browser Return فقط UX است.
- [ ] Verify را در `verify_pending` و قبل از `verification_deadline_at` با `merchant_order_id` منطبق صدا می‌زند.
- [ ] Timeout Verify / `cancellation_pending` را غیرقابل Fulfill می‌داند و Success جعل نمی‌کند.
- [ ] Retry و سناریوی Timeout و Return زودهنگام را پوشش می‌دهد.

**Merchant Frontend**

- [ ] فقط URL برگشتی BNPL را باز می‌کند؛ هیچ API Key یا Secret ندارد.
- [ ] Merchant Endpointها را مستقیم از مرورگر صدا نمی‌زند.
- [ ] Return را موفقیت مالی تلقی نمی‌کند؛ وضعیت را از Backend می‌پرسد.

**BNPL Hosted Frontend**

- [ ] نمایش Offer و نسل آن طبق Mode شعبه انجام می‌شود.
- [ ] START فقط Offer تهیه می‌کند؛ OTP از Endpoint مستقل `/otp` است.
- [ ] Generation/OTP به Confirm ارسال می‌شود و Reconfirm با OTP تازه انجام می‌شود.
- [ ] بعد از Timeout وضعیت Confirm Poll می‌شود؛ Poll عملیات مالی جدید ایجاد نمی‌کند.

## ۱۴. اقلامی که تیم BNPL باید برای هر محیط تحویل بدهد

این مقادیر از خود سورس قابل حدس‌زدن نیستند و نباید در مستند برای Damz یا Digikala جعل شوند:

- آدرس API در Sandbox/Production و Origin صفحهٔ Hosted Checkout.
- Terminal ID و روش تحویل امن API Key، مشخصات Merchant Panel و Provision.
- `external_online_store_id`های واقعی و Mode هر Branch (`on_create` یا `on_start`).
- Return URL و نام Destination مورد توافق.
- اکانت‌های تست، شماره‌های مجاز تست OTP و دادهٔ آزمون مناسب محیط.
- پنجرهٔ ۳۰ دقیقه‌ای Verify Merchant (`checkout_merchant_verification/v1`) و رفتار Job `checkout_verification_expiry`.
- سیاست دسترسی به `data.offer.available_credit_minor` در پاسخ Partner `on_create`؛ این فیلد **اکنون در کد وجود دارد** و تصمیم تغییر آن، موضوعی مستقل از مستندسازی است.

## ۱۵. نقشهٔ سورس برای نگه‌داری قرارداد

ارجاع‌های کلیدی dual-credit: `checkoutsessionrs/select_credit_kind.go`، `checkoutsessionuc/partner_select_credit_kind.go`، `checkoutsessionuc/credit_kinds_preview.go`، `merchantrs/list_credit_kinds.go`، `merchantrs/set_credit_kind.go`.


| بخش سند | مرجع پیاده‌سازی |
| --- | --- |
| Route و جدایی Auth Surfaceها | `pkg/adapter/restful/gin/routes/register.go`؛ `checkoutsessionrs/resource.go` |
| Terminal Auth و Permission | `middleware/partner_terminal_auth.go`؛ `merchantterminaluc/authenticate.go`؛ `merchantterminal/permissions.go` |
| Terminal lifecycle | `merchantterminalrs/resource.go`؛ `merchantterminaluc/provision.go` |
| Create، Schema و Offer | `checkoutsessionrs/create_partner.go`؛ `checkoutsessionrs/serdser.go`؛ `checkoutsessionrs/swagger_schema.go`؛ `checkoutsessionuc/create_partner.go`؛ `checkoutsessionuc/session.go` |
| Mode و Customer Resolution | `model/merchant_branch_profile.go`؛ `checkoutsessionuc/external_prepare.go` |
| Hosted GET/START/OTP/Confirm/Status/Return | `checkoutsessionrs/hosted_external.go`؛ `checkoutsessionrs/request_otp.go`؛ `checkoutsessionrs/confirmation_status.go`؛ `checkoutsessionuc/external_start.go`؛ `checkoutsessionuc/external_otp_request.go` |
| Inquiry/Cancel | `checkoutsessionuc/get_partner_status.go`؛ `checkoutsessionrs/cancel.go` |
| Merchant Verify | `checkoutsessionrs/verify_by_merchant.go`؛ `checkoutsessionuc/verify_by_merchant.go`؛ `policy/checkoutverification/v1` |
| Job انقضای Verify | `task/jobs/checkoutsession/verification_expiry.go`؛ `checkoutsessionuc/expire_pending_verifications.go` |
| پاسخ‌های استاندارد | `pkg/adapter/restful/gin/serdser/success_response.go`؛ `error_response.go` |

مسیرهای بالا زیر `pkg/core/` یا `pkg/adapter/` مطابق نام پکیج قرار می‌گیرند. هنگام تغییر قرارداد، نخست DTO/Route/Use Case را بررسی و سپس **همین یک سند** را به‌روزرسانی کنید؛ مستند جداگانه برای Internal/External Partner ایجاد نکنید.


**قاعدهٔ نگه‌داری ترجمه:** هر تغییر در Route، DTO، Permission، پنجرهٔ Verify یا State Machine باید همزمان در [فارسی](partner-checkout.md) و [English](partner-checkout.en.md) اعمال شود.
