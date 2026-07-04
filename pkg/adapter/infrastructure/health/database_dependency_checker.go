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
	healthRepo repo.Health,
) (*DatabaseDependencyChecker, error) {
	const op = "health.NewDatabaseDependencyChecker"

	if pool == nil {
		return nil, fmt.Errorf("%s: database dependency checker pool is nil", op)
	}

	if healthRepo == nil {
		return nil, fmt.Errorf("%s: database dependency checker health repository is nil", op)
	}

	return &DatabaseDependencyChecker{
		pool:   pool,
		health: healthRepo,
	}, nil
}

// Check verifies that the database is reachable and able to serve requests.
func (c *DatabaseDependencyChecker) Check(ctx context.Context) error {
	const op = "health.DatabaseDependencyChecker.Check"

	if err := c.pool.Conn(
		ctx,
		func(ctx context.Context, conn repo.Conn) error {
			healthQ := c.health.Conn(conn)
			if err := healthQ.Ping(ctx); err != nil {
				return fmt.Errorf("%s: database: %w", op, err)
			}

			return nil
		},
	); err != nil {
		return cerr.ServiceUnavailable(err)
	}

	return nil
}
