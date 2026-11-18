package installmentlifecycleuc

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	collectionsv3 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v3"
	"github.com/stretchr/testify/require"
)

func TestOverdueBusinessDaysThrough_GraceThenCatchUp(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	due := time.Date(2026, 11, 1, 18, 0, 0, 0, loc)
	installment := model.Installment{DueAt: due, GracePeriodDays: 4}
	overdueAt := installment.OverdueEligibleAt(loc)

	// Days 2–5 (relative to due day) are still within grace: no fee days yet.
	graceDay := time.Date(due.Year(), due.Month(), due.Day(), 12, 0, 0, 0, loc).AddDate(0, 0, 4)
	require.True(t, graceDay.Before(overdueAt))
	require.Empty(t, overdueBusinessDaysThrough(installment, graceDay, loc))

	// Day 6 / overdue midnight: first fee day.
	days := overdueBusinessDaysThrough(installment, overdueAt, loc)
	require.Len(t, days, 1)
	require.Equal(t, overdueAt.In(loc).Format("2006-01-02"), lateFeeBusinessDateKey(days[0], loc))

	// Multi-day catch-up through day 8.
	day8 := overdueAt.In(loc).AddDate(0, 0, 2)
	days = overdueBusinessDaysThrough(installment, day8, loc)
	require.Len(t, days, 3)
}

func TestLateFeeAmountMatchesV3(t *testing.T) {
	require.Equal(t, uint64(167), collectionsv3.LateFeeAmountMinor(100_000))
}
