#!/bin/sh
# Prepare Loki filesystem storage dirs then exec the upstream binary.
set -eu

mkdir -p \
	/loki/chunks \
	/loki/rules \
	/loki/compactor \
	/loki/tsdb-index \
	/loki/tsdb-cache

exec /usr/bin/loki -config.file=/etc/loki/config.yaml "$@"
