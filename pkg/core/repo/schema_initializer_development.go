//go:build development

package repo

import "context"

// SchemaInitializer populates a freshly created schema with base DDL and
// development seed data. Implementations use the transaction supplied at
// construction time.
type SchemaInitializer interface {
	// InitDevSchema applies development bootstrap DDL and seed data.
	InitDevSchema(ctx context.Context) error
}
