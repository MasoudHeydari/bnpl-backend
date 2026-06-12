package migrationsrp

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
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

// Register inserts a migration record into the `migrations` table using the
// current UTC time.
func Register[Q postgres.Queryer](ctx context.Context, q Q, migName string) error {
	_, err := q.Exec(ctx, "INSERT INTO migrations(name, created_at) VALUES ($1, $2)", migName, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("registering migrations(%q): %w", migName, err)
	}

	return nil
}
