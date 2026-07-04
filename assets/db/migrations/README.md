# Incremental database migrations

The canonical **pre-release** database structure is
`pkg/adapter/db/postgres/migrationsrp/000_schema.sql`.

The first Production release freezes that baseline. Afterward, every schema
change must be a new lexically ordered `.sql` migration in this directory,
for example `000001_add_contract_provider_ref.sql`.

There are deliberately **no initial migrations**: the five development-only
migration files were squashed into the embedded baseline before the first
Production release. Fresh `db init-dev` or `db init-prod` installs the complete
baseline. `db migrate` applies only future incremental migrations and safely
succeeds if no `.sql` migrations exist.

Already-applied migrations are immutable. Editing even whitespace changes
their SHA-256 checksums and makes a previously migrated database reject replay.
Never rewrite database history to conceal a checksum mismatch.

Use `-- bnpl:transaction=off` as the first non-empty line only when PostgreSQL
requires a nontransactional operation such as `CREATE INDEX CONCURRENTLY`.
Dirty migrations require explicit operator inspection.

Prefer expand/contract migrations when rolling out schema changes.
