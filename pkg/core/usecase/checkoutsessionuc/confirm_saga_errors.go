package checkoutsessionuc

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

type confirmationExternalErrorKind int

const (
	confirmationExternalRetry confirmationExternalErrorKind = iota
	confirmationExternalReconcile
	confirmationExternalManualReview
)

// classifyConfirmationExternalError maps external-step failures to saga outcomes.
// When handled is false the caller should propagate err unchanged.
func classifyConfirmationExternalError(
	err error,
	stepAttemptCount int,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	now time.Time,
	idempotentReplaySafe bool,
) (result saga.StepResult, handled bool) {
	if err == nil {
		return saga.StepResult{}, false
	}

	kind := confirmationExternalErrorClass(err, idempotentReplaySafe)
	reason := boundedConfirmationSagaReason(err.Error())

	switch kind {
	case confirmationExternalRetry:
		retryAt := now.Add(confirmationSagaRetryDelay(stepAttemptCount, retryBaseDelay, retryMaxDelay))

		return saga.StepResult{
			Outcome: saga.StepOutcomeRetry,
			RetryAt: &retryAt,
			Reason:  &reason,
		}, true

	case confirmationExternalManualReview:
		return saga.StepResult{Outcome: saga.StepOutcomeManualReview, Reason: &reason}, true

	case confirmationExternalReconcile:
		return saga.StepResult{Outcome: saga.StepOutcomeReconcile, Reason: &reason}, true

	default:
		return saga.StepResult{}, false
	}
}

func confirmationExternalErrorClass(err error, idempotentReplaySafe bool) confirmationExternalErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return confirmationExternalRetry
	}

	// HTTP client disconnect must not abandon a durable confirmation attempt.
	if errors.Is(err, context.Canceled) {
		return confirmationExternalRetry
	}

	var appErr *cerr.Error
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case cerr.CodeServiceUnavailable:
			return confirmationExternalRetry

		case cerr.CodeConflict, cerr.CodeInvalidArgument:
			if idempotentReplaySafe {
				return confirmationExternalRetry
			}

			return confirmationExternalReconcile

		case cerr.CodeInternalError:
			return confirmationExternalReconcile
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return confirmationExternalRetry
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return confirmationExternalRetry
	}

	// Idempotent external operations (Contract prepare by Decision, FC submit)
	// treat remaining failures as scheduled retries so Waiting/ProcessDue can
	// resume. Permanent conflicts are classified above as reconcile/manual.
	if idempotentReplaySafe {
		return confirmationExternalRetry
	}

	return confirmationExternalReconcile
}
