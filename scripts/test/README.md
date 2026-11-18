# Manual / harness test scripts

Python and shell helpers used to smoke-test live BNPL flows and to bootstrap
integration-test Postgres. Keep new flow scripts here — do not drop them at
`scripts/` root.

| Path | Purpose |
| --- | --- |
| `lib/` | Shared Python helpers (`api_client`, CLI styling) |
| `checkout-flow/` | Hosted checkout confirm smoke |
| `merchant-terminal/` | Merchant terminal lifecycle smoke |
| `testing/` | `make test-integration` Postgres harness |

Ops scripts stay outside this tree (`scripts/runtime`, `scripts/kafka`,
`scripts/verification`).
