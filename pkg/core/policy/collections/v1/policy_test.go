package v1_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
)

func TestEvaluate_DefaultEntry_FirstInstantAfterGrace(t *testing.T) {
	t.Parallel()

	dueAt := time.Date(2026, time.March, 1, 0, 0, 0, 0, time.UTC)
	grace := uint(5)
	eligibleAt := dueAt.Add(5 * 24 * time.Hour)
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	before := v1.New().Evaluate([]v1.ObligationFact{{
		InstallmentID:             installmentID,
		ContractID:                &contractID,
		DueAt:                     dueAt,
		GracePeriodDays:           grace,
		Status:                    model.DueInstallmentStatus,
		IsContractuallyApplicable: true,
	}}, eligibleAt.Add(-time.Nanosecond), false)
	require.False(t, before.IsDefaultEligible)

	at := v1.New().Evaluate([]v1.ObligationFact{{
		InstallmentID:             installmentID,
		ContractID:                &contractID,
		DueAt:                     dueAt,
		GracePeriodDays:           grace,
		Status:                    model.DueInstallmentStatus,
		IsContractuallyApplicable: true,
	}}, eligibleAt, false)
	require.True(t, at.IsDefaultEligible)
	require.Equal(t, []model.InstallmentID{installmentID}, at.TriggerInstallmentIDs)
}

func TestEvaluate_RecoveryRequiresAllDueSettled(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	overdueID := model.InstallmentID(uuid.New())
	dueTodayID := model.InstallmentID(uuid.New())
	futureID := model.InstallmentID(uuid.New())

	eval := v1.New().Evaluate([]v1.ObligationFact{
		{
			InstallmentID:             overdueID,
			ContractID:                &contractID,
			DueAt:                     now.Add(-10 * 24 * time.Hour),
			GracePeriodDays:           5,
			Status:                    model.OverdueInstallmentStatus,
			IsFullySettled:            true,
			IsContractuallyApplicable: true,
		},
		{
			InstallmentID:             dueTodayID,
			ContractID:                &contractID,
			DueAt:                     now.Add(-time.Hour),
			GracePeriodDays:           5,
			Status:                    model.DueInstallmentStatus,
			IsFullySettled:            false,
			IsContractuallyApplicable: true,
		},
		{
			InstallmentID:             futureID,
			ContractID:                &contractID,
			DueAt:                     now.Add(48 * time.Hour),
			GracePeriodDays:           5,
			Status:                    model.ScheduledInstallmentStatus,
			IsFullySettled:            false,
			IsContractuallyApplicable: true,
		},
	}, now, true)

	require.False(t, eval.IsRecovered)
	require.Equal(t, []model.InstallmentID{dueTodayID}, eval.UnsettledDueInstallmentIDs)

	recovered := v1.New().Evaluate([]v1.ObligationFact{
		{
			InstallmentID:             overdueID,
			ContractID:                &contractID,
			DueAt:                     now.Add(-10 * 24 * time.Hour),
			GracePeriodDays:           5,
			Status:                    model.PaidInstallmentStatus,
			IsFullySettled:            true,
			IsContractuallyApplicable: true,
		},
		{
			InstallmentID:             dueTodayID,
			ContractID:                &contractID,
			DueAt:                     now.Add(-time.Hour),
			GracePeriodDays:           5,
			Status:                    model.PaidInstallmentStatus,
			IsFullySettled:            true,
			IsContractuallyApplicable: true,
		},
		{
			InstallmentID:             futureID,
			ContractID:                &contractID,
			DueAt:                     now.Add(48 * time.Hour),
			GracePeriodDays:           5,
			Status:                    model.ScheduledInstallmentStatus,
			IsFullySettled:            false,
			IsContractuallyApplicable: true,
		},
	}, now, true)

	require.True(t, recovered.IsRecovered)
	require.Empty(t, recovered.UnsettledDueInstallmentIDs)
}

func TestEvaluate_ZeroGraceDefaultsAtDue(t *testing.T) {
	t.Parallel()

	dueAt := time.Date(2026, time.March, 1, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	eval := v1.New().Evaluate([]v1.ObligationFact{{
		InstallmentID:             installmentID,
		ContractID:                &contractID,
		DueAt:                     dueAt,
		GracePeriodDays:           0,
		Status:                    model.DueInstallmentStatus,
		IsContractuallyApplicable: true,
	}}, dueAt, false)

	require.True(t, eval.IsDefaultEligible)
}

func TestDurationGraceAcrossDSTAndDelayedLifecycle(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	contract := model.ContractID(uuid.New())
	for _, due := range []time.Time{time.Date(2026, 3, 7, 15, 30, 0, 0, loc), time.Date(2026, 10, 31, 15, 30, 0, 0, loc)} {
		for _, days := range []uint{0, 1, 3, 45} {
			i := model.Installment{ID: model.InstallmentID(uuid.New()), ContractID: &contract, DueAt: due, GracePeriodDays: days, Status: model.ScheduledInstallmentStatus}
			facts := []v1.ObligationFact{v1.FactFromUnpaidInstallment(i, false)}
			boundary := due.Add(time.Duration(days) * 24 * time.Hour)
			require.False(t, v1.New().Evaluate(facts, boundary.Add(-time.Nanosecond), false).IsDefaultEligible)
			require.True(t, v1.New().Evaluate(facts, boundary, false).IsDefaultEligible)
			require.True(t, v1.New().Evaluate(facts, boundary.Add(48*time.Hour), false).IsDefaultEligible)
			require.True(t, i.IsOverdueEligible(i.OverdueEligibleAt()))
		}
	}
}
