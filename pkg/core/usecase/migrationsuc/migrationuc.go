package migrationsuc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

// UseCase orchestrates filesystem-backed SQL migrations against the target database.
// It assumes the migration repository and pool coordinate a single database and
// should not be used concurrently to apply the same migration set.
type UseCase struct {
	pool       repo.Pool
	migrations repo.Migrations
	migDirPath string
}

// New constructs a migration use case.
// The caller retains ownership of pool and migrations and must ensure they
// remain valid for the lifetime of any use case calls.
func New(pool repo.Pool, migrations repo.Migrations, migDirPath string) *UseCase {
	return &UseCase{
		pool:       pool,
		migrations: migrations,
		migDirPath: migDirPath,
	}
}

// MigrateAll reads migration files from the configured directory and applies
// them sequentially by filename order returned from the filesystem.
// Each migration runs in its own transaction through MigrateOne. The method
// returns on the first failure and is not safe to run concurrently against the
// same database.
func (uc *UseCase) MigrateAll(ctx context.Context) error {
	migrationsFileName, err := os.ReadDir(uc.migDirPath)
	if err != nil {
		return fmt.Errorf("MigrateAll.os.readDir: %w", err)
	}
	for _, migName := range migrationsFileName {
		err = uc.MigrateOne(ctx, migName.Name())
		if err != nil {
			return fmt.Errorf("MigrateAll.uc.MigrateOne: %w", err)
		}
	}

	return nil
}

// MigrateOne applies a single migration file and registers it in the
// `migrations` table within the same transaction.
// The method reads the migration file from disk, executes its SQL statements,
// and records completion only if all statements commit successfully.
func (uc *UseCase) MigrateOne(ctx context.Context, migName string) error {
	migAbsPath := filepath.Join(uc.migDirPath, migName)
	migStatementsBytes, err := os.ReadFile(migAbsPath)
	if err != nil {
		return fmt.Errorf("MigrateOne.os.readFile: %w", err)
	}

	err = uc.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			return conn.Tx(
				ctx,
				func(ctx context.Context, tx repo.Tx) error {
					// Execution and registration share a transaction so a failed
					// registration cannot leave an applied-but-untracked migration.
					q := uc.migrations.Tx(tx)
					err = q.Migrate(ctx, migName, string(migStatementsBytes))
					if err != nil {
						return fmt.Errorf("MigrateOne.q.Migrate: %w", err)
					}

					err = q.Register(ctx, migName)
					if err != nil {
						return fmt.Errorf("MigrateOne.q.Register: %w", err)
					}

					return nil
				},
			)
		},
	)
	if err != nil {
		return fmt.Errorf("MigrateOne.pool.Conn: %w", err)
	}

	return nil
}
