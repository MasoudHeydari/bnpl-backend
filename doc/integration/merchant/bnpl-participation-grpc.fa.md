# قرارداد gRPC مشارکت مرچنت برای مسعود

این مسیر مستقیم و داخلیِ BNPL به Merchant است. REST قبلی حفظ شده است. خوداظهاری و شمارهٔ نسخه به قرارداد موجود اضافه شده‌اند؛ نام تاپیک و نوع رویداد Kafka تغییر نکرده‌اند. موضوع Kafka همان `merchant.business.line.participation.requests.v1` و نوع پیام `merchant_product_participation_requested` است. Proto مرجع: [merchant_participation.proto](../contracts/external/participation/v1/proto/parstis/merchant/participation/v1/merchant_participation.proto). سرویس: `parstis.merchant.participation.v1.MerchantParticipationService`. برای Go از `protoc-gen-go` و `protoc-gen-go-grpc` استفاده کنید و مسیر `go_package` را با گزینهٔ `Mparstis/merchant/participation/v1/merchant_participation.proto=<module>/...` برای هر دو مولد به ماژول خودتان نگاشت کنید. Proto به فایل اختصاصی دیگری وابسته نیست. خطاهای غنی با انواع استاندارد `google.rpc` هستند.

## جریان Consumer و Callback

1. از Kafka، `event_id`، `request_id`، `merchant_id`، `application_id` و `environment` را بردارید. قبل از پردازش، محیط پیام را با محیط سرویس خودتان تطبیق دهید. Actor پیام، مجوز محسوب نمی‌شود.
2. با `GetParticipationRequest` و همان `request_id`، وضعیت فعلی را بخوانید. شناسه‌های Merchant و پرونده، business line و محیط را با پیام مقایسه کنید. فقط از روی پیام Kafka حساب جدید نسازید. `REJECTED`، `SUSPENDED` یا پروندهٔ غیر از `APPROVED` را به‌صورت فرمان ساخت تلقی نکنید. در `READY` حساب را دوباره نسازید. اگر `self_declaration` حاضر نیست یا نسخه صفر است، provisioning را انجام ندهید. برای نسخهٔ مثبتِ منطبق، هر سه مقدار را طبق سیاست BNPL بررسی کنید. اگر callback قبلاً در صف محلی ثبت شده است، نتیجهٔ همان عمل را بازیابی کنید. `trade_name` برای Hosted Checkout و پیامک، نه به‌عنوان نام حقوقی. `context_id` ممکن است هنوز حاضر نباشد و حتی وقتی حاضر است مدرک عضویت یا اختیار پنل نیست.
3. ساخت حساب BNPL و ثبت callback را به‌صورت idempotent و پایدار انجام دهید. پس از ثبت پایدار نتیجه یا انتقال به صف پایدار، offset را commit کنید. یکتایی حساب مؤثر روی `merchant_id + environment`، یکتایی ارزیابی روی `request_id + declaration_revision` و dedup پیام ورودی روی `event_id` است. رویداد قدیمی‌تر از GET جاری را پردازش نکنید. اگر رویداد جلوتر از GET است، دوباره بخوانید؛ با snapshot قدیمی اقدام نکنید. آماده‌شدن Financial Sandbox، شعبه، Terminal و مدیر را شرط ساخت حساب نکنید. `READY` یعنی حساب BNPL برای همین درخواست و محیط آماده شده است، نه اینکه فروش یا تسویه مجاز است.
4. با `ReportParticipationProgress` یک snapshot کامل بفرستید: `event_id` UUID یکتای **callback**، `request_id` UUID همان درخواست، `owner_version` مثبت و صعودی برای هر درخواست، `status`، نسخهٔ مثبت اظهارنامه در `declaration_revision` و فیلدهای اختیاری مربوط به آن. برای نبود اظهارنامه (نسخهٔ صفر) فیلد نسخهٔ Callback را ارسال نکنید؛ `ACCEPTED`، `PROVISIONING` و `READY` در این حالت رد می‌شوند. همهٔ UUIDها در فرم استاندارد کوچک‌حرف. هر بار تلاش مجدد با **همان event ID، همان version و همان payload**. درخواست موفق، وضعیت جاری Merchant را برمی‌گرداند، حتی اگر گزارش قدیمی یا تکراری باشد. نسخهٔ کوچک‌تر وضعیت را عقب نمی‌برد؛ نسخهٔ برابر فقط برای وضعیت و محتوای برابر پذیرفته می‌شود؛ event ID یکسان با محتوای متفاوت تعارض است. مالک BNPL ترتیب وضعیت‌ها را تعیین می‌کند؛ Merchant براساس نسخهٔ مالک snapshot را اعمال می‌کند، نه براساس ترتیب دریافت یا یک ترتیب عمومی بین enumها. نسخهٔ projection اولیه ۰ است.
5. `GetOutletSetupEligibility` با `merchant_id` و `outlet_id`، فقط برای SANDBOX است. `true` یعنی شعبه از نظر Merchant برای راه‌اندازی سندباکس واجد شرایط است: تعلق به همان Merchant، وضعیت قابل‌استفاده، آماده بودن حساب سندباکس و مدیر، و نداشتن مانع دارک‌استور/اینماد. شعبهٔ ناموجود، متعلق به مرچنت دیگر یا غیرواجد شرایط `false` است. این متد ترمینال نمی‌سازد، مجوز کاربر پنل یا مجوز خرید صادر نمی‌کند و مانع provisioning حساب نیست.

### نمونهٔ پاسخ خواندن درخواست قبل از آماده‌شدن مدیر

نمونه در قالب JSON متناظر با Proto است. ارتباط واقعی Protobuf باینری است؛ در Go از enumها و presence واقعی فیلدهای optional استفاده کنید. `ownerVersion` از نوع int64 است.

```json
{
  "merchantId": "c4f139c9-8a44-42dd-9aad-6a0f60200b6d",
  "applicationId": "4d16a4fa-8bdc-4e50-9bb7-6d88fd7c1846",
  "contractingSubjectId": "f014c11d-be85-47e2-adde-29618dcbb2f6",
  "tradeName": "فروشگاه آزمایشی",
  "workspaceStatus": "PENDING",
  "administratorStatus": "PENDING",
  "commercialStatus": "inactive",
  "baseApplicationStatus": "APPROVED",
  "participation": {
    "requestId": "0bb456eb-945a-401d-b5d7-d51a974a7426",
    "businessLineCode": "bnpl",
    "environment": "PARTICIPATION_ENVIRONMENT_SANDBOX",
    "status": "PARTICIPATION_STATUS_REQUESTED",
    "ownerVersion": "0",
    "declarationRevision": "1",
    "credentialManagementOwner": "bnpl"
  },
  "selfDeclaration": {
    "maxAmountMinor": "1000000",
    "maxAmountCurrency": "IRR",
    "maxTenureMonths": 6
  }
}
```

## ثبت و اصلاح خوداظهاری

در draft یا درخواست خط جدید، زیر انتخاب `code: "bnpl"` فیلد `self_declaration` را با سه فیلد زیر بفرستید. مقدارها را کاربر مرچنت اظهار می‌کند، نه Worker یا Seed. Merchant فقط ساختار را می‌سنجد: مبلغ مثبت تا سقف `int64`، مدت مثبت تا سقف `int32` و کد ارز سه حرف بزرگ. حدود تجاری، فهرست ارزهای قابل قبول و واحد دقیق را BNPL تعیین می‌کند. `IRR` به معنی ریال است، نه تومان. مثال صرفاً مصنوعی است و policy را تعیین نمی‌کند.

```json
{
  "command_id": "b1c76f35-6a19-4387-a0d5-1b08d3697ea2",
  "expected_declaration_revision": 1,
  "self_declaration": {
    "max_amount_minor": 1000000,
    "max_amount_currency": "IRR",
    "max_tenure_months": 6
  }
}
```

این نمونه بدنهٔ `PUT /api/v1/merchant-onboarding/bnpl/participations/{requestId}/self-declaration` از مسیر Envoy است. این endpoint فقط برای مرچنت و عضویت مجاز همان کاربر، روی درخواست SANDBOX در `CHANGES_REQUESTED` یا برای تکمیل نخستین اظهارنامهٔ درخواست `REQUESTED` بدون نسخه باز است. `expected_declaration_revision` برای تکمیل نخستین بار صفر است. تکرار همان `command_id` با بدنهٔ یکسان درخواست جدید یا رویداد جدید نمی‌سازد؛ اگر دو اصلاح رقابت کنند یکی می‌برد. همان `request_id` و `owner_version` حفظ می‌شوند، وضعیت به `REQUESTED` برمی‌گردد و در همان تراکنش نسخهٔ مثبت بعدی و رویداد تازه ثبت می‌شوند. پیام Kafka فقط `declaration_revision` را به داده‌های قبلی اضافه می‌کند، نه ارقام. نسخه‌ای که رد شده قابل بازنویسی نیست. Callback نسخهٔ قدیمی با `business_line.declaration_revision_conflict` رد می‌شود، حتی اگر `owner_version` بزرگ باشد. این خطا را با retry خودکار حل نکنید؛ اول snapshot جدید را بخوانید.

## Resolve مجوز پنل BNPL

`GET /api/v1/merchant-onboarding/bnpl/setup-workspace` را با **Bearer کاربر در Context مرچنت** از Envoy صدا بزنید، بدون فرستادن subject/merchant/context/membership در query یا body. مسیر داخلی Merchant پس از rewrite برابر `/merchants/onboarding/bnpl/setup-workspace` است. BNPL نباید هدر اعتماد Gateway را تولید کند و نباید توکن را صرفاً decode کند. توکن سرویس یا mTLS پایلوت جای توکن کاربر نیست. استفاده از این مسیر به JWT audience معتبر همان مسیر و تنظیم صحیح Envoy Dev وابسته است.

پاسخ موفق در `data` فیلدهای `merchant_id`، `context_id`، `membership_id` و `outlet_scope: "ALL_OUTLETS"` دارد. Merchant نگاشت Context به Merchant را از DB خودش می‌خواند. Context فعال از نوع merchant، Membership فعالِ همان subject/context/tenant از Identity و مجوز `merchant:enrollment:manage:merchant` الزامی‌اند. نه دسترسی `:any` به‌تنهایی و نه scope شعبه‌ای کافی نیست. این تصمیم فقط برای **راه‌اندازی سراسری BNPL همان مرچنت** است، نه مجوز همهٔ عملیات پنل، خرید یا تسویه. قبل از هر عملیات مربوط دوباره Resolve کنید و Merchant خروجی را با منبع BNPL تطبیق دهید؛ پاسخ را به‌عنوان مجوز بلندمدت cache نکنید. اگر مسیر یا Identity قابل‌تأیید نیست، عملیات بسته بماند. قرارداد دسترسی‌های دیگر پنل جداست.

### قواعد گزارش

- `READY`: `external_account_reference` الزامی است؛ شناسهٔ داخلی و غیرحساس با الگوی `[A-Za-z0-9._:-]{1,128}`. Terminal ID، API key یا خروجی خام Provider را نفرستید.
- `CHANGES_REQUESTED` و `REJECTED`: حداقل یک issue لازم است. حداکثر ۳۲ issue. فیلدهای مجاز: `trade_name`، `contracting_subject_id`، `initial_merchant_admin_subject_id`، `business_lines`، `evidence_references`، `terms_version`، `self_declaration`. کد با الگوی `[a-z0-9._-]{1,64}` و متن خوانا حداکثر ۵۰۰ کاراکتر بدون control character، اطلاعات شخصی یا secret.
- مثال issue: `{"field":"business_lines","code":"bnpl.outlet_attestation_required","message":"مدرک تأیید فروشگاه لازم است."}`. ورودی issue را ثابت و کنترل‌شده تولید کنید؛ دادهٔ خام لاگ یا Provider را وارد نکنید.
- وضعیت‌های گزارش: `REQUESTED`، `UNDER_REVIEW`، `CHANGES_REQUESTED`، `ACCEPTED`، `PROVISIONING`، `READY`، `REJECTED` و `SUSPENDED`. مقادیر `UNSPECIFIED` یا ناشناخته مجاز نیستند.

## خطاها و Retry در Go

از `status.FromError(err)` و سپس `Details()` استفاده کنید. در پاسخ‌های غنی، `google.rpc.ErrorInfo` دارای domain ثابت `parstis.merchant.participation.v1` است. `reason` کد پایدار Merchant است؛ تصمیم را از متن خطا نگیرید. خطای فیلد `google.rpc.BadRequest` با مسیر snake_case و توضیح ثابت دارد. `RetryInfo` و delay از سرور تضمین نمی‌شود. قطع TLS، deadline بیرون از سرور و قطع شبکه ممکن است فقط status حمل‌ونقل داشته باشند.

| gRPC status | نمونه reason یا شکل پاسخ | اقدام |
|---|---|---|
| `OK` | snapshot جاری، شامل replay و stale | Account را دوباره نسازید و نتیجهٔ جاری را بپذیرید. |
| `INVALID_ARGUMENT` | `business_line.invalid_request`، `business_line.invalid_correlation_id` یا `business_line.declaration_required` | ورودی را اصلاح کنید. همان payload را retry نکنید. |
| `UNAUTHENTICATED` / `PERMISSION_DENIED` | `business_line.service_authentication_required` / `business_line.reporter_forbidden` | زنجیرهٔ TLS، URI SAN و تنظیم workload را بررسی کنید. |
| `NOT_FOUND` | `business_line.request_not_found` | شناسه/محیط/خط تجاری را بررسی کنید. تفاوت رکورد ناموجود و محیط دیگر عمداً افشا نمی‌شود. |
| `FAILED_PRECONDITION` | `business_line.declaration_revision_conflict`، `business_line.version_conflict`، `business_line.event_identity_conflict`، `business_line.request_not_active`، `business_line.secret_material_forbidden` یا `business_line.sandbox_setup_only` | خطای قطعی. نسخه یا event تازه برای پنهان‌کردن تعارض تولید نکنید؛ وضعیت را reconcile کنید. |
| `UNAVAILABLE` / `DEADLINE_EXCEEDED` | خطای موقت یا نتیجهٔ نامعلوم | برای خواندن و callback همان درخواست را با backoff، jitter و سقف تلاش تکرار کنید. اگر مشکل TLS پایدار است، برای اصلاح به عملیات ارجاع دهید. |
| `INTERNAL` | `business_line.internal_error` | گزارش پایدار را نگه دارید و بررسی عملیاتی کنید. آن را موفق حساب نکنید. |

## احراز سرویس و مواردی که تیم Merchant باید تحویل دهد

- مقصد، **DNS داخلی Merchant با پورت TCP 9090** (یا پورت صریح استقرار) است؛ مقصد نهایی بعد از تأیید Dev اعلام می‌شود. از `api.dev.dlend.ir` و Envoy عمومی استفاده نکنید. سرور باید DNS نام مقصد را در SAN داشته باشد. Do not disable hostname verification.
- mTLS اجباری است. BNPL با cert/key اختصاصی client و CA مورداعتمادِ سرور Merchant وصل می‌شود. cert پایلوت باید URI SAN برابر `spiffe://parstis/service/bnpl-sandbox` داشته باشد. CN، JWT کاربر، هدر Envoy، Kafka client ID و consumer group جایگزین این هویت نیستند. اعتبارنامهٔ LIVE باید از SANDBOX جدا باشد. کلید خصوصی فقط از کانال امنِ استقرار در اختیار مالک workload قرار می‌گیرد.
- در Merchant، CA مربوط به client، هویت مجاز، UUID سوژهٔ سرویس برای audit و محیط `SANDBOX` به‌صورت صریح ثبت می‌شوند. فعال‌سازی اختیاری است و به `MERCHANT_SELF_SERVICE_ENABLED=true` نیاز دارد. مسیرهای cert/key باید PEM خواندنیِ runtime را نشان دهند؛ کلید خصوصی داخل image، Git، لاگ یا فایل ارسالی قرار نمی‌گیرد.
- تنظیمات Merchant: `MERCHANT_PARTICIPATION_GRPC_ENABLED` (پیش‌فرض false)، `..._HOST`، `..._PORT`، `..._TLS_CERTIFICATE_FILE`، `..._TLS_PRIVATE_KEY_FILE`، `..._TLS_CLIENT_CA_FILE`، `..._REPORTER_SERVICE_NAME`، `..._REPORTER_SUBJECT_ID`، `..._ENVIRONMENT`. transport فعلی فقط یک هویت/محیط در هر listener را می‌پذیرد. جداسازی شبکه و CA همچنان مسئولیت استقرار است.
- `x-correlation-id` اختیاری است؛ حداکثر ۱۲۸ کاراکتر ASCII از حروف/اعداد یا `._:-`. فقط برای trace/audit است و idempotency را تعیین نمی‌کند.
- برای تست Dev باید آدرس نهایی + CA عمومی، گواهی هویت پایلوت از مسیر امن، ACL و اتصال Kafka، درخواست تازه با شناسهٔ واقعی Dev و fixture یک outlet مشخص را هماهنگ کنیم. سپس round-trip callback، replay، رد هویت دیگر، و خرید/لغو سندباکس با Financial بررسی شوند. این سند به معنی provision یا deploy نیست.
- محیط Dev از دادهٔ تازه استفاده می‌کند؛ داده‌های قدیمی برای این پایلوت مهم نیستند و backfill/compatibility layer در محدوده نیست. این تغییر V1 دیتابیس Merchant را نیز توسعه می‌دهد: در Dev از دیتابیس تازه با مایگریشن و runtime-grants استفاده کنید. روی دیتابیس با V1 قبلی repair یا بازنویسی checksum نکنید. این تغییر به معنی اجرای migration یا reset نیست. عملیات محافظت‌شده تا تست Envoy، Context و Membership واقعی باز نشوند.
