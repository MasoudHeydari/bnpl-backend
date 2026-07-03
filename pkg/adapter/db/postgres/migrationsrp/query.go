package migrationsrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Migrate executes the SQL statements for a single migration.
// migName is used only for error context; the statements are executed exactly
// as provided and may contain multiple SQL commands.
func Migrate[Q postgres.Queryer](ctx context.Context, q Q, migName, migStatements string) error {
	_, err := q.Exec(ctx, migStatements)
	if err != nil {
		return fmt.Errorf("migrating migrations(%q): %w", migName, err)
	}

	return nil
}

// Register inserts a migration record into the `migrations` table and returns
// the persisted row.
func Register[Q postgres.Queryer](ctx context.Context, q Q, migName string, createdAt time.Time) (model.Migration, error) {
	if err := postgres.RequiredTime("created_at", createdAt); err != nil {
		return model.Migration{}, err
	}

	const query = `
INSERT INTO migrations(name, created_at)
VALUES ($1, $2)
RETURNING id, name, created_at
`

	return postgres.QueryOne[model.Migration](
		ctx,
		q,
		fmt.Sprintf("register migration %q", migName),
		query,
		migName,
		createdAt,
	)
}
