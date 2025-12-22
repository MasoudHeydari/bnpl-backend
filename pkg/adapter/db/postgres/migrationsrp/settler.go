package migrationsrp

import (
	"context"
	_ "embed"
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type Settler struct {
	tx repo.Tx // destination database transaction
}

func NewSettler(tx repo.Tx) *Settler {
	return &Settler{tx: tx}
}

//go:embed 000_schema.sql
var schemaDDLStatements string

//go:embed 000_dev.sql
var devDataStatements string

//go:embed 000_prod.sql
var prodDataStatements string

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
