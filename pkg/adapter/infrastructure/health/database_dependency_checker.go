package health

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/health"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify DatabaseDependencyChecker implements the core dependency checker
// contract.
var _ health.DependencyChecker = (*DatabaseDependencyChecker)(nil)

// DatabaseDependencyChecker verifies that the application's database is
// available.
//
// The checker performs a lightweight database health check using the configured
// health repository. If no database dependencies are configured, the check is
// treated as successful.
type DatabaseDependencyChecker struct {
	pool   repo.Pool
	health repo.Health
}

// NewDatabaseDependencyChecker constructs a database dependency checker.
func NewDatabaseDependencyChecker(
	pool repo.Pool,
	health repo.Health,
) *DatabaseDependencyChecker {
	return &DatabaseDependencyChecker{
		pool:   pool,
		health: health,
	}
}

// Check verifies that the database is reachable and able to serve requests.
func (c *DatabaseDependencyChecker) Check(ctx context.Context) error {
	if c.pool == nil || c.health == nil {
		return nil
	}

	if err := c.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			if err := c.health.Conn(conn).Ping(ctx); err != nil {
				return fmt.Errorf("database: %w", err)
			}

			return nil
		},
	); err != nil {
		return cerr.ServiceUnavailable(err)
	}

	return nil
}
