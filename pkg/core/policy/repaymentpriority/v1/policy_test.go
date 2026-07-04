package v1_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentpriority/v1"
)

func TestSelect_EarliestPastDueAcrossContracts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contractA := model.ContractID(uuid.New())
	contractB := model.ContractID(uuid.New())
	earlier := model.InstallmentID(uuid.New())
	later := model.InstallmentID(uuid.New())

	selection := v1.New().Select([]v1.Candidate{
		{
			Installment: model.Installment{
				ID:         later,
				ContractID: &contractA,
				DueAt:      now.Add(-24 * time.Hour),
				Status:     model.DueInstallmentStatus,
			},
		},
		{
			Installment: model.Installment{
				ID:         earlier,
				ContractID: &contractB,
				DueAt:      now.Add(-72 * time.Hour),
				Status:     model.OverdueInstallmentStatus,
			},
		},
	}, now)

	require.NotNil(t, selection.Selected)
	require.Equal(t, earlier, selection.Selected.ID)
	require.False(t, selection.HasUnresolvedPayment)
}

func TestSelect_DoesNotSkipPendingPriority(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contractA := model.ContractID(uuid.New())
	contractB := model.ContractID(uuid.New())
	priorityID := model.InstallmentID(uuid.New())
	otherID := model.InstallmentID(uuid.New())

	selection := v1.New().Select([]v1.Candidate{
		{
			Installment: model.Installment{
				ID:         priorityID,
				ContractID: &contractA,
				DueAt:      now.Add(-48 * time.Hour),
				Status:     model.DueInstallmentStatus,
			},
			HasUnresolvedPayment: true,
		},
		{
			Installment: model.Installment{
				ID:         otherID,
				ContractID: &contractB,
				DueAt:      now.Add(-24 * time.Hour),
				Status:     model.DueInstallmentStatus,
			},
		},
	}, now)

	require.NotNil(t, selection.Selected)
	require.Equal(t, priorityID, selection.Selected.ID)
	require.True(t, selection.HasUnresolvedPayment)
}

func TestSelect_FallsBackToEarliestScheduled(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contractA := model.ContractID(uuid.New())
	contractB := model.ContractID(uuid.New())
	soon := model.InstallmentID(uuid.New())
	later := model.InstallmentID(uuid.New())

	selection := v1.New().Select([]v1.Candidate{
		{
			Installment: model.Installment{
				ID:         later,
				ContractID: &contractA,
				DueAt:      now.Add(72 * time.Hour),
				Status:     model.ScheduledInstallmentStatus,
			},
		},
		{
			Installment: model.Installment{
				ID:         soon,
				ContractID: &contractB,
				DueAt:      now.Add(24 * time.Hour),
				Status:     model.ScheduledInstallmentStatus,
			},
		},
	}, now)

	require.NotNil(t, selection.Selected)
	require.Equal(t, soon, selection.Selected.ID)
}
