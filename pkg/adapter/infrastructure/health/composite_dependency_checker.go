// Package health provides infrastructure implementations for application
// health checks.
package health

import (
	"context"

	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
)

// Verify CompositeDependencyChecker implements the core dependency checker
// contract.
var _ corehealth.DependencyChecker = (*CompositeDependencyChecker)(nil)

// CompositeDependencyChecker runs multiple dependency checks as a single
// readiness check.
//
// It is useful when readiness depends on more than one runtime dependency,
// such as database, cache, message broker, or external services.
type CompositeDependencyChecker struct {
	checkers []corehealth.DependencyChecker
}

// NewCompositeDependencyChecker constructs a dependency checker that evaluates
// all provided checkers in order.
func NewCompositeDependencyChecker(
	checkers ...corehealth.DependencyChecker,
) *CompositeDependencyChecker {
	return &CompositeDependencyChecker{
		checkers: checkers,
	}
}

// Check verifies all configured dependencies.
//
// It returns nil only when every dependency check succeeds. The first failed
// dependency stops the sequence and its error is returned.
func (c *CompositeDependencyChecker) Check(ctx context.Context) error {
	for _, checker := range c.checkers {
		if checker == nil {
			continue
		}

		if err := checker.Check(ctx); err != nil {
			return err
		}
	}

	return nil
}
