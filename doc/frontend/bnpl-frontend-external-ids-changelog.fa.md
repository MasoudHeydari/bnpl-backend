# تغییرات قرارداد شناسه‌های بیرونی برای Frontend BNPL

> **تاریخ:** ۲۰۲۶-۱۱-۱۰  
> **مخاطب:** تیم Frontend مشتری، پنل Merchant و Hosted Checkout  
> **مستند کامل به‌روزشده:** [bnpl-frontend.fa.md](bnpl-frontend.fa.md) · [English](bnpl-frontend-external-ids-changelog.en.md)

## خلاصه

شناسه‌هایی که متعلق به **سرویس/سیستم بیرونی** هستند، در Request/Response عمومی با پیشوند `external_` می‌آیند تا از PK داخلی BNPL جدا شوند.

| قبل | بعد | کجا |
| --- | --- | --- |
| `purchase_context.pos_gps_id` | `purchase_context.external_pos_gps_id` | `POST /checkout-decisions` (Request و Response) |
| `purchase_context.online_store_id` | `purchase_context.external_online_store_id` | همان |
| `merchant_id` در پروفایل Merchant | `external_merchant_id` | `GET /merchant` و پاسخ Terminal |
| `branch_id` در Branch | `external_branch_id` | `GET /merchant/branches` |
| `online_store_id` در Partner Create | `external_online_store_id` | `POST /partner/checkout-sessions` (Backend پارتنر؛ Browser نه) |
| `merchant_id` در Webhook checkout | `external_merchant_id` | Envelope رویدادهای `bnpl.checkout.*.v1` |

## چه چیزی عوض نشده

- `customer_id` در پاسخ‌هایی که FK داخلی به `customer_profiles.id` است (مثلاً بعضی Projectionهای داخلی/Sales) — این PK داخلی BNPL است، نه Identity Subject.
- هویت مشتری احرازشده همچنان از Gateway با `X-Auth-Subject-Id` می‌آید؛ Frontend آن را در Body نمی‌فرستد.
- `merchant_order_id`، `merchant_state`، `terminal_id` و توکن `cs_` تغییر نام نداده‌اند.
- مسیرها، Permissionها و State Machine Hosted Checkout ثابت‌اند.

## تأثیر روی UI

1. **Checkout مستقیم مشتری:** در فرم/Client Decision، کلیدهای `purchase_context` را به نام‌های جدید تغییر دهید. ارسال نام قدیمی `422 VALIDATION_FAILED` می‌دهد.
2. **پنل Merchant:** اگر `merchant_id` / `branch_id` را در State، Type یا نمایش پروفایل نگه می‌داشتید، به `external_merchant_id` / `external_branch_id` مهاجرت دهید.
3. **خطاهای validation:** `details.fields` و `violations[].fields` با **همان کلید JSON (snake_case)** برمی‌گردند؛ مثلاً `external_online_store_id` نه `ExternalOnlineStoreID`.
4. **Hosted Checkout UI:** بدنهٔ Partner Create را Browser صدا نمی‌زند؛ فقط مطمئن شوید Deep Link / Analytics همچنان فقط توکن `cs_` را حمل می‌کند.

## نمونهٔ Decision Online (جدید)

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

## نمونهٔ خطای validation

~~~json
{
  "code": "VALIDATION_FAILED",
  "message": "The request contains invalid fields.",
  "details": {
    "fields": ["purchase_context.external_pos_gps_id"],
    "violations": [
      {
        "code": "REQUIRED",
        "message": "...",
        "fields": ["purchase_context.external_pos_gps_id"]
      }
    ]
  }
}
~~~

برای Partner Backend نیز همین قرارداد در [partner-checkout.md](../integration/partner-checkout.md) به‌روز شده است.
