package repo

import "context"

// Schema creates connection- and transaction-bound query helpers for schema
// administration statements.
type Schema interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) SchemaConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) SchemaTxQueryer
}

// SchemaConnQueryer exposes schema administration operations on a plain
// connection.
type SchemaConnQueryer interface {
	// SchemaQueryer provides shared schema administration operations.
	SchemaQueryer
}

// SchemaTxQueryer exposes schema administration operations inside a
// transaction.
type SchemaTxQueryer interface {
	// SchemaQueryer provides shared schema administration operations.
	SchemaQueryer
}

// SchemaQueryer defines the schema-management operations required by database bootstrap workflows.
// Implementations are expected to execute privileged DDL against trusted
// application-controlled schema and role names, typically within the
// initialization use case's administrative transaction.
type SchemaQueryer interface {
	// CreateSchema creates schema when it does not already exist as part of the
	// application bootstrap flow.
	// Callers are expected to pass a trusted schema name because implementations
	// may need to interpolate identifiers into DDL statements.
	CreateSchema(ctx context.Context, schema string) error

	// CreateRoleIfNotExists ensures role exists with login capability and the
	// supplied password.
	// The operation is intended for administrative bootstrap and may require
	// elevated database privileges.
	CreateRoleIfNotExists(ctx context.Context, role Role, password string) error

	// GrantPrivileges grants the application role the privileges it needs on the
	// managed schema after schema creation.
	// Callers should treat schema and role as trusted identifiers controlled by
	// the application configuration.
	GrantPrivileges(ctx context.Context, schema string, role Role) error

	// SetSearchPath configures role so new sessions resolve objects from schema
	// by default.
	// This is part of one-time bootstrap rather than per-request query handling.
	SetSearchPath(ctx context.Context, schema string, role Role) error

	// Exists reports whether schema is already present in the database catalog.
	// Used by production bootstrap to refuse wiping an existing managed schema.
	Exists(ctx context.Context, schema string) (bool, error)

	// DropIfExists drops schema and all contained objects when it already exists.
	// The operation is destructive and is used only by development reset flows
	// such as `init-dev`.
	DropIfExists(ctx context.Context, schema string) error
}
