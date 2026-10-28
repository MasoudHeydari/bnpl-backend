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
GIN_HOST_VALUE="${GIN_HOST:-}"
GIN_PORT_VALUE="${GIN_PORT:-}"
METRICS_HOST_VALUE="${METRICS_HOST:-}"
METRICS_PORT_VALUE="${METRICS_PORT:-}"
METRICS_PATH_VALUE="${METRICS_PATH:-/metrics}"
APP_ENV="${APP_ENVIRONMENT:-}"

# Print a startup banner.
# Development gets a detailed operator-oriented dump.
# Production (and any non-development environment) stays minimal to reduce
# operational reconnaissance surface in shared log sinks.
log "=================================================="
log "BNPL Startup Script"
log "APP_ENVIRONMENT=${APP_ENV}"
log "CONFIG_FILE=${CONFIG_FILE:-}"
log "LISTEN_ADDR=${GIN_HOST_VALUE}:${GIN_PORT_VALUE}"
log "LOG_LEVEL=${LOG_LEVEL:-}"

if [ "${APP_ENV}" = "development" ]; then
	log "APP_EXECUTABLE=${APP}"
	log "GIN_MODE=${GIN_MODE:-}"
	log "SWAGGER_ENABLED=${GIN_SWAGGER_ENABLED:-}"
	log "HEALTH_ENABLED=${HEALTH_ENABLED:-}"
	log "METRICS_PROVIDER=${METRICS_PROVIDER:-}"
	log "METRICS_ENABLED=${METRICS_ENABLED:-}"
	if [ "${METRICS_ENABLED:-}" = "true" ]; then
		log "METRICS_ADDR=${METRICS_HOST_VALUE}:${METRICS_PORT_VALUE}${METRICS_PATH_VALUE}"
	fi
	log "LOG_PROVIDER=${LOG_PROVIDER:-}"
	log "LOG_FORMAT=${LOG_FORMAT:-}"
	log "MESSAGING_PROVIDER=${MESSAGING_PROVIDER:-}"
	log "CACHE_PROVIDER=${CACHE_PROVIDER:-}"
	log "TASK_PROVIDER=${TASK_PROVIDER:-}"
	log "EXTERNAL_CREDIT_ASSESSMENT_PROVIDER=${EXTERNAL_CREDIT_ASSESSMENT_PROVIDER:-}"
	log "EXTERNAL_FINANCIAL_CORE_PROVIDER=${EXTERNAL_FINANCIAL_CORE_PROVIDER:-}"
	log "EXTERNAL_IDENTITY_PROVIDER=${EXTERNAL_IDENTITY_PROVIDER:-}"
	log "POSTGRES_HOST=${DB_HOST}"
	log "POSTGRES_PORT=${DB_PORT}"
	log "POSTGRES_DB=${POSTGRES_DB:-}"
	log "Working Directory=$(pwd)"
	log "Current User=$(whoami)"
	log "Hostname=$(hostname 2>/dev/null || echo unknown)"
fi

log "=================================================="
log "Starting application..."

# Replace shell with the Go application so it becomes PID 1.
exec "${APP}"
