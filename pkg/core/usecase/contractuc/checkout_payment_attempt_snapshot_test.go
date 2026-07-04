package contractuc

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidateCheckoutPaymentAttemptSnapshot_AcceptsPartialApprovalUpfront(t *testing.T) {
	t.Parallel()

	first := model.Installment{
		ID:                   model.InstallmentID(uuid.New()),
		PrincipalAmountMinor: 25_000,
		UpfrontAmountMinor:   50_000,
		TotalAmountMinor:     75_000,
	}
	items, err := model.BuildPaymentAttemptItems(first, nil)
	require.NoError(t, err)

	require.NoError(t, validateCheckoutPaymentAttemptSnapshot(first, nil, items))
}
