#!/bin/sh
# Build all four images first. No database, listener or external provider starts.
set -eu
version=${IMAGE_VERSION:-v0.1.0}
runtime_name=${IMAGE_NAME:-bnpl-backend}
admin_name=${DBADMIN_IMAGE_NAME:-$runtime_name-dbadmin}
work=$(mktemp -d)
container=
cleanup() {
	if [ -n "$container" ]; then docker rm "$container" >/dev/null; fi
	rm -rf "$work"
}
trap cleanup EXIT
for environment in development production; do
	for kind in runtime dbadmin; do
		if [ "$kind" = runtime ]; then
			name=$runtime_name
			binary=bnpl
			other=bnpl-dbadmin
		else
			name=$admin_name
			binary=bnpl-dbadmin
			other=bnpl
		fi
		image="$name:$version-$environment"
		if docker image inspect --format '{{json .Config.Env}}' "$image" | grep -Eq 'POSTGRES_(ADMIN|MIGRATOR|NORMAL).*PASS='; then
			echo "Database credentials baked into image" >&2
			exit 1
		fi
		test "$(docker image inspect --format '{{.Config.User}}' "$image")" = bnpluser
		docker run --rm --entrypoint /bin/sh "$image" -c \
			"test -x ./$binary && test ! -e ./$other && test -f configs/$kind.yaml && test ! -e .env && test ! -e .pgpass && test \"\$(id -u)\" -ne 0"
		docker run --rm --entrypoint "./$binary" "$image" --help >/dev/null
		if [ "$kind" = runtime ]; then
			docker run --rm --entrypoint /bin/sh "$image" -c 'test ! -e configs/dbadmin.yaml && test ! -e assets/db'
			if docker run --rm --entrypoint ./bnpl "$image" db >/dev/null 2>&1; then
				echo "Runtime image exposes db" >&2
				exit 1
			fi
		else
			docker run --rm "$image" >/dev/null
			docker run --rm --entrypoint /bin/sh "$image" -c 'test ! -e configs/runtime.yaml && test -f assets/db/migrations/000005_runtime_privileges.sql && test -f assets/db/audits/installment_integrity.sql'
			if docker run --rm "$image" serve >/dev/null 2>&1; then
				echo "Admin image exposes serve" >&2
				exit 1
			fi
			if [ "$environment" = production ] && docker run --rm "$image" db init-dev >/dev/null 2>&1; then
				echo "Production admin image exposes reset" >&2
				exit 1
			fi
		fi
		container=$(docker create --entrypoint /bin/sh "$image" -c true)
		docker cp "$container:/home/bnpluser/app/$binary" "$work/$binary-$environment" >/dev/null
		docker rm "$container" >/dev/null
		container=
		go version -m "$work/$binary-$environment" >"$work/build-info"
		grep -q -- "-tags=go_json,nomsgpack,$environment" "$work/build-info"
		echo "$image: contents, command boundaries, unprivileged user and build profile verified."
	done
done
