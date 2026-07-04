package contractuc

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidatePaymentOccurredInTime(t *testing.T) {
	t.Parallel()

	deadline := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	contract := model.Contract{ExpiresAt: deadline}

	t.Run("paid before deadline is valid", func(t *testing.T) {
		t.Parallel()

		err := validatePaymentOccurredInTime(contract, deadline.Add(-time.Second))
		require.NoError(t, err)
	})

	t.Run("paid after deadline is rejected", func(t *testing.T) {
		t.Parallel()

		err := validatePaymentOccurredInTime(contract, deadline.Add(time.Second))
		require.Error(t, err)

		var reconErr *ReconciliationError
		require.ErrorAs(t, err, &reconErr)
		assert.Equal(t, model.PaymentAfterDeadlinePaymentReconciliationReason, reconErr.Reason)
	})

	t.Run("zero paid at is invalid", func(t *testing.T) {
		t.Parallel()

		err := validatePaymentOccurredInTime(contract, time.Time{})
		require.Error(t, err)
		assert.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
	})
}

func TestValidateExpiredContractPayment(t *testing.T) {
	t.Parallel()

	deadline := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	contract := model.Contract{
		Status:    model.ExpiredContractStatus,
		ExpiresAt: deadline,
	}

	t.Run("on-time payment requires reconciliation", func(t *testing.T) {
		t.Parallel()

		err := validateExpiredContractPayment(contract, deadline.Add(-time.Second))
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrPaymentRequiresReconciliation))

		var reconErr *ReconciliationError
		require.ErrorAs(t, err, &reconErr)
		assert.Equal(t, model.ContractExpiredPaymentWasOnTimePaymentReconciliationReason, reconErr.Reason)
	})

	t.Run("late payment is rejected", func(t *testing.T) {
		t.Parallel()

		err := validateExpiredContractPayment(contract, deadline.Add(time.Second))
		require.Error(t, err)
		assert.False(t, errors.Is(err, ErrPaymentRequiresReconciliation))

		var reconErr *ReconciliationError
		require.ErrorAs(t, err, &reconErr)
		assert.Equal(t, model.PaymentAfterDeadlinePaymentReconciliationReason, reconErr.Reason)
	})
}
