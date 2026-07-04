package checkoutsessionuc

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

type timeoutNetError struct{}

func (timeoutNetError) Error() string   { return "i/o timeout" }
func (timeoutNetError) Timeout() bool   { return true }
func (timeoutNetError) Temporary() bool { return true }

func TestClassifyConfirmationExternalError(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		10,
		18,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	base := time.Second
	maxDelay := time.Minute

	insufficientCredit := cerr.NewPublic(
		cerr.CodeConflict,
		"error.insufficient_credit_for_contract",
		errors.New("available credit is zero"),
		cerr.Details{Violations: []cerr.Violation{{
			Code:       "INSUFFICIENT_CREDIT",
			MessageKey: "error.insufficient_credit_for_contract",
		}}},
	)

	tests := []struct {
		name         string
		err          error
		attemptCount int
		want         saga.StepOutcome
	}{
		{
			name:         "deadline exceeded is an ambiguous retry",
			err:          context.DeadlineExceeded,
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "wrapped deadline exceeded stays retryable",
			err:          fmt.Errorf("fc status: %w", context.DeadlineExceeded),
			attemptCount: 2,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "request cancellation is not a permanent failure",
			err:          context.Canceled,
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "wrapped cancellation stays retryable",
			err:          fmt.Errorf("http: %w", context.Canceled),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "database mapper internal error wrapping cancellation stays retryable",
			err:          fmt.Errorf("get saga by id: %w", cerr.InternalError(context.Canceled)),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "service unavailable is transient",
			err:          cerr.ServiceUnavailable(errors.New("fc unavailable")),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "too many requests backs off",
			err:          cerr.NewPublic(cerr.CodeTooManyRequests, cerr.DefaultMessageKey(cerr.CodeTooManyRequests), errors.New("slow down"), cerr.Details{}),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name: "wrapped transient classification retries",
			err: fmt.Errorf("adapter: %w", extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassTransient,
				errors.New("dependency unavailable"),
			)),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name: "ambiguous classified failure retries with the same identity",
			err: cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassAmbiguous,
				fmt.Errorf("deadline: %w", context.DeadlineExceeded),
			)),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "network timeout retries",
			err:          fmt.Errorf("dial: %w", timeoutNetError{}),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "network operation error retries",
			err:          &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name: "grpc unavailable shape after adapter normalization retries",
			err: cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassTransient,
				errors.New("financial core unavailable (grpc_code=Unavailable)"),
			)),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "deterministic invalid argument is manual review",
			err:          cerr.InvalidArgument(errors.New("immutable amount")),
			attemptCount: 1,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name: "permanent immutable reference conflict reconciles",
			err: cerr.NewPublic(
				cerr.CodeConflict,
				"error.financial_core_checkout_payment_conflict",
				errors.New("stored amount disagrees"),
				cerr.Details{},
			),
			attemptCount: 1,
			want:         saga.StepOutcomeReconcile,
		},
		{
			name: "classified permanent conflict reconciles",
			err: cerr.Conflict(extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassPermanentConflict,
				errors.New("identity already exists"),
			)),
			attemptCount: 1,
			want:         saga.StepOutcomeReconcile,
		},
		{
			name: "retryable concurrency conflict retries",
			err: cerr.Conflict(extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassRetryableConcurrency,
				errors.New("aborted"),
			)),
			attemptCount: 3,
			want:         saga.StepOutcomeRetry,
		},
		{
			name: "superseded payment retry conflict retries within budget",
			err: cerr.NewPublic(
				cerr.CodeConflict,
				"error.checkout_payment_attempt_superseded_by_newer_retry",
				errors.New("newer attempt won"),
				cerr.Details{},
			),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name: "superseded payment retry conflict still retries on eighth retry",
			err: cerr.NewPublic(
				cerr.CodeConflict,
				"error.checkout_payment_attempt_superseded_by_newer_retry",
				errors.New("newer attempt won"),
				cerr.Details{},
			),
			attemptCount: 8,
			want:         saga.StepOutcomeRetry,
		},
		{
			name: "superseded payment retry conflict escalates after budget",
			err: cerr.NewPublic(
				cerr.CodeConflict,
				"error.checkout_payment_attempt_superseded_by_newer_retry",
				errors.New("newer attempt won"),
				cerr.Details{},
			),
			attemptCount: 9,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name:         "ambiguous conflict retries before the bound",
			err:          cerr.Conflict(errors.New("contract create conflict could not be resolved")),
			attemptCount: 1,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "ambiguous conflict retries on eighth retry",
			err:          cerr.Conflict(errors.New("contract create conflict could not be resolved")),
			attemptCount: 8,
			want:         saga.StepOutcomeRetry,
		},
		{
			name:         "ambiguous conflict stops after the retry bound",
			err:          cerr.Conflict(errors.New("contract create conflict could not be resolved")),
			attemptCount: 9,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name:         "ambiguous FC deadline escalates to reconcile after budget",
			err:          context.DeadlineExceeded,
			attemptCount: 9,
			want:         saga.StepOutcomeReconcile,
		},
		{
			name:         "persistent transient escalates to manual review after budget",
			err:          cerr.ServiceUnavailable(errors.New("fc unavailable")),
			attemptCount: 9,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name:         "unknown internal error is manual review",
			err:          errors.New("unexpected nil pointer"),
			attemptCount: 1,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name:         "cerr internal error is manual review",
			err:          cerr.InternalError(errors.New("invariant broken")),
			attemptCount: 1,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name: "classified unknown failure is manual review",
			err: extcheckoutpayment.NewFailure(
				extcheckoutpayment.FailureClassUnknown,
				errors.New("unmapped grpc code"),
			),
			attemptCount: 1,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name:         "idempotency payload conflict is manual review",
			err:          cerr.IdempotencyConflict(errors.New("same key different body")),
			attemptCount: 1,
			want:         saga.StepOutcomeManualReview,
		},
		{
			name:         "wrapped deterministic business rejection compensates",
			err:          fmt.Errorf("prepare contract: %w", insufficientCredit),
			attemptCount: 1,
			want:         saga.StepOutcomeCompensate,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, handled := classifyConfirmationPrepareError(
				tt.err,
				tt.attemptCount,
				8,
				base,
				maxDelay,
				now,
			)
			require.True(t, handled)
			require.Equal(t, tt.want, result.Outcome)
			require.NotNil(t, result.Reason)
			require.NotEmpty(t, *result.Reason)

			if tt.want == saga.StepOutcomeRetry {
				require.NotNil(t, result.RetryAt)
				require.True(t, result.RetryAt.After(now))
				return
			}

			require.Nil(t, result.RetryAt)
		})
	}
}
