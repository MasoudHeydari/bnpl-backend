package installmentrepaymentuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestSelectPriorityInstallment_EarliestPastDueAcrossContracts(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contractA := model.ContractID(uuid.New())
	contractB := model.ContractID(uuid.New())
	earlierPastDue := model.InstallmentID(uuid.New())
	laterPastDue := model.InstallmentID(uuid.New())
	scheduled := model.InstallmentID(uuid.New())

	rows := []model.Installment{
		{
			ID:               laterPastDue,
			ContractID:       &contractA,
			InstallmentOrder: 1,
			DueAt:            now.Add(-24 * time.Hour),
			Status:           model.DueInstallmentStatus,
		},
		{
			ID:               earlierPastDue,
			ContractID:       &contractB,
			InstallmentOrder: 1,
			DueAt:            now.Add(-72 * time.Hour),
			Status:           model.OverdueInstallmentStatus,
		},
		{
			ID:               scheduled,
			ContractID:       &contractA,
			InstallmentOrder: 2,
			DueAt:            now.Add(48 * time.Hour),
			Status:           model.ScheduledInstallmentStatus,
		},
	}

	selected := selectPriorityInstallment(rows, now)
	require.NotNil(t, selected)
	require.Equal(t, earlierPastDue, selected.ID)
}

func TestSelectPriorityInstallment_SameContractSequenceBlocksLater(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.March, 10, 12, 0, 0, 0, time.UTC)
	contractID := model.ContractID(uuid.New())
	first := model.InstallmentID(uuid.New())
	second := model.InstallmentID(uuid.New())

	rows := []model.Installment{
		{
			ID:               first,
			ContractID:       &contractID,
			InstallmentOrder: 1,
			DueAt:            now.Add(-48 * time.Hour),
			Status:           model.DueInstallmentStatus,
		},
		{
			ID:               second,
			ContractID:       &contractID,
			InstallmentOrder: 2,
			DueAt:            now.Add(-24 * time.Hour),
			Status:           model.DueInstallmentStatus,
		},
	}

	selected := selectPriorityInstallment(rows, now)
	require.NotNil(t, selected)
	require.Equal(t, first, selected.ID)
}
