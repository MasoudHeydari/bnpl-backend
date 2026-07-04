package paymentresult

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestMapPaymentResultStatus(t *testing.T) {
	t.Parallel()

	t.Run("succeeded", func(t *testing.T) {
		t.Parallel()

		status, err := MapPaymentResultStatus("Succeeded")
		require.NoError(t, err)
		assert.Equal(t, model.SucceededFinancialPaymentResultStatus, status)
	})

	t.Run("failed", func(t *testing.T) {
		t.Parallel()

		status, err := MapPaymentResultStatus("failed")
		require.NoError(t, err)
		assert.Equal(t, model.FailedFinancialPaymentResultStatus, status)
	})

	t.Run("canceled", func(t *testing.T) {
		t.Parallel()

		status, err := MapPaymentResultStatus("canceled")
		require.NoError(t, err)
		assert.Equal(t, model.CanceledFinancialPaymentResultStatus, status)
	})

	t.Run("canceled british spelling", func(t *testing.T) {
		t.Parallel()

		status, err := MapPaymentResultStatus("canceled")
		require.NoError(t, err)
		assert.Equal(t, model.CanceledFinancialPaymentResultStatus, status)
	})

	t.Run("unknown status errors", func(t *testing.T) {
		t.Parallel()

		_, err := MapPaymentResultStatus("pending")
		require.Error(t, err)
	})
}
