package repo

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// SchemaSettings exposes the configuration and repository dependencies needed
// by the database initialization use case.
type SchemaSettings interface {
	// ConnectionPool opens only an administrative or migrator-role pool.
	// logger is the shared process logger used for optional query logging.
	ConnectionPool(ctx context.Context, r Role, logger log.Logger) (Pool, error)
	// GetRolePassword returns the configured password for the requested role.
	GetRolePassword(r Role) (string, error)
	// ConnectionInfo returns database name, host, and port for operational use.
	ConnectionInfo() (dbName, host string, port int)
	// NewSchemaRepo returns the schema repository implementation.
	NewSchemaRepo() Schema
	// SchemaInitializer binds a schema initializer to tx.
	SchemaInitializer(tx Tx) SchemaInitializer
}
