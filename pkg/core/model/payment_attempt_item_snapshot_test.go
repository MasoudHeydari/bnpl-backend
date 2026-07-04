package model

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestBuildPaymentAttemptItems_IncludesUpfrontAmount(t *testing.T) {
	t.Parallel()

	installmentID := InstallmentID(uuid.New())
	chargeID := InstallmentChargeID(uuid.New())
	installment := Installment{
		ID:                   installmentID,
		PrincipalAmountMinor: 40_000,
		UpfrontAmountMinor:   60_000,
	}
	charges := []InstallmentCharge{{
		ID:            chargeID,
		InstallmentID: installmentID,
		ChargeType:    BNPLFeeInstallmentChargeType,
		ChargeStatus:  OfferedInstallmentChargeStatus,
		AmountMinor:   5_000,
		PayerType:     CustomerInstallmentChargePayerType,
	}}

	items, err := BuildPaymentAttemptItems(installment, charges)
	require.NoError(t, err)
	require.Len(t, items, 3)
	require.Equal(t, PrincipalPaymentAttemptItemType, items[0].ItemType)
	require.Equal(t, UpfrontPaymentAttemptItemType, items[1].ItemType)
	require.Equal(t, installmentID.UUID(), items[1].SourceID)
	require.Equal(t, uint64(60_000), items[1].AmountMinor)
	require.Equal(t, DebitPaymentAttemptItemEffect, items[1].Effect)

	payable, ok := PayableAmountFromItems(items)
	require.True(t, ok)
	require.Equal(t, uint64(105_000), payable)
}
