package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MigrationsQueryer applies migration statements.
type MigrationsQueryer interface {
	// Migrate applies migStatements under the given migration name.
	Migrate(ctx context.Context, migName, migStatements string) error
}

// MigrationsTxQueryer extends MigrationsQueryer with atomic registration.
type MigrationsTxQueryer interface {
	// MigrationsQueryer provides shared migration statement execution.
	MigrationsQueryer

	// Register records that a migration was applied inside the current transaction.
	Register(ctx context.Context, migName, checksum string, createdAt time.Time) (model.Migration, error)
}

// MigrationsConnQueryer exposes migration coordination and non-transactional execution.
type MigrationsConnQueryer interface {
	// MigrationsQueryer provides shared migration statement execution.
	MigrationsQueryer

	// AcquireLock obtains the migration advisory lock.
	AcquireLock(ctx context.Context) error

	// ReleaseLock releases the migration advisory lock.
	ReleaseLock(ctx context.Context) error

	// ListApplied returns migrations already recorded as applied.
	ListApplied(ctx context.Context) ([]model.Migration, error)

	// Register records that a migration was applied.
	Register(ctx context.Context, migName, checksum string, createdAt time.Time) (model.Migration, error)

	// MarkDirty records the start of a non-transactional migration before any
	// schema statement is executed.
	MarkDirty(ctx context.Context, migName, checksum string, startedAt time.Time) (model.Migration, error)

	// MarkApplied clears the dirty marker after non-transactional SQL completed.
	MarkApplied(ctx context.Context, migName, checksum string, appliedAt time.Time) (model.Migration, error)
}

// Migrations creates connection- and transaction-bound query helpers.
type Migrations interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) MigrationsConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) MigrationsTxQueryer
}
