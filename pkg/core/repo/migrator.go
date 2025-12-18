package repo

import "context"

type Migrator interface {
	MigrateAll(ctx context.Context) error
	MigrateSome(ctx context.Context, migrationsName string) error
}
