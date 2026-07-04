# Frontend wire change: external ID prefix

> **Date:** 2026-11-10  
> **Audience:** Customer app, merchant admin UI, Hosted Checkout frontend  
> **Updated full guide:** [bnpl-frontend.en.md](bnpl-frontend.en.md) · [فارسی](bnpl-frontend-external-ids-changelog.fa.md)

## Summary

IDs that belong to an **external system** now use an `external_` prefix on the public HTTP wire so they are not confused with BNPL-internal primary keys.

| Before | After | Where |
| --- | --- | --- |
| `purchase_context.pos_gps_id` | `purchase_context.external_pos_gps_id` | `POST /checkout-decisions` request/response |
| `purchase_context.online_store_id` | `purchase_context.external_online_store_id` | same |
| Merchant profile `merchant_id` | `external_merchant_id` | `GET /merchant`, terminal responses |
| Branch `branch_id` | `external_branch_id` | `GET /merchant/branches` |
| Partner Create `online_store_id` | `external_online_store_id` | `POST /partner/checkout-sessions` (merchant server, not browser) |
| Checkout webhook `merchant_id` | `external_merchant_id` | `bnpl.checkout.*.v1` envelopes |

## Unchanged

- Internal `customer_id` foreign keys that mean `customer_profiles.id`.
- Authenticated customer identity still arrives via Gateway `X-Auth-Subject-Id`; frontend must not invent it in the body.
- `merchant_order_id`, `merchant_state`, `terminal_id`, and the public `cs_` token are unchanged.
- Hosted Checkout routes, permissions, and state machine are unchanged.

## Frontend impact

1. **Customer Decision create:** rename `purchase_context` keys. Old names yield `422 VALIDATION_FAILED`.
2. **Merchant panel:** migrate profile/branch types from `merchant_id` / `branch_id` to `external_merchant_id` / `external_branch_id`.
3. **Validation errors:** `details.fields` use JSON snake_case keys (e.g. `external_online_store_id`), never Go identifiers.
4. **Hosted Checkout UI:** still does not call Partner Create; keep carrying only the `cs_` token.

## Example (online Decision)

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

Partner Backend docs: [partner-checkout.en.md](../integration/partner-checkout.en.md).
