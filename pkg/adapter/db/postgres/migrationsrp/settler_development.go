//go:build development

package migrationsrp

import (
	"context"
	_ "embed"
	"fmt"
)

//go:embed 000_dev.sql
var devDataStatements string

// InitDevSchema creates the base schema and inserts development seed data
// inside the caller-managed transaction.
func (sm1 *Settler) InitDevSchema(ctx context.Context) error {
	const op = "migrationsrp.Settler.InitDevSchema"

	if err := validateBaselineSchema(); err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if _, err := sm1.tx.Exec(ctx, schemaDDLStatements); err != nil {
		return fmt.Errorf(
			"%s: creating tables: %w", op, err,
		)
	}

	if _, err := sm1.tx.Exec(ctx, devDataStatements); err != nil {
		return fmt.Errorf(
			"%s: filling tables with development data: %w", op, err,
		)
	}

	return nil
}
