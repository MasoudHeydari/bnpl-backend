package model_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/stretchr/testify/require"
)

func TestRepaymentAggregateRetainsBNPLBreakdown(t *testing.T) {
	installment := model.Installment{ID: model.InstallmentID(uuid.New()), PrincipalAmountMinor: 100, Currency: model.IRRCurrency}
	charge := func(kind model.InstallmentChargeType, amount uint64) model.InstallmentCharge {
		return model.InstallmentCharge{ID: model.InstallmentChargeID(uuid.New()), InstallmentID: installment.ID, ChargeType: kind, ChargeStatus: model.AppliedInstallmentChargeStatus, AmountMinor: amount, Currency: installment.Currency, PayerType: model.CustomerInstallmentChargePayerType, RecipientType: model.PlatformInstallmentChargeRecipientType}
	}
	for _, tc := range []struct {
		name    string
		charges []model.InstallmentCharge
		want    uint64
	}{
		{"principal", nil, 100},
		{"fee", []model.InstallmentCharge{charge(model.BNPLFeeInstallmentChargeType, 10)}, 110},
		{"late fee", []model.InstallmentCharge{charge(model.LateFeeInstallmentChargeType, 20)}, 120},
		{"discount", []model.InstallmentCharge{charge(model.DiscountInstallmentChargeType, 10)}, 90},
		{"waiver", []model.InstallmentCharge{charge(model.WaiverInstallmentChargeType, 20)}, 80},
		{"net principal still retains separate items", []model.InstallmentCharge{charge(model.TaxInstallmentChargeType, 10), charge(model.DiscountInstallmentChargeType, 10)}, 100},
		{"multiple components", []model.InstallmentCharge{charge(model.CommissionInstallmentChargeType, 10), charge(model.BNPLFeeInstallmentChargeType, 20), charge(model.TaxInstallmentChargeType, 5)}, 135},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, model.ValidateCustomerPaymentComponents(installment, tc.charges))
			amount, ok, err := model.CustomerPayableAmount(installment, tc.charges)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, tc.want, amount)
			items, err := model.BuildPaymentAttemptItems(installment, tc.charges)
			require.NoError(t, err)
			require.Len(t, items, len(tc.charges)+1)
			require.EqualValues(t, 100, items[0].AmountMinor)
		})
	}
	merchant := charge(model.CommissionInstallmentChargeType, 20)
	merchant.PayerType = model.MerchantInstallmentChargePayerType
	amount, ok, err := model.CustomerPayableAmount(installment, []model.InstallmentCharge{merchant})
	require.NoError(t, err)
	require.True(t, ok)
	require.EqualValues(t, 100, amount)
	bad := charge(model.BNPLFeeInstallmentChargeType, 10)
	bad.Currency = model.USDCurrency
	require.Error(t, model.ValidateCustomerPaymentComponents(installment, []model.InstallmentCharge{bad}))
	for _, amount := range []uint64{101} {
		_, ok, err = model.CustomerPayableAmount(installment, []model.InstallmentCharge{charge(model.DiscountInstallmentChargeType, amount)})
		require.NoError(t, err)
		require.False(t, ok)
	}
}
