#!/bin/sh
# Apply Hamravesh-friendly Grafana defaults, then hand off to upstream /run.sh.
set -eu

export GF_SECURITY_ADMIN_USER="${GF_SECURITY_ADMIN_USER:-admin}"
export GF_AUTH_ANONYMOUS_ENABLED="${GF_AUTH_ANONYMOUS_ENABLED:-false}"
export GF_USERS_ALLOW_SIGN_UP="${GF_USERS_ALLOW_SIGN_UP:-false}"
export GF_SERVER_HTTP_ADDR="${GF_SERVER_HTTP_ADDR:-0.0.0.0}"
export GF_ANALYTICS_REPORTING_ENABLED="${GF_ANALYTICS_REPORTING_ENABLED:-false}"
export GF_ANALYTICS_CHECK_FOR_UPDATES="${GF_ANALYTICS_CHECK_FOR_UPDATES:-false}"
export GF_PATHS_PROVISIONING="${GF_PATHS_PROVISIONING:-/etc/grafana/provisioning}"

if [ -z "${GF_SECURITY_ADMIN_PASSWORD:-}" ] && [ -z "${GF_SECURITY_ADMIN_PASSWORD__FILE:-}" ]; then
	echo "GF_SECURITY_ADMIN_PASSWORD (or GF_SECURITY_ADMIN_PASSWORD__FILE) is required" >&2
	exit 1
fi

exec /run.sh "$@"
