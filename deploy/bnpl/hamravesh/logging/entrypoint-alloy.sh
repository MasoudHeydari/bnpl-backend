#!/bin/sh
# Alloy runtime bootstrap (PID 1).
#
# Darkoob disk mounts at /var/log/services are typically root-owned and hide the
# ownership baked in the image. Fix ownership for bnpluser, then start
# supervisord. Program stanzas run Alloy/BNPL/logrotate as bnpluser — the Go
# process itself is never root.
set -eu

LOG_ROOT="${LOG_SERVICES_ROOT:-/var/log/services}"

mkdir -p \
	"${LOG_ROOT}/bnpl-service" \
	"${LOG_ROOT}/alloy-data" \
	"${LOG_ROOT}/logrotate"

chown -R bnpluser:bnpluser "${LOG_ROOT}"

exec /usr/bin/supervisord -c /etc/supervisord.conf
