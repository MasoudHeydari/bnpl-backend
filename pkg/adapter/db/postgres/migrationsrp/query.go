package migrationsrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const migrationAdvisoryLockID int64 = 0x424e504c4d494752 // "BNPLMIGR"

// AcquireLock takes the migrations advisory lock for exclusive schema work.
func AcquireLock[Q postgres.Queryer](ctx context.Context, q Q) error {
	const query = `SELECT pg_advisory_lock($1)`
	const op = "migrationsrp.AcquireLock"

	if _, err := q.Exec(ctx, query, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("%s: acquire migration advisory lock: %w", op, err)
	}

	return nil
}

// ReleaseLock releases the migrations advisory lock.
func ReleaseLock[Q postgres.Queryer](ctx context.Context, q Q) error {
	const query = `SELECT pg_advisory_unlock($1)`
	const op = "migrationsrp.ReleaseLock"

	if _, err := q.Exec(ctx, query, migrationAdvisoryLockID); err != nil {
		return fmt.Errorf("%s: release migration advisory lock: %w", op, err)
	}

	return nil
}

// ListApplied returns applied migration records ordered by id.
func ListApplied[Q postgres.Queryer](ctx context.Context, q Q) ([]model.Migration, error) {
	if err := ensureMigrationMetadata(ctx, q); err != nil {
		return nil, err
	}

	const query = `SELECT id, name, checksum, dirty, created_at FROM migrations ORDER BY id ASC`
	return postgres.QueryAll[model.Migration](ctx, q, "list applied migrations", query)
}

func ensureMigrationMetadata[Q postgres.Queryer](ctx context.Context, q Q) error {
	const addChecksum = `ALTER TABLE migrations ADD COLUMN IF NOT EXISTS checksum text`
	const op = "migrationsrp.ensureMigrationMetadata"

	if _, err := q.Exec(ctx, addChecksum); err != nil {
		return fmt.Errorf("%s: ensure migrations checksum column: %w", op, err)
	}

	const backfillChecksum = `
UPDATE migrations
SET checksum = 'legacy:' || id::text || ':' || name
WHERE checksum IS NULL OR checksum = ''
`
	if _, err := q.Exec(ctx, backfillChecksum); err != nil {
		return fmt.Errorf("%s: backfill legacy migration checksums: %w", op, err)
	}

	const requireChecksum = `ALTER TABLE migrations ALTER COLUMN checksum SET NOT NULL`
	if _, err := q.Exec(ctx, requireChecksum); err != nil {
		return fmt.Errorf("%s: require migrations checksum: %w", op, err)
	}

	const addDirty = `ALTER TABLE migrations ADD COLUMN IF NOT EXISTS dirty boolean NOT NULL DEFAULT false`
	if _, err := q.Exec(ctx, addDirty); err != nil {
		return fmt.Errorf("%s: ensure migrations dirty column: %w", op, err)
	}

	return nil
}

// Migrate executes the SQL statements for a single migration.
func Migrate[Q postgres.Queryer](ctx context.Context, q Q, migName, migStatements string) error {
	const op = "migrationsrp.Migrate"

	_, err := q.Exec(ctx, migStatements)
	if err != nil {
		return fmt.Errorf("%s: migrating migrations(%q): %w", op, migName, err)
	}

	return nil
}

// Register inserts a migration record and its immutable content checksum.
func Register[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	migName string,
	checksum string,
	createdAt time.Time,
) (model.Migration, error) {
	const op = "migrationsrp.Register"

	if err := postgres.RequiredTime("created_at", createdAt); err != nil {
		return model.Migration{}, err
	}

	if migName == "" || checksum == "" {
		return model.Migration{}, fmt.Errorf("%s: migration name and checksum are required", op)
	}

	const query = `
INSERT INTO migrations(name, checksum, dirty, created_at)
VALUES ($1, $2, false, $3)
RETURNING id, name, checksum, dirty, created_at
`
	return postgres.QueryOne[model.Migration](
		ctx,
		q,
		fmt.Sprintf("register migration %q", migName),
		query,
		migName,
		checksum,
		createdAt,
	)
}

// MarkDirty persists a durable pre-execution marker for a non-transactional
// migration. A crash after this point leaves an explicit operator-visible state
// instead of making the next run guess whether DDL was partially applied.
func MarkDirty[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	migName string,
	checksum string,
	startedAt time.Time,
) (model.Migration, error) {
	const op = "migrationsrp.MarkDirty"
	if err := ensureMigrationMetadata(ctx, q); err != nil {
		return model.Migration{}, err
	}

	if err := postgres.RequiredTime("created_at", startedAt); err != nil {
		return model.Migration{}, err
	}

	if migName == "" || checksum == "" {
		return model.Migration{}, fmt.Errorf("%s: migration name and checksum are required", op)
	}

	const query = `
INSERT INTO migrations(name, checksum, dirty, created_at)
VALUES ($1, $2, true, $3)
RETURNING id, name, checksum, dirty, created_at
`
	return postgres.QueryOne[model.Migration](
		ctx,
		q,
		"mark migration dirty",
		query,
		migName,
		checksum,
		startedAt,
	)
}

// MarkApplied finalizes a previously dirty non-transactional migration.
func MarkApplied[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	migName string,
	checksum string,
	appliedAt time.Time,
) (model.Migration, error) {
	const op = "migrationsrp.MarkApplied"
	if err := postgres.RequiredTime("created_at", appliedAt); err != nil {
		return model.Migration{}, err
	}

	const query = `
UPDATE migrations
SET dirty = false,
    created_at = $3
WHERE name = $1
  AND checksum = $2
  AND dirty = true
RETURNING id, name, checksum, dirty, created_at
`
	return postgres.QueryOne[model.Migration](
		ctx,
		q,
		"mark migration applied",
		query,
		migName,
		checksum,
		appliedAt,
	)
}
