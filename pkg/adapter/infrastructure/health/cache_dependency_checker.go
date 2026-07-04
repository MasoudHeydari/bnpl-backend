package health

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
)

// Verify RedisDependencyChecker implements the core dependency checker
// contract.
var _ corehealth.DependencyChecker = (*RedisDependencyChecker)(nil)

// RedisDependencyChecker verifies that the application's Redis-backed cache is
// available.
//
// The checker is intended for readiness checks. If Redis is required for normal
// request handling, a failed Redis check should make readiness fail so the
// application stops receiving new work.
type RedisDependencyChecker struct {
	cache cache.Cache
}

// NewRedisDependencyChecker constructs a Redis dependency checker.
func NewRedisDependencyChecker(cache cache.Cache) *RedisDependencyChecker {
	return &RedisDependencyChecker{
		cache: cache,
	}
}

// Check verifies that Redis is reachable and able to serve lightweight
// operations.
func (c *RedisDependencyChecker) Check(ctx context.Context) error {
	const op = "health.RedisDependencyChecker.Check"

	if c.cache == nil {
		return nil
	}

	if err := c.cache.Ping(ctx); err != nil {
		return cerr.ServiceUnavailable(fmt.Errorf("%s: redis: %w", op, err))
	}

	return nil
}
