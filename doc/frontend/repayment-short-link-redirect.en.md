# Repayment SMS Short Link → Installments Page

**Audience:** Frontend team (customer web/app)  
**Owner:** BNPL Backend  
**Last updated:** 2026-11-20  
**Related:** [Backend ↔ Frontend integration guide](bnpl-frontend.en.md) (§3 envelopes, §7 installments/repayment), [Local HTTPS (mkcert) guide](../local-development/mkcert/README.md)  
**Other language:** [فارسی](repayment-short-link-redirect.fa.md)

---

## 0. TL;DR

1. The SMS link looks like `https://<SHORT_LINK_PUBLIC_HOST>/s/<CODE>` — for example `https://dev.dlend.ir/s/aB72xQ`.
2. BNPL answers that request with **`302 Found`** and `Location: <SHORT_LINK_REPAYMENT_DESTINATION_URL>` (today `…/installments`). The **browser** performs the navigation.
3. The landing page URL carries **no** short code, **no** installment id, and **no** customer id. It is a static destination.
4. Your job on `/installments`: authenticate the user, call `GET /api/v1/installments`, render the repayment position, and start the payment (§4.2 single-pay, §4.3 repayment batch).
5. Do **not** `fetch()` / XHR the short URL. It is a browser navigation endpoint, not an API.

## 1. URL contract

| Environment | Public short link | Redirect destination (configured) | API origin |
| --- | --- | --- | --- |
| Local dev | `https://dpay.ir:8443/s/<CODE>` | `https://dpay.ir/installments` (or `https://dpay.ir:5173/installments` when Vite serves the page) | `https://api.dpay.ir:8443` |
| Hamravesh dev | `https://dev.dlend.ir/s/<CODE>` | `https://dev.dlend.ir/installments` | `https://api.dev.dlend.ir` |

Rules:

- The host comes from `SHORT_LINK_PUBLIC_HOST` and the path prefix is `/s` (`shortlinkuc.PublicPathPrefix`). **Do not hardcode a host, port, or environment URL** in the frontend; read it from your deployment configuration.
- The code is an opaque `1..64` character `[A-Za-z0-9_-]` segment (generated codes are 8 alphanumeric characters). Treat it as case-sensitive and never decode or guess it.
- The resolver is registered on **GET only**. A `HEAD`/`POST` there is not a redirect.
- The request must arrive on the exact configured public host. The same code requested on another host is `404` by design.

## 2. What BNPL does (nothing for you to implement)

~~~text
Customer taps SMS link
        │
        ▼
GET https://<SHORT_LINK_PUBLIC_HOST>/s/<CODE>
        │  Envoy (no JWT required; Host preserved)
        ▼
BNPL short-link resolver
        │  302 Found
        │  Location: https://<frontend>/installments
        ▼
Browser loads your /installments page
~~~

Responses from the resolver:

| Status | When | Body |
| --- | --- | --- |
| `302` | Valid, non-expired, non-revoked code, correct host | Empty/irrelevant redirect body — read `Location` only |
| `400` | Blank/malformed code | **Empty body**, no JSON |
| `404` | Unknown, revoked, or expired code; wrong `Host`; internal failure (masked) | **Empty body**, no JSON |

Every response also carries `X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id`, plus:

- `Cache-Control: no-store`
- `Pragma: no-cache`
- `Referrer-Policy: no-referrer`

Consequences you must design for:

- **Never parse the redirect body** and never rely on `Referer` — it is suppressed by `Referrer-Policy: no-referrer`. If you need “arrived from SMS” attribution, it must be measured at the edge (gateway logs for the `302`), not in the page.
- Responses are not cached, so every tap re-validates the code server-side.
- There is **no API call** you need to (or can) make to resolve a code. The page cannot even see the code.

## 3. What the landing page must do

The destination path (`/installments` today) is configured by ops via `SHORT_LINK_REPAYMENT_DESTINATION_URL`. Your app must serve whatever path is configured, on the public site host. If your real route is different (e.g. `/repayment`), ask ops to change the env value — do not silently redirect to another path inside the app.

On page load:

1. **Authenticate.** If there is no valid session, start the Identity login flow and return to the same path afterwards (e.g. `?next=/installments`). Do not show repayment data to anonymous users.
2. **Do not send identity.** Send `Authorization: Bearer <identity-access-token>` and let the gateway inject `X-Auth-*`. Never send `X-Auth-Subject-Id`, `X-Auth-Tenant-Id`, `X-Auth-Context-Id`, or `X-Auth-Permissions` from the browser, and never send a customer id or mobile number.
3. **Read the customer-wide repayment position:**

   ~~~http
   GET /api/v1/installments?limit=20 HTTP/1.1
   Host: <api-origin>
   Authorization: Bearer <identity-access-token>
   Accept: application/json
   Accept-Language: fa-IR
   ~~~

   Permission: `bnpl:installments:read:self`.

4. **Render states:**

   | State | Signal | UI |
   | --- | --- | --- |
   | No unpaid installments | `200`, `data.installments: []` | Success/empty state (“no unpaid installments”) — **never** treat as 404/error |
   | Has due/overdue | `data.repayment.due_installments[]`, `due_count`, `unpaid_count` | Show the amount, due date, late fee, and status |
   | Payable now | `data.repayment.next_payable` with `payable_amount_known: true` and no `payment_block_reason` | Enable the “Pay” action for that installment |
   | Payment in progress | `has_pending_payment: true` | Show “payment pending”; poll the read endpoint instead of starting a new payment |
   | Blocked | `payment_block_reason` present | Explain the block; do not enable payment |
   | Not authenticated | `401` from the API | Login, then come back |

5. **Start the payment.** Pick exactly one path and keep it per UX (see §4.2 and §4.3):
   - one-tap “pay next”: `POST …/pay` for `data.repayment.next_payable` only;
   - “settle everything overdue in one payment”: create a repayment batch with every overdue installment, then start it.

   Never mix the two for the same installment: while a batch holds an installment claim, single-pay is rejected (`409`, or `422` when the database claim guard is the one that fires), and a batch should not be created while a single payment attempt is in flight. On `202`/`200` with `redirect_url`, navigate the browser to it with `window.location.assign(redirect_url)`.

6. **After the provider flow returns**, do not trust the browser return as proof of payment. Re-read `GET /api/v1/installments` (and/or the contract) until the installment disappears from the unpaid list or `has_pending_payment` clears; show an authoritative pending/settled state. Settlement is decided by the backend, never by the redirect.

## 4. API calls

Interactive specification: `<api-origin>/bnpl/swagger/index.html` (Swagger UI is enabled in dev; the gateway maps `/bnpl/swagger` to the backend `/swagger`).

### 4.1 Read installments

~~~http
GET /api/v1/installments?limit=20&after_id=<uuid>&after_due_at=<RFC3339Nano>
~~~

Response (`200`):

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

Notes:

- Items are ordered **nearest due date first** (`due_at ASC`). Only `scheduled`, `due`, and `overdue` installments with an attached contract are returned.
- `data.repayment` is **customer-wide** and is never scoped to the current page.
- `customer_payable_amount_minor` may be `null` when the server amount is not yet known.
- Paging is keyset: use `meta.pagination.keyset.next_after_id` and `next_after_due_at` as the next request's `after_id` / `after_due_at` while `has_next` is true.

### 4.2 Start one installment payment

~~~http
POST /api/v1/contracts/{contract_id}/installments/{installment_id}/pay HTTP/1.1
Host: <api-origin>
Authorization: Bearer <identity-access-token>
Content-Type: application/json
Idempotency-Key: <stable-unique-key-for-this-intention>
Accept-Language: fa-IR

{ "channel": "gateway" }
~~~

Permission: `bnpl:payments:start:self`. `Idempotency-Key` is **required** (max 256 chars); reuse the same key when retrying the same user intention.

Response: `202 Accepted` for a newly queued attempt, `200 OK` for an idempotent replay.

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

Rules:

- `redirect_url` and `expires_at` are optional. If `redirect_url` is absent, do **not** navigate — show a pending state and re-read the installments endpoint.
- Only the server's **globally selected priority installment** may be paid: that is exactly `data.repayment.next_payable`. Per-contract sequence is enforced too. Any other installment is rejected with `409` (`error.installment_is_not_the_current_priority_repayment_target` or `error.installment_is_not_the_next_payable_installment_in_sequence`). `error.installment_already_has_an_active_payment_attempt` means an attempt or an active batch claim already exists — re-read instead of retrying.
- `channel` must be one of `gateway`, `bank_transfer`, `wallet`.
- Do not retry financial `POST`s blindly; reuse the `Idempotency-Key`, and after an ambiguous result re-read the installments endpoint.

### 4.3 Settle several installments at once (repayment batch)

Use this path only when the product UX is “settle everything overdue in one payment”. Single-pay and batch are both supported contracts; choose per UX and never mix them for the same installment.

~~~http
POST /api/v1/repayment-batches HTTP/1.1
Host: <api-origin>
Authorization: Bearer <identity-access-token>
Content-Type: application/json
Idempotency-Key: <stable-unique-key-for-this-intention>

{ "installment_ids": ["<installment-id>", "<installment-id>"] }
~~~

Permission: `bnpl:payments:start:self`. Response: `202` with `{ "id", "status", "total_amount_minor", "currency" }`.

The server validates the selection and rejects violations with `409`:

1. Every **overdue** installment of the customer — across both credit products and all merchants — must be included.
2. Within one contract you cannot skip an earlier unpaid installment.
3. `scheduled`/`due` installments that are not yet overdue are optional.

Then start the batch:

~~~http
POST /api/v1/repayment-batches/{id}/start HTTP/1.1
Authorization: Bearer <identity-access-token>
Idempotency-Key: <stable-unique-key-for-this-start>
~~~

`202` with `{ "id", "status", "redirect_url"? }`. Navigate to `redirect_url` when present; `status` in the start response uses the Financial Core collection vocabulary, not the BNPL status enum.

Poll `GET /api/v1/repayment-batches/{id}` for the durable BNPL status: `created`, `prepared`, `starting`, `requires_action`, `reconciling`, `accounting_pending`, `posted`, `failed`, `expired`, `manual_review`. Only `posted` means settled — `202`, a redirect, or the provider's “succeeded” are not settlement.

Two practical points:

- A pending batch does **not** set `has_pending_payment` in `GET /api/v1/installments` (that flag reflects payment attempts only). Track the batch id and its status yourself, and disable other pay actions until the batch is terminal.
- In the other direction, once a batch claims an installment, single-pay for it is rejected (`409`/`422`), so do not retry the same installment through the other path.

## 5. Money, dates, statuses

- Amounts are **integer minor units** (`*_minor`) with `currency: "IRR"`. Do not use floating point, and do not silently convert Rial to Toman in data; if you display Toman, do it as a presentation-only formatting rule agreed with product.
- JavaScript `Number` cannot represent all large integers exactly; if you serialize/parse, keep them as integers (or bigint/string) end to end.
- `due_at` is an RFC3339 UTC instant. Display it in `Asia/Tehran` (and support the user's locale); do not recompute due dates locally.
- `status` is `scheduled | due | overdue`. `grace_period_days` tells you how long after `due_at` the late fee starts.
- `has_pending_payment` means a payment attempt already exists — poll instead of starting another one.
- Read `meta.code` / error `code`, not the localized `message` text.

## 6. CORS and deployment

The landing page and the API are different origins (e.g. `https://dev.dlend.ir` page → `https://api.dev.dlend.ir` API), so browser calls need CORS from the gateway.

Origins already allowed (see the Envoy configs):

- **Local dev:** `http://localhost:5173`, `http://127.0.0.1:5173`, `http://localhost:3000`, `http://localhost:3333`, `http://127.0.0.1:3000`, `http://localhost:8080`, `https://frontend.bnpl.test:5173`, and any `http://192.168.*` / `https://192.168.*`.
- **Hamravesh dev:** `https://dev.dlend.ir`, `https://api.dev.dlend.ir`, plus the localhost/`192.168.*` entries above.

If your page origin is not in that list, CORS will fail before your code runs. Adding an origin is an **infrastructure change** (Envoy `allow_origin_string_match` in `deploy/gateway/envoy/envoy.yaml` / the local dev config), not a frontend fix. Alternatively, use a dev server proxy so API calls are same-origin during local work.

Also required from deployment/ops:

1. The public host must reach the gateway, and `/s/*` must be routed to `bnpl_backend` (preserving `Host`). Otherwise the `302` never happens.
2. The page must be reachable at the configured destination path on that same host (`…/installments`). Note: the SMS link target host and the page host are the **same** host now, which means the site edge must route `/s/*` to BNPL and everything else to your app.

## 7. Local testing

1. Map the public host to the machine running the gateway (e.g. `/etc/hosts` → `192.168.1.104 dpay.ir api.dpay.ir`, matching your listener IP) and trust the local certificate — see the [mkcert guide](../local-development/mkcert/README.md).
2. Run the gateway (`192.168.1.104:8443` in the sample config) and your app (Vite HTTPS on `:5173`).
3. Point the redirect at your dev page. In your local `.env`, the destination must be `https` and its hostname must be in `SHORT_LINK_ALLOWED_DESTINATION_HOST`; the **port is ignored** by that check, so this is valid:

   ~~~dotenv
   SHORT_LINK_PUBLIC_HOST=dpay.ir
   SHORT_LINK_ALLOWED_DESTINATION_HOST=dpay.ir
   SHORT_LINK_REPAYMENT_DESTINATION_URL=https://dpay.ir:5173/installments
   ~~~

4. There is **no public API to create a short link**. For local testing, insert a code directly (development database only):

   ~~~sql
   INSERT INTO short_links (code, destination_url)
   VALUES ('devTest', 'https://dpay.ir:5173/installments')
   RETURNING code;
   ~~~

   Or reuse an existing one:

   ~~~sql
   SELECT code, destination_url, expires_at, revoked_at
   FROM short_links
   WHERE revoked_at IS NULL
     AND (expires_at IS NULL OR expires_at > now())
   ORDER BY created_at DESC
   LIMIT 20;
   ~~~

5. Open `https://dpay.ir:8443/s/devTest` (or `curl -i` it, **without** following redirects, to inspect the `Location` header).

   ~~~bash
   curl -sS -D - -o /dev/null https://dpay.ir:8443/s/devTest
   # expect: HTTP/2 302, location: https://dpay.ir:5173/installments
   #         cache-control: no-store, referrer-policy: no-referrer
   ~~~

6. On the page, check the browser console for CORS errors. If your page origin is `https://dpay.ir:5173` and you call the gateway cross-origin, that origin is **not** in the local allowlist — either add it to the local Envoy config or proxy `/api` through Vite (same-origin).

## 8. Known limitations / open items

- **No per-installment deep link.** The destination is static; there is no `?installment_id=`/`?code=` to consume. If product wants “open the exact installment from the SMS”, that is a backend contract change (allowed destination + query parameter), not something the frontend can infer.
- **No branded error page for bad links.** Revoked/expired/unknown codes produce an **empty-bodied `404`** at the short-link host, so the browser shows a blank/plain 404 and your app is never loaded. A branded “this link is invalid/expired” page requires an edge-level mapping (or a backend change); it does not exist today.
- **Legacy links.** Links issued before the `/s` prefix existed (`…/<code>` or the old `s.*` host) return `404` now. Treat any previously printed/SMS links as expired.
- **Scheme-less SMS URL.** The stored `payment_url` is `host/s/<code>` (no `https://`) by design; the Notification SMS template is responsible for rendering a clickable link. Coordinate with the Notification/SMS team if tapping does not open a browser.
- **Referrer is suppressed.** Do not gate anything on `document.referrer`; it is empty by design on the landing page.
- **The payment return URL is backend-owned.** `FINANCIAL_CORE_PAYMENT_RETURN_URL` is a single HTTPS URL that BNPL sends to Financial Core as the success/failure/cancel return target for **both** single-pay and batch start; the frontend cannot pass its own per request. Today the dev manifests leave it empty and the local sample points at the API host, which has no `/payments/return` handler — there is no frontend return route yet. Agree a path with ops (e.g. `https://dev.dlend.ir/payments/return`), have it configured, and treat arrival there as “outcome unknown”: invalidate and re-read.
- **`has_pending_payment` is about payment attempts.** It does not cover batch claims (see §4.3); if batches are part of the UX, keep your own pending state derived from the batch id.

## 9. Acceptance checklist

- [ ] Tapping the SMS link in a browser lands on the configured path (no blank 404) and shows the page.
- [ ] Anonymous user is sent to login and returns to the same page.
- [ ] Page renders “no unpaid installments” for an empty list (HTTP 200), not an error.
- [ ] Due/overdue installments render amount, due date, late fee, and status from the API.
- [ ] “Pay” is driven by `next_payable` and respects `payment_block_reason` / `has_pending_payment`.
- [ ] `POST …/pay` sends `Idempotency-Key`, handles `202` and `200`, and follows `redirect_url` when present.
- [ ] Returning from the provider does not mark anything paid locally; the page re-reads the authoritative state.
- [ ] Amounts are integer minor units; no float math; no silent Rial→Toman conversion.
- [ ] CORS works from the real page origin in each environment.
- [ ] Only `/s/<code>` is treated as a navigation URL; the app never `fetch()`es it.
- [ ] The payment path is chosen per UX (single-pay or batch) and not mixed for the same installment.
- [ ] If batches are used, every overdue installment is included and the batch is polled until `posted`.
- [ ] A return page exists at the configured URL and re-reads state without trusting query parameters.
