package collectionsuc_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestCollectionsPolicy_EntryAndRecoveryBoundaries(t *testing.T) {
	t.Parallel()

	policy := collectionsv1.New()
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	t.Run("inside grace is not default eligible", func(t *testing.T) {
		t.Parallel()

		dueAt := now.Add(-24 * time.Hour)
		eval := policy.Evaluate([]collectionsv1.ObligationFact{{
			InstallmentID:             installmentID,
			ContractID:                &contractID,
			DueAt:                     dueAt,
			GracePeriodDays:           2,
			Status:                    model.DueInstallmentStatus,
			IsFullySettled:            false,
			IsContractuallyApplicable: true,
		}}, now, false)

		require.False(t, eval.IsDefaultEligible)
		require.False(t, eval.IsRecovered)
	})

	t.Run("first instant after grace is default eligible", func(t *testing.T) {
		t.Parallel()

		dueAt := now.Add(-48 * time.Hour)
		eval := policy.Evaluate([]collectionsv1.ObligationFact{{
			InstallmentID:             installmentID,
			ContractID:                &contractID,
			DueAt:                     dueAt,
			GracePeriodDays:           2,
			Status:                    model.DueInstallmentStatus,
			IsFullySettled:            false,
			IsContractuallyApplicable: true,
		}}, now, false)

		require.True(t, eval.IsDefaultEligible)
		require.Equal(t, []model.InstallmentID{installmentID}, eval.TriggerInstallmentIDs)
	})

	t.Run("recovery requires all due installments settled", func(t *testing.T) {
		t.Parallel()

		overdueID := model.InstallmentID(uuid.New())
		dueID := model.InstallmentID(uuid.New())
		futureID := model.InstallmentID(uuid.New())

		facts := []collectionsv1.ObligationFact{
			{
				InstallmentID:             overdueID,
				ContractID:                &contractID,
				DueAt:                     now.Add(-72 * time.Hour),
				GracePeriodDays:           1,
				Status:                    model.PaidInstallmentStatus,
				IsFullySettled:            true,
				IsContractuallyApplicable: true,
			},
			{
				InstallmentID:             dueID,
				ContractID:                &contractID,
				DueAt:                     now.Add(-12 * time.Hour),
				GracePeriodDays:           3,
				Status:                    model.DueInstallmentStatus,
				IsFullySettled:            false,
				IsContractuallyApplicable: true,
			},
			{
				InstallmentID:             futureID,
				ContractID:                &contractID,
				DueAt:                     now.Add(30 * 24 * time.Hour),
				GracePeriodDays:           3,
				Status:                    model.ScheduledInstallmentStatus,
				IsFullySettled:            false,
				IsContractuallyApplicable: true,
			},
		}

		eval := policy.Evaluate(facts, now, true)
		require.False(t, eval.IsRecovered)
		require.Contains(t, eval.UnsettledDueInstallmentIDs, dueID)

		facts[1].IsFullySettled = true
		facts[1].Status = model.PaidInstallmentStatus
		eval = policy.Evaluate(facts, now, true)
		require.True(t, eval.IsRecovered)
	})

	t.Run("unresolved payment blocks recovery and marks uncertainty", func(t *testing.T) {
		t.Parallel()

		eval := policy.Evaluate([]collectionsv1.ObligationFact{{
			InstallmentID:             installmentID,
			ContractID:                &contractID,
			DueAt:                     now.Add(-24 * time.Hour),
			GracePeriodDays:           0,
			Status:                    model.DueInstallmentStatus,
			IsFullySettled:            false,
			HasUnresolvedPayment:      true,
			IsContractuallyApplicable: true,
		}}, now, true)

		require.False(t, eval.IsRecovered)
		require.True(t, eval.HasPaymentUncertainty)
	})
}
