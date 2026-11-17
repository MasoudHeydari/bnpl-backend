package customercredituc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"time"
)

// monthlyPreviousCycleBlocked is a product-specific spending restriction.
// It does NOT expire the account, change a credit limit, reserve cash or
// release exposure. Once FC confirms every old installment and BNPL commits
// that result, the restriction disappears without another limit decision.
func monthlyPreviousCycleBlocked(
	ctx context.Context, q repo.CreditAccountProductQueryer,
	customerID model.CustomerID, currency model.Currency, at time.Time,
) (bool, error) {
	cutoff, err := jalali.StartOfFollowingTehranMonth(at)
	if err != nil {
		return false, cerr.InternalError(fmt.Errorf("compute monthly credit cycle boundary: %w", err))
	}
	return q.HasUnpaidMonthlyCycleBefore(ctx, customerID, currency, cutoff)
}
