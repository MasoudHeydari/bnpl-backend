package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MigrationsQueryer applies migration statements.
type MigrationsQueryer interface {
	// Migrate executes the migration statements for migName.
	// Implementations are expected to use migName for tracing or error context
	// and to execute migStatements exactly as supplied by the migration use case.
	Migrate(ctx context.Context, migName, migStatements string) error
}

// MigrationsTxQueryer extends MigrationsQueryer with migration registration
// inside a transaction.
type MigrationsTxQueryer interface {
	MigrationsQueryer

	// Register persists migName as an applied migration within the current transaction.
	// Callers use this after Migrate so execution and registration commit atomically.
	Register(ctx context.Context, migName string, createdAt time.Time) (model.Migration, error)
}

// MigrationsConnQueryer exposes migration execution on a plain connection.
type MigrationsConnQueryer interface {
	MigrationsQueryer
}

// Migrations creates connection- and transaction-bound query helpers for the
// migration repository.
type Migrations interface {
	// Conn binds the repository to c for non-transactional migration operations.
	Conn(Conn) MigrationsConnQueryer

	// Tx binds the repository to tx for transactional migration execution and
	// registration.
	Tx(Tx) MigrationsTxQueryer
}
