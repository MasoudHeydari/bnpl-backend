# راهنمای یکپارچه اتصال Partner به BNPL

**زبان:** فارسی · [English](partner-checkout.en.md)  
**راهنمای Frontend خود BNPL:** [فارسی](../frontend/bnpl-frontend.fa.md) / [English](../frontend/bnpl-frontend.en.md)

**مخاطب:** تیم Backend، Frontend، QA و عملیات همهٔ Merchantها؛ از جمله Damz، دیجی‌کالا و سایر فروشگاه‌ها.  
**وضعیت:** قرارداد اجرایی فعلی؛ مبنای سورس شاخهٔ `feat-add-decision-domain` در کامیت `e71c4e621a842087928dfa37dbf92571ee3f2c27`.  
**Base path:** `/api/v1`. آدرس کامل Sandbox و Production را از تیم BNPL بگیرید.  
**مرجع نهایی اختلاف:** Route Registration، DTO و Use Caseهای فعلی BNPL؛ این سند جایگزین دو راهنمای قدیمی Internal و External است.

## ۱. اصل قرارداد: یک مسیر برای همهٔ Partnerها

در API تجاری **هیچ تقسیم‌بندی Internal/External برای Damz و Digikala وجود ندارد**. Backend همهٔ Partnerها از همان `/api/v1/partner/**` با `X-Terminal-Id` و `X-API-Key` استفاده می‌کند. مسیر قدیمی `/api/v1/internal/checkout-sessions` بخشی از قرارداد فعلی نیست. در Core ممکن است `InternalCheckoutSessionOrigin` یا Use Caseهای داخلی وجود داشته باشد؛ آن‌ها API مستقلی برای Merchantهای «داخلی» تعریف نمی‌کنند. Session ساخته‌شده با `CreatePartner` از origin مربوط به Partner Hosted Checkout استفاده می‌کند.

آنچه میان فروشگاه‌ها فرق می‌کند **تنظیمات سروری Merchant/Branch و مجوزهای ترمینال** است؛ نه تشخیص داخلی یا خارجی بودن نام فروشگاه. تفاوت مهم `merchant_branch_profiles.offer_preparation_mode` در بخش ۶ توضیح داده شده است.

### چهار قانون غیرقابل چشم‌پوشی

1. Merchant Backend فقط Partner API را صدا می‌زند؛ APIهای دارای `/checkout-sessions/{token}` متعلق به **Frontend میزبانی‌شدهٔ BNPL** هستند.
2. Merchant باید `checkout_url` دریافتی را بدون بازسازی به مرورگر بدهد؛ `X-API-Key` هرگز وارد مرورگر نشود.
3. Browser Return، redirect درگاه و query string آن **مدرک پرداخت موفق نیستند**.
4. Merchant سفارش را فقط با **Inquiry دارای `status=completed`** یا **Webhook امضاشدهٔ `bnpl.checkout.completed.v1`**، پس از تطبیق شناسه/سفارش/مبلغ/ارز، نهایی کند. تغییر وضعیت سفارش در سامانهٔ Merchant باید Idempotent باشد.

## ۲. نقشهٔ بازیگران و مسیرها

~~~text
Merchant Backend -- (Terminal ID + API Key) --> BNPL /api/v1/partner/*
       |
       | POST /partner/checkout-sessions
       +<-- checkout_session_id (public cs_ token), checkout_url, status, expires_at
       |    [optional offer only when branch mode = on_create]
       |
Merchant Frontend -- redirect to checkout_url --> BNPL Hosted Checkout UI
                                                  |
                                                  | GET /checkout-sessions/{token}
                                                  | POST .../start    (prepare/read offer)
                                                  | POST .../otp      (request SMS)
                                                  | POST .../confirm  (OTP + Confirm & Pay)
                                                  | GET .../confirmation-status (read-only)
                                                  | GET .../return   (UX redirect only)
                                                  v
                                 BNPL processes contract/payment internally
                                                  |
Merchant Backend <-- Signed webhook ----------------+
Merchant Backend ---- Partner Inquiry -------------> BNPL
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
| `bnpl:checkout-sessions:create:merchant` | Create و Cancel |
| `bnpl:checkout-sessions:read:merchant` | Inquiry |
| `bnpl:checkout-return-destinations:manage:merchant` | پیکربندی Return Destination |
| `bnpl:checkout-webhooks:manage:merchant` | پیکربندی Webhook |

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

### ۳.۴. ثبت Webhook Destination

~~~http
PUT /api/v1/partner/webhook-destination
Content-Type: application/json
X-Terminal-Id: ...
X-API-Key: ...
~~~

~~~json
{
  "webhook_url": "https://shop.example/webhooks/bnpl",
  "signing_secret": "<long-random-secret-at-least-32-bytes>",
  "enabled": true
}
~~~

در پیاده‌سازی فعلی یک Webhook Destination برای هر Merchant نگهداری می‌شود، نه برای هر سفارش. `signing_secret` بار اول الزامی است؛ بعداً حذف آن یعنی استفاده از Secret ذخیره‌شده و ارسال مقدار جدید یعنی Rotation. طول Secret بین ۳۲ تا ۵۱۲ کاراکتر است؛ پاسخ، آن را برنمی‌گرداند و فقط `webhook_url` و `enabled` را دارد. `enabled` با حذف، مقدار قبلی را حفظ می‌کند.

Webhook URL مستقل از Return URL است؛ مقصد Webhook نباید مستقیماً از ورودی Checkout Create گرفته شود. پیش از استفادهٔ واقعی، از HTTPS و دسترسی و Policy مقصد مطابق محیط BNPL اطمینان حاصل کنید.

### ۳.۵. Online Store و تنظیم شعبه

برای Create، Merchant به `online_store_id` معتبر و متعلق به همان Merchant نیاز دارد. BNPL از این شناسه، Merchant Branch آنلاین و سیاست‌های آن را پیدا می‌کند؛ Merchant حق ندارد به‌جای آن `merchant_id` یا `branch_id` دلخواه بفرستد.

`offer_preparation_mode` روی **MerchantBranchProfile** است؛ Merchant آن را در بدنهٔ Create یا هدر Terminal انتخاب نمی‌کند. مقدار در زمان ساخت Session Snapshot می‌شود.

## ۴. فهرست API دقیق

### Partner Backend API

| Method | Path | Permission |
| --- | --- | --- |
| `POST` | `/api/v1/partner/checkout-sessions` | `bnpl:checkout-sessions:create:merchant` |
| `GET` | `/api/v1/partner/checkout-sessions/{checkout_session_id}` | `bnpl:checkout-sessions:read:merchant` |
| `POST` | `/api/v1/partner/checkout-sessions/{checkout_session_id}/cancel` | `bnpl:checkout-sessions:create:merchant` |
| `PUT` | `/api/v1/partner/return-destinations/{destination_name}` | `bnpl:checkout-return-destinations:manage:merchant` |
| `PUT` | `/api/v1/partner/webhook-destination` | `bnpl:checkout-webhooks:manage:merchant` |

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

| Field | الزام و معنی |
| --- | --- |
| `online_store_id` | الزامی؛ UUID آنلاین‌استور متعلق به Merchant احرازشده |
| `merchant_order_id` | الزامی، حداکثر ۲۵۶؛ شناسهٔ سفارش Merchant |
| `idempotency_key` | اختیاری، حداکثر ۲۵۶؛ در صورت حذف، `merchant_order_id` استفاده می‌شود. پیشنهاد: همیشه صریح بفرستید |
| `customer_reference` | اختیاری، حداکثر ۲۵۶؛ فقط Correlation فروشگاه، **نه شناسهٔ BNPL Customer** |
| `customer_phone` | الزامی، حداکثر ۳۲؛ مطابق Jurisdiction Merchant اعتبارسنجی/Canonicalize و در DB رمز می‌شود؛ نمونهٔ ایران: `09…` یا `+989…` |
| `return_destination` | الزامی؛ نام از پیش ثبت‌شده و فعال، نه URL |
| `merchant_state` | اختیاری، حداکثر ۲۵۶؛ دادهٔ Opaque غیرحساس برای Browser Return/Webhook |
| `requested_amount_minor` | عدد صحیح مثبت، حداکثر `MaxInt64`؛ مبلغ در واحد Minor ارز |
| `currency` | الزامی؛ enum مدل: `IRR`، `EUR`، `USD`؛ پذیرش نهایی به Merchant/Branch/Policy بستگی دارد |

`identity_subject_id`، `merchant_id`، `branch_id`، `offer_preparation_mode`، `webhook_url` و URL برگشت آزاد بخشی از بدنهٔ این API نیستند. برای پرداخت ریالی، مبلغ را با واحد **IRR** ارسال کنید؛ اگر سیستم Merchant تومان ذخیره می‌کند، تبدیل واحد را قبل از درخواست با کنترل Overflow انجام دهید.

**HTTP:** برای ساخت جدید `201`، برای Replay همان عملیات `200`. پاسخ موفق در BNPL با Envelope `data` و `meta` می‌آید:

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

نمونه‌ها فقط شکل قرارداد را نشان می‌دهند؛ زمان‌ها، توکن و پیامِ Localized واقعی ممکن است فرق داشته باشند. `checkout_url` را **Opaque** نگه دارید و بازسازی یا دستکاری نکنید. `checkout_session_id` همان توکن عمومی مسیر Inquiry/Cancel/Return/Webhook است، نه UUID داخلی.

## ۶. تفاوت سروری `on_create` و `on_start`

**این دو Mode برای هر Merchant (Damz یا Digikala) ممکن‌اند؛ نوع Merchant نیستند.**

| رفتار | `on_create` | `on_start` (پیش‌فرض) |
| --- | --- | --- |
| Partner Create | پس از ساخت Session، موبایل از Customer Reference Service Resolve و Decision/Offer تهیه می‌شود | Session + handoff ساخته می‌شود؛ تهیهٔ Offer به START موکول می‌شود |
| اگر مشتری در Customer Reference موجود/فعال نباشد | ممکن است در Create به `unavailable` برسد | ممکن است در اولین START به `unavailable` برسد |
| `data.offer` در Partner Create | **می‌تواند وجود داشته باشد** | **وجود ندارد** |
| Hosted GET پیش از START | در صورت تهیهٔ موفق Offer می‌تواند آن را نمایش دهد | ممکن است هنوز Offer آماده نباشد |
| Hosted START | Offer قبلی را اعتبارسنجی/بازپخش می‌کند | Offer را آماده و نسخهٔ آن را برمی‌گرداند |
| ارسال OTP | فقط `POST .../otp` | فقط `POST .../otp` |
| تأیید خرید | همیشه OTP/Confirm طبق فلو یکسان | همان |

### قرارداد واقعی `offer` در Partner Create

در `on_create`، **برخلاف نسخه‌های قدیمی مستندات**، پاسخ `CreatePartner` می‌تواند `data.offer` داشته باشد. برای Outcome تأییدشده، Schema شامل این فیلدها است:

- `outcome` = `approved`؛ فیلدهای `requested_amount_minor`، `approved_financed_amount_minor`، `required_down_payment_amount_minor`، `currency`
- `available_credit_minor`، `offer_expires_at`، `installment_count`، `payable_now_amount_minor`، `total_payable_amount_minor`
- `installments[]` با `installment_order`، `due_at`، `total_amount_minor`
- `checkout_action` با `type`، `payment_required` و `payment_amount_minor`

برای Outcome `denied` فقط `outcome` در Offer منتشر می‌شود. `on_create` ممکن است به‌جای Offer، وضعیت `unavailable` تولید کند.

**هشدار حریم خصوصی/حداقل‌سازی داده:** در پیاده‌سازی فعلی Partner Create ممکن است `available_credit_minor` را برگرداند. بنابراین پاسخ را محرمانه و صرفاً در Backend/کانال مجاز پردازش کنید؛ آن را در Log عمومی یا Analytics غیرمجاز قرار ندهید. Merchant برای Fulfillment نباید از Offer یا available credit به‌عنوان مدرک پرداخت استفاده کند. این سند رفتار **فعلی کد** را شرح می‌دهد، نه توصیه به افشای چنین داده‌ای در آینده.

در مقابل، DTO مربوط به **Hosted GET/START/Confirm** عمداً `available_credit_minor` و شناسهٔ BNPL Customer را حذف می‌کند.

## ۷. فلو Hosted UI: دقیقاً `GET → START → OTP → CONFIRM`

این بخش برای تیم Frontend میزبانی‌شدهٔ BNPL و برای آگاهی QA/Backend Merchant است؛ Merchant Backend این Endpointها را صدا نمی‌زند.

1. **GET** `/checkout-sessions/{token}`: وضعیت، `merchant.display_name`، مبلغ، `evaluation_generation`، در صورت آماده بودن `offer`، `verification.type=sms_otp` و `verification.destination` ماسک‌شده را بخوانید. GET هیچ OTP نمی‌فرستد.
2. **START** با JSON اختیاری `{"evaluation_generation":N}`: اگر Offer هنوز آماده نیست (`on_start` نخستین بار)، Body خالی/`{}` یا Generation صفر قابل قبول است. اگر Offer از قبل نمایش داده شده، Generation مشاهده‌شده را بفرستید. پاسخ شامل Offer و Generation معتبر است. **START هیچ SMS ارسال نمی‌کند.**
3. **OTP**: `POST /checkout-sessions/{token}/otp` با `{"evaluation_generation":N}`. سرویس Challenge/Rate Limit را مدیریت و درخواست Notification را برای SMS صادر می‌کند یا Challenge فعال را بدون SMS دوم بازپخش می‌کند. پاسخ شامل `otp_sent`، `otp_expires_at`، `otp_next_issue_at` و `otp_availability` است.
4. **CONFIRM**: `POST /checkout-sessions/{token}/confirm` با `{"evaluation_generation":N,"otp":"123456"}`. OTP به همان Generation وصل است، یک‌بارمصرف است و برای همهٔ Partnerها لازم است. نتیجه می‌تواند `reconfirm_required=true` و Offer تازه بدهد؛ در این حالت هیچ تأییدی با OTP نسل قبل مجاز نیست و باید فلو START → OTP → CONFIRM برای Generation جدید تکرار شود.
5. **تکمیل پرداخت**: `payment_required=true` یعنی UI با `redirect_url` دریافتی ادامه دهد؛ صرف دریافت URL یا Browser Return تأیید مالی نیست. اگر `payment_required=false` و Checkout واقعاً `completed` باشد، UI می‌تواند به بازگشت Merchant هدایت شود.
6. **ابهام/Timeout در Confirm**: `GET .../confirmation-status` صرفاً Poll خواندنی است و Payment URL یا عملیات مالی جدید نمی‌سازد. مقادیر `confirmation_status` از جمله `not_started`، `processing`، `payment_action_required`، `payment_processing`، `reconfirm_required`، `retry_available`، `completed`، `needs_attention`، `canceled`، `expired` و `unavailable` هستند؛ فیلد `next_action` راهنمای اقدام UI است.

برای `confirmation-status`، حالت `payment_action_required` به این معنی نیست که Poll خودش لینک جدید می‌سازد؛ ادامهٔ مجاز پرداخت باید از فلو احراز هویت و Confirm پیروی کند. محدودیت‌های صدور مجدد OTP با `otp_next_issue_at` و `otp_availability` هدایت می‌شوند.

### Browser Return

`GET /api/v1/checkout-sessions/{token}/return` پاسخ **HTTP 302** به Return URL ثبت‌شده و ذخیره‌شده در Checkout است. پارامترهای خروجی صرفاً `checkout_session_id` و در صورت وجود `state=merchant_state` هستند. ورودی مثل `?url=...` مقصد را عوض نمی‌کند. صفحهٔ Merchant بعد از بازگشت باید Pending نشان دهد و از Backend Merchant وضعیت را بپرسد؛ از Query String موفقیت را استنتاج نکند.

## ۸. Inquiry و نهایی‌کردن سفارش

~~~http
GET /api/v1/partner/checkout-sessions/{checkout_session_id}
X-Terminal-Id: ...
X-API-Key: ...
~~~

`200` در `data`:

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

`completed_at` فقط هنگام تکمیل وجود دارد. Inquiry وضعیت را **نمی‌نویسد**؛ ممکن است Session بازِ گذشته از Deadline را در پاسخ `expired` نشان دهد، بدون اینکه الزاماً همان لحظه State در DB تغییر کند. دسترسی به Session Merchant دیگر `404` است تا امکان Enumeration ایجاد نشود.

پیش از Fulfill، دقیقاً این پنج مورد را با سفارش ذخیره‌شدهٔ Merchant تطبیق دهید: `checkout_session_id`، `merchant_order_id`، `amount_minor`، `currency` و `status == completed`. عملیات Fulfill باید در DB خود Merchant Idempotent و اتمیک باشد.

### وضعیت‌های Checkout برای Merchant

| `status` | معنی | Fulfill؟ |
| --- | --- | --- |
| `open` | Checkout قابل ادامه؛ Offer ممکن است هنوز تهیه نشده باشد | خیر |
| `confirming` | تأیید مشتری/آغاز عملیات در حال انجام | خیر |
| `processing` | نتیجهٔ معتبر عملیات مالی هنوز در راه است | خیر |
| `completed` | Checkout با نتیجهٔ معتبر کامل شده است | **بله، پس از تطبیق** |
| `canceled` | لغوشده توسط مشتری یا Merchant | خیر |
| `expired` | مهلت Checkout پایان یافته | خیر |
| `unavailable` | امکان ارائهٔ اعتبار/تکمیل پیشنهاد وجود ندارد | خیر |

هیچ وضعیت Merchant-facing به نام `failed` در این State Machine وجود ندارد؛ خطای پرداخت قابل تکرار ممکن است Checkout را دوباره به `open` ببرد. `confirming/processing` را پرداخت موفق یا قابل Cancel فرض نکنید.

## ۹. لغو Checkout توسط Merchant

~~~http
POST /api/v1/partner/checkout-sessions/{checkout_session_id}/cancel
X-Terminal-Id: ...
X-API-Key: ...
~~~

در صورت مجاز بودن، `200` و `data` شامل `checkout_session_id`، `status=canceled` و `return_url` است. لغو Session از پیش `canceled` Idempotent است. فقط Checkout باز و قابل لغو را می‌توان لغو کرد؛ اگر Confirm/Payment شروع شده یا نتیجه مبهم است، درخواست می‌تواند `409 CONFLICT` بدهد. `completed`، `expired` و `unavailable` قابل لغو نیستند. در Conflict، Inquiry کنید و پرداخت نامعلوم را «ناموفق» فرض نکنید.

## ۱۰. Webhook: امضا، Event و تحویل حداقل یک‌بار

وقتی Destination فعال باشد، رویدادهای وضعیت Checkout از Transactional Outbox برای Delivery HTTP آماده می‌شوند. انتشار حداقل یک‌بار است؛ دریافت تکراری کاملاً طبیعی است. اگر Webhook نرسید، Inquiry همچنان مسیر معتبر دریافت نتیجه است.

| `event_type` | `data.status` |
| --- | --- |
| `bnpl.checkout.completed.v1` | `completed` |
| `bnpl.checkout.canceled.v1` | `canceled` |
| `bnpl.checkout.expired.v1` | `expired` |
| `bnpl.checkout.unavailable.v1` | `unavailable` |

ساختار Envelope:

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

`completed_at` تنها روی رویداد تکمیل، `reason` در لغو (مثل `customer_canceled` / `merchant_canceled`) و `merchant_state` تنها در صورت وجود معنی‌دارند. هیچ شناسهٔ Financial Core، PSP، PaymentAttempt یا جزئیات اعتبار مشتری در Event نیست.

`event_id` از توکن عمومی + نوع Event + Status ساخته می‌شود و در Retry همان رویداد ثابت می‌ماند:

~~~text
{checkout_session_id}:{event_type}:{status}
~~~

### Headers و HMAC دقیق

| Header | مقدار |
| --- | --- |
| `Content-Type` | `application/json` |
| `User-Agent` | `bnpl-webhook/1` |
| `X-BNPL-Webhook-Id` | همان `event_id` |
| `X-BNPL-Webhook-Timestamp` | Unix seconds زمان **همین تلاش Delivery** |
| `X-BNPL-Webhook-Signature` | `v1=<hex lower-case HMAC-SHA256>` |
| `X-BNPL-Webhook-Event` | `event_type` در صورت وجود Header مربوطه در Outbox |

پیام Canonical برای HMAC دقیقاً اتصال UTF-8 این اجزاست:

~~~text
v1\n<event-id>\n<unix-timestamp-decimal>\n<raw-http-body>
~~~

یعنی بین سه فیلد اول LF واقعی قرار دارد و `body` **بایت‌های خام** HTTP است، نه JSON بازسازی‌شده. نمونهٔ مرجع برای Receiver:

~~~python
import hashlib
import hmac

def verify_webhook(secret: bytes, event_id: str, timestamp: str,
                   raw_body: bytes, received_signature: str) -> bool:
    canonical = (f"v1\n{event_id}\n{timestamp}\n").encode("utf-8") + raw_body
    expected = "v1=" + hmac.new(secret, canonical, hashlib.sha256).hexdigest()
    return hmac.compare_digest(expected, received_signature)
~~~

Receiver همچنین باید Timestamp را عددی و تازه بودن آن را نسبت به پنجرهٔ توافق‌شده کنترل کند، `event_id` را برای Dedupe در DB ذخیره کند، نتیجه را اتمیک اعمال کند و فقط بعد از ثبت پایدار `2xx` بدهد؛ Duplicate قبلاً ثبت‌شده نیز می‌تواند `2xx` بگیرد. روی رویداد `completed` هم ابتدا سفارش/مبلغ/ارز/توکن تطبیق شود.

Secret در سمت BNPL هنگام ارسال از Registry فعلی خوانده می‌شود؛ تغییر مقصد یا Rotation می‌تواند روی Deliveryهای معوق اثر بگذارد. گیرنده باید برنامهٔ Rotation و Retry داشته باشد.

## ۱۱. Idempotency، Error و سناریوهای شکست

### Idempotency

- اگر Create Timeout/قطع ارتباط داشت، **همان `idempotency_key` و همان مفاد تجاری** را دوباره بفرستید.
- `idempotency_key` در **Body Create** است، نه Header `Idempotency-Key` مربوط به Provision ترمینال.
- استفادهٔ دوباره از یک Key با شرایط تجاری یا `customer_phone` متفاوت، Conflict است. شماره Canonical توسط BNPL مقایسه می‌شود.
- برای تلاش تازه پس از وضعیت Terminal ناموفق (`canceled`/`expired`/`unavailable`)، **Key جدید** بدهید. یک سفارش `completed` نباید Checkout تازه بگیرد.
- اگر یک Checkout فعال برای همان `merchant_order_id` وجود دارد، با Key جدید نباید عملیات پرداخت موازی برای همان سفارش بسازید؛ ابتدا وضعیت قبلی را تعیین تکلیف کنید.
- Merchant باید نتیجهٔ Webhook و Fulfill را نیز Idempotent پردازش کند.

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
| `on_create` و `on_start` | اولی می‌تواند `data.offer` داشته باشد؛ دومی در Create ندارد |
| GET Hosted | بدون ارسال SMS یا ایجاد Decision |
| START Hosted | آماده‌سازی/بازیابی Offer، **بدون OTP** |
| OTP دوباره در Challenge معتبر | رفتار محدود و Idempotent، نه SMS بی‌قیدوشرط |
| Confirm با OTP/Generation نادرست | رد یا درخواست Reconfirm، بدون نهایی‌سازی غیرمجاز |
| Confirm Timeout | `confirmation-status` برای وضعیت؛ نه فرض Success |
| Webhook دوبار | یک Fulfillment |
| Browser Return پیش از Webhook | نمایش Pending و Inquiry Backend |
| Merchant Cancel همزمان با Confirm | ممکن است Conflict؛ Inquiry، بدون فرض لغو مالی |
| Inquiry برای Session Merchant دیگر | `404` |
| Session `completed` و Key جدید برای همان سفارش | عدم ایجاد Checkout دوم |
| انقضای Session در حالی که Open است | `expired`، عدم Fulfillment |

## ۱۲. نمونهٔ عملی Backend Merchant

نمونهٔ دستورات؛ آدرس‌ها و Credentialها باید برای محیط خودتان تنظیم شوند:

~~~bash
# BNPL_API_BASE is the origin only, e.g. https://api.example
curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/return-destinations/default" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"return_url":"https://shop.example/checkout/bnpl/return","enabled":true}'

curl -sS -X PUT "$BNPL_API_BASE/api/v1/partner/webhook-destination" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d '{"webhook_url":"https://shop.example/webhooks/bnpl","signing_secret":"<long-random-secret-at-least-32-bytes>","enabled":true}'

curl -sS -X POST "$BNPL_API_BASE/api/v1/partner/checkout-sessions" \
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY" \
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
  -H "X-Terminal-Id: $TERMINAL_ID" -H "X-API-Key: $API_KEY"
~~~

Backend Merchant باید پس از Create توکن، Key، شناسهٔ سفارش، مبلغ و ارز مورد انتظار و وضعیت را ذخیره کند. Frontend Merchant فقط:

~~~javascript
window.location.assign(checkout_url); // returned by BNPL; do not reconstruct
~~~

در صفحهٔ Return از Backend خودش نتیجهٔ Inquiry را بگیرد. **Merchant Backend هرگز `/start`، `/otp`، `/confirm` یا `/return` توکنی را صدا نمی‌زند.**

## ۱۳. چک‌لیست راه‌اندازی و مالکیت مسئولیت‌ها

**Merchant Backend**

- [ ] Terminal Credential را از پنل گرفته و در Secret Manager ذخیره کرده است.
- [ ] تمام Partner APIها را با دو Header ترمینال، در Backend و روی HTTPS صدا می‌زند.
- [ ] Permissionهای ترمینال برای Create/Read/Config روشن‌اند.
- [ ] `online_store_id` و `return_destination` درست Provision شده‌اند.
- [ ] Webhook Destination و Secret بلند/تصادفی ثبت شده‌اند.
- [ ] `customer_phone` در Create از منبع قابل اعتماد سفارش می‌آید؛ `customer_reference` هویت مشتری نیست.
- [ ] حالت `on_create` و `on_start` را بدون hardcode کردن نوع Merchant تحمل می‌کند.
- [ ] Idempotency Key/توکن/مبلغ/ارز سفارش را پایدار ذخیره می‌کند.
- [ ] فقط از Inquiry یا Webhook امضاشدهٔ `completed` با تطبیق کامل، سفارش را Fulfill می‌کند.
- [ ] Webhook را با Raw Body + HMAC + Timestamp + Dedupe دریافت می‌کند.
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
- `online_store_id`های واقعی و Mode هر Branch (`on_create` یا `on_start`).
- Return URL و نام Destination مورد توافق، Webhook URL و روش مدیریت Secret.
- اکانت‌های تست، شماره‌های مجاز تست OTP و دادهٔ آزمون مناسب محیط.
- پنجرهٔ توافق‌شدهٔ Freshness برای Webhook، Policy Retry و Runbook Rotation.
- سیاست دسترسی به `data.offer.available_credit_minor` در پاسخ Partner `on_create`؛ این فیلد **اکنون در کد وجود دارد** و تصمیم تغییر آن، موضوعی مستقل از مستندسازی است.

## ۱۵. نقشهٔ سورس برای نگه‌داری قرارداد

| بخش سند | مرجع پیاده‌سازی |
| --- | --- |
| Route و جدایی Auth Surfaceها | `pkg/adapter/restful/gin/routes/register.go`؛ `checkoutsessionrs/resource.go` |
| Terminal Auth و Permission | `middleware/partner_terminal_auth.go`؛ `merchantterminaluc/authenticate.go`؛ `merchantterminal/permissions.go` |
| Terminal lifecycle | `merchantterminalrs/resource.go`؛ `merchantterminaluc/provision.go` |
| Create، Schema و Offer | `checkoutsessionrs/create_partner.go`؛ `checkoutsessionrs/serdser.go`؛ `checkoutsessionrs/swagger_schema.go`؛ `checkoutsessionuc/create_partner.go`؛ `checkoutsessionuc/session.go` |
| Mode و Customer Resolution | `model/merchant_branch_profile.go`؛ `checkoutsessionuc/external_prepare.go` |
| Hosted GET/START/OTP/Confirm/Status/Return | `checkoutsessionrs/hosted_external.go`؛ `checkoutsessionrs/request_otp.go`؛ `checkoutsessionrs/confirmation_status.go`؛ `checkoutsessionuc/external_start.go`؛ `checkoutsessionuc/external_otp_request.go` |
| Inquiry/Cancel | `checkoutsessionuc/get_partner_status.go`؛ `checkoutsessionrs/cancel.go` |
| Webhook Event و Signature | `core/event/checkoutlifecycle/event.go`؛ `checkoutsessionuc/lifecycle_outbox.go`؛ `merchantwebhook/http/signature.go`؛ `merchantwebhook/http/dispatcher.go` |
| پاسخ‌های استاندارد | `pkg/adapter/restful/gin/serdser/success_response.go`؛ `error_response.go` |

مسیرهای بالا زیر `pkg/core/` یا `pkg/adapter/` مطابق نام پکیج قرار می‌گیرند. هنگام تغییر قرارداد، نخست DTO/Route/Use Case را بررسی و سپس **همین یک سند** را به‌روزرسانی کنید؛ مستند جداگانه برای Internal/External Partner ایجاد نکنید.


**قاعدهٔ نگه‌داری ترجمه:** هر تغییر در Route، DTO، Permission، Webhook یا State Machine باید همزمان در [فارسی](partner-checkout.md) و [English](partner-checkout.en.md) اعمال شود.
