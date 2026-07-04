# Incremental database migrations

`pkg/adapter/db/postgres/migrationsrp/000_schema.sql` is the frozen pre-production baseline.
Do not evolve the schema by editing that file after the baseline is established.

Every schema change must be added here as a lexically ordered `.sql` file, for example:

```text
000001_add_contract_provider_ref.sql
000002_create_reconciliation_index.sql
```

Fresh `db init-dev` / `db init-prod` installations apply the frozen baseline first and then
run this same incremental migration chain. Existing databases run only pending migrations.

Use `-- bnpl:transaction=off` as the first non-empty line only when PostgreSQL requires a
non-transactional operation such as `CREATE INDEX CONCURRENTLY`. Those migrations are
recorded as dirty before execution and require explicit operator verification if execution
or finalization fails.

Prefer expand/contract changes for rolling deployments. Do not drop/rename a column in the
same release that introduces its replacement.
