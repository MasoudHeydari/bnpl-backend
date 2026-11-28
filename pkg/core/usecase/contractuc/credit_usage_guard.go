package contractuc

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// NewCreditUsageGuard fail-closes new financed commitments when collections
// (or other) restrictions prohibit new credit usage. Lock the legal
// person before customer/contract rows; then check current restrictions before
// a new commitment. Replays of existing commitments stay idempotent.
type NewCreditUsageGuard interface {
	LockCreditOwnerForCheckout(
		ctx context.Context,
		tx repo.Tx,
		externalSubjectID model.ExternalSubjectID,
	) error

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

// LockCreditOwnerForCheckout is a no-op only for isolated contract tests.
func (AllowAllNewCreditUsage) LockCreditOwnerForCheckout(
	context.Context,
	repo.Tx,
	model.ExternalSubjectID,
) error {
	return nil
}

// AssertNewCreditAllowed always allows new credit usage.
func (AllowAllNewCreditUsage) AssertNewCreditAllowed(
	context.Context,
	repo.Tx,
	model.CustomerID,
	time.Time,
) error {
	return nil
}
