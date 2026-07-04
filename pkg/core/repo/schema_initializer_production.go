//go:build production

package repo

import "context"

// SchemaInitializer populates a freshly created schema with base DDL and
// production seed data. Implementations use the transaction supplied at
// construction time.
type SchemaInitializer interface {
	// InitProdSchema applies production bootstrap DDL and seed data.
	InitProdSchema(ctx context.Context) error
}
