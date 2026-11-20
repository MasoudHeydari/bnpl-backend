# Admin merchant credit-kinds smoke

Interactive script for admin GET/PUT credit-kind eligibility:

1. Choose merchant from local-dev fixtures (`000_dev.sql` — Damz / Digikala) — arrow ↑/↓ then Enter
2. `GET /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds`
3. Choose credit kind (`pay_in_installments` / `pay_next_month`) — arrow ↑/↓ then Enter
4. Choose desired state (enable / disable) — arrow ↑/↓ then Enter
5. `PUT /api/v1/admin/merchants/{merchant_profile_id}/credit-kinds/{credit_kind}`
6. `GET` again to verify

`merchant_profile_id` is the **internal** BNPL id (`merchant_profiles.id`), not
`external_merchant_id`.

## Run

BNPL must be reachable. Prefer the **app port**, not Envoy:

```bash
./scripts/test/merchant-credit-kinds/run_credit_kinds_flow.sh
```

Or export the same vars and run
`python3 scripts/test/merchant-credit-kinds/credit_kinds_flow.py`.

## Why direct BNPL (not Envoy)

Admin routes trust gateway `x-auth-*` headers that Envoy normally injects from
JWT. This smoke injects those headers itself, so the base URL should be
something like `http://127.0.0.1:7080`. Against Envoy, inbound `x-auth-*` are
stripped unless you supply a real admin JWT.

## Env

| Variable | Default |
| --- | --- |
| `BNPL_API_BASE` | `http://127.0.0.1:7080` |
| `BNPL_ADMIN_SUBJECT_ID` | random UUID |
| `BNPL_ADMIN_READ_PERM` | `bnpl:merchants:read:any` |
| `BNPL_ADMIN_UPDATE_PERM` | `bnpl:merchants:credit-kinds:update:any` |
| `BNPL_CA_CERT` / `BNPL_SSL_INSECURE` | same TLS helpers as other smoke scripts |

Also loads unset keys from the repo `.env` via `setdefault`.

Merchants are parsed from
`pkg/adapter/db/postgres/migrationsrp/000_dev.sql` at runtime.
