#!/bin/bash
# Creates isolated application databases on first Postgres boot.
# POSTGRES_DB (bnpl by default) is already created by the official entrypoint.
set -euo pipefail

for db in identity customer financial_core notification; do
  exists="$(psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER}" --dbname "${POSTGRES_DB}" \
    -tAc "SELECT 1 FROM pg_database WHERE datname = '${db}'")"
  if [[ "${exists}" != "1" ]]; then
    psql -v ON_ERROR_STOP=1 --username "${POSTGRES_USER}" --dbname "${POSTGRES_DB}" \
      -c "CREATE DATABASE ${db}"
  fi
done
