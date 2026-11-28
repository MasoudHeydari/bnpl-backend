# Credit allocation and durable credit requests

## Scope and ownership

`CreditOwner` remains the legal-person credit owner; `CustomerProfile` remains the account identity. Each product has its own CreditAccount, and another customer account may only enroll in existing usable shared credit. An import never creates an authoritative person, a CustomerProfile or a credit balance. Eligibility uses plain-text national code and canonical mobile, verified against CRS when processing. Do not put those raw fields in customer responses, audit metadata or logs. The new administration endpoints are privileged; apply the deployment's existing PII access controls.

Both default rules are initially `disabled`. A person not covered by a configured rule can submit a request successfully; they do not receive a default limit. Group membership is single and versioned. Multiple mobile aliases may refer to one national code; an already bound verified person can use their person rule through another verified account. Conflicting imported mobile/national-code pairs fail closed. An individual rule wins over the current group rule and then the default rule. `inherit` removes the named override; `disabled` defers; `deny` terminates a request explicitly. Disabled groups do not fall through to public allocation. Group changes do not mutate active accounts. Existing Trusted import also registers eligibility in `trusted_customers` (200 million toman installment, 3 million toman monthly); existing explicit membership is preserved. Only the original Trusted verification can grant the Trusted checkout policy; group membership alone cannot do so.

`fixed_limit` is permitted only for group and individual scopes. `currency` is mandatory for allocation commands; only `IRR` is supported. `fixed_limit` uses the selected rule's amount and the existing compiled policy's currency, maximum (2 billion IRR) and validity (365 days). `risk_based` is reserved and rejected at configuration time. Future work must use the existing `creditassessment.Client` port, fetch and validate signals outside transactions, calculate a governed versioned result, and recheck the rule and request lease before commit. The legacy policy preview is not risk scoring and is not used to grant requests. No default fixed amount is substituted for missing signals. Financial Core remains the payment/money authority.

## Administration

All paths below have `/api/v1/admin` prefix. Gateway injects authenticated admin subject, tenant, context and permissions. Identity/FC must provision the three new permission codes before use. Never allow public access to these headers. Rules and memberships are immutable revisions, not in-place rewrites. Administrative writes create an audit outbox record with actor and resource identifiers.

| Method | Path | Permission |
|---|---|---|
| GET | `/credits/groups` | `bnpl:credit:allocations:read:any` |
| PUT | `/credits/groups/{group_code}` | `bnpl:credit:allocations:manage:any` |
| GET | `/credits/eligibility` | `bnpl:credit:allocations:read:any` |
| GET | `/credits/eligibility/{eligibility_id}` | `bnpl:credit:allocations:read:any` |
| PUT | `/credits/eligibility` | `bnpl:credit:allocations:manage:any` |
| POST | `/credits/eligibility/import` | `bnpl:credit:allocations:manage:any` |
| GET | `/credits/allocation/rules` | `bnpl:credit:allocations:read:any` |
| PUT | `/credits/allocation/rules` | `bnpl:credit:allocations:manage:any` |
| GET | `/credits/allocation/defaults` | `bnpl:credit:allocations:read:any` |
| PUT | `/credits/allocation/defaults` | `bnpl:credit:allocations:manage:any` |
| GET | `/credits/requests` | `bnpl:credit:requests:read:any` |

Create group `bank_employees` with `PUT /credits/groups/bank_employees`:

```json
{"name":"Bank employees","enabled":true}
```

Set its installment credit to 50 million toman (500 million IRR):

```json
{"scope":"group","group_code":"bank_employees","credit_kind":"pay_in_installments","currency":"IRR","strategy":"fixed_limit","fixed_amount_minor":500000000}
```

Use a separate rule for `pay_next_month`. No inference between products. A group without a rule for a product inherits the default unless disabled. A single-person `PUT /credits/eligibility` takes `national_code`, `mobile_e164`, and nullable `group_code`; null ends membership. `POST /credits/eligibility/import` accepts a JSON array, not CSV or multipart. The default limits are 1,000 items and 2 MiB; configure `CREDIT_ELIGIBILITY_IMPORT_MAX_ITEMS` and `CREDIT_ELIGIBILITY_IMPORT_MAX_BODY_BYTES`. The whole import is atomic.

```json
[{"national_code":"0012345678","mobile_e164":"+989120000001","group_code":"bank_employees"}]
```

Use the returned entry ID in an individual rule (valid before registration):

```json
{"scope":"individual","eligibility_id":"00000000-0000-4000-8000-000000000001","credit_kind":"pay_in_installments","currency":"IRR","strategy":"fixed_limit","fixed_amount_minor":750000000}
```

The example ID is a placeholder. An `inherit` revision with zero amount removes the override. `GET /credits/eligibility?group_code=bank_employees&limit=100&after=<uuid>` uses the last entry ID as cursor. `GET /credits/requests?credit_kind=pay_in_installments&limit=100&before=<RFC3339>` returns pending/processing requests; timestamp filtering is inclusive, not a unique cursor.

## Customer request lifecycle

`POST /api/v1/customer-credit/initiate` submits both products; `?credit_kind=...` submits one. A durable request is uniquely open per person/currency/product. The originating account receives `credit_request` with ID/status/requested_at and a public reason for rejection. Other accounts cannot read the originating account's request details. `GET /api/v1/customer-credit` reads the real stored request, not an inferred application status. No request field means the account has not submitted a request for the product. Account status is independent. A pending request returns no fabricated Decision or CreditAccount. Mixed outcomes use 201 for any new grant, otherwise 202 for any newly pending request, otherwise 200 for replay/rejection. For the default-off ordinary customer, show `CREDIT_REQUEST_RECEIVED` and the localized `meta.message`. Modify the embedded `fa-IR.yaml` and `en-US.yaml` catalogs before deployment; no runtime override table.

Workers resolve the immutable original mobile through CRS, not a later gateway header. Definite not-found or changed national code/account ID rejects with `CREDIT_REQUEST_IDENTITY_MISMATCH`; transient dependency errors, inactive or incomplete identity defer for retry. A rejected identity request is retained and a new verified request is allowed. Unlisted customers remain pending, not rejected. Explicit denial is replayed until the rule changes. Authorization, identity, allocation, and credit account status are checked before granting. Configuration takes a singleton exclusive lock; grants hold its shared form and then the legal-person lock. A worker lease fences stale workers. Decision, limit journal, request completion and the existing audit outbox are atomic. No network call takes place inside the DB transaction.

## Rollout and operations

Named eligibility or rule changes wake affected pending requests. Public credit is disabled initially. The default command takes only product, currency and an enabled flag; it never accepts an amount:

```json
{"credit_kind":"pay_in_installments","currency":"IRR","enabled":false}
```

PUT `/credits/allocation/defaults`. Enabling is rejected until the real risk-based assessor exists. Once implemented, new requests and dormant requests will be assessed automatically, with no administrator approval or amount assignment and no separate rollout operation. The worker selects a bounded oldest-first batch under the configuration fence and revalidates the original phone and current rules before grant. A disabled named override must not fall through to the public rule. The current risk-based path cannot grant credit and never substitutes a fixed amount. Worker processes up to 100 due requests per run, default 30 seconds, lease 2 minutes and retry 1 minute. Active accounts are not automatically resized. Configure `TASK_CREDIT_REQUEST_PROCESSING_*` using the example settings. TODO: coordinate credit-granted notification types and template variables with the Notification team; do not emit any new notification command yet. Existing audit outbox remains unchanged.

Pre-production: reset/reinitialize the schema using the existing destructive dev workflow after backing up any dev data you want to keep. No incremental migration is included. The reviewed Runtime privileges and schema digest are updated. Do not run the reset on production. Regenerate Swagger using the repository's existing Makefile target and run `make test-all` in a Go 1.26.2 environment before merge; older grant tests require adapting their fixtures to explicit allocation rules rather than expecting default credit.
