package decisionuc

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// previousMonthlyCycleUnpaid checks the BNPL contractual installment state.
// A prior month's pending installment blocks NEW monthly spending starting at
// midnight on the first of the next Jalali month, even though its due date is
// the fifth. The rule neither changes credit limit nor marks money settled.
func previousMonthlyCycleUnpaid(
	ctx context.Context, q repo.CreditAccountQueryer,
	customerID model.CustomerID, currency model.Currency, now time.Time,
) (bool, error) {
	productQ, ok := q.(repo.CreditAccountProductQueryer)
	if !ok {
		return false, cerr.InternalError(fmt.Errorf("monthly credit cycle lookup is unavailable"))
	}
	cutoff, err := jalali.StartOfFollowingTehranMonth(now)
	if err != nil {
		return false, cerr.InternalError(fmt.Errorf("compute Jalali monthly cycle: %w", err))
	}
	return productQ.HasUnpaidMonthlyCycleBefore(ctx, customerID, currency, cutoff)
}
