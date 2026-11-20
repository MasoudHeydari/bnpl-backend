# لینک کوتاه پیامک بازپرداخت ← صفحهٔ اقساط

**مخاطب:** تیم فرانت‌اند (وب/اپ مشتری)  
**مالک:** BNPL Backend  
**آخرین به‌روزرسانی:** ۲۰۲۶-۱۱-۲۰  
**مستندات مرتبط:** [راهنمای یکپارچگی فرانت و بک‌اند](bnpl-frontend.fa.md)، [راهنمای HTTPS محلی (mkcert)](../local-development/mkcert/README.md)  
**زبان دیگر:** [English](repayment-short-link-redirect.en.md)

---

## ۰. خلاصهٔ سریع

1. لینک پیامک این شکلی است: `https://<SHORT_LINK_PUBLIC_HOST>/s/<CODE>` — مثلاً `https://dev.dlend.ir/s/aB72xQ`.
2. BNPL به همان درخواست **`302 Found`** با هدر `Location: <SHORT_LINK_REPAYMENT_DESTINATION_URL>` (امروز `…/installments`) برمی‌گرداند و **مرورگر** خودش ناوبری را انجام می‌دهد.
3. آدرس صفحهٔ مقصد **هیچ** کد کوتاه، شناسهٔ قسط یا شناسهٔ مشتری ندارد؛ یک مقصد ثابت است.
4. کار شما در `/installments`: احراز هویت کاربر، صدا زدن `GET /api/v1/installments`، نمایش وضعیت بازپرداخت، و شروع پرداخت (بند ۴.۲ single-pay یا بند ۴.۳ repayment batch).
5. لینک کوتاه را با `fetch()`/XHR صدا نزنید؛ این یک endpoint ناوبری مرورگر است، نه API.

## ۱. قرارداد URL

| محیط | لینک کوتاه عمومی | مقصد ریدایرکت (تنظیم‌شده) | Origin API |
| --- | --- | --- | --- |
| Local dev | `https://dpay.ir:8443/s/<CODE>` | `https://dpay.ir/installments` (یا `https://dpay.ir:5173/installments` وقتی Vite صفحه را سرو می‌کند) | `https://api.dpay.ir:8443` |
| Hamravesh dev | `https://dev.dlend.ir/s/<CODE>` | `https://dev.dlend.ir/installments` | `https://api.dev.dlend.ir` |

قواعد:

- هاست از `SHORT_LINK_PUBLIC_HOST` می‌آید و پیشوند مسیر `/s` است (`shortlinkuc.PublicPathPrefix`). **هاست، پورت یا URL محیط را در فرانت هاردکد نکنید**؛ از کانفیگ استقرار بخوانید.
- کد یک سگمنت مبهم `1..64` کاراکتری از `[A-Za-z0-9_-]` است (کدهای تولیدی ۸ کاراکتر alphanumeric هستند). به بزرگی/کوچکی حروف حساس است؛ آن را decode یا حدس نزنید.
- روت Resolver فقط **GET** است؛ `HEAD`/`POST` روی آن ریدایرکت نیست.
- درخواست باید دقیقاً روی همان هاست عمومی تنظیم‌شده بیاید. همان کد روی هاست دیگر طبق طراحی `404` می‌دهد.

## ۲. BNPL چه می‌کند (چیزی برای پیاده‌سازی شما نیست)

~~~text
کاربر روی لینک SMS می‌زند
        │
        ▼
GET https://<SHORT_LINK_PUBLIC_HOST>/s/<CODE>
        │  Envoy (بدون نیاز به JWT؛ Host حفظ می‌شود)
        ▼
Short-link resolver در BNPL
        │  302 Found
        │  Location: https://<frontend>/installments
        ▼
مرورگر صفحهٔ /installments شما را بارگذاری می‌کند
~~~

پاسخ‌های Resolver:

| Status | چه زمانی | Body |
| --- | --- | --- |
| `302` | کد معتبر، منقضی‌نشده، باطل‌نشده و هاست درست | بدنهٔ ریدایرکت بی‌اهمیت است — فقط `Location` را بخوانید |
| `400` | کد خالی/بدشکل | **بدنهٔ خالی**، بدون JSON |
| `404` | کد ناشناخته/باطل/منقضی؛ هاست اشتباه؛ خطای داخلی (ماسک‌شده) | **بدنهٔ خالی**، بدون JSON |

همهٔ پاسخ‌ها `X-Trace-Id`، `X-Request-Id` و `X-Correlation-Id` دارند، به‌علاوه:

- `Cache-Control: no-store`
- `Pragma: no-cache`
- `Referrer-Policy: no-referrer`

نتیجه‌هایی که باید در طراحی لحاظ کنید:

- **بدنهٔ ریدایرکت را parse نکنید** و به `Referer` تکیه نکنید — با `Referrer-Policy: no-referrer` حذف می‌شود. اگر به انتساب «از SMS آمده» نیاز دارید، باید در لبه (لاگ‌های gateway روی `302`) اندازه‌گیری شود، نه در صفحه.
- پاسخ‌ها cache نمی‌شوند؛ هر کلیک دوباره کد را سمت سرور اعتبارسنجی می‌کند.
- **هیچ فراخوانی API** برای resolve کردن کد لازم (یا ممکن) نیست؛ صفحه حتی کد را نمی‌بیند.

## ۳. صفحهٔ مقصد چه باید بکند

مسیر مقصد (امروز `/installments`) توسط ops در `SHORT_LINK_REPAYMENT_DESTINATION_URL` تنظیم می‌شود. اپ شما باید همان مسیر را روی هاست عمومی سایت سرو کند. اگر روت واقعی شما چیز دیگری است (مثلاً `/repayment`)، از ops بخواهید مقدار env را عوض کند؛ داخل اپ ساکت به مسیر دیگری ریدایرکت نکنید.

هنگام بارگذاری صفحه:

1. **احراز هویت.** اگر session معتبر نیست، فلوی لاگین Identity را شروع کنید و بعد به همان مسیر برگردید (مثلاً `?next=/installments`). اطلاعات بازپرداخت را به کاربر ناشناس نشان ندهید.
2. **هویت نفرستید.** `Authorization: Bearer <identity-access-token>` بفرستید و بگذارید gateway هدرهای `X-Auth-*` را تزریق کند. هرگز `X-Auth-Subject-Id`، `X-Auth-Tenant-Id`، `X-Auth-Context-Id` یا `X-Auth-Permissions` را از مرورگر نفرستید و هیچ‌جا شناسهٔ مشتری یا شمارهٔ موبایل نگذارید.
3. **وضعیت بازپرداخت کل مشتری را بخوانید:**

   ~~~http
   GET /api/v1/installments?limit=20 HTTP/1.1
   Host: <api-origin>
   Authorization: Bearer <identity-access-token>
   Accept: application/json
   Accept-Language: fa-IR
   ~~~

   مجوز: `bnpl:installments:read:self`.

4. **حالت‌ها را رندر کنید:**

   | حالت | نشانه | UI |
   | --- | --- | --- |
   | قسط پرداخت‌نشده ندارد | `200` و `data.installments: []` | حالت موفق/خالی («قسط پرداخت‌نشده‌ای ندارید») — **هرگز** آن را 404/خطا تلقی نکنید |
   | قسط سررسید/معوق دارد | `data.repayment.due_installments[]`، `due_count`، `unpaid_count` | مبلغ، تاریخ سررسید، جریمهٔ دیرکرد و وضعیت را نشان دهید |
   | قابل پرداخت | `data.repayment.next_payable` با `payable_amount_known: true` و بدون `payment_block_reason` | دکمهٔ «پرداخت» برای همان قسط فعال شود |
   | پرداخت در جریان | `has_pending_payment: true` | «پرداخت در حال انجام»؛ به‌جای شروع پرداخت جدید، read endpoint را poll کنید |
   | مسدود | وجود `payment_block_reason` | دلیل مسدودی را توضیح دهید؛ پرداخت را فعال نکنید |
   | احراز هویت نشده | `401` از API | لاگین و بازگشت به همین صفحه |

5. **شروع پرداخت.** دقیقاً یک مسیر را انتخاب کنید و همان را برای آن UX نگه دارید (بند ۴.۲ و ۴.۳):
   - «پرداخت قسط بعدی» با یک تپ: `POST …/pay` فقط برای `data.repayment.next_payable`؛
   - «تسویهٔ همهٔ معوقات در یک پرداخت»: ساخت repayment batch با همهٔ اقساط معوق و سپس start کردن آن.

   این دو را برای یک قسط مخلوط نکنید: تا وقتی یک batch روی قسط claim دارد، single-pay رد می‌شود (`409` یا `422` وقتی گارد claim دیتابیس فعال شود) و وقتی یک تلاش پرداخت در جریان است نباید batch ساخت. روی `202`/`200` با `redirect_url`، مرورگر را با `window.location.assign(redirect_url)` به آن ببرید.

6. **بعد از بازگشت از درگاه**، بازگشت مرورگر را مدرک پرداخت ندانید. `GET /api/v1/installments` (و/یا قرارداد) را دوباره بخوانید تا قسط از لیست پرداخت‌نشده حذف شود یا `has_pending_payment` پاک شود؛ وضعیت معتبر را نشان دهید. تسویه را همیشه بک‌اند تعیین می‌کند، نه ریدایرکت.

## ۴. فراخوانی‌های API

مشخصات تعاملی: `<api-origin>/bnpl/swagger/index.html` (Swagger UI در محیط dev فعال است؛ gateway مسیر `/bnpl/swagger` را به `/swagger` بک‌اند map می‌کند).

### ۴.۱ خواندن اقساط

~~~http
GET /api/v1/installments?limit=20&after_id=<uuid>&after_due_at=<RFC3339Nano>
~~~

پاسخ (`200`):

~~~json
{
  "data": {
    "installments": [
      {
        "id": "0a1b2c3d-4e5f-6789-abcd-ef0123456789",
        "contract_id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
        "merchant_profile_id": "8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c",
        "merchant_branch_profile_id": "9f2b3c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d",
        "installment_order": 2,
        "principal_amount_minor": 25000000,
        "total_amount_minor": 25000000,
        "late_fee_amount_minor": 0,
        "customer_payable_amount_minor": 25000000,
        "currency": "IRR",
        "due_at": "2026-09-09T12:00:00Z",
        "grace_period_days": 4,
        "status": "due"
      }
    ],
    "repayment": {
      "due_installments": [],
      "due_count": 1,
      "unpaid_count": 3,
      "next_payable": {
        "installment_id": "0a1b2c3d-4e5f-6789-abcd-ef0123456789",
        "contract_id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
        "installment_order": 2,
        "payable_amount_minor": 25000000,
        "outstanding_amount_minor": 25000000,
        "currency": "IRR",
        "due_at": "2026-09-09T12:00:00Z",
        "status": "due",
        "has_pending_payment": false,
        "payable_amount_known": true
      },
      "has_pending_payment": false
    }
  },
  "meta": {
    "status": "success",
    "code": "UNPAID_INSTALLMENTS_LISTED",
    "status_code": 200,
    "pagination": {
      "type": "keyset",
      "returned": 1,
      "has_next": false,
      "has_previous": false,
      "keyset": { "limit": 20 }
    }
  }
}
~~~

نکات:

- آیتم‌ها **نزدیک‌ترین سررسید اول** مرتب‌اند (`due_at ASC`). فقط اقساط `scheduled`، `due` و `overdue` که قرارداد دارند برمی‌گردند.
- `data.repayment` **در سطح کل مشتری** است و به صفحهٔ جاری محدود نیست.
- `customer_payable_amount_minor` ممکن است `null` باشد وقتی مبلغ سمت سرور هنوز قطعی نیست.
- صفحه‌بندی keyset است: تا وقتی `has_next` درست است، `meta.pagination.keyset.next_after_id` و `next_after_due_at` را به‌عنوان `after_id` و `after_due_at` درخواست بعدی بفرستید.

### ۴.۲ شروع پرداخت یک قسط

~~~http
POST /api/v1/contracts/{contract_id}/installments/{installment_id}/pay HTTP/1.1
Host: <api-origin>
Authorization: Bearer <identity-access-token>
Content-Type: application/json
Idempotency-Key: <stable-unique-key-for-this-intention>
Accept-Language: fa-IR

{ "channel": "gateway" }
~~~

مجوز: `bnpl:payments:start:self`. هدر `Idempotency-Key` **الزامی** است (حداکثر ۲۵۶ کاراکتر)؛ برای retry همان قصد کاربر، همان key را دوباره بفرستید.

پاسخ: `202 Accepted` برای تلاش تازه و `200 OK` برای replay idempotent.

~~~json
{
  "data": {
    "result": "queued",
    "payment_attempt": {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "installment_id": "0a1b2c3d-4e5f-6789-abcd-ef0123456789",
      "amount_minor": 25000000,
      "currency": "IRR",
      "state": "created",
      "channel": "gateway",
      "redirect_url": "https://pay.example/session/abc",
      "expires_at": "2026-09-09T12:30:00Z"
    }
  },
  "meta": { "status": "success", "code": "PAYMENT_INTENT_CREATED", "status_code": 202 }
}
~~~

قواعد:

- `redirect_url` و `expires_at` اختیاری‌اند. اگر `redirect_url` نبود، ناوبری **نکنید** — حالت «در انتظار» نشان دهید و دوباره اقساط را بخوانید.
- فقط **قسطِ دارای اولویت سراسریِ انتخاب‌شده توسط سرور** قابل پرداخت است: همان `data.repayment.next_payable`. ترتیب داخل قرارداد هم اعمال می‌شود. پرداخت هر قسط دیگر با `409` رد می‌شود (`error.installment_is_not_the_current_priority_repayment_target` یا `error.installment_is_not_the_next_payable_installment_in_sequence`). `error.installment_already_has_an_active_payment_attempt` یعنی یک تلاش پرداخت یا claim یک batch فعال از قبل وجود دارد — به‌جای retry، وضعیت را دوباره بخوانید.
- `channel` یکی از `gateway`، `bank_transfer` یا `wallet` است.
- `POST`های مالی را کورکورانه retry نکنید؛ همان `Idempotency-Key` را دوباره بفرستید و بعد از نتیجهٔ مبهم، read endpoint را دوباره بخوانید.

### ۴.۳ تسویهٔ چند قسط با هم (repayment batch)

این مسیر را فقط وقتی استفاده کنید که UX محصول «تسویهٔ همهٔ معوقات در یک پرداخت» است. single-pay و batch هر دو قرارداد معتبرند؛ بر اساس UX یکی را انتخاب کنید و برای یک قسط هرگز هر دو را مخلوط نکنید.

~~~http
POST /api/v1/repayment-batches HTTP/1.1
Host: <api-origin>
Authorization: Bearer <identity-access-token>
Content-Type: application/json
Idempotency-Key: <stable-unique-key-for-this-intention>

{ "installment_ids": ["<installment-id>", "<installment-id>"] }
~~~

مجوز: `bnpl:payments:start:self`. پاسخ: `202` با `{ "id", "status", "total_amount_minor", "currency" }`.

سرور انتخاب را اعتبارسنجی می‌کند و تخلف را با `409` رد می‌کند:

1. **همهٔ** اقساط معوق مشتری — در هر دو محصول و همهٔ merchantها — باید داخل انتخاب باشند.
2. داخل یک قرارداد نمی‌توان قسط پرداخت‌نشدهٔ زودتر را رد کرد.
3. اقساط `scheduled`/`due` که هنوز معوق نشده‌اند اختیاری‌اند.

سپس batch را start کنید:

~~~http
POST /api/v1/repayment-batches/{id}/start HTTP/1.1
Authorization: Bearer <identity-access-token>
Idempotency-Key: <stable-unique-key-for-this-start>
~~~

`202` با `{ "id", "status", "redirect_url"? }`. اگر `redirect_url` بود مرورگر را به آن ببرید؛ `status` در پاسخ start با واژگان Financial Core است، نه enum وضعیت BNPL.

برای وضعیت ماندگار BNPL، `GET /api/v1/repayment-batches/{id}` را poll کنید: `created`، `prepared`، `starting`، `requires_action`، `reconciling`، `accounting_pending`، `posted`، `failed`، `expired`، `manual_review`. فقط `posted` یعنی تسویه؛ `202`، ریدایرکت یا «succeeded» سمت provider تسویه نیست.

دو نکتهٔ عملی:

- یک batch در جریان، `has_pending_payment` را در `GET /api/v1/installments` روشن **نمی‌کند** (آن فلگ فقط payment attemptها را پوشش می‌دهد). شناسه و وضعیت batch را خودتان نگه دارید و تا terminal شدن آن، دکمه‌های پرداخت دیگر را غیرفعال کنید.
- در جهت عکس، وقتی batch روی قسطی claim دارد، single-pay برای همان قسط رد می‌شود (`409`/`422`)؛ پس همان قسط را از مسیر دیگر retry نکنید.

## ۵. پول، تاریخ و وضعیت‌ها

- مبالغ **عدد صحیح در واحد خرد** (`*_minor`) با `currency: "IRR"` هستند. از ممیز شناور استفاده نکنید و ریال را ساکت به تومان تبدیل نکنید؛ اگر تومان نمایش می‌دهید، فقط یک قاعدهٔ نمایشی توافق‌شده با محصول باشد.
- `Number` در جاوااسکریپت همهٔ اعداد بزرگ را دقیق نگه نمی‌دارد؛ اگر serialize/parse می‌کنید، تا آخر عدد صحیح (یا bigint/string) بمانید.
- `due_at` یک لحظهٔ UTC با فرمت RFC3339 است. آن را در `Asia/Tehran` (و با locale کاربر) نمایش دهید؛ تاریخ سررسید را محلی بازمحاسبه نکنید.
- `status` یکی از `scheduled | due | overdue` است. `grace_period_days` می‌گوید جریمهٔ دیرکرد چند روز بعد از `due_at` شروع می‌شود.
- `has_pending_payment` یعنی یک تلاش پرداخت وجود دارد — poll کنید و تلاش جدید شروع نکنید.
- `meta.code` و `code` خطا را بخوانید، نه متن ترجمه‌شدهٔ `message` را.

## ۶. CORS و استقرار

صفحهٔ مقصد و API دو Origin متفاوت‌اند (مثلاً صفحه `https://dev.dlend.ir` و API `https://api.dev.dlend.ir`)، پس فراخوانی‌های مرورگر به CORS از سمت gateway نیاز دارند.

Originهای مجاز فعلی (طبق کانفیگ Envoy):

- **Local dev:** `http://localhost:5173`، `http://127.0.0.1:5173`، `http://localhost:3000`، `http://localhost:3333`، `http://127.0.0.1:3000`، `http://localhost:8080`، `https://frontend.bnpl.test:5173` و هر `http://192.168.*` / `https://192.168.*`.
- **Hamravesh dev:** `https://dev.dlend.ir`، `https://api.dev.dlend.ir` و همان موارد localhost/`192.168.*`.

اگر Origin صفحهٔ شما در این لیست نیست، CORS قبل از اجرای کد شما fail می‌شود. اضافه کردن Origin یک **تغییر زیرساختی** است (فیلد `allow_origin_string_match` در `deploy/gateway/envoy/envoy.yaml` / کانفیگ لوکال)، نه اصلاح فرانت. در کار محلی می‌توانید از proxy سرور dev استفاده کنید تا فراخوانی API same-origin شود.

موارد لازم از سمت استقرار/ops:

1. هاست عمومی باید به gateway برسد و `/s/*` به `bnpl_backend` روت شود (با حفظ `Host`). وگرنه `302` اصلاً رخ نمی‌دهد.
2. صفحه باید روی همان هاست، در مسیر مقصد تنظیم‌شده (`…/installments`) در دسترس باشد. توجه: هاست لینک SMS و هاست صفحه الان **یکی** است؛ یعنی لبهٔ سایت باید `/s/*` را به BNPL و بقیهٔ مسیرها را به اپ شما بفرستد.

## ۷. تست محلی

1. هاست عمومی را به دستگاهی که gateway روی آن اجراست map کنید (مثلاً `/etc/hosts` → `192.168.1.104 dpay.ir api.dpay.ir` مطابق IP Listener خودتان) و گواهی محلی را trust کنید — [راهنمای mkcert](../local-development/mkcert/README.md).
2. gateway (در کانفیگ نمونه `192.168.1.104:8443`) و اپ خودتان (Vite با HTTPS روی `:5173`) را بالا بیاورید.
3. مقصد ریدایرکت را به صفحهٔ dev خودتان بدهید. در `.env` محلی، مقصد باید `https` باشد و hostname آن در `SHORT_LINK_ALLOWED_DESTINATION_HOST` باشد؛ این بررسی **پورت را نادیده می‌گیرد**، پس این مقدار معتبر است:

   ~~~dotenv
   SHORT_LINK_PUBLIC_HOST=dpay.ir
   SHORT_LINK_ALLOWED_DESTINATION_HOST=dpay.ir
   SHORT_LINK_REPAYMENT_DESTINATION_URL=https://dpay.ir:5173/installments
   ~~~

4. **API عمومی برای ساخت لینک کوتاه وجود ندارد.** برای تست محلی، یک کد را مستقیم در دیتابیس توسعه insert کنید:

   ~~~sql
   INSERT INTO short_links (code, destination_url)
   VALUES ('devTest', 'https://dpay.ir:5173/installments')
   RETURNING code;
   ~~~

   یا از یک رکورد موجود استفاده کنید:

   ~~~sql
   SELECT code, destination_url, expires_at, revoked_at
   FROM short_links
   WHERE revoked_at IS NULL
     AND (expires_at IS NULL OR expires_at > now())
   ORDER BY created_at DESC
   LIMIT 20;
   ~~~

5. `https://dpay.ir:8443/s/devTest` را باز کنید (یا با `curl -i` و **بدون** follow کردن ریدایرکت، هدر `Location` را ببینید).

   ~~~bash
   curl -sS -D - -o /dev/null https://dpay.ir:8443/s/devTest
   # انتظار: HTTP/2 302، location: https://dpay.ir:5173/installments
   #         cache-control: no-store، referrer-policy: no-referrer
   ~~~

6. در صفحه، خطاهای CORS کنسول مرورگر را چک کنید. اگر Origin صفحهٔ شما `https://dpay.ir:5173` است و API را cross-origin صدا می‌زنید، این Origin در allowlist لوکال **نیست** — یا آن را به کانفیگ Envoy لوکال اضافه کنید یا `/api` را از Vite پروکسی کنید (same-origin).

## ۸. محدودیت‌ها و موارد باز

- **دیپ‌لینک به قسط خاص وجود ندارد.** مقصد ثابت است و `?installment_id=`/`?code=` وجود ندارد. اگر محصول می‌خواهد «دقیقاً همان قسط از SMS باز شود»، این یک تغییر قرارداد بک‌اند است (مقصد مجاز + query parameter)، نه چیزی که فرانت بتواند استنتاج کند.
- **صفحهٔ خطای برندشده برای لینک بد وجود ندارد.** کد باطل/منقضی/ناشناخته روی هاست لینک کوتاه **`404` با بدنهٔ خالی** می‌دهد، پس مرورگر یک صفحهٔ خالی/ساده نشان می‌دهد و اپ شما اصلاً load نمی‌شود. صفحهٔ «این لینک نامعتبر/منقضی است» نیازمند mapping در لبه (یا تغییر بک‌اند) است و امروز وجود ندارد.
- **لینک‌های قدیمی.** لینک‌هایی که قبل از اضافه‌شدن پیشوند `/s` صادر شده‌اند (`…/<code>` یا هاست قدیمی `s.*`) الان `404` می‌دهند. هر لینک قبلاً چاپ/ارسال‌شده را منقضی بدانید.
- **URL بدون scheme در SMS.** مقدار ذخیره‌شدهٔ `payment_url` طبق طراحی `host/s/<code>` است (بدون `https://`)؛ قالب پیامک Notification مسئول لینک‌کردن قابل‌کلیک است. اگر با تپ کردن مرورگر باز نشد، با تیم Notification/SMS هماهنگ کنید.
- **Referrer حذف می‌شود.** هیچ رفتاری را به `document.referrer` گره نزنید؛ در صفحهٔ مقصد عمداً خالی است.
- **مسیر بازگشت از درگاه توسط بک‌اند تعیین می‌شود.** `FINANCIAL_CORE_PAYMENT_RETURN_URL` یک URL واحد HTTPS است که BNPL آن را به‌عنوان مقصد بازگشت success/failure/cancel برای **هر دو** مسیر single-pay و batch start به Financial Core می‌دهد؛ فرانت نمی‌تواند per-request مسیر خودش را بفرستد. امروز در منیفست‌های dev خالی است و در نمونهٔ لوکال به هاست API اشاره می‌کند که هندلر `/payments/return` ندارد — یعنی هنوز روت بازگشت فرانت وجود ندارد. مسیر را با ops توافق کنید (مثلاً `https://dev.dlend.ir/payments/return`)، تنظیم شود، و رسیدن به آن را «نتیجه نامعلوم» بدانید: invalidate و بازخوانی.
- **`has_pending_payment` مربوط به payment attempt است.** claimهای batch را پوشش نمی‌دهد (بند ۴.۳)؛ اگر batch در UX هست، وضعیت pending را از شناسهٔ batch خودتان نگه دارید.

## ۹. چک‌لیست پذیرش

- [ ] تپ روی لینک SMS در مرورگر به مسیر تنظیم‌شده می‌رسد (نه 404 خالی) و صفحه نمایش داده می‌شود.
- [ ] کاربر ناشناس به لاگین می‌رود و به همان صفحه برمی‌گردد.
- [ ] لیست خالی به‌صورت «قسط پرداخت‌نشده‌ای ندارید» نمایش داده می‌شود (HTTP 200)، نه خطا.
- [ ] اقساط سررسید/معوق با مبلغ، تاریخ سررسید، جریمهٔ دیرکرد و وضعیت از API رندر می‌شوند.
- [ ] دکمهٔ «پرداخت» از روی `next_payable` ساخته می‌شود و `payment_block_reason` / `has_pending_payment` را رعایت می‌کند.
- [ ] `POST …/pay` هدر `Idempotency-Key` دارد، `202` و `200` را هندل می‌کند و در صورت وجود `redirect_url` به آن می‌رود.
- [ ] بازگشت از درگاه چیزی را محلی «پرداخت‌شده» نمی‌کند؛ صفحه وضعیت معتبر را دوباره می‌خواند.
- [ ] مبالغ عدد صحیح minor unit هستند؛ محاسبهٔ اعشاری و تبدیل ساکت ریال→تومان وجود ندارد.
- [ ] CORS از Origin واقعی صفحه در هر محیط کار می‌کند.
- [ ] فقط `/s/<code>` به‌عنوان URL ناوبری استفاده می‌شود و اپ آن را `fetch()` نمی‌کند.
- [ ] مسیر پرداخت بر اساس UX انتخاب شده (single-pay یا batch) و برای یک قسط مخلوط نشده است.
- [ ] اگر batch استفاده می‌شود، همهٔ اقساط معوق در آن هستند و batch تا `posted` شدن poll می‌شود.
- [ ] صفحهٔ بازگشت روی URL تنظیم‌شده وجود دارد و بدون اتکا به query parameterها وضعیت را بازخوانی می‌کند.
