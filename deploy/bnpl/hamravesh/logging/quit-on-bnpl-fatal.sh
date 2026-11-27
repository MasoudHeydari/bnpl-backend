#!/bin/sh
# Supervisor eventlistener: exit the container when program:bnpl reaches FATAL.
# Implements the Supervisor event protocol (READY / RESULT).
set -eu

printf 'READY\n'

while true; do
	# Header line: "ver:3.0 server:supervisor ... len:NNN"
	IFS= read -r header || exit 0

	len=0
	old_ifs=$IFS
	IFS=' '
	# shellcheck disable=SC2086
	set -- $header
	IFS=$old_ifs

	for token in "$@"; do
		case "$token" in
		len:*)
			len=${token#len:}
			;;
		esac
	done

	payload=""
	if [ "$len" -gt 0 ] 2>/dev/null; then
		payload=$(dd bs=1 count="$len" 2>/dev/null || true)
	fi

	case "$payload" in
	*processname:bnpl*)
		echo "[quit-on-bnpl-fatal] bnpl FATAL — shutting down supervisord" >&2
		# SIGTERM to PID 1 (supervisord) ends the container.
		kill -s TERM 1 2>/dev/null || true
		;;
	esac

	printf 'RESULT 2\nOK'
done
