package decisionuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// NewCreditUsageGuard fail-closes new financing when collections (or other)
// restrictions prohibit new credit usage. Implementations must be safe to call
// inside an open transaction after the customer profile row is locked.
type NewCreditUsageGuard interface {
	AssertNewCreditAllowed(
		ctx context.Context,
		tx repo.Tx,
		customerID model.CustomerID,
		now time.Time,
	) error
}

// AllowAllNewCreditUsage is a no-op guard for tests that do not exercise
// collections restrictions.
type AllowAllNewCreditUsage struct{}

// AssertNewCreditAllowed always allows new credit usage.
func (AllowAllNewCreditUsage) AssertNewCreditAllowed(
	context.Context,
	repo.Tx,
	model.CustomerID,
	time.Time,
) error {
	return nil
}
