package migrationsrp

import (
	"context"
	_ "embed"
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

// Settler initializes a freshly created schema by executing embedded SQL
// assets inside a caller-managed transaction.
type Settler struct {
	tx repo.Tx // destination database transaction
}

// NewSettler constructs a schema initializer bound to tx.
// The transaction must remain open for the lifetime of the initializer call.
func NewSettler(tx repo.Tx) *Settler {
	return &Settler{tx: tx}
}

//go:embed 000_schema.sql
var schemaDDLStatements string

//go:embed 000_dev.sql
var devDataStatements string

//go:embed 000_prod.sql
var prodDataStatements string

// InitDevSchema creates the base schema and inserts development seed data
// inside the caller-managed transaction.
func (sm1 *Settler) InitDevSchema(ctx context.Context) error {
	if _, err := sm1.tx.Exec(ctx, schemaDDLStatements); err != nil {
		return fmt.Errorf(
			"creating tables: %w", err,
		)
	}

	if _, err := sm1.tx.Exec(ctx, devDataStatements); err != nil {
		return fmt.Errorf(
			"filling tables with development data: %w", err,
		)
	}

	return nil
}

// InitProdSchema creates the base schema and inserts production seed data
// inside the caller-managed transaction.
func (sm1 *Settler) InitProdSchema(ctx context.Context) error {
	if _, err := sm1.tx.Exec(ctx, schemaDDLStatements); err != nil {
		return fmt.Errorf(
			"creating tables: %w", err,
		)
	}

	if _, err := sm1.tx.Exec(ctx, prodDataStatements); err != nil {
		return fmt.Errorf(
			"filling tables with production data: %w", err,
		)
	}

	return nil
}
