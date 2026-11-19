#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$ROOT"

# Hit BNPL app directly so synthetic admin X-Auth-* headers are trusted.
# Do not point this at Envoy (api.bnpl.test) unless you have a real admin JWT.
export BNPL_API_BASE="${BNPL_API_BASE:-http://127.0.0.1:8080/}"

export BNPL_ADMIN_READ_PERM="${BNPL_ADMIN_READ_PERM:-bnpl:merchants:read:any}"
export BNPL_ADMIN_UPDATE_PERM="${BNPL_ADMIN_UPDATE_PERM:-bnpl:merchants:credit-kinds:update:any}"

# Prefer mkcert CA when calling HTTPS locally.
if [[ -z "${BNPL_CA_CERT:-}" && -f "${HOME}/.local/share/mkcert/rootCA.pem" ]]; then
	export BNPL_CA_CERT="${HOME}/.local/share/mkcert/rootCA.pem"
fi
# export BNPL_SSL_INSECURE=1

exec python3 "$ROOT/scripts/test/merchant-credit-kinds/credit_kinds_flow.py"
