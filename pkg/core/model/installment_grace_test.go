package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInstallment_OverdueEligibleAt(t *testing.T) {
	t.Parallel()

	dueAt := time.Date(
		2026,
		7,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)

	t.Run("zero day grace", func(t *testing.T) {
		t.Parallel()

		installment := Installment{DueAt: dueAt, GracePeriodDays: 0}
		assert.Equal(t, dueAt.AddDate(0, 0, 1), installment.OverdueEligibleAt())
	})

	t.Run("five day grace", func(t *testing.T) {
		t.Parallel()

		installment := Installment{DueAt: dueAt, GracePeriodDays: 5}
		assert.Equal(t, dueAt.AddDate(0, 0, 6), installment.OverdueEligibleAt())
	})
}

func TestInstallment_IsOverdueEligible(t *testing.T) {
	t.Parallel()

	dueAt := time.Date(
		2026,
		7,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	installment := Installment{DueAt: dueAt, GracePeriodDays: 5}
	eligibleAt := dueAt.AddDate(0, 0, 6)

	assert.False(t, installment.IsOverdueEligible(eligibleAt.Add(-time.Second)))
	assert.True(t, installment.IsOverdueEligible(eligibleAt))
}
