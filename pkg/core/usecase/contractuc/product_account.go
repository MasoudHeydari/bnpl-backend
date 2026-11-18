package contractuc

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// lockCheckoutCreditAccount selects the independently governed account named
// by the Decision's immutable policy. A monthly checkout must never consume
// installment capacity or vice versa; no amount is reserved by this lock.
func lockCheckoutCreditAccount(
	ctx context.Context, q repo.CreditAccountTxQueryer,
	customerID model.CustomerID, currency model.Currency, kind model.CreditKind,
) (model.CreditAccount, error) {
	switch model.EffectiveCreditKind(kind) {
	case model.PayInInstallmentsCreditKind:
		return q.LockByCustomerIDAndCurrency(ctx, customerID, currency)

	case model.PayNextMonthCreditKind:
		productQ, ok := q.(repo.CreditAccountProductTxQueryer)
		if !ok {
			return model.CreditAccount{}, cerr.InternalError(fmt.Errorf("credit account repository has no monthly product support"))
		}

		return productQ.LockByCustomerIDCurrencyAndKind(ctx, customerID, currency, kind)
	default:
		return model.CreditAccount{}, cerr.InternalError(fmt.Errorf("unsupported checkout credit kind %q", kind))
	}
}
