package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreditAccountProductQueryer adds product-scoped reads without changing the
// existing pay-in-four query contract. Do not read an account by customer and
// currency alone when the checkout explicitly selects a different CreditKind.
type CreditAccountProductQueryer interface {
	GetByCustomerIDCurrencyAndKind(context.Context, model.CustomerID, model.Currency, model.CreditKind) (model.CreditAccount, error)
	ListByCustomerID(context.Context, model.CustomerID) ([]model.CreditAccount, error)
}

// CreditAccountProductTxQueryer provides transaction-scoped mutations of a
// particular product. All limit/exposure changes still use the same CreditAccount
// row locks and immutable credit_transactions journal as legacy accounts.
type CreditAccountProductTxQueryer interface {
	// Existing operations remain available when selecting product-scoped
	// accounts. This also keeps immutable credit journal writes on one TX.
	CreditAccountTxQueryer
	CreditAccountProductQueryer
	LockByCustomerIDCurrencyAndKind(context.Context, model.CustomerID, model.Currency, model.CreditKind) (model.CreditAccount, error)
	CreateWithLimitForKind(
		context.Context,
		model.CustomerID, model.Currency, model.CreditKind,
		uint64, model.DecisionID, time.Time, *time.Time, time.Time, time.Time,
	) (model.CreditAccount, error)
	ExpireDueByCustomerIDCurrencyAndKind(context.Context, model.CustomerID, model.Currency, model.CreditKind, time.Time) error
}
