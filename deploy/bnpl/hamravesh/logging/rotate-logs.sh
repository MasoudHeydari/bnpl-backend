#!/bin/sh
# Periodic logrotate for BNPL JSONL. Runs as bnpluser under supervisord.
set -eu

INTERVAL_SECONDS="${LOGROTATE_CHECK_INTERVAL_SECONDS:-60}"
CONF="${LOGROTATE_CONF:-/home/bnpluser/app/logging/logrotate.conf}"
STATE="${LOGROTATE_STATE:-/var/log/services/logrotate/status}"

mkdir -p "$(dirname "$STATE")"

echo "[rotate-logs] starting (interval=${INTERVAL_SECONDS}s conf=${CONF} state=${STATE})"

while true; do
	# -s keeps state on the BNPL persistent disk so rotations survive restarts.
	if ! logrotate -s "$STATE" "$CONF"; then
		echo "[rotate-logs] logrotate exited non-zero (will retry)" >&2
	fi

	sleep "$INTERVAL_SECONDS"
done
