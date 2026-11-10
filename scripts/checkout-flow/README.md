# Hosted checkout confirm flow

Interactive smoke script:

1. Partner create session
2. Hosted GET (`GET /api/v1/checkout-sessions/{token}` — customer lands on BNPL page)
3. Hosted start
4. Hosted OTP request
5. Wait for OTP on CLI
6. Hosted confirm

## Run

Edit credentials in `run_hosted_confirm_flow.sh`, then:

```bash
./scripts/checkout-flow/run_hosted_confirm_flow.sh
```

Or export the same vars yourself and run `python3 scripts/checkout-flow/hosted_confirm_flow.py`.

Required env:

- `BNPL_TERMINAL_ID`
- `BNPL_API_KEY`
- `BNPL_EXTERNAL_ONLINE_STORE_ID` (legacy alias: `BNPL_ONLINE_STORE_ID`)

Partner Create body uses `external_online_store_id` (not `online_store_id`).

Also loads unset keys from the repo `.env` via `setdefault`.

`checkout_session_id` in the create response is the public `cs_...` token and is reused for hosted `GET`, `/start`, `/otp`, and `/confirm`.

TTY output is colorized: each call prints a **REQUEST** (headers + body) then **RESPONSE** JSON block, plus colored steps/status. Disable with `NO_COLOR=1` / `BNPL_NO_COLOR=1`; force with `FORCE_COLOR=1` / `BNPL_FORCE_COLOR=1`.
