#!/bin/sh

# Exit immediately when any command fails.
set -e

# Print timestamped log messages.
log() {
    echo "[$(date -Iseconds)] $*"
}

# Runtime defaults.
APP="${APP_EXECUTABLE:-./bnpl}"
DB_HOST="${POSTGRES_HOST:-bnpl-db}"
DB_PORT="${POSTGRES_PORT:-5432}"

log "=================================================="
log "BNPL Startup Script"
log "APP_EXECUTABLE=${APP}"
log "CONFIG_FILE=${CONFIG_FILE}"
log "DB_INIT_COMMAND=${DB_INIT_COMMAND}"
log "POSTGRES_HOST=${DB_HOST}"
log "POSTGRES_PORT=${DB_PORT}"
log "Working Directory=$(pwd)"
log "Current User=$(whoami)"
log "=================================================="

# Run database initialization with retries.
# This protects startup from the window where the TCP port is open
# but PostgreSQL is not fully ready for login/query execution yet.
run_db_init() {
    max_attempts="${DB_INIT_MAX_ATTEMPTS:-10}"
    wait_time="${DB_INIT_RETRY_INTERVAL_SECONDS:-3}"
    attempt=1

    while [ "$attempt" -le "$max_attempts" ]; do
        log "Running database initialization attempt ${attempt}/${max_attempts}..."

        if "${APP}" db "${DB_INIT_COMMAND}"; then
            log "[✓] Database initialization completed"
            return 0
        fi

        log "Database initialization failed; retrying in ${wait_time}s..."
        sleep "$wait_time"
        attempt=$((attempt + 1))
    done

    log "[✗] Database initialization failed after ${max_attempts} attempts"
    return 1
}

log "[1/2] Initializing database..."

# Run destructive DB initialization only when explicitly requested.
#
# Supported examples:
#   DB_INIT_COMMAND=init-dev
#   DB_INIT_COMMAND=init-prod
#
# If DB_INIT_COMMAND is empty, initialization is skipped.
if [ -n "${DB_INIT_COMMAND:-}" ]; then
    run_db_init
else
    log "Skipping database initialization because DB_INIT_COMMAND is not set"
fi

log "[2/2] Starting application..."

# Replace shell with the Go application so it becomes PID 1.
exec "${APP}"
