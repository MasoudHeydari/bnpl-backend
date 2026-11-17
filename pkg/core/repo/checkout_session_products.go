package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutSessionProductTxQueryer supports immutable product selection on a
// newly created checkout, without changing the legacy session Create contract.
type CheckoutSessionProductTxQueryer interface {
	// SelectCreditKindBeforeDecision advances evaluation_generation when
	// the customer changes product, fencing concurrent Decision evaluation.
	SelectCreditKindBeforeDecision(context.Context, model.CheckoutSessionID, model.CreditKind, time.Time) (model.CheckoutSession, error)
	SetCreditKindBeforeDecision(
		context.Context,
		model.CheckoutSessionID,
		model.CreditKind,
		time.Time,
	) (model.CheckoutSession, error)
}
