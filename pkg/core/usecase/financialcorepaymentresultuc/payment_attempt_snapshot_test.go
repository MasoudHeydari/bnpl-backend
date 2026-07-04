package financialcorepaymentresultuc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidatePaymentAttemptSnapshot_AcceptsUpfrontItem(t *testing.T) {
	t.Parallel()

	installment := model.Installment{
		ID:                   model.InstallmentID(uuid.New()),
		PrincipalAmountMinor: 30_000,
		UpfrontAmountMinor:   70_000,
	}
	items, err := model.BuildPaymentAttemptItems(installment, nil)
	require.NoError(t, err)

	attempt := model.PaymentAttempt{
		ID:          model.PaymentAttemptID(uuid.New()),
		AmountMinor: 100_000,
		Items:       items,
	}
	for i := range attempt.Items {
		attempt.Items[i].PaymentAttemptID = attempt.ID
	}

	reason, ok := validatePaymentAttemptSnapshot(attempt, installment, nil, 100_000)
	require.True(t, ok)
	require.Empty(t, reason)
}
