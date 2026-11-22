package paymentattemptreconciliationuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const (
	attemptOutcomeCanceledRetryExhausted = "canceled_retry_exhausted"
	attemptOutcomeExpired                = "expired"

	manualReviewReasonRetryBudgetExhausted = "retry_budget_exhausted"
	manualReviewReasonRequestBuildFailed   = "request_build_failed"
	manualReviewReasonMissingPaymentIntent = "missing_payment_intent_for_status_recovery"
)

// Observer records operational flow checkpoints for payment attempt
// reconciliation scheduling. Implementations must not modify domain state or
// return errors that affect business execution.
//
// All checkpoints are emitted only after the database transaction for the
// affected payment attempt committed.
type Observer interface {
	// ReconciliationRequested records a committed reconciliation request
	// scheduled for a payment attempt generation.
	ReconciliationRequested(
		ctx context.Context,
		paymentAttemptID model.PaymentAttemptID,
		generation int,
	)

	// ManualReviewRequired records a committed escalation of a payment attempt
	// to manual review. reasonCode is a stable machine-readable code.
	ManualReviewRequired(
		ctx context.Context,
		paymentAttemptID model.PaymentAttemptID,
		reasonCode string,
	)

	// AttemptClosedLocally records a committed local terminal closure of a
	// payment attempt that no longer needs Financial Core reconciliation
	// ("canceled_retry_exhausted" or "expired").
	AttemptClosedLocally(
		ctx context.Context,
		paymentAttemptID model.PaymentAttemptID,
		outcome string,
	)
}
