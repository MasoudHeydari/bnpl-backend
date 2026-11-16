package financialcorepaymentreconciliationuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

const (
	reconciliationOutcomeSubmitted           = "submitted"
	reconciliationOutcomeSucceeded           = "succeeded"
	reconciliationOutcomeFailed              = "failed"
	reconciliationOutcomeCanceled            = "canceled"
	reconciliationOutcomePermanentlyRejected = "permanently_rejected"

	reconciliationMismatchPayloadConflict = "event_identity_payload_mismatch"
	reconciliationMismatchStaleSuperseded = "stale_superseded"
)

// observeCommitted records the outcome of one Process attempt. It must be
// called only after the database transaction committed so success checkpoints
// never describe uncommitted work.
//
// Inbox duplicates are reported as duplicates and never as new resolutions.
func (uc *UseCase) observeCommitted(ctx context.Context, input ProcessInput, output ProcessOutput) {
	attemptID := input.Data.BNPLPaymentAttemptID

	if output.inboxDuplicate {
		uc.observer.InboxDuplicateDetected(ctx, input.EventID, attemptID, string(output.Result))
		return
	}

	switch output.Result {
	case ProcessResultResolvedSubmitted:
		uc.observer.ReconciliationResolved(ctx, input.EventID, attemptID, reconciliationOutcomeSubmitted)

	case ProcessResultResolvedSucceeded:
		uc.observer.ReconciliationResolved(ctx, input.EventID, attemptID, reconciliationOutcomeSucceeded)

	case ProcessResultStillReconciliationRequired:
		uc.observer.ReconciliationRetryScheduled(ctx, input.EventID, attemptID)

	case ProcessResultResolvedFailed:
		uc.observer.ReconciliationFailed(ctx, input.EventID, attemptID, reconciliationOutcomeFailed)

	case ProcessResultResolvedCanceled:
		uc.observer.ReconciliationFailed(ctx, input.EventID, attemptID, reconciliationOutcomeCanceled)

	case ProcessResultPermanentlyRejected:
		uc.observer.ReconciliationFailed(ctx, input.EventID, attemptID, reconciliationOutcomePermanentlyRejected)

	case ProcessResultPayloadConflictRecorded:
		uc.observer.ReconciliationMismatch(ctx, input.EventID, attemptID, reconciliationMismatchPayloadConflict)

	case ProcessResultStaleSuperseded:
		uc.observer.ReconciliationMismatch(ctx, input.EventID, attemptID, reconciliationMismatchStaleSuperseded)
	}
}

// observeApplicationConflict records a committed reconciliation application
// conflict persisted after the payment-application transaction rolled back.
func (uc *UseCase) observeApplicationConflict(
	ctx context.Context,
	input ProcessInput,
	output ProcessOutput,
	applicationErr *financialcorepaymentresultuc.ReconciliationApplicationError,
) {
	attemptID := input.Data.BNPLPaymentAttemptID

	if output.inboxDuplicate {
		uc.observer.InboxDuplicateDetected(ctx, input.EventID, attemptID, string(output.Result))
		return
	}

	uc.observer.ReconciliationMismatch(ctx, input.EventID, attemptID, string(applicationErr.Reason))
}
