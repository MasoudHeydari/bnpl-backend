package v1_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentreminder/v1"
)

func fact(due time.Time, grace uint, status model.InstallmentStatus, pending bool) v1.InstallmentFact {
	return v1.InstallmentFact{
		InstallmentID:        model.InstallmentID(uuid.New()),
		DueAt:                due,
		GracePeriodDays:      grace,
		LeadDays:             3,
		Status:               status,
		HasUnresolvedPayment: pending,
	}
}

func atHour(day time.Time, hour int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
}

func TestEvaluateAll_IndependentInstallmentsSameDay(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("Asia/Tehran", 3*3600+30*60)
	day := time.Date(2026, 6, 10, 0, 0, 0, 0, loc)
	a := fact(day, 3, model.DueInstallmentStatus, false)
	b := fact(day, 3, model.DueInstallmentStatus, false)

	decisions := v1.New().EvaluateAll(atHour(day, 10), day, []v1.InstallmentFact{a, b})
	require.Len(t, decisions, 2)
	require.Equal(t, model.DueTodayReminderType, decisions[0].ReminderType)
	require.Equal(t, model.DueTodayReminderType, decisions[1].ReminderType)
	require.NotEqual(t, decisions[0].InstallmentID, decisions[1].InstallmentID)
}

func TestEvaluateAll_PriorityOrdersButDoesNotSuppress(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("Asia/Tehran", 3*3600+30*60)
	day := time.Date(2026, 6, 10, 0, 0, 0, 0, loc)
	dueToday := fact(day, 3, model.DueInstallmentStatus, false)
	graceFinal := fact(day.AddDate(0, 0, -3), 3, model.OverdueInstallmentStatus, false)

	decisions := v1.New().EvaluateAll(atHour(day, 13), day, []v1.InstallmentFact{dueToday, graceFinal})
	require.Len(t, decisions, 2)
	require.Equal(t, model.GraceFinalDayReminderType, decisions[0].ReminderType)
	require.Equal(t, model.DueTodayReminderType, decisions[1].ReminderType)
}

func TestEvaluateAll_PreDueLeadWindow(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("Asia/Tehran", 3*3600+30*60)
	day := time.Date(2026, 6, 10, 0, 0, 0, 0, loc)
	due := day.AddDate(0, 0, 2)
	f := fact(due, 3, model.ScheduledInstallmentStatus, false)

	require.Empty(t, v1.New().EvaluateAll(atHour(day, 9), day, []v1.InstallmentFact{f}))

	decisions := v1.New().EvaluateAll(atHour(day, 10), day, []v1.InstallmentFact{f})
	require.Len(t, decisions, 1)
	require.Equal(t, model.PreDueReminderType, decisions[0].ReminderType)
}

func TestEvaluateAll_PendingAndPaidExcluded(t *testing.T) {
	t.Parallel()

	loc := time.FixedZone("Asia/Tehran", 3*3600+30*60)
	day := time.Date(2026, 6, 10, 0, 0, 0, 0, loc)
	pending := fact(day, 3, model.DueInstallmentStatus, true)
	paid := fact(day, 3, model.PaidInstallmentStatus, false)
	ok := fact(day, 3, model.DueInstallmentStatus, false)

	decisions := v1.New().EvaluateAll(atHour(day, 10), day, []v1.InstallmentFact{pending, paid, ok})
	require.Len(t, decisions, 1)
	require.Equal(t, ok.InstallmentID, decisions[0].InstallmentID)
}

func TestNotificationIdempotencyKey_Stable(t *testing.T) {
	t.Parallel()

	id := model.InstallmentID(uuid.MustParse("11111111-1111-1111-1111-111111111111"))
	day := time.Date(2026, 6, 10, 15, 0, 0, 0, time.FixedZone("Asia/Tehran", 3*3600+30*60))
	key := v1.NotificationIdempotencyKey(id, model.DueTodayReminderType, day)
	require.Equal(t, "repayment:11111111-1111-1111-1111-111111111111:due_today:2026-06-10", key)
}
