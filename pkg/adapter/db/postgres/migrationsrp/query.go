package migrationsrp

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/adapter/db/postgres"
)

func Migrate[Q postgres.Queryer](ctx context.Context, q Q, migName, migStatements string) error {
	_, err := q.Exec(ctx, migStatements)
	if err != nil {
		return fmt.Errorf("migrating migrations(%q): %w", migName, err)
	}

	return nil
}

func Register[Q postgres.Queryer](ctx context.Context, q Q, migName string) error {
	_, err := q.Exec(ctx, "INSERT INTO migrations(name, created_at) VALUES ($1, $2)", migName, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("registering migrations(%q): %w", migName, err)
	}

	return nil
}
