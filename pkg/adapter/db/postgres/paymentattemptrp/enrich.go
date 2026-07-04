package paymentattemptrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// EnrichFinancialPaymentIdentityIfAbsent fills missing Financial Core payment identity
// fields without overwriting non-empty stored identifiers.
func EnrichFinancialPaymentIdentityIfAbsent[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID *string,
	financialPaymentAttemptID *string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.EnrichFinancialPaymentIdentityIfAbsent"

	attempt, err := LockByID(ctx, q, id)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	if financialPaymentIntentID == "" {
		return model.PaymentAttempt{}, cerr.Conflict(fmt.Errorf("%s: financial payment intent id is required", op))
	}

	if attempt.FinancialPaymentIntentID != nil &&
		*attempt.FinancialPaymentIntentID != "" &&
		*attempt.FinancialPaymentIntentID != financialPaymentIntentID {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: financial core payment intent does not match local attempt", op),
		)
	}

	if financialPaymentSessionID != nil &&
		attempt.FinancialPaymentSessionID != nil &&
		*attempt.FinancialPaymentSessionID != "" &&
		*attempt.FinancialPaymentSessionID != *financialPaymentSessionID {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: financial core payment session does not match local attempt", op),
		)
	}

	if financialPaymentAttemptID != nil &&
		attempt.FinancialPaymentAttemptID != nil &&
		*attempt.FinancialPaymentAttemptID != "" &&
		*attempt.FinancialPaymentAttemptID != *financialPaymentAttemptID {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: financial core payment attempt does not match local attempt", op),
		)
	}

	const query = `
UPDATE payment_attempts
SET
	financial_payment_intent_id = CASE
		WHEN $2::text IS NOT NULL AND financial_payment_intent_id IS NULL THEN $2
		ELSE financial_payment_intent_id
	END,
	financial_payment_session_id = CASE
		WHEN $3::text IS NOT NULL AND financial_payment_session_id IS NULL THEN $3
		ELSE financial_payment_session_id
	END,
	financial_payment_attempt_id = CASE
		WHEN $4::text IS NOT NULL AND financial_payment_attempt_id IS NULL THEN $4
		ELSE financial_payment_attempt_id
	END,
	updated_at = $5
WHERE id = $1
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"enrich payment attempt financial identity if absent",
		query,
		id,
		financialPaymentIntentID,
		financialPaymentSessionID,
		financialPaymentAttemptID,
		updatedAt,
	)
}
