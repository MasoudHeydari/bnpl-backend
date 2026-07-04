package checkoutsessionuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
)

func TestConfirmationRetryBudget_MaxRetryAttemptsExcludesInitial(t *testing.T) {
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
	transient := cerr.ServiceUnavailable(extcheckoutpayment.NewFailure(
		extcheckoutpayment.FailureClassTransient,
		errors.New("fc unavailable"),
	))

	t.Run("budget_8_allows_attempts_1_through_8", func(t *testing.T) {
		t.Parallel()

		for _, count := range []int{1, 7, 8} {
			result, handled := classifyConfirmationExternalError(
				transient,
				count,
				8,
				base,
				maxDelay,
				now,
			)
			require.True(t, handled, "count=%d", count)
			require.Equal(
				t,
				saga.StepOutcomeRetry,
				result.Outcome,
				"count=%d",
				count,
			)
			require.NotNil(t, result.RetryAt, "count=%d", count)
		}
	})

	t.Run("budget_8_escalates_on_attempt_9", func(t *testing.T) {
		t.Parallel()

		result, handled := classifyConfirmationExternalError(
			transient,
			9,
			8,
			base,
			maxDelay,
			now,
		)
		require.True(t, handled)
		require.Equal(t, saga.StepOutcomeManualReview, result.Outcome)
		require.Nil(t, result.RetryAt)
	})

	t.Run("small_budgets", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			max   int
			count int
			want  saga.StepOutcome
		}{
			{max: 1, count: 1, want: saga.StepOutcomeRetry},
			{max: 1, count: 2, want: saga.StepOutcomeManualReview},
			{max: 2, count: 2, want: saga.StepOutcomeRetry},
			{max: 2, count: 3, want: saga.StepOutcomeManualReview},
			{max: 3, count: 3, want: saga.StepOutcomeRetry},
			{max: 3, count: 4, want: saga.StepOutcomeManualReview},
		}

		for _, tt := range cases {
			result, handled := classifyConfirmationExternalError(
				transient,
				tt.count,
				tt.max,
				base,
				maxDelay,
				now,
			)
			require.True(t, handled)
			require.Equal(
				t,
				tt.want,
				result.Outcome,
				"max=%d count=%d",
				tt.max,
				tt.count,
			)
		}
	})

	t.Run("ambiguous_exhaustion_reconciles", func(t *testing.T) {
		t.Parallel()

		result, handled := classifyConfirmationExternalError(
			context.DeadlineExceeded,
			9,
			8,
			base,
			maxDelay,
			now,
		)
		require.True(t, handled)
		require.Equal(t, saga.StepOutcomeReconcile, result.Outcome)
	})

	t.Run("permanent_conflict_ignores_budget", func(t *testing.T) {
		t.Parallel()

		err := cerr.Conflict(extcheckoutpayment.NewFailure(
			extcheckoutpayment.FailureClassPermanentConflict,
			errors.New("identity already exists"),
		))
		result, handled := classifyConfirmationExternalError(
			err,
			1,
			8,
			base,
			maxDelay,
			now,
		)
		require.True(t, handled)
		require.Equal(t, saga.StepOutcomeReconcile, result.Outcome)
	})
}
