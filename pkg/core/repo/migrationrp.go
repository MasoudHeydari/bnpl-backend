package repo

import "context"

type MigrationsQueryer interface {
	Migrate(ctx context.Context, migName, migStatements string) error
}

type MigrationsTxQueryer interface {
	MigrationsQueryer

	Register(ctx context.Context, migName string) error
}

type MigrationsConnQueryer interface {
	MigrationsQueryer
}

type Migrations interface {
	Conn(Conn) MigrationsConnQueryer
	Tx(Tx) MigrationsTxQueryer
}
