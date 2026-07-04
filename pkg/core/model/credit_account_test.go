package model_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCreditAccount_AvailableAmountMinor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		limit    uint64
		exposure uint64
		want     uint64
	}{
		{name: "full available", limit: 100, exposure: 0, want: 100},
		{name: "partial available", limit: 100, exposure: 40, want: 60},
		{name: "fully consumed", limit: 100, exposure: 100, want: 0},
		{name: "limit below exposure", limit: 40, exposure: 80, want: 0},
		{name: "zero limit", limit: 0, exposure: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			account := model.CreditAccount{
				LimitAmountMinor:    tt.limit,
				ExposureAmountMinor: tt.exposure,
			}
			require.Equal(t, tt.want, account.AvailableAmountMinor())
		})
	}
}

func TestCreditAccount_IsUsableAt(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		9,
		18,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	future := now.Add(time.Hour)
	past := now.Add(-time.Hour)

	t.Run("active without expiry", func(t *testing.T) {
		t.Parallel()
		require.True(t, model.CreditAccount{Status: model.ActiveCreditAccountStatus}.IsUsableAt(now))
	})

	t.Run("active before expiry", func(t *testing.T) {
		t.Parallel()
		require.True(t, model.CreditAccount{
			Status:    model.ActiveCreditAccountStatus,
			ExpiresAt: &future,
		}.IsUsableAt(now))
	})

	t.Run("active at or after expiry", func(t *testing.T) {
		t.Parallel()
		require.False(t, model.CreditAccount{
			Status:    model.ActiveCreditAccountStatus,
			ExpiresAt: &past,
		}.IsUsableAt(now))
	})

	t.Run("expired status", func(t *testing.T) {
		t.Parallel()
		require.False(t, model.CreditAccount{Status: model.ExpiredCreditAccountStatus}.IsUsableAt(now))
	})
}

func TestCreditTransaction_Validate_BusinessFactShape(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	accountID := model.CreditAccountID(uuid.New())
	refID := uuid.New()

	valid := func(mutate func(*model.CreditTransaction)) model.CreditTransaction {
		tx := model.CreditTransaction{
			CreditAccountID: accountID,
			Type:            model.LimitSetCreditTransactionType,
			AmountMinor:     1,
			Currency:        model.IRRCurrency,
			Reason:          model.CreditAssessmentCreditTransactionReason,
			ReferenceType:   model.DecisionCreditTransactionReferenceType,
			ReferenceID:     refID,
			OccurredAt:      now,
			CreatedAt:       now,
		}
		mutate(&tx)
		return tx
	}

	require.NoError(t, valid(func(*model.CreditTransaction) {}).Validate())

	require.NoError(t, valid(func(tx *model.CreditTransaction) {
		tx.Type = model.ExposureIncreasedCreditTransactionType
		tx.Reason = model.ContractActivatedCreditTransactionReason
		tx.ReferenceType = model.ContractCreditTransactionReferenceType
	}).Validate())

	require.NoError(t, valid(func(tx *model.CreditTransaction) {
		tx.Type = model.ExposureDecreasedCreditTransactionType
		tx.Reason = model.InstallmentPaidCreditTransactionReason
		tx.ReferenceType = model.InstallmentCreditTransactionReferenceType
	}).Validate())

	require.NoError(t, valid(func(tx *model.CreditTransaction) {
		tx.Type = model.ExposureDecreasedCreditTransactionType
		tx.Reason = model.ContractCanceledCreditTransactionReason
		tx.ReferenceType = model.ContractCreditTransactionReferenceType
	}).Validate())

	err := valid(func(tx *model.CreditTransaction) {
		tx.Type = model.ExposureIncreasedCreditTransactionType
		tx.Reason = model.InstallmentPaidCreditTransactionReason
		tx.ReferenceType = model.InstallmentCreditTransactionReferenceType
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "exposure_increased")

	err = valid(func(tx *model.CreditTransaction) {
		tx.Type = model.LimitIncreasedCreditTransactionType
		tx.Reason = model.ContractActivatedCreditTransactionReason
		tx.ReferenceType = model.ContractCreditTransactionReferenceType
	}).Validate()
	require.Error(t, err)
	require.Contains(t, err.Error(), "limit credit transactions")
}
