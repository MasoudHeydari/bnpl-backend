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
