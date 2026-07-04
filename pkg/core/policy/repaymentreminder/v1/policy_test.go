package v1_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	v1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentreminder/v1"
)

func tehran() *time.Location {
	loc, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		panic(err)
	}

	return loc
}

func businessDay(loc *time.Location, year int, month time.Month, day int) time.Time {
	return time.Date(year, month, day, 0, 0, 0, 0, loc)
}

func atHour(day time.Time, hour int) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, day.Location())
}

func fact(
	dueDay time.Time,
	grace uint,
	status model.InstallmentStatus,
	pending bool,
) v1.InstallmentFact {
	return v1.InstallmentFact{
		InstallmentID:        model.InstallmentID(uuid.New()),
		DueAt:                dueDay.UTC(),
		GracePeriodDays:      grace,
		Status:               status,
		HasUnresolvedPayment: pending,
	}
}

func TestEvaluateCustomer_PreDue(t *testing.T) {
	loc := tehran()
	day := businessDay(loc, 2026, time.March, 10)
	due := businessDay(loc, 2026, time.March, 11)

	decision := v1.New().EvaluateCustomer(
		atHour(day, 10),
		day,
		[]v1.InstallmentFact{fact(due, 5, model.ScheduledInstallmentStatus, false)},
		false,
	)

	require.True(t, decision.ShouldSend)
	require.Equal(t, model.PreDueReminderType, decision.ReminderType)
	require.Equal(t, v1.TemplatePreDue, decision.TemplateID)
}

func TestEvaluateCustomer_DueToday(t *testing.T) {
	loc := tehran()
	day := businessDay(loc, 2026, time.March, 10)

	decision := v1.New().EvaluateCustomer(
		atHour(day, 10),
		day,
		[]v1.InstallmentFact{fact(day, 3, model.DueInstallmentStatus, false)},
		false,
	)

	require.True(t, decision.ShouldSend)
	require.Equal(t, model.DueTodayReminderType, decision.ReminderType)
}

func TestEvaluateCustomer_GraceAndFinalDay(t *testing.T) {
	loc := tehran()
	due := atHour(businessDay(loc, 2026, time.March, 10), 18)

	t.Run("mid grace", func(t *testing.T) {
		day := businessDay(loc, 2026, time.March, 12) // D+2 of grace=3
		decision := v1.New().EvaluateCustomer(
			atHour(day, 13),
			day,
			[]v1.InstallmentFact{fact(due, 3, model.DueInstallmentStatus, false)},
			false,
		)
		require.True(t, decision.ShouldSend)
		require.Equal(t, model.GracePeriodReminderType, decision.ReminderType)
	})

	t.Run("final grace day uses grace days not hardcoded 5", func(t *testing.T) {
		day := businessDay(loc, 2026, time.March, 13) // D+3
		decision := v1.New().EvaluateCustomer(
			atHour(day, 13),
			day,
			[]v1.InstallmentFact{fact(due, 3, model.DueInstallmentStatus, false)},
			false,
		)
		require.True(t, decision.ShouldSend)
		require.Equal(t, model.GraceFinalDayReminderType, decision.ReminderType)
		require.Equal(t, v1.TemplateGraceFinalDay, decision.TemplateID)
	})

	t.Run("no reminder after contractual grace", func(t *testing.T) {
		day := businessDay(loc, 2026, time.March, 14) // D+4 with grace=3
		decision := v1.New().EvaluateCustomer(
			atHour(day, 13),
			day,
			[]v1.InstallmentFact{fact(due, 3, model.OverdueInstallmentStatus, false)},
			false,
		)
		require.False(t, decision.ShouldSend)
		require.Equal(t, v1.SuppressNoEligible, decision.SuppressReason)
	})
}

func TestEvaluateCustomer_ZeroGraceHasNoGraceReminders(t *testing.T) {
	loc := tehran()
	due := atHour(businessDay(loc, 2026, time.March, 10), 18)
	day := businessDay(loc, 2026, time.March, 11)

	decision := v1.New().EvaluateCustomer(
		atHour(day, 13),
		day,
		[]v1.InstallmentFact{fact(due, 0, model.DueInstallmentStatus, false)},
		false,
	)

	require.False(t, decision.ShouldSend)
}

func TestEvaluateCustomer_BeforeWindowAndAlreadySent(t *testing.T) {
	loc := tehran()
	day := businessDay(loc, 2026, time.March, 10)

	before := v1.New().EvaluateCustomer(
		atHour(day, 9),
		day,
		[]v1.InstallmentFact{fact(day, 5, model.DueInstallmentStatus, false)},
		false,
	)
	require.False(t, before.ShouldSend)
	require.Equal(t, v1.SuppressBeforeWindow, before.SuppressReason)

	already := v1.New().EvaluateCustomer(
		atHour(day, 10),
		day,
		[]v1.InstallmentFact{fact(day, 5, model.DueInstallmentStatus, false)},
		true,
	)
	require.False(t, already.ShouldSend)
	require.Equal(t, v1.SuppressAlreadySentToday, already.SuppressReason)
}

func TestEvaluateCustomer_PriorityAndPaidPending(t *testing.T) {
	loc := tehran()
	day := businessDay(loc, 2026, time.March, 12)
	dueToday := businessDay(loc, 2026, time.March, 12)
	dueGraceFinal := businessDay(loc, 2026, time.March, 9) // D+3 with grace=3

	preDueID := model.InstallmentID(uuid.New())
	finalID := model.InstallmentID(uuid.New())

	decision := v1.New().EvaluateCustomer(
		atHour(day, 13),
		day,
		[]v1.InstallmentFact{
			{
				InstallmentID:   preDueID,
				DueAt:           atHour(dueToday, 0).UTC(),
				GracePeriodDays: 5,
				Status:          model.DueInstallmentStatus,
			},
			{
				InstallmentID:   finalID,
				DueAt:           atHour(dueGraceFinal, 18).UTC(),
				GracePeriodDays: 3,
				Status:          model.DueInstallmentStatus,
			},
			fact(dueToday, 5, model.PaidInstallmentStatus, false),
			fact(dueToday, 5, model.DueInstallmentStatus, true),
		},
		false,
	)

	require.True(t, decision.ShouldSend)
	require.Equal(t, model.GraceFinalDayReminderType, decision.ReminderType)
	require.Equal(t, []model.InstallmentID{finalID}, decision.InstallmentIDs)
}

func TestEvaluateCustomer_TimezoneBoundary(t *testing.T) {
	loc := tehran()
	// 2026-03-11 06:30 UTC is 2026-03-11 10:00+0330 — business day March 11 at due-today hour.
	nowUTC := time.Date(2026, time.March, 11, 6, 30, 0, 0, time.UTC)
	nowLocal := nowUTC.In(loc)
	day := businessDay(loc, nowLocal.Year(), nowLocal.Month(), nowLocal.Day())
	due := day // due today in Tehran

	decision := v1.New().EvaluateCustomer(
		nowLocal,
		day,
		[]v1.InstallmentFact{fact(due, 5, model.DueInstallmentStatus, false)},
		false,
	)

	require.True(t, decision.ShouldSend)
	require.Equal(t, model.DueTodayReminderType, decision.ReminderType)
}

func TestCalendarDSTAndGraceExpiry(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)
	// Spring-forward is 23 elapsed hours, but one calendar day.
	due := time.Date(2026, 3, 8, 18, 0, 0, 0, loc)
	day := time.Date(2026, 3, 9, 0, 0, 0, 0, loc)
	d := v1.New().EvaluateCustomer(atHour(day, 13), day, []v1.InstallmentFact{fact(due, 3, model.ScheduledInstallmentStatus, false)}, false)
	require.True(t, d.ShouldSend)
	require.Equal(t, model.GracePeriodReminderType, d.ReminderType)
	// A delayed job after the contractual instant must not send a grace warning.
	expiry := (model.Installment{DueAt: due, GracePeriodDays: 3}).OverdueEligibleAt(loc)
	for _, now := range []time.Time{expiry, expiry.Add(time.Hour)} {
		business := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
		d = v1.New().EvaluateCustomer(now, business, []v1.InstallmentFact{fact(due, 3, model.DueInstallmentStatus, false)}, false)
		require.False(t, d.ShouldSend)
	}
	// A calendar delivery window ends at the next local midnight, including DST.
	oldDay := time.Date(2026, 3, 8, 0, 0, 0, 0, loc)
	nextDay := oldDay.AddDate(0, 0, 1)
	d = v1.New().EvaluateCustomer(nextDay, oldDay, []v1.InstallmentFact{fact(oldDay, 3, model.DueInstallmentStatus, false)}, false)
	require.False(t, d.ShouldSend)
	require.Equal(t, v1.SuppressStaleWindow, d.SuppressReason)
}
