package repo

import (
	"context"
)

type SchemaSettings interface {
	ConnectionPool(ctx context.Context, r Role) (Pool, error)
	GetRolePassword(r Role) (string, error)
	ConnectionInfo() (dbName, host string, port int)
	NewSchemaRepo() Schema
	SchemaInitializer(tx Tx) SchemaInitializer
}
