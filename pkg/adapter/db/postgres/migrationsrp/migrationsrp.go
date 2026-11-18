// Package migrationsrp implements the migration repository with PostgreSQL SQL
// execution primitives.
//
// It belongs to the infrastructure layer and provides the concrete repository
// used by the migration application use case.
//
// Pre-release note: 000_schema.sql is the single authoritative baseline. Dual-
// credit and CollectionPayment tables live in that baseline (not a separate
// upgrade file). Application startup never drops or recreates an existing
// database; local developers who already initialized a schema from an older
// baseline must recreate the development database (for example make db-init-
// development) so the complete 000_schema.sql is applied cleanly.
package migrationsrp
