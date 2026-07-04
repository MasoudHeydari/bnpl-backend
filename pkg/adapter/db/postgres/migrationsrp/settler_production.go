//go:build production

package migrationsrp

import (
	"context"
	_ "embed"
	"fmt"
)

//go:embed 000_prod.sql
var prodDataStatements string

// InitProdSchema creates the base schema and inserts production seed data
// inside the caller-managed transaction.
// Callers must ensure schema bnpl was created by a non-destructive production
// bootstrap (init-prod) and does not already contain application objects.
func (sm1 *Settler) InitProdSchema(ctx context.Context) error {
	const op = "migrationsrp.Settler.InitProdSchema"

	if err := validateBaselineSchema(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if _, err := sm1.tx.Exec(ctx, schemaDDLStatements); err != nil {
		return fmt.Errorf(
			"%s: creating tables: %w", op, err,
		)
	}

	if _, err := sm1.tx.Exec(ctx, prodDataStatements); err != nil {
		return fmt.Errorf(
			"%s: filling tables with production data: %w", op, err,
		)
	}

	return nil
}
