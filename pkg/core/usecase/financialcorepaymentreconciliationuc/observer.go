package financialcorepaymentreconciliationuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records operational flow checkpoints for inbound Financial Core
// payment reconciliation results. Implementations must not modify domain state
// or return errors that affect business execution.
//
// All checkpoints are emitted only after the database transaction that applied
// the reconciliation result committed. A redelivered event whose inbox row
// already exists is reported through InboxDuplicateDetected and is never
// reported as a new resolution.
type Observer interface {
	// InboxDuplicateDetected records a redelivered event whose inbox row
	// already exists. inboxResult is the stable prior processing outcome.
	InboxDuplicateDetected(
		ctx context.Context,
		eventID string,
		paymentAttemptID model.PaymentAttemptID,
		inboxResult string,
	)

	// ReconciliationResolved records a committed authoritative resolution.
	// outcome is "submitted" (payment session confirmed, not paid) or
	// "succeeded" (payment applied).
	ReconciliationResolved(
		ctx context.Context,
		eventID string,
		paymentAttemptID model.PaymentAttemptID,
		outcome string,
	)

	// ReconciliationRetryScheduled records a committed outcome that keeps the
	// payment attempt in reconciliation and schedules another check.
	ReconciliationRetryScheduled(
		ctx context.Context,
		eventID string,
		paymentAttemptID model.PaymentAttemptID,
	)

	// ReconciliationFailed records a committed terminal reconciliation outcome
	// ("failed", "canceled", or "permanently_rejected").
	ReconciliationFailed(
		ctx context.Context,
		eventID string,
		paymentAttemptID model.PaymentAttemptID,
		outcome string,
	)

	// ReconciliationMismatch records a committed conflicting or superseded
	// reconciliation result that was not applied.
	ReconciliationMismatch(
		ctx context.Context,
		eventID string,
		paymentAttemptID model.PaymentAttemptID,
		reasonCode string,
	)
}
