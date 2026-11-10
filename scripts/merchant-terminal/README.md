# Merchant terminal lifecycle smoke

Non-interactive script that exercises panel terminal APIs and partner authenticate:

1. `GET /api/v1/merchant/terminal`
2. `POST` Provision (if empty) **or** Rotate with `grace_period_seconds=0` (if terminal already exists)
3. Idempotent replay of that issuance (`Idempotency-Key`)
4. Partner auth probe: wrong key → 401
5. Rotate with grace → both keys accepted; `GET` shows `retiring`
6. Retire retiring credential → old key 401; new key still accepted
7. Print final `terminal_id` + `api_key` (and `export` lines) for other partner APIs

Each HTTP call prints a colored **REQUEST** (headers + body) then **RESPONSE** block.

Docs for each flow stay under [`doc/integration/merchant-terminal/`](../../doc/integration/merchant-terminal/).

## Run

BNPL must be reachable (local compose / `make run`). Prefer the **app port**, not Envoy:

```bash
./scripts/merchant-terminal/run_terminal_lifecycle_flow.sh
```

Or export the same vars and run `python3 scripts/merchant-terminal/terminal_lifecycle_flow.py`.

## Why direct BNPL (not Envoy)

Panel routes trust gateway `x-auth-*` headers that Envoy normally injects from JWT. This smoke injects those headers itself, so the base URL should be something like `http://127.0.0.1:8080`. Against Envoy, inbound `x-auth-*` are stripped unless you supply a real merchant JWT.

## Env

| Variable | Default |
| --- | --- |
| `BNPL_API_BASE` | `http://127.0.0.1:8080` |
| `BNPL_EXTERNAL_MERCHANT_ID` | Damz seed `1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc` |
| `BNPL_MERCHANT_TERMINAL_PERM` | `bnpl:merchant-terminals:manage:merchant` |
| `BNPL_ROTATE_GRACE_SECONDS` | `3600` |
| `BNPL_CA_CERT` / `BNPL_SSL_INSECURE` | same TLS helpers as checkout-flow |

Also loads unset keys from the repo `.env` via `setdefault`.

TTY output is colorized (JSON keys/strings/numbers, HTTP status, steps). Disable with `NO_COLOR=1` or `BNPL_NO_COLOR=1`; force with `FORCE_COLOR=1` / `BNPL_FORCE_COLOR=1`.

## Side effects

If the merchant already has a terminal (e.g. Damz used by checkout-flow), the script **rotates** credentials and prints the new `terminal_id` / `api_key`. Update `scripts/checkout-flow/run_hosted_confirm_flow.sh` afterward if you still need that key.
