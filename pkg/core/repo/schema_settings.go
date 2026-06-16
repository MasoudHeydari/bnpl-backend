package repo

import (
	"context"
)

// SchemaSettings exposes the configuration and repository dependencies needed
// by the database initialization use case.
type SchemaSettings interface {
	// ConnectionPool opens a pool for the requested database role.
	ConnectionPool(ctx context.Context, r Role) (Pool, error)
	// GetRolePassword returns the configured password for the requested role.
	GetRolePassword(r Role) (string, error)
	// ConnectionInfo returns database name, host, and port for operational use.
	ConnectionInfo() (dbName, host string, port int)
	// NewSchemaRepo returns the schema repository implementation.
	NewSchemaRepo() Schema
	// SchemaInitializer binds a schema initializer to tx.
	SchemaInitializer(tx Tx) SchemaInitializer
}
