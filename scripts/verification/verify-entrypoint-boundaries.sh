#!/bin/sh
# Compile first with all four Make targets. No DB/network service is started.
set -eu
runtime_deps=$(mktemp)
admin_deps=$(mktemp)
trap 'rm -f "$runtime_deps" "$admin_deps"' EXIT
for environment in development production; do
	go list -tags="go_json nomsgpack $environment" -deps ./cmd/bnpl >"$runtime_deps"
	go list -tags="go_json nomsgpack $environment" -deps ./cmd/bnpl-dbadmin >"$admin_deps"
	if grep -Eq '/(migrationsuc|migrationsrp|schemarp|dbadminconfig)$|/cmd/bnpl-dbadmin/' "$runtime_deps"; then
		echo "Runtime imports privileged composition" >&2
		exit 1
	fi
	if grep -Eq 'gin-gonic|IBM/sarama|redis/go-redis|/adapter/config$|/cmd/bnpl/bootstrap|/core/usecase/(checkout|contract|installment|collections)|/adapter/external/' "$admin_deps"; then
		echo "DB Admin imports business runtime dependencies" >&2
		exit 1
	fi
	runtime="./build/bin/bnpl-$environment"
	admin="./build/bin/bnpl-dbadmin-$environment"
	"$runtime" --help >/dev/null
	"$admin" --help >/dev/null
	if "$runtime" db >/dev/null 2>&1; then
		echo "Runtime exposes db" >&2
		exit 1
	fi
	if "$admin" serve >/dev/null 2>&1; then
		echo "Admin exposes serve" >&2
		exit 1
	fi
	if "$admin" policy-assignment >/dev/null 2>&1; then
		echo "Admin exposes business CLI" >&2
		exit 1
	fi
done
if ./build/bin/bnpl-dbadmin-production db init-dev >/dev/null 2>&1; then
	echo "Production admin exposes destructive reset" >&2
	exit 1
fi
echo "Entrypoint dependency and command boundaries verified."
