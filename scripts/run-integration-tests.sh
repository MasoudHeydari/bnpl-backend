#!/usr/bin/env bash

set -Eeuo pipefail

die() {
    printf 'integration test setup: %s\n' "$*" >&2
    exit 1
}

require_external_environment() {
    local name
    for name in \
        COLLECTIONS_TEST_DATABASE_URL \
        DATABASE_SECURITY_TEST_URL \
        COLLECTIONS_RUNTIME_TEST_URL \
        POSTGRES_HOST \
        POSTGRES_PORT \
        POSTGRES_DB \
        POSTGRES_USER \
        POSTGRES_PASSWORD; do
        if [[ -z "${!name:-}" ]]; then
            die "${name} is required when INTEGRATION_POSTGRES_MODE=external"
        fi
    done
}

if [[ "$#" -eq 0 ]]; then
    die "test command is required"
fi

mode="${INTEGRATION_POSTGRES_MODE:-auto}"
case "$mode" in
    external)
        require_external_environment
        printf '%s\n' 'Using the explicitly configured disposable PostgreSQL test cluster.'
        exec "$@"
        ;;
    auto) ;;
    *) die "unsupported INTEGRATION_POSTGRES_MODE: ${mode} (expected auto or external)" ;;
esac

engine="${INTEGRATION_CONTAINER_ENGINE:-docker}"
case "$engine" in
    docker | podman) ;;
    *) die "unsupported container engine: ${engine} (expected docker or podman)" ;;
esac
command -v "$engine" >/dev/null 2>&1 || die "${engine} is not installed"

image="${INTEGRATION_POSTGRES_IMAGE:-postgres:18-alpine}"
container="bnpl-integration-${UID:-0}-$$"
database="bnpl_integration"
admin_user="admin"
admin_password="bnpl-integration-admin"
runtime_user="bnpl"
runtime_password="security-test-runtime"
started=false

cleanup() {
    if [[ "$started" == true ]]; then
        "$engine" rm --force "$container" >/dev/null 2>&1 || true
    fi
}

on_exit() {
    local status=$?
    trap - EXIT INT TERM
    cleanup
    exit "$status"
}

trap on_exit EXIT
trap 'exit 130' INT TERM

printf 'Starting isolated PostgreSQL container %s from %s.\n' "$container" "$image"
"$engine" run --detach --rm \
    --name "$container" \
    --publish '127.0.0.1::5432' \
    --env "POSTGRES_DB=${database}" \
    --env "POSTGRES_USER=${admin_user}" \
    --env "POSTGRES_PASSWORD=${admin_password}" \
    "$image" >/dev/null
started=true

port=""
for _ in {1..30}; do
    port_output=$("$engine" port "$container" '5432/tcp' 2>/dev/null || true)
    port=$(sed -n 's/.*:\([0-9][0-9]*\)$/\1/p' <<<"$port_output" | head -n 1)
    [[ -n "$port" ]] && break
    sleep 1
done
[[ -n "$port" ]] || die "could not determine the isolated PostgreSQL port"

ready=false
for _ in {1..60}; do
    if "$engine" exec "$container" pg_isready --quiet --username "$admin_user" --dbname "$database"; then
        ready=true
        break
    fi
    sleep 1
done
[[ "$ready" == true ]] || die "isolated PostgreSQL did not become ready"

baseline="pkg/adapter/db/postgres/migrationsrp/000_schema.sql"
[[ -f "$baseline" ]] || die "baseline schema not found: ${baseline}"

"$engine" exec "$container" psql \
    --set ON_ERROR_STOP=1 \
    --username "$admin_user" \
    --dbname "$database" \
    --command "CREATE SCHEMA bnpl; CREATE ROLE ${runtime_user} LOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '${runtime_password}';" \
    >/dev/null
"$engine" exec --interactive "$container" psql \
    --set ON_ERROR_STOP=1 \
    --username "$admin_user" \
    --dbname "$database" \
    <"$baseline" \
    >/dev/null
"$engine" exec "$container" psql \
    --set ON_ERROR_STOP=1 \
    --username "$admin_user" \
    --dbname "$database" \
    --command "ALTER ROLE ${admin_user} IN DATABASE ${database} SET search_path = bnpl, pg_catalog;" \
    >/dev/null

admin_url="postgres://${admin_user}:${admin_password}@127.0.0.1:${port}/${database}?sslmode=disable"
runtime_url="postgres://${runtime_user}:${runtime_password}@127.0.0.1:${port}/${database}?sslmode=disable"

printf 'PostgreSQL is ready on loopback port %s; running integration tests.\n' "$port"
env \
    COLLECTIONS_TEST_DATABASE_URL="$admin_url" \
    DATABASE_SECURITY_TEST_URL="$admin_url" \
    COLLECTIONS_RUNTIME_TEST_URL="$runtime_url" \
    POSTGRES_HOST='127.0.0.1' \
    POSTGRES_PORT="$port" \
    POSTGRES_DB="$database" \
    POSTGRES_USER="$admin_user" \
    POSTGRES_PASSWORD="$admin_password" \
    POSTGRES_SSL_MODE='disable' \
    "$@"
