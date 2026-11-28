#!/usr/bin/env bash

set -Eeuo pipefail

# ==================================================
# Default configuration
# ==================================================

DB_HOST="57cfb9fc-72e1-40d3-ac92-ac9b807a5e41.hadb.ir"
DB_PORT="26532"
DB_USER="postgres"

POSTGRES_IMAGE="docker.arvancloud.ir/postgres:18"
DISCOVERY_DB="postgres"

TARGET_DB=""
TARGET_SCHEMA=""

LOCK_TIMEOUT="60s"

FORCE=false
DRY_RUN=false
TERMINATE_CONNECTIONS=false
PRESERVE_MIGRATIONS=true

# ==================================================
# Logging
# ==================================================

log_info() {
    printf '[INFO] %s\n' "$*"
}

log_success() {
    printf '[SUCCESS] %s\n' "$*"
}

log_warning() {
    printf '[WARNING] %s\n' "$*" >&2
}

log_error() {
    printf '[ERROR] %s\n' "$*" >&2
}

separator() {
    printf '%s\n' \
        '============================================================'
}

# ==================================================
# Usage
# ==================================================

usage() {
    cat <<EOF
Usage: $(basename "$0") [OPTIONS]

Connection options:
  --host HOST              PostgreSQL host (default: $DB_HOST)
  --port PORT              PostgreSQL port (default: $DB_PORT)
  --user USER              PostgreSQL username (default: $DB_USER)
  --username USER          Alias for --user
  --role USER              Alias for --user
  --image IMAGE            Docker image (default: $POSTGRES_IMAGE)

Target options:
  --db DATABASE            Target a specific database
  --schema SCHEMA          Target a specific schema

Cleanup options:
  --force                  Suppress ordinary triggers
  --terminate-connections  Terminate other database connections
  --lock-timeout TIMEOUT   Lock timeout (default: $LOCK_TIMEOUT)
  --dry-run                Report without deleting records
  --preserve-migrations    Preserve migration history (default)
  --include-migrations     Also truncate migration history

General options:
  -h, --help               Show this help message

If no database or schema is specified, all non-template
databases and non-system schemas are processed.

Only Docker is required.
The PostgreSQL password is requested once.
EOF
}

# ==================================================
# Parse command-line arguments
# ==================================================

while [[ $# -gt 0 ]]; do
    case "$1" in
        --host|--port|--user|--username|--role|--image|\
        --db|--schema|--lock-timeout)

            if [[ $# -lt 2 || -z "$2" || "$2" == --* ]]; then
                log_error "Missing value for $1"
                exit 1
            fi

            case "$1" in
                --host)
                    DB_HOST="$2"
                    ;;
                --port)
                    DB_PORT="$2"
                    ;;
                --user|--username|--role)
                    DB_USER="$2"
                    ;;
                --image)
                    POSTGRES_IMAGE="$2"
                    ;;
                --db)
                    TARGET_DB="$2"
                    ;;
                --schema)
                    TARGET_SCHEMA="$2"
                    ;;
                --lock-timeout)
                    LOCK_TIMEOUT="$2"
                    ;;
            esac

            shift 2
            ;;

        --force)
            FORCE=true
            shift
            ;;

        --dry-run)
            DRY_RUN=true
            shift
            ;;

        --terminate-connections)
            TERMINATE_CONNECTIONS=true
            shift
            ;;

        --preserve-migrations)
            PRESERVE_MIGRATIONS=true
            shift
            ;;

        --include-migrations)
            PRESERVE_MIGRATIONS=false
            shift
            ;;

        -h|--help)
            usage
            exit 0
            ;;

        *)
            log_error "Unknown argument: $1"
            usage
            exit 1
            ;;
    esac
done

# ==================================================
# Validate configuration
# ==================================================

if [[ ! "$DB_PORT" =~ ^[0-9]{1,5}$ ]]; then
    log_error "Invalid PostgreSQL port: $DB_PORT"
    exit 1
fi

if (( 10#$DB_PORT < 1 || 10#$DB_PORT > 65535 )); then
    log_error "PostgreSQL port must be between 1 and 65535."
    exit 1
fi

if [[ ! "$LOCK_TIMEOUT" =~ ^[0-9]+(ms|s|min|h)?$ ]]; then
    log_error "Invalid lock timeout: $LOCK_TIMEOUT"
    exit 1
fi

# ==================================================
# Check Docker
# ==================================================

if ! command -v docker >/dev/null 2>&1; then
    log_error "Docker is not installed."
    exit 1
fi

if ! docker info >/dev/null 2>&1; then
    log_error "Docker daemon is not accessible."
    exit 1
fi

# ==================================================
# Pull PostgreSQL client image
# ==================================================

if ! docker image inspect "$POSTGRES_IMAGE" >/dev/null 2>&1; then
    log_info "Pulling PostgreSQL image: $POSTGRES_IMAGE"
    docker pull "$POSTGRES_IMAGE"
fi

# ==================================================
# Display configuration
# ==================================================

echo
separator
echo "POSTGRESQL CLEANUP CONFIGURATION"
separator

printf '%-25s %s\n' \
    "Host:"                   "$DB_HOST" \
    "Port:"                   "$DB_PORT" \
    "User:"                   "$DB_USER" \
    "Docker image:"           "$POSTGRES_IMAGE" \
    "Target database:"        "${TARGET_DB:-ALL}" \
    "Target schema:"          "${TARGET_SCHEMA:-ALL}" \
    "Lock timeout:"           "$LOCK_TIMEOUT" \
    "Force:"                  "$FORCE" \
    "Dry run:"                "$DRY_RUN" \
    "Terminate connections:"  "$TERMINATE_CONNECTIONS" \
    "Preserve migrations:"    "$PRESERVE_MIGRATIONS"

separator
echo

if [[ "$DRY_RUN" == true ]]; then
    log_info "DRY RUN: No records will be deleted."
else
    log_warning "All records in the targeted tables will be deleted."
    log_warning "This operation is intended for development environments."
fi

if [[ "$FORCE" == true && "$DRY_RUN" == false ]]; then
    log_warning "Ordinary database triggers will be suppressed."
fi

if [[ "$TERMINATE_CONNECTIONS" == true && "$DRY_RUN" == false ]]; then
    log_warning "Other connections to target databases will be terminated."
fi

echo

# ==================================================
# Password handling
# ==================================================

IFS= read -r -s -p \
    "PostgreSQL password for ${DB_USER}: " \
    DB_PASSWORD

echo

PGPASS_FILE="$(mktemp)"

cleanup() {
    rm -f -- "$PGPASS_FILE"
}

trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

chmod 600 "$PGPASS_FILE"

escape_pgpass() {
    local value="$1"

    value="${value//\\/\\\\}"
    value="${value//:/\\:}"

    printf '%s' "$value"
}

printf '%s:%s:*:%s:%s\n' \
    "$(escape_pgpass "$DB_HOST")" \
    "$(escape_pgpass "$DB_PORT")" \
    "$(escape_pgpass "$DB_USER")" \
    "$(escape_pgpass "$DB_PASSWORD")" \
    > "$PGPASS_FILE"

unset DB_PASSWORD

# ==================================================
# Docker-based PostgreSQL client
# ==================================================

psql() {
    docker run \
        --rm \
        --interactive \
        --network host \
        --user "$(id -u):$(id -g)" \
        --volume "${PGPASS_FILE}:/tmp/pgpass:ro" \
        --env PGPASSFILE=/tmp/pgpass \
        --env PGCONNECT_TIMEOUT=10 \
        --env PGAPPNAME=postgres-database-cleanup \
        --entrypoint psql \
        "$POSTGRES_IMAGE" \
        --no-psqlrc \
        --no-password \
        --host="$DB_HOST" \
        --port="$DB_PORT" \
        --username="$DB_USER" \
        --set=ON_ERROR_STOP=1 \
        "$@"
}

# ==================================================
# Discover databases
# ==================================================

echo
log_info "Discovering PostgreSQL databases..."

DATABASES_RAW="$(
    psql \
        --quiet \
        --tuples-only \
        --no-align \
        --dbname="$DISCOVERY_DB" \
        --set=target_db="$TARGET_DB" <<'SQL'

SELECT datname
FROM pg_database
WHERE datallowconn = TRUE
  AND datistemplate = FALSE
  AND (
      :'target_db' = ''
      OR datname = :'target_db'
  )
ORDER BY datname;

SQL
)"

if [[ -z "$DATABASES_RAW" ]]; then
    log_error "No matching databases found."
    exit 1
fi

mapfile -t DATABASES <<< "$DATABASES_RAW"

echo
log_info "Discovered ${#DATABASES[@]} database(s):"

printf '  - %s\n' "${DATABASES[@]}"

# ==================================================
# SQL cleanup procedure
# ==================================================

read -r -d '' TRUNCATE_SQL <<'SQL' || true

BEGIN;

SET LOCAL client_min_messages = NOTICE;
SET LOCAL row_security = off;

SELECT set_config(
    'lock_timeout',
    :'lock_timeout',
    true
) AS cleanup_lock_timeout \gset

SELECT
    set_config(
        'cleanup.target_schema',
        :'target_schema',
        true
    ) AS cleanup_schema,
    set_config(
        'cleanup.force',
        :'force',
        true
    ) AS cleanup_force,
    set_config(
        'cleanup.dry_run',
        :'dry_run',
        true
    ) AS cleanup_dry_run,
    set_config(
        'cleanup.preserve_migrations',
        :'preserve_migrations',
        true
    ) AS cleanup_preserve
\gset

-- =================================================
-- Cleanup
-- =================================================

DO $cleanup$
DECLARE
    target_schema TEXT :=
        current_setting('cleanup.target_schema');

    force_mode BOOLEAN :=
        current_setting('cleanup.force')::BOOLEAN;

    dry_run BOOLEAN :=
        current_setting('cleanup.dry_run')::BOOLEAN;

    preserve_migrations BOOLEAN :=
        current_setting('cleanup.preserve_migrations')::BOOLEAN;

    migration_tables TEXT[] := ARRAY[
        'migrations',
        'schema_migrations',
        'flyway_schema_history',
        'goose_db_version',
        'gorp_migrations',
        'atlas_schema_revisions',
        'databasechangelog',
        'databasechangeloglock',
        'alembic_version',
        '__efmigrationshistory'
    ];

    target_oids OID[];
    table_list TEXT;
    lock_list TEXT;

    tbl RECORD;

    row_count BIGINT;
    total_rows BIGINT := 0;
    total_tables INTEGER := 0;
    preserved_tables INTEGER := 0;

BEGIN

    -- =============================================
    -- Validate target schema
    -- =============================================

    IF target_schema <> ''
       AND NOT EXISTS (
           SELECT 1
           FROM pg_namespace
           WHERE nspname = target_schema
       )
    THEN
        RAISE EXCEPTION
            'Schema "%" does not exist.',
            target_schema;
    END IF;

    -- =============================================
    -- Discover target tables
    -- =============================================

    SELECT array_agg(
        c.oid ORDER BY n.nspname, c.relname
    )
    INTO target_oids
    FROM pg_class c
    JOIN pg_namespace n
        ON n.oid = c.relnamespace
    WHERE c.relkind IN ('r', 'p')
      AND n.nspname <> 'information_schema'
      AND n.nspname !~ '^pg_'
      AND (
          target_schema = ''
          OR n.nspname = target_schema
      )
      AND NOT (
          preserve_migrations
          AND lower(c.relname) = ANY(migration_tables)
      )
      AND NOT EXISTS (
          SELECT 1
          FROM pg_depend d
          WHERE d.classid = 'pg_class'::regclass
            AND d.objid = c.oid
            AND d.refclassid = 'pg_extension'::regclass
            AND d.deptype = 'e'
      );

    -- =============================================
    -- Report preserved migration tables
    -- =============================================

    IF preserve_migrations THEN

        FOR tbl IN
            SELECT
                n.nspname AS schema_name,
                c.relname AS table_name
            FROM pg_class c
            JOIN pg_namespace n
                ON n.oid = c.relnamespace
            WHERE c.relkind IN ('r', 'p')
              AND n.nspname <> 'information_schema'
              AND n.nspname !~ '^pg_'
              AND lower(c.relname) = ANY(migration_tables)
              AND (
                  target_schema = ''
                  OR n.nspname = target_schema
              )
            ORDER BY n.nspname, c.relname
        LOOP

            RAISE NOTICE 'PRESERVED: %.%',
                tbl.schema_name,
                tbl.table_name;

            preserved_tables := preserved_tables + 1;

        END LOOP;

    END IF;

    IF target_oids IS NULL THEN
        RAISE NOTICE 'No matching application tables found.';
        RETURN;
    END IF;

    -- =============================================
    -- Protect excluded inheritance descendants
    -- =============================================

    IF EXISTS (
        WITH RECURSIVE descendants(relid) AS (
            SELECT unnest(target_oids)

            UNION

            SELECT i.inhrelid
            FROM pg_inherits i
            JOIN descendants d
                ON d.relid = i.inhparent
        )
        SELECT 1
        FROM descendants
        WHERE NOT (relid = ANY(target_oids))
    ) THEN
        RAISE EXCEPTION
            'Target tables have excluded inheritance or partition descendants.';
    END IF;

    -- =============================================
    -- Build table and lock lists
    -- =============================================

    SELECT
        string_agg(
            CASE
                WHEN c.relkind = 'p' THEN
                    format('%I.%I', n.nspname, c.relname)
                ELSE
                    format('ONLY %I.%I', n.nspname, c.relname)
            END,
            ', ' ORDER BY n.nspname, c.relname
        ),
        string_agg(
            format('ONLY %I.%I', n.nspname, c.relname),
            ', ' ORDER BY n.nspname, c.relname
        )
    INTO table_list, lock_list
    FROM pg_class c
    JOIN pg_namespace n
        ON n.oid = c.relnamespace
    WHERE c.oid = ANY(target_oids);

    -- =============================================
    -- Lock tables before counting
    -- =============================================

    IF NOT dry_run THEN

        EXECUTE
            'LOCK TABLE ' || lock_list ||
            ' IN ACCESS EXCLUSIVE MODE';

    END IF;

    -- =============================================
    -- Count records
    -- =============================================

    RAISE NOTICE
        '------------------------------------------------------------';

    FOR tbl IN
        SELECT
            n.nspname AS schema_name,
            c.relname AS table_name
        FROM pg_class c
        JOIN pg_namespace n
            ON n.oid = c.relnamespace
        WHERE c.oid = ANY(target_oids)
        ORDER BY n.nspname, c.relname
    LOOP

        EXECUTE format(
            'SELECT COUNT(*) FROM ONLY %I.%I',
            tbl.schema_name,
            tbl.table_name
        )
        INTO row_count;

        total_rows := total_rows + row_count;
        total_tables := total_tables + 1;

        IF dry_run THEN

            RAISE NOTICE '[DRY RUN] %.%: % rows',
                tbl.schema_name,
                tbl.table_name,
                row_count;

        ELSE

            RAISE NOTICE '%.%: % rows to delete',
                tbl.schema_name,
                tbl.table_name,
                row_count;

        END IF;

    END LOOP;

    -- =============================================
    -- Dry-run mode
    -- =============================================

    IF dry_run THEN

        RAISE NOTICE
            '------------------------------------------------------------';

        RAISE NOTICE 'Database: %', current_database();
        RAISE NOTICE 'Target tables: %', total_tables;
        RAISE NOTICE 'Preserved migration tables: %', preserved_tables;
        RAISE NOTICE 'Rows eligible for deletion: %', total_rows;
        RAISE NOTICE 'No records deleted.';

        RETURN;

    END IF;

    -- =============================================
    -- Force mode
    -- =============================================

    IF force_mode THEN

        RAISE NOTICE
            'FORCE MODE: Suppressing ordinary triggers.';

        PERFORM set_config(
            'session_replication_role',
            'replica',
            true
        );

    END IF;

    -- =============================================
    -- Truncate all targeted tables
    -- =============================================

    EXECUTE
        'TRUNCATE TABLE ' || table_list ||
        ' RESTART IDENTITY RESTRICT';

    -- =============================================
    -- Print database summary
    -- =============================================

    RAISE NOTICE
        '------------------------------------------------------------';

    RAISE NOTICE 'Database: %', current_database();
    RAISE NOTICE 'Tables truncated: %', total_tables;
    RAISE NOTICE 'Preserved migration tables: %', preserved_tables;
    RAISE NOTICE 'Total rows deleted: %', total_rows;

    RAISE NOTICE
        '------------------------------------------------------------';

END;
$cleanup$;

COMMIT;

SQL

# ==================================================
# Terminate other database connections
# ==================================================

terminate_connections() {
    local database="$1"

    psql \
        --dbname="$database" \
        --quiet <<'SQL'

SELECT
    pid,
    usename,
    application_name,
    pg_terminate_backend(pid, 5000) AS terminated
FROM pg_stat_activity
WHERE datname = current_database()
  AND pid <> pg_backend_pid()
  AND backend_type = 'client backend';

SQL
}

# ==================================================
# Process databases
# ==================================================

SUCCESS_COUNT=0
FAILED_COUNT=0

SUCCESSFUL_DATABASES=()
FAILED_DATABASES=()

for DB_NAME in "${DATABASES[@]}"; do

    echo
    separator
    echo "DATABASE: $DB_NAME"
    separator

    # ----------------------------------------------
    # Terminate existing connections
    # ----------------------------------------------

    if [[ "$TERMINATE_CONNECTIONS" == true &&
          "$DRY_RUN" == false ]]; then

        log_info "Terminating other connections to $DB_NAME..."

        if ! terminate_connections "$DB_NAME"; then

            log_error "Failed to terminate connections."

            FAILED_DATABASES+=("$DB_NAME")
            ((FAILED_COUNT += 1))

            continue

        fi

    fi

    # ----------------------------------------------
    # Execute cleanup
    # ----------------------------------------------

    if psql \
        --dbname="$DB_NAME" \
        --set=target_schema="$TARGET_SCHEMA" \
        --set=lock_timeout="$LOCK_TIMEOUT" \
        --set=force="$FORCE" \
        --set=dry_run="$DRY_RUN" \
        --set=preserve_migrations="$PRESERVE_MIGRATIONS" \
        <<< "$TRUNCATE_SQL" 2>&1; then

        if [[ "$DRY_RUN" == true ]]; then
            log_success "Dry run completed: $DB_NAME"
        else
            log_success "Cleanup completed: $DB_NAME"
        fi

        SUCCESSFUL_DATABASES+=("$DB_NAME")
        ((SUCCESS_COUNT += 1))

    else

        log_error "Cleanup failed: $DB_NAME"

        FAILED_DATABASES+=("$DB_NAME")
        ((FAILED_COUNT += 1))

    fi

done

# ==================================================
# Final report
# ==================================================

echo
separator
echo "FINAL REPORT"
separator

printf '%-25s %s\n' \
    "Total databases:"      "${#DATABASES[@]}" \
    "Successful databases:" "$SUCCESS_COUNT" \
    "Failed databases:"     "$FAILED_COUNT" \
    "Dry run:"              "$DRY_RUN" \
    "Force:"                "$FORCE" \
    "Preserve migrations:"  "$PRESERVE_MIGRATIONS"

if [[ "$SUCCESS_COUNT" -gt 0 ]]; then

    echo
    echo "Successful databases:"

    printf '  [OK] %s\n' "${SUCCESSFUL_DATABASES[@]}"

fi

if [[ "$FAILED_COUNT" -gt 0 ]]; then

    echo
    echo "Failed databases:"

    printf '  [FAILED] %s\n' "${FAILED_DATABASES[@]}"

fi

separator

if [[ "$FAILED_COUNT" -gt 0 ]]; then

    log_error "Database cleanup finished with errors."
    exit 1

fi

if [[ "$DRY_RUN" == true ]]; then

    log_success "Dry run completed. No records were deleted."

else

    log_success "Database cleanup completed successfully."

fi

exit 0
