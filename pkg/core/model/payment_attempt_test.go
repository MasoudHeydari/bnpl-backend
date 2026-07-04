package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPaymentAttemptState_Transitions(t *testing.T) {
	t.Parallel()

	t.Run("created to submitted", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, CreatedPaymentAttemptState.ValidateTransitionTo(SubmittedPaymentAttemptState))
	})

	t.Run("submitted to succeeded", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, SubmittedPaymentAttemptState.ValidateTransitionTo(SucceededPaymentAttemptState))
	})

	t.Run("submitted to reconciliation_required", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, SubmittedPaymentAttemptState.ValidateTransitionTo(ReconciliationRequiredPaymentAttemptState))
	})

	t.Run("succeeded is terminal", func(t *testing.T) {
		t.Parallel()
		assert.True(t, SucceededPaymentAttemptState.IsTerminal())
		err := SucceededPaymentAttemptState.ValidateTransitionTo(FailedPaymentAttemptState)
		require.Error(t, err)
	})

	t.Run("succeeded to canceled rejected", func(t *testing.T) {
		t.Parallel()
		require.Error(t, SucceededPaymentAttemptState.ValidateTransitionTo(CanceledPaymentAttemptState))
	})
}

func TestPaymentChannel_IsValid(t *testing.T) {
	t.Parallel()

	assert.True(t, GatewayPaymentChannel.IsValid())
	assert.False(t, PaymentChannel("cash").IsValid())
}

func TestPaymentReconciliationReason_IsValid(t *testing.T) {
	t.Parallel()

	assert.True(t, PaymentAfterDeadlinePaymentReconciliationReason.IsValid())
	assert.False(t, PaymentReconciliationReason("unknown").IsValid())
}

func TestManualReviewIsNotFinancialFinality(t *testing.T) {
	if ManualReviewRequiredPaymentAttemptState.IsTerminal() {
		t.Fatal("manual review must retain ambiguous financial outcome")
	}

	if !ManualReviewRequiredPaymentAttemptState.CanTransitionTo(SucceededPaymentAttemptState) {
		t.Fatal("authoritative late success must remain applicable")
	}
}

func TestPaymentAttemptState_ExpiredTransitions(t *testing.T) {
	t.Parallel()

	t.Run("created to expired", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, CreatedPaymentAttemptState.ValidateTransitionTo(ExpiredPaymentAttemptState))
	})

	t.Run("session creation reconciliation_required to expired", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ReconciliationRequiredPaymentAttemptState.ValidateTransitionTo(ExpiredPaymentAttemptState))
	})

	t.Run("submitted cannot expire", func(t *testing.T) {
		t.Parallel()
		require.Error(t, SubmittedPaymentAttemptState.ValidateTransitionTo(ExpiredPaymentAttemptState))
	})

	t.Run("succeeded cannot expire", func(t *testing.T) {
		t.Parallel()
		require.Error(t, SucceededPaymentAttemptState.ValidateTransitionTo(ExpiredPaymentAttemptState))
	})

	t.Run("failed cannot expire", func(t *testing.T) {
		t.Parallel()
		require.Error(t, FailedPaymentAttemptState.ValidateTransitionTo(ExpiredPaymentAttemptState))
	})

	t.Run("canceled cannot expire", func(t *testing.T) {
		t.Parallel()
		require.Error(t, CanceledPaymentAttemptState.ValidateTransitionTo(ExpiredPaymentAttemptState))
	})

	t.Run("expired is not financial terminal", func(t *testing.T) {
		t.Parallel()
		assert.False(t, ExpiredPaymentAttemptState.IsTerminal())
	})

	t.Run("expired accepts late authoritative outcomes", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, ExpiredPaymentAttemptState.ValidateTransitionTo(SubmittedPaymentAttemptState))
		require.NoError(t, ExpiredPaymentAttemptState.ValidateTransitionTo(SucceededPaymentAttemptState))
		require.NoError(t, ExpiredPaymentAttemptState.ValidateTransitionTo(FailedPaymentAttemptState))
		require.NoError(t, ExpiredPaymentAttemptState.ValidateTransitionTo(CanceledPaymentAttemptState))
	})

	t.Run("expired cannot reopen reconciliation_required", func(t *testing.T) {
		t.Parallel()
		require.Error(t, ExpiredPaymentAttemptState.ValidateTransitionTo(ReconciliationRequiredPaymentAttemptState))
	})
}

func TestPaymentAttempt_CanReserveFinancialSessionCreate_RejectsExpired(t *testing.T) {
	t.Parallel()

	attempt := PaymentAttempt{State: ExpiredPaymentAttemptState}
	assert.False(t, attempt.CanReserveFinancialSessionCreate())
}

func TestPaymentAttempt_BlocksNewPaymentAttempt_ExpiredUnresolved(t *testing.T) {
	t.Parallel()

	assert.True(t, PaymentAttempt{
		State:                    ExpiredPaymentAttemptState,
		FinancialOutcomeResolved: false,
	}.BlocksNewPaymentAttempt())

	assert.False(t, PaymentAttempt{
		State:                    ExpiredPaymentAttemptState,
		FinancialOutcomeResolved: true,
	}.BlocksNewPaymentAttempt())
}

func TestPaymentAttempt_BlocksNewPaymentAttempt_LocalSessionCreationCancelDoesNotBlock(t *testing.T) {
	t.Parallel()

	failure := SessionCreationRetryExhaustedFailureCode
	assert.False(t, PaymentAttempt{
		Purpose:                  InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                    CanceledPaymentAttemptState,
		FailureCode:              &failure,
		FinancialOutcomeResolved: false,
	}.BlocksNewPaymentAttempt())
}

func TestPaymentAttempt_BlocksNewPaymentAttempt_AbandonedSessionCreationExpireDoesNotBlock(t *testing.T) {
	t.Parallel()

	assert.False(t, PaymentAttempt{
		Purpose:                  InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                    ExpiredPaymentAttemptState,
		FinancialOutcomeResolved: false,
	}.BlocksNewPaymentAttempt())
}
