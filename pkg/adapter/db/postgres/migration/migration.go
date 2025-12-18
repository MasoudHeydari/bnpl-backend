package migration

import (
	"context"
	_ "embed"
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type Migrations struct {
	tx      repo.Tx
	dirPath string
}

func New(tx repo.Tx, dirPath string) repo.Migrator {
	return &Migrations{tx: tx, dirPath: dirPath}
}

func NewInitializer(tx repo.Tx) repo.SchemaInitializer {
	return NewSettler(tx)
}

func (m *Migrations) MigrateAll(ctx context.Context) error {
	return m.migrateToLastMinorVersion(ctx)
}

func (m *Migrations) MigrateSome(ctx context.Context, migrationsName string) error {
	// TODO: Implement this later...
	return nil
}

var lastMinorVersionStatements string

func (m *Migrations) migrateToLastMinorVersion(
	ctx context.Context,
) error {
	if _, err := m.tx.Exec(
		ctx, lastMinorVersionStatements,
	); err != nil {
		return fmt.Errorf(
			"failed to apply %q", err,
		)
	}
	return nil
}
