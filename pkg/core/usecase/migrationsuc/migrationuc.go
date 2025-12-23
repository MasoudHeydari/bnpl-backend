package migrationsuc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type UseCase struct {
	pool       repo.Pool
	migrations repo.Migrations
	migDirPath string
}

func New(pool repo.Pool, migrations repo.Migrations, migDirPath string) *UseCase {
	return &UseCase{
		pool:       pool,
		migrations: migrations,
		migDirPath: migDirPath,
	}
}

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
