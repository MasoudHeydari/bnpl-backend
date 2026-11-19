package checkoutsessionrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ReplaceCreditKindAndInvalidateEvaluation persists Partner credit-kind
// selection under compare-and-swap fencing.
//
// First selection (unset credit_kind): sets kind without bumping generation.
// Replacement (different kind): sets kind, clears decision_id, and increments
// evaluation_generation exactly once. Same-kind no-ops must be handled by the
// caller before this UPDATE.
//
// The caller must already hold the checkout row lock in the same transaction.
func ReplaceCreditKindAndInvalidateEvaluation[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	kind model.CreditKind,
	expectedGeneration int,
	expectedDecisionID *model.DecisionID,
	now time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.ReplaceCreditKindAndInvalidateEvaluation"

	if !kind.IsValid() {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: invalid credit kind %q", op, kind),
		)
	}

	if expectedGeneration < 1 {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: expected evaluation generation must be >= 1", op),
		)
	}

	if err := postgres.RequiredTime("updated_at", now); err != nil {
		return model.CheckoutSession{}, err
	}

	// First selection: credit_kind IS NULL → set kind, keep generation.
	// Replacement: credit_kind IS DISTINCT FROM $2 → set kind, clear Decision,
	// bump generation once. Same-kind rows do not match (caller short-circuits).
	const query = `
UPDATE checkout_sessions
SET credit_kind = $2,
    decision_id = CASE
      WHEN credit_kind IS NOT NULL AND credit_kind IS DISTINCT FROM $2 THEN NULL
      ELSE decision_id
    END,
    evaluation_generation = CASE
      WHEN credit_kind IS NOT NULL AND credit_kind IS DISTINCT FROM $2 THEN evaluation_generation + 1
      ELSE evaluation_generation
    END,
    updated_at = $3
WHERE id = $1
  AND status = $4
  AND evaluation_generation = $5
  AND decision_id IS NOT DISTINCT FROM $6::uuid
  AND contract_id IS NULL
  AND confirmation_attempt_id IS NULL
  AND (
    credit_kind IS NULL
    OR credit_kind IS DISTINCT FROM $2
  )
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"replace checkout credit kind and invalidate evaluation",
		query,
		id,
		kind,
		now,
		model.OpenCheckoutSessionStatus,
		expectedGeneration,
		expectedDecisionID,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CheckoutSession{}, cerr.Conflict(fmt.Errorf(
				"%s: checkout credit kind selection is no longer allowed",
				op,
			))
		}

		return model.CheckoutSession{}, err
	}

	return row, nil
}
