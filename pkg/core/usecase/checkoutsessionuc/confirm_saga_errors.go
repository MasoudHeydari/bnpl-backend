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

const retryableCheckoutPaymentConflictMessageKey cerr.MessageKey = "error.checkout_payment_attempt_superseded_by_newer_retry"

// classifyConfirmationPrepareError maps a Contract-prepare failure to a Saga
// outcome. A deterministic pre-contract rejection compensates the claim.
// Every other failure uses the same retryability rules as Financial Core calls.
func classifyConfirmationPrepareError(
	err error,
	stepAttemptCount int,
	maxRetryAttempts int,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	now time.Time,
) (saga.StepResult, bool) {
	if isDeterministicOfferNoLongerHonorable(err) {
		reason := boundedConfirmationSagaReason(err.Error())

		return saga.StepResult{Outcome: saga.StepOutcomeCompensate, Reason: &reason}, true
	}

	return classifyConfirmationExternalError(
		err,
		stepAttemptCount,
		maxRetryAttempts,
		retryBaseDelay,
		retryMaxDelay,
		now,
	)
}

// classifyConfirmationExternalError maps external-step failures to saga outcomes.
// When handled is false the caller should propagate err unchanged.
//
// maxRetryAttempts counts additional retries after the initial execution.
// With maxRetryAttempts=8, StepAttemptCount values 1..8 may schedule another
// retry; StepAttemptCount=9 exhausts the budget (1 initial + 8 retries).
// StepAttemptCount increments when a worker successfully claims the Saga, so a
// claim that crashes before the step body still consumes budget and cannot
// exceed the configured bound.
//
// Retry budget exhaustion never invents payment failure. Ambiguous Financial
// Core outcomes escalate to reconciliation; persistent transient dependency
// failures escalate to manual review; permanent financial conflicts reconcile
// immediately; deterministic invalid input goes to manual review immediately.
func classifyConfirmationExternalError(
	err error,
	stepAttemptCount int,
	maxRetryAttempts int,
	retryBaseDelay time.Duration,
	retryMaxDelay time.Duration,
	now time.Time,
) (result saga.StepResult, handled bool) {
	if err == nil {
		return saga.StepResult{}, false
	}

	kind := confirmationExternalErrorClass(err, stepAttemptCount, maxRetryAttempts)
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

func confirmationExternalErrorClass(
	err error,
	stepAttemptCount int,
	maxRetryAttempts int,
) confirmationExternalErrorKind {
	// Exhaust only after the initial attempt plus maxRetryAttempts retries.
	// Example: maxRetryAttempts=8 → escalate when StepAttemptCount > 8 (i.e. 9).
	budgetExhausted := maxRetryAttempts > 0 && stepAttemptCount > maxRetryAttempts

	// Caller cancellation is not a business failure. Schedule a backoff retry
	// so a fresh worker context can resume the same idempotent step.
	if errors.Is(err, context.Canceled) {
		if budgetExhausted {
			return confirmationExternalManualReview
		}

		return confirmationExternalRetry
	}

	// A deadline does not prove the remote operation failed to commit.
	// After the retry budget, treat it as an ambiguous Financial Core outcome.
	if errors.Is(err, context.DeadlineExceeded) {
		if budgetExhausted {
			return confirmationExternalReconcile
		}

		return confirmationExternalRetry
	}

	var classified *extcheckoutpayment.ClassifiedFailure
	if errors.As(err, &classified) {
		switch classified.Class {
		case extcheckoutpayment.FailureClassTransient:
			if budgetExhausted {
				return confirmationExternalManualReview
			}

			return confirmationExternalRetry

		case extcheckoutpayment.FailureClassAmbiguous:
			if budgetExhausted {
				return confirmationExternalReconcile
			}

			return confirmationExternalRetry

		case extcheckoutpayment.FailureClassRetryableConcurrency:
			if budgetExhausted {
				return confirmationExternalManualReview
			}

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
			if budgetExhausted {
				return confirmationExternalManualReview
			}

			return confirmationExternalRetry

		case cerr.CodeInvalidArgument, cerr.CodeValidationFailed, cerr.CodeIdempotencyConflict:
			return confirmationExternalManualReview

		case cerr.CodeConflict, cerr.CodeContractAlreadyExists:
			if appErr.MessageKey == retryableCheckoutPaymentConflictMessageKey {
				if budgetExhausted {
					return confirmationExternalManualReview
				}

				return confirmationExternalRetry
			}

			if isPermanentFinancialConflict(appErr.MessageKey) {
				return confirmationExternalReconcile
			}

			// Ambiguous concurrency conflict: bounded retry, then manual review.
			if budgetExhausted {
				return confirmationExternalManualReview
			}

			return confirmationExternalRetry

		case cerr.CodeInternalError:
			return confirmationExternalManualReview
		}
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		if budgetExhausted {
			return confirmationExternalReconcile
		}

		return confirmationExternalRetry
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		if budgetExhausted {
			return confirmationExternalManualReview
		}

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
