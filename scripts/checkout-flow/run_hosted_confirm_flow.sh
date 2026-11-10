#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

# Fill these before running.
export BNPL_API_BASE="${BNPL_API_BASE:-http://192.168.1.104:8080/}"
export BNPL_TERMINAL_ID="${BNPL_TERMINAL_ID:-7236125465094479262}"
export BNPL_API_KEY="${BNPL_API_KEY:-bnpl_OvU0Mgvdo9MlLH35F9znYwItKvRmAeqwYYi8QSHLcm0}"
export BNPL_EXTERNAL_ONLINE_STORE_ID="${BNPL_EXTERNAL_ONLINE_STORE_ID:-${BNPL_ONLINE_STORE_ID:-8e1a2b3c-4d5e-4f6a-8b7c-9d0e1f2a3b4c}}"
export BNPL_CUSTOMER_PHONE="${BNPL_CUSTOMER_PHONE:-+989127434738}"

# Optional overrides:
 export BNPL_AMOUNT_MINOR=1500000000
# export BNPL_RETURN_DESTINATION=default
# export BNPL_CURRENCY=IRR
# Prefer mkcert CA for api.bnpl.test (Envoy). Fallback is handled in Python.
if [[ -z "${BNPL_CA_CERT:-}" && -f "${HOME}/.local/share/mkcert/rootCA.pem" ]]; then
	export BNPL_CA_CERT="${HOME}/.local/share/mkcert/rootCA.pem"
fi
# export BNPL_SSL_INSECURE=1

if [[ "$BNPL_TERMINAL_ID" == "..." || "$BNPL_API_KEY" == "..." || "$BNPL_EXTERNAL_ONLINE_STORE_ID" == "..." ]]; then
	echo "Set BNPL_TERMINAL_ID, BNPL_API_KEY, and BNPL_EXTERNAL_ONLINE_STORE_ID in $0 (or export them before running)." >&2
	exit 1
fi

exec python3 "$ROOT/scripts/checkout-flow/hosted_confirm_flow.py"
