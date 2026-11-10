#!/bin/sh
set -eu
: "${GATEWAY_TRUST_PROOF:?required}"
: "${IDENTITY_ACCESS_JWT_ISSUER:?required}"
: "${IDENTITY_ACCESS_JWT_AUDIENCE:?required}"
escape_sed() { printf '%s' "$1" | sed 's/[\\&|]/\\&/g'; }
umask 077
sed \
  -e "s|__DPAY_GATEWAY_TRUST_PROOF__|$(escape_sed "$GATEWAY_TRUST_PROOF")|g" \
  -e "s|__DPAY_JWT_ISSUER__|$(escape_sed "$IDENTITY_ACCESS_JWT_ISSUER")|g" \
  -e "s|__DPAY_JWT_AUDIENCE__|$(escape_sed "$IDENTITY_ACCESS_JWT_AUDIENCE")|g" \
  /etc/envoy/envoy.template.yaml > /tmp/dpay-envoy.yaml
exec envoy -c /tmp/dpay-envoy.yaml --log-level info
