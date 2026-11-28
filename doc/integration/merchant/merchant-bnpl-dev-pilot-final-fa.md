# قرارداد نهایی پایلوت Dev بین Merchant و BNPL

**مخاطب: مسعود و تیم BNPL · مبنا: `main` در `03fbda843`**

## تصمیم نهایی دربارهٔ داده‌های قدیمی

**ما فقط در محیط Dev هستیم و داده‌های قدیمی برای ما هیچ اهمیتی ندارند.** پایلوت را با دیتابیس و داده‌های تازه و ایزوله شروع می‌کنیم. هیچ پیام قدیمی برای پردازش نداریم. انتقال یا backfill داده‌های قبلی، replay دستی پیام‌های قدیمی، حفظ کلیدها و ترمینال‌های قبلی و هر نوع لایهٔ سازگاری یا مسیر Legacy خارج از محدوده است. فقط migration اولیه روی دیتابیس تازه اجرا می‌شود. این تصمیم مجوز پاک‌کردن دیتابیس فعلی یا تغییر تاریخچهٔ Flyway نیست.

تحویل مجدد **همان پیام جدید** هنوز ممکن است؛ idempotency Consumer و Callback باید برقرار بماند. سه فیلد پیشنهادی `application_id`، `status` و `owner_version` در GET فعلی وجود ندارند و افزودنشان پیش‌شرط این پایلوت نیست. برای این فاز آن‌ها را فیلد الزامی Adapter نکنید. اگر در آینده نیاز به بررسی وضعیت جاری یا لغو صریح مشارکت پیدا شد، آن قرارداد جداگانه طراحی می‌شود.

## محدوده و ترتیب پایلوت

- یک BNPL Sandbox مستقل، یک مرچنت تازه، یک فروشگاه آنلاین با نگاشت تأییدشده و یک ترمینال در سطح مرچنت. `LIVE`، چندترمینالی، کلید محدود به شعبه و هیچ پرداخت یا تسویهٔ واقعی در محدوده نیستند.
- Merchant مالک مرجع `merchant_id`، پرونده و تعلق `outlet_id` است. BNPL مالک حساب عملیاتی، سیاست اعتباری Sandbox، ترمینال، کلید API و نگاشت منبع خرید است. `subject_id` شناسهٔ اپراتور است؛ `online_store_id` با `outlet_id` یکی نیست. مرجع دائمی مالکیت Online Store هنوز تعیین نشده و در پایلوت از نگاشت تأییدشده استفاده می‌کنیم.
- بعد از دریافت درخواست، BNPL حساب را مستقل از Workspace، Administrator، Outlet، Terminal و Financial Core ایجاد می‌کند و نتیجهٔ ماندگار را گزارش می‌دهد. **`setup-eligibility` پیش‌شرط ساخت حساب نیست**؛ بعد از `READY` شدن حساب و آمادگی مدیر، برای راه‌اندازی فروشگاه چک می‌شود. صدور ترمینال و کلید یا مجازشدن Checkout نتیجهٔ خودکار `READY` نیست.
- نام پایلوت `bnpl-sandbox`، Client ID برابر `bnpl-sandbox`، Consumer Group برابر `bnpl-sandbox-consumer-group` و Quarantine Topic برابر `bnpl.sandbox.consumer.quarantine.v1` است. هویت امنیتی سرویس و جداسازی محیط مستقل از Consumer Group هستند. این نام‌ها به معنی ایجاد تاپیک یا ACL در Dev نیستند.

نمونه‌های این سند از کد استخراج شده‌اند و دادهٔ واقعی Dev یا پیام ثبت‌شده روی Broker نیستند. [قرارداد پایهٔ روی main](https://github.com/pinto-team/parstis_super_app/blob/03fbda843e275eed16f2260482ef0e255d790371/merchant-service/docs/bnpl-dev-pilot-contract.md).

## ۰. قرارداد Kafka از Merchant به BNPL

- Topic: `merchant.business.line.participation.requests.v1`
- Key: UTF-8 text of the Merchant-owned `merchant_id` UUID, not the operator's ID.
- Value: UTF-8 JSON. Fields are **top-level**, not inside a nested `payload`.

```json
{
  "event_id": "a5166f33-b950-4355-a835-a8216014d6be",
  "event_type": "merchant_product_participation_requested",
  "schema_version": 1,
  "correlation_id": "c4f139c9-8a44-42dd-9aad-6a0f60200b6d",
  "occurred_at": "2026-11-28T08:30:00Z",
  "actor_subject_id": "f014c11d-be85-47e2-adde-29618dcbb2f6",
  "actor_membership_id": null,
  "actor_context_id": null,
  "merchant_id": "c4f139c9-8a44-42dd-9aad-6a0f60200b6d",
  "application_id": "4d16a4fa-8bdc-4e50-9bb7-6d88fd7c1846",
  "request_id": "0bb456eb-945a-401d-b5d7-d51a974a7426",
  "business_line_code": "bnpl",
  "environment": "SANDBOX"
}
```

| Kafka header | UTF-8 value for the example |
| --- | --- |
| `x-event-id` | `a5166f33-b950-4355-a835-a8216014d6be` |
| `x-event-type` | `merchant_product_participation_requested` |
| `x-aggregate-type` | `merchant_enrollment` |
| `x-aggregate-id` | `c4f139c9-8a44-42dd-9aad-6a0f60200b6d` |
| `x-schema-version` | `1` (text header, integer in JSON) |
| `x-correlation-id` | Not set by the current Merchant publishing projection. The JSON still contains `correlation_id`. |

Actor metadata is not authorization and is not an account identifier. Use
`event_id` for replay, `request_id` for the durable provisioning command, and
`merchant_id + environment` for uniqueness of the effective account. Commit
the Kafka offset only after committing the result or handing off to durable
local work. A replay with different content is a conflict, not a second
provisioning command. Do not transmit terminals, keys, OTPs or documents.

## ۱. خواندن درخواست مشارکت

`GET /internal/v1/business-lines/bnpl/requests/{requestId}`، نمونهٔ کامل پاسخ ۲۰۰:

```json
{
  "data": {
    "merchant_id": "c4f139c9-8a44-42dd-9aad-6a0f60200b6d",
    "request_id": "0bb456eb-945a-401d-b5d7-d51a974a7426",
    "business_line": "bnpl",
    "environment": "SANDBOX",
    "contracting_subject_id": "f014c11d-be85-47e2-adde-29618dcbb2f6",
    "trade_name": "فروشگاه آزمایشی",
    "context_id": null,
    "workspace_status": "PENDING",
    "administrator_status": "PENDING",
    "commercial_status": "inactive",
    "base_application_status": "APPROVED"
  },
  "meta": {
    "status": "success",
    "code": "MERCHANT_ONBOARDING_OK",
    "message": "Merchant onboarding request completed.",
    "status_code": 200,
    "timestamp": "2026-11-28T08:30:00Z",
    "trace_id": null,
    "request_id": "http-req-example",
    "correlation_id": null,
    "pagination": null
  }
}
```

`context_id` ممکن است هنوز `null` باشد. `data.request_id` شناسهٔ مشارکت است؛ `meta.request_id` شناسهٔ درخواست HTTP. `trade_name` برای نمایش است. این سه مسیر داخلی با هویت سرویس، شناسهٔ گزارش‌دهندهٔ تنظیم‌شده و claim خط کسب‌وکار `bnpl` کنترل می‌شوند؛ Client ID کافکا نقش هویت امنیتی ندارد.

## ۲. Callback و Schema فیلد `issues`

`PUT /internal/v1/business-lines/bnpl/requests/{requestId}/progress`، نمونهٔ درخواست:

```json
{
  "event_id": "30dfce08-466e-4fe1-9c10-d170483b6fa2",
  "owner_version": 1,
  "status": "CHANGES_REQUESTED",
  "external_account_reference": null,
  "issues": [
    {
      "field": "business_lines",
      "code": "bnpl.profile_incomplete",
      "message": "Sandbox account configuration is incomplete."
    }
  ]
}
```

`issues` اختیاری و پیش‌فرضش `[]` است؛ حداکثر ۳۲ آیتم. هر آیتم سه رشتهٔ الزامی دارد. مقادیر مجاز `field`: `trade_name`، `contracting_subject_id`، `initial_merchant_admin_subject_id`، `business_lines`، `evidence_references`، `terms_version` و `self_declaration`. الگوی `code`: `^[a-z0-9._-]{1,64}$`. `message` غیرخالی، حداکثر ۵۰۰ کاراکتر و بدون کاراکتر کنترلی است. اطلاعات شخصی یا کلید در آن نگذارید. `CHANGES_REQUESTED` و `REJECTED` حداقل یک issue می‌خواهند. `READY` به `external_account_reference` غیرتهی و غیرمحرمانه مثل UUID پروفایل نیاز دارد؛ الگوی آن `^[A-Za-z0-9._:-]{1,128}$` است. در payload درخواست، `request_id` در URL است، نه در body. پاسخ ۲۰۰، snapshot جاری را در `data` با این فیلدها برمی‌گرداند: `request_id`، `code`، `environment`، `status`، `owner_version`، `external_account_reference`، `issues` و `credential_management_owner`.

## ۳. گذار وضعیت و retry

وضعیت‌های مجاز: `REQUESTED`، `UNDER_REVIEW`، `CHANGES_REQUESTED`، `ACCEPTED`، `PROVISIONING`، `READY`، `REJECTED` و `SUSPENDED`. Merchant فعلاً گراف گذار جداگانه‌ای تحمیل نمی‌کند؛ BNPL مالک چرخهٔ وضعیت است و گزارش باید snapshot کامل همان نسخه باشد. نسخهٔ مثبت و بالاتر اعمال می‌شود، حتی اگر بین نسخه‌ها فاصله باشد. نسخهٔ پایین‌تر وضعیت را عقب نمی‌برد؛ نسخهٔ برابر و محتوای برابر یا تکرار دقیق همان رویداد، ۲۰۰ و وضعیت جاری می‌دهد. نسخهٔ برابر با محتوای متفاوت یا استفادهٔ دوباره از `event_id` با محتوای متفاوت، ۴۰۹ می‌دهد. برای timeout یا ۵۰۳ همان payload، `event_id` و نسخه را دوباره بفرستید. در مسیر موفق می‌توانید `PROVISIONING` و بعد `READY` بدهید؛ نیازی نیست تمام وضعیت‌های میانی را بسازید. `READY` فقط آمادگی ماندگار حساب BNPL است.

## ۴. آمادگی راه‌اندازی فروشگاه

`GET /internal/v1/business-lines/bnpl/merchants/{merchantId}/environments/SANDBOX/outlets/{outletId}/setup-eligibility`

نمونهٔ `data`: `{"eligible_for_sandbox_setup":true,"environment":"SANDBOX"}`. برای زوج UUID معتبر ولی ناموجود یا غیرواجدشرایط، **۲۰۰ با `false`** برمی‌گردد. برای `true`، فروشگاه باید متعلق به مرچنت باشد، مدیر اولیه و مشارکت Sandbox هر دو `READY` باشند و مرچنت و فروشگاه در وضعیت `inactive` یا `active` باشند. محل فقط‌آماده‌سازی و فروشگاه مجازی بدون تأیید اینماد رد می‌شوند. این مسیر مالکیت `online_store_id`، آمادگی ترمینال یا مجوز Checkout را تأیید نمی‌کند. برای پایلوت همان نگاشت تأییدشده را جدا می‌دهیم.

## ۵. Context معتبر Merchant در پنل

`subject_id` فقط هویت اپراتور است. از توکن معتبر برای audience پنل و پشت Gateway مورد اعتماد، `subject_id`، `tenant_id`، `context_id` و `membership_id` بگیرید؛ هدر ارسالی از کلاینت مرجع اختیار نیست. در هر عملیات، Context باید فعال و از نوع Merchant باشد و به `merchant_id` هدف bind شده باشد. عضویت جاری باید فعال، متعلق به همان subject و همان tenant/context باشد. برای پایلوت، مجوز عملیات و scope جاری `ALL_OUTLETS` هم لازم‌اند؛ `SELECTED_OUTLETS` یا scope نامشخص را رد کنید. `context_id` در GET مشارکت صرفاً مرجع است و مجوز اپراتور نیست. Merchant فعلاً یک endpoint داخلی واحد برای resolve و تأیید تمام این شروط به BNPL نمی‌دهد؛ اتصال به منابع معتبر Identity/Merchant و دسترسی سرویس را در Dev جدا نهایی می‌کنیم. تا آن موقع عملیات پنل را با فرض معتبر بودن context باز نکنید.

در مسیر مالی، ذخیرهٔ ماندگار نوع و شناسهٔ دریافت‌کننده مطابق پاسخ Financial درست است. اگر آن پاسخ برای نوع دریافت‌کننده شناسه ندارد، همان نبودن شناسه را حفظ کنید و از `merchant_id` یا context مقدار نسازید. Consumer و Callback را می‌توانید بدون انتظار برای Financial جلو ببرید.

## ۶. تحویل زیرساخت و معیار قبولی

**سمت Merchant/زیرساخت:** Base URL داخلی و دسترسی سرویس BNPL، آدرس Broker و TLS/SASL و ACL، و Fixture شامل `merchant_id`، `outlet_id`، `online_store_id` و `request_id` با نگاشت تأییدشده را بعد از تأیید محیط تحویل می‌دهیم. دسترسی پیشنهادی BNPL برای این اتصال: `READ/DESCRIBE` روی تاپیک مشارکت، `READ` روی Consumer Group و `WRITE/DESCRIBE` روی Quarantine Topic. قرارداد منابع معتبر Authorization پنل و Financial Sandbox جداگانه نهایی می‌شوند.

**سمت BNPL:** Consumer و Callback ماندگار را بدون انتظار برای Financial پیاده کنید. با تکرار پیام یا قطع ارتباط، حساب یا ترمینال دوم نباید ایجاد شود. تا مکانیزم معتبر Context و Scope آماده نشده، عملیات پنل بسته بماند. زمان آمادگی Consumer و Callback را پس از بررسی Adapterها اعلام کنید.

**تست مشترک:** یک پروندهٔ تازه را تأیید کنیم، رویداد واقعی Kafka را با نمونهٔ سند مقایسه کنیم، حساب را بسازیم و Callback پایدار را ببینیم. تکرار پیام و Callback، رسیدن به آمادگی فروشگاه و رد منبع نامعتبر را جدا تست کنیم. خرید و لغو فقط بعد از اثبات ایزوله‌بودن Financial، روی دو Checkout آزمایشی جدا و با نوع لغو مشخص انجام می‌شوند. Recipient را دقیقاً مطابق پاسخ Financial ذخیره کنید؛ شناسهٔ غایب را با مقدار ساختگی پر نکنید.
