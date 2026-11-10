#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT"

# Hit BNPL app directly so synthetic panel X-Auth-* headers are trusted.
# Do not point this at Envoy (api.bnpl.test) unless you have a real merchant JWT.
export BNPL_API_BASE="${BNPL_API_BASE:-http://192.168.1.104:8080/}"

# Damz from 000_dev.sql (merchant_profiles.external_merchant_id).
export BNPL_EXTERNAL_MERCHANT_ID="${BNPL_EXTERNAL_MERCHANT_ID:-1e7c3dd4-0e8b-4ec1-aede-2a119f3ffecc}"
export BNPL_MERCHANT_TERMINAL_PERM="${BNPL_MERCHANT_TERMINAL_PERM:-bnpl:merchant-terminals:manage:merchant}"

# Grace used for the rotate → retire portion of the smoke (seconds).
export BNPL_ROTATE_GRACE_SECONDS="${BNPL_ROTATE_GRACE_SECONDS:-3600}"

# Prefer mkcert CA when calling HTTPS locally.
if [[ -z "${BNPL_CA_CERT:-}" && -f "${HOME}/.local/share/mkcert/rootCA.pem" ]]; then
	export BNPL_CA_CERT="${HOME}/.local/share/mkcert/rootCA.pem"
fi
# export BNPL_SSL_INSECURE=1

exec python3 "$ROOT/scripts/merchant-terminal/terminal_lifecycle_flow.py"
