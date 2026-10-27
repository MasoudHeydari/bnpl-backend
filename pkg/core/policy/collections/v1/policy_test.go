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
