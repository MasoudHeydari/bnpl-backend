#!/bin/sh
# Ensure positions + scrape roots exist, then exec Promtail with env expansion.
set -eu

mkdir -p /var/lib/promtail /var/log/services

exec /usr/bin/promtail \
	-config.file=/etc/promtail/config.yml \
	-config.expand-env=true \
	"$@"
