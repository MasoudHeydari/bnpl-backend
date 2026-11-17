package checkoutsessionrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// SetCreditKindBeforeDecision may only update an unconfirmed new checkout.
// An already-linked Decision or Contract must never change credit products.
func SetCreditKindBeforeDecision[Q postgres.Queryer](
	ctx context.Context, q Q, id model.CheckoutSessionID, kind model.CreditKind, now time.Time,
) (model.CheckoutSession, error) {
	if !kind.IsValid() {
		return model.CheckoutSession{}, cerr.InvalidArgument(fmt.Errorf("invalid checkout credit kind %q", kind))
	}
	const query = `
UPDATE checkout_sessions SET credit_kind = $2, updated_at = $3
WHERE id = $1 AND status = 'open' AND decision_id IS NULL AND contract_id IS NULL
RETURNING` + checkoutSessionColumns
	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx, q, "set checkout credit kind before decision", query, id, kind, now,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CheckoutSession{}, cerr.Conflict(fmt.Errorf("checkout credit kind is immutable after decision"))
		}
		return model.CheckoutSession{}, err
	}
	return row, nil
}

// SelectCreditKindBeforeDecision atomically changes the product and increments
// the evaluation generation. The caller must already hold a row lock.
func SelectCreditKindBeforeDecision[Q postgres.Queryer](
	ctx context.Context, q Q, id model.CheckoutSessionID,
	kind model.CreditKind, now time.Time,
) (model.CheckoutSession, error) {
	if !kind.IsValid() {
		return model.CheckoutSession{}, cerr.InvalidArgument(fmt.Errorf("invalid credit kind %q", kind))
	}
	const query = `
UPDATE checkout_sessions
SET credit_kind = $2,
    evaluation_generation = evaluation_generation + 1,
    updated_at = $3
WHERE id = $1 AND status = 'open'
  AND decision_id IS NULL AND contract_id IS NULL
  AND credit_kind IS DISTINCT FROM $2
RETURNING` + checkoutSessionColumns
	updated, err := postgres.QueryOne[model.CheckoutSession](
		ctx, q, "select checkout credit product", query, id, kind, now,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(fmt.Errorf("checkout product selection is no longer allowed"))
	}
	return updated, err
}
