package model_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestNextPayableInSequence_SelectsEarliestUnpaidOrder(t *testing.T) {
	t.Parallel()

	firstID := model.InstallmentID(uuid.New())
	secondID := model.InstallmentID(uuid.New())
	thirdID := model.InstallmentID(uuid.New())

	installments := []model.Installment{
		{ID: firstID, InstallmentOrder: 1, Status: model.PaidInstallmentStatus},
		{ID: thirdID, InstallmentOrder: 3, Status: model.ScheduledInstallmentStatus},
		{ID: secondID, InstallmentOrder: 2, Status: model.DueInstallmentStatus},
	}

	next, ok := model.NextPayableInSequence(installments)
	require.True(t, ok)
	require.Equal(t, secondID, next.ID)
	require.Equal(t, 2, next.InstallmentOrder)
}

func TestIsNextPayableInSequence_RejectsSkippingEarlierUnpaid(t *testing.T) {
	t.Parallel()

	firstID := model.InstallmentID(uuid.New())
	secondID := model.InstallmentID(uuid.New())
	thirdID := model.InstallmentID(uuid.New())

	installments := []model.Installment{
		{ID: firstID, InstallmentOrder: 1, Status: model.PaidInstallmentStatus},
		{ID: secondID, InstallmentOrder: 2, Status: model.DueInstallmentStatus},
		{ID: thirdID, InstallmentOrder: 3, Status: model.ScheduledInstallmentStatus},
	}

	third := installments[2]
	require.False(t, model.IsNextPayableInSequence(third, installments))
	require.True(t, model.IsNextPayableInSequence(installments[1], installments))
}

func TestNextPayableInSequence_NoneWhenAllPaid(t *testing.T) {
	t.Parallel()

	_, ok := model.NextPayableInSequence([]model.Installment{
		{ID: model.InstallmentID(uuid.New()), InstallmentOrder: 1, Status: model.PaidInstallmentStatus},
		{ID: model.InstallmentID(uuid.New()), InstallmentOrder: 2, Status: model.PaidInstallmentStatus},
	})
	require.False(t, ok)
}
