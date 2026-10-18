package checkoutsessionuc

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

type confirmationExternalErrorKind int

const (
	confirmationExternalRetry confirmationExternalErrorKind = iota
	confirmationExternalReconcile
	confirmationExternalManualReview
)

// confirmationSagaAmbiguousConflictRetryLimit bounds retries of a Conflict that
// has not been classified as a permanent economic mismatch. A concurrency race
// can converge; the same unchanged conflict must not be retried forever.
const confirmationSagaAmbiguousConflictRetryLimit = 8

const retryableCheckoutPaymentConflictMessageKey cerr.MessageKey = "error.checkout_payment_attempt_superseded_by_newer_retry"

// classifyConfirmationExternalError maps external-step failures to saga outcomes.
// When handled is false the caller should propagate err unchanged.
//
// idempotentReplaySafe is intentionally not an input. An operation can be safe
// to repeat and still be unable to succeed until the input or the remote
// economic state changes.
// classifyConfirmationPrepareError maps a Contract-prepare failure to a Saga
// outcome. A deterministic pre-contract rejection compensates the claim.
// Every other failure uses the same retryability rules as Financial Core calls.
func classifyConfirmationPrepareError(
	err error,
	stepAttemptCount int,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	now time.Time,
) (saga.StepResult, bool) {
	if isDeterministicOfferNoLongerHonorable(err) {
		reason := boundedConfirmationSagaReason(err.Error())

		return saga.StepResult{Outcome: saga.StepOutcomeCompensate, Reason: &reason}, true
	}

	return classifyConfirmationExternalError(err, stepAttemptCount, retryBaseDelay, retryMaxDelay, now)
}

func classifyConfirmationExternalError(
	err error,
	stepAttemptCount int,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	now time.Time,
) (result saga.StepResult, handled bool) {
	if err == nil {
		return saga.StepResult{}, false
	}

	kind := confirmationExternalErrorClass(err, stepAttemptCount)
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

func confirmationExternalErrorClass(err error, stepAttemptCount int) confirmationExternalErrorKind {
	// Caller cancellation is not a business failure. Schedule a backoff retry
	// so a fresh worker context can resume the same idempotent step.
	if errors.Is(err, context.Canceled) {
		return confirmationExternalRetry
	}

	// A deadline does not prove the remote operation failed to commit.
	if errors.Is(err, context.DeadlineExceeded) {
		return confirmationExternalRetry
	}

	var classified *extcheckoutpayment.ClassifiedFailure
	if errors.As(err, &classified) {
		switch classified.Class {
		case extcheckoutpayment.FailureClassTransient,
			extcheckoutpayment.FailureClassAmbiguous,
			extcheckoutpayment.FailureClassRetryableConcurrency:
			return confirmationExternalRetry

		case extcheckoutpayment.FailureClassPermanentInvalidInput,
			extcheckoutpayment.FailureClassUnknown:
			return confirmationExternalManualReview

		case extcheckoutpayment.FailureClassPermanentConflict:
			return confirmationExternalReconcile
		}
	}

	var appErr *cerr.Error
	if errors.As(err, &appErr) {
		switch appErr.Code {
		case cerr.CodeServiceUnavailable, cerr.CodeTooManyRequests:
			return confirmationExternalRetry

		case cerr.CodeInvalidArgument, cerr.CodeValidationFailed, cerr.CodeIdempotencyConflict:
			return confirmationExternalManualReview

		case cerr.CodeConflict, cerr.CodeContractAlreadyExists:
			if appErr.MessageKey == retryableCheckoutPaymentConflictMessageKey {
				return confirmationExternalRetry
			}

			if isPermanentFinancialConflict(appErr.MessageKey) {
				return confirmationExternalReconcile
			}

			if stepAttemptCount >= confirmationSagaAmbiguousConflictRetryLimit {
				return confirmationExternalManualReview
			}

			return confirmationExternalRetry

		case cerr.CodeInternalError:
			return confirmationExternalManualReview
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

	return confirmationExternalManualReview
}

func isPermanentFinancialConflict(key cerr.MessageKey) bool {
	switch key {
	case "error.financial_core_checkout_payment_conflict",
		"error.checkout_payment_session_financial_core_conflict",
		"error.financial_core_conflicting_session_for_submitted_checkout_attempt",
		"error.checkout_payment_attempt_conflicts_with_stored_contract_session",
		"error.financial_core_conflicting_collection_obligation",
		"error.checkout_payment_attempt_no_longer_matches_contract":
		return true

	default:
		return false
	}
}
