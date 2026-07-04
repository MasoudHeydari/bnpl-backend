package contractuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// TestActivationExposureRecording documents post-Financial-Core activation: principal
// exposure is increased and a credit journal entry is appended without re-checking
// available credit capacity.
func TestActivationExposureRecording_IncreasesExposureAndAppendsTransaction(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	accountID := model.CreditAccountID(uuid.New())
	contractID := model.ContractID(uuid.New())
	currency := model.IRRCurrency
	remainingPrincipal := uint64(70_000)
	paidAt := now.Add(-time.Minute)

	creditAccountQ := mock.NewCreditAccountTxQueryer(t)
	updatedAccount := model.CreditAccount{
		ID:                  accountID,
		Currency:            currency,
		ExposureAmountMinor: remainingPrincipal,
	}

	creditAccountQ.EXPECT().AddExposure(ctx, accountID, remainingPrincipal, now).
		Return(updatedAccount, nil).Once()
	creditAccountQ.EXPECT().AppendTransaction(
		ctx,
		accountID,
		model.ExposureIncreasedCreditTransactionType,
		remainingPrincipal,
		currency,
		model.ContractActivatedCreditTransactionReason,
		model.ContractCreditTransactionReferenceType,
		contractID.UUID(),
		paidAt,
		now,
	).Return(model.CreditTransaction{}, nil).Once()

	account, err := creditAccountQ.AddExposure(ctx, accountID, remainingPrincipal, now)
	require.NoError(t, err)
	require.Equal(t, remainingPrincipal, account.ExposureAmountMinor)

	_, err = creditAccountQ.AppendTransaction(
		ctx,
		accountID,
		model.ExposureIncreasedCreditTransactionType,
		remainingPrincipal,
		currency,
		model.ContractActivatedCreditTransactionReason,
		model.ContractCreditTransactionReferenceType,
		contractID.UUID(),
		paidAt,
		now,
	)
	require.NoError(t, err)
}
