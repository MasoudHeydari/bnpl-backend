package repo

import (
	"context"
)

type SchemaSettings interface {
	ConnectionPool(ctx context.Context, r Role) (Pool, error)
	ConnectionInfo() (dbName, host string, port int)
	NewSchemaRepo() Schema
	SchemaInitializer(tx Tx) SchemaInitializer
}
