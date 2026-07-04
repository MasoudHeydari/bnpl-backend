package v1_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/calendar/jalali"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
)

func TestBuildInstallmentSchedule_PrincipalSplitMatchesFinancingSplit(t *testing.T) {
	t.Parallel()

	financedAmountMinor := uint64(100_003)
	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	financingResult := v1.New().EvaluateFinancing(checkout.FinancingInput{
		RequestedAmountMinor: financedAmountMinor,
		CreditAccount:        modelCreditAccount(financedAmountMinor, 0),
		Eligibility:          approvedEligibility(),
	})

	scheduleResult, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: financedAmountMinor,
		UpfrontAmountMinor:  0,
		InstallmentCount:    4,
	})
	require.NoError(t, err)

	require.Equal(t, financingResult.FirstInstallmentPrincipalAmountMinor, scheduleResult.Installments[0].PrincipalAmountMinor)
	require.Equal(t, financingResult.RegularInstallmentPrincipalAmountMinor, scheduleResult.Installments[1].PrincipalAmountMinor)
}

func TestBuildInstallmentSchedule_UpfrontPlacedEntirelyOnFirstInstallment(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	result, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		UpfrontAmountMinor:  25_000,
		InstallmentCount:    4,
	})
	require.NoError(t, err)

	require.Equal(t, uint64(25_000), result.Installments[0].UpfrontAmountMinor)
	for _, installment := range result.Installments[1:] {
		require.Equal(t, uint64(0), installment.UpfrontAmountMinor)
	}
}

func TestBuildInstallmentSchedule_TotalEqualsPrincipalPlusUpfrontPlusFee(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	result, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		UpfrontAmountMinor:  25_000,
		InstallmentCount:    4,
	})
	require.NoError(t, err)

	for _, installment := range result.Installments {
		require.Equal(
			t,
			installment.PrincipalAmountMinor+installment.UpfrontAmountMinor+installment.BaseFeeAmountMinor,
			installment.TotalAmountMinor,
		)
		require.Equal(t, uint64(0), installment.BaseFeeAmountMinor)
	}
}

func TestBuildInstallmentSchedule_FirstInstallmentDueAtPaymentWindow(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	result, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		InstallmentCount:    4,
	})
	require.NoError(t, err)

	require.Equal(t, now.Add(15*time.Minute), result.Installments[0].DueAt)
}

func TestBuildInstallmentSchedule_RegularInstallmentsFollowJalaliMonthTenor(t *testing.T) {
	t.Parallel()

	// 2026-08-08 is 1405/05/17. Later installments must stay on Jalali day 17
	// even when crossing from 31-day months into 30-day months.
	now := time.Date(2026, 8, 8, 13, 43, 37, 98452000, time.UTC)

	result, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		InstallmentCount:    4,
	})
	require.NoError(t, err)

	require.Equal(t, jalali.Date{Year: 1405, Month: 5, Day: 17}, jalali.FromTime(result.Installments[0].DueAt))
	require.Equal(t, jalali.Date{Year: 1405, Month: 6, Day: 17}, jalali.FromTime(result.Installments[1].DueAt))
	require.Equal(t, jalali.Date{Year: 1405, Month: 7, Day: 17}, jalali.FromTime(result.Installments[2].DueAt))
	require.Equal(t, jalali.Date{Year: 1405, Month: 8, Day: 17}, jalali.FromTime(result.Installments[3].DueAt))

	require.Equal(t, jalali.AddMonthsClamped(now, 1), result.Installments[1].DueAt)
	require.Equal(t, jalali.AddMonthsClamped(now, 2), result.Installments[2].DueAt)
	require.Equal(t, jalali.AddMonthsClamped(now, 3), result.Installments[3].DueAt)
}

func TestBuildInstallmentSchedule_JalaliEndOfMonthClamping(t *testing.T) {
	t.Parallel()

	// Farvardin 31 + 6 Jalali months -> Mehr 30.
	now := jalali.Date{Year: 1404, Month: 1, Day: 31}.ToTime(
		time.Date(1, 1, 1, 12, 0, 0, 0, time.UTC),
		time.UTC,
	)

	result, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		InstallmentCount:    4,
	})
	require.NoError(t, err)

	require.Equal(t, jalali.Date{Year: 1404, Month: 2, Day: 31}, jalali.FromTime(result.Installments[1].DueAt))
	require.Equal(t, jalali.Date{Year: 1404, Month: 3, Day: 31}, jalali.FromTime(result.Installments[2].DueAt))
	require.Equal(t, jalali.Date{Year: 1404, Month: 4, Day: 31}, jalali.FromTime(result.Installments[3].DueAt))

	mehr, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		InstallmentCount:    7,
	})
	require.NoError(t, err)
	require.Equal(t, jalali.Date{Year: 1404, Month: 7, Day: 30}, jalali.FromTime(mehr.Installments[6].DueAt))
}

func TestBuildInstallmentSchedule_ClampRecomputesFromOriginalJalaliAnchor(t *testing.T) {
	t.Parallel()

	now := jalali.Date{Year: 1404, Month: 1, Day: 31}.ToTime(
		time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		time.UTC,
	)

	result, err := v1.New().BuildInstallmentSchedule(checkout.ScheduleInput{
		Now:                 now,
		FinancedAmountMinor: 100_000,
		InstallmentCount:    8,
	})
	require.NoError(t, err)

	// Month index 7 is Aban; day stays clamped from the original Farvardin 31
	// anchor, not from a previously clamped Mehr 30.
	require.Equal(t, jalali.Date{Year: 1404, Month: 8, Day: 30}, jalali.FromTime(result.Installments[7].DueAt))
}

func TestBuildInstallmentSchedule_ErrorPaths(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		input checkout.ScheduleInput
	}{
		{
			name: "zero installment count",
			input: checkout.ScheduleInput{
				Now:                 now,
				FinancedAmountMinor: 100_000,
				InstallmentCount:    0,
			},
		},
		{
			name: "zero financed amount",
			input: checkout.ScheduleInput{
				Now:                 now,
				FinancedAmountMinor: 0,
				InstallmentCount:    4,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := v1.New().BuildInstallmentSchedule(tt.input)
			require.Error(t, err)
		})
	}
}

func TestBuildInstallmentSchedule_DeterministicForIdenticalInput(t *testing.T) {
	t.Parallel()

	input := checkout.ScheduleInput{
		Now:                 time.Date(2026, 8, 8, 13, 43, 37, 0, time.UTC),
		FinancedAmountMinor: 100_000,
		UpfrontAmountMinor:  25_000,
		InstallmentCount:    4,
	}
	policy := v1.New()

	first, err := policy.BuildInstallmentSchedule(input)
	require.NoError(t, err)
	second, err := policy.BuildInstallmentSchedule(input)
	require.NoError(t, err)

	require.Equal(t, first, second)
}
