package v2_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	v2 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v2"
)

func TestTerms_ThreeInstallmentsStartingOneJalaliMonthAfterAnchor(t *testing.T) {
	t.Parallel()

	terms := v2.New().Terms()
	require.Equal(t, 3, terms.InstallmentCount)
	require.Equal(t, 1, terms.FirstInstallmentDueAfterMonths)
	require.Equal(t, checkout.FullFinancingOnly, terms.FinancingMode)
	require.Equal(t, checkout.ActivationAfterMerchantFunding, terms.ActivationMode)
	require.Equal(t, checkout.ScheduleFromContractActivation, terms.ScheduleAnchor)
}

func TestBuildInstallmentSchedule_DueAtOneTwoThreeJalaliMonthsAfterPurchase(t *testing.T) {
	t.Parallel()

	now := time.Date(
		2026,
		3,
		10,
		9,
		0,
		0,
		0,
		time.UTC,
	)
	policy := v2.New()
	terms := policy.Terms()

	result, err := policy.BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 90_000,
		UpfrontAmountMinor:  0,
		InstallmentCount:    terms.InstallmentCount,
	})
	require.NoError(t, err)
	require.Len(t, result.Installments, 3)

	require.Equal(t, jalali.AddMonthsClamped(now, 1), result.Installments[0].DueAt)
	require.Equal(t, jalali.AddMonthsClamped(now, 2), result.Installments[1].DueAt)
	require.Equal(t, jalali.AddMonthsClamped(now, 3), result.Installments[2].DueAt)

	require.Equal(t, 1, result.Installments[0].Order)
	require.Equal(t, 2, result.Installments[1].Order)
	require.Equal(t, 3, result.Installments[2].Order)
}
