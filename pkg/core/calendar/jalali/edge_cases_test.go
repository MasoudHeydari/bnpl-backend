package jalali

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDaysInMonth_KnownLengths(t *testing.T) {
	t.Parallel()

	// Farvardin..Shahrivar = 31, Mehr..Bahman = 30.
	for month := 1; month <= 6; month++ {
		require.Equal(t, 31, DaysInMonth(1402, month))
	}

	for month := 7; month <= 11; month++ {
		require.Equal(t, 30, DaysInMonth(1402, month))
	}

	// Independent Gregorian fixtures:
	// 1402 started 2023-03-21; 1403 started 2024-03-20 → 365 days → Esfand 29.
	require.Equal(t, 29, DaysInMonth(1402, 12))
	require.Equal(t, Date{Year: 1402, Month: 12, Day: 29}, FromTime(time.Date(2024, 3, 19, 12, 0, 0, 0, time.UTC)))

	// 1403 started 2024-03-20; 1404 started 2025-03-21 → 366 days → Esfand 30.
	require.Equal(t, 30, DaysInMonth(1403, 12))
	require.Equal(t, Date{Year: 1403, Month: 12, Day: 30}, FromTime(time.Date(2025, 3, 20, 12, 0, 0, 0, time.UTC)))

	// 1399 was also a Jalali leap year. 1399/12/30 == 2021-03-20.
	require.Equal(t, 30, DaysInMonth(1399, 12))
	require.Equal(t, Date{Year: 1399, Month: 12, Day: 30}, FromTime(time.Date(2021, 3, 20, 12, 0, 0, 0, time.UTC)))
}

func TestAddMonthsClamped_EsfandLeapAndNonLeap(t *testing.T) {
	t.Parallel()

	wall := time.Date(1, 1, 1, 15, 30, 0, 0, time.UTC)

	// 31 Farvardin into non-leap Esfand clamps to 29.
	farvardin31 := Date{Year: 1402, Month: 1, Day: 31}.ToTime(wall, time.UTC)
	esfandNonLeap := AddMonthsClamped(farvardin31, 11)
	require.Equal(t, Date{Year: 1402, Month: 12, Day: 29}, FromTime(esfandNonLeap))

	// 31 Farvardin into leap Esfand clamps to 30.
	farvardin31Leap := Date{Year: 1403, Month: 1, Day: 31}.ToTime(wall, time.UTC)
	esfandLeap := AddMonthsClamped(farvardin31Leap, 11)
	require.Equal(t, Date{Year: 1403, Month: 12, Day: 30}, FromTime(esfandLeap))

	// 30 Bahman into non-leap Esfand clamps to 29.
	bahman30 := Date{Year: 1402, Month: 11, Day: 30}.ToTime(wall, time.UTC)
	require.Equal(t, Date{Year: 1402, Month: 12, Day: 29}, FromTime(AddMonthsClamped(bahman30, 1)))
}

func TestAddMonthsClamped_YearBoundaryEsfandToFarvardin(t *testing.T) {
	t.Parallel()

	wall := time.Date(1, 1, 1, 9, 0, 0, 0, time.UTC)

	esfandNonLeap := Date{Year: 1402, Month: 12, Day: 15}.ToTime(wall, time.UTC)
	require.Equal(t, Date{Year: 1403, Month: 1, Day: 15}, FromTime(AddMonthsClamped(esfandNonLeap, 1)))

	esfandLeap := Date{Year: 1403, Month: 12, Day: 30}.ToTime(wall, time.UTC)
	require.Equal(t, Date{Year: 1404, Month: 1, Day: 30}, FromTime(AddMonthsClamped(esfandLeap, 1)))
}

func TestAddMonthsClamped_TwoMonthCadenceFromDay31(t *testing.T) {
	t.Parallel()

	wall := time.Date(1, 1, 1, 12, 0, 0, 0, time.UTC)
	anchor := Date{Year: 1404, Month: 1, Day: 31}.ToTime(wall, time.UTC)

	// Recompute each tenor from the original Farvardin 31 anchor.
	require.Equal(t, Date{Year: 1404, Month: 3, Day: 31}, FromTime(AddMonthsClamped(anchor, 2)))
	require.Equal(t, Date{Year: 1404, Month: 5, Day: 31}, FromTime(AddMonthsClamped(anchor, 4)))
	require.Equal(t, Date{Year: 1404, Month: 7, Day: 30}, FromTime(AddMonthsClamped(anchor, 6)))
	require.Equal(t, Date{Year: 1404, Month: 9, Day: 30}, FromTime(AddMonthsClamped(anchor, 8)))
}

func TestBusinessTimezoneNearMidnight(t *testing.T) {
	t.Parallel()

	tehran, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	// 2026-03-20 21:30 UTC is already 2026-03-21 01:00 in Tehran (UTC+3:30).
	utcEvening := time.Date(2026, 3, 20, 21, 30, 0, 0, time.UTC)
	tehranLocal := utcEvening.In(tehran)
	require.Equal(t, 21, tehranLocal.Day())
	require.Equal(t, Date{Year: 1405, Month: 1, Day: 1}, FromTime(tehranLocal))

	// Computing Jalali from the UTC civil date would yield Esfand 29 of 1404.
	utcCivil := FromTime(utcEvening)
	require.Equal(t, Date{Year: 1404, Month: 12, Day: 29}, utcCivil)
	require.NotEqual(t, utcCivil, FromTime(tehranLocal))
}

func TestAddCalendarSemantics_BerlinDST(t *testing.T) {
	t.Parallel()

	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)

	// Spring-forward 2026-03-29: 02:00 → 03:00 in Berlin.
	beforeSpring := time.Date(2026, 3, 28, 12, 30, 0, 0, berlin)
	afterSpring := beforeSpring.AddDate(0, 0, 1)
	require.Equal(t, 29, afterSpring.Day())
	require.Equal(t, 12, afterSpring.Hour())
	require.Equal(t, 30, afterSpring.Minute())

	// Fall-back 2026-10-25: 03:00 → 02:00 in Berlin.
	beforeFall := time.Date(2026, 10, 24, 12, 30, 0, 0, berlin)
	afterFall := beforeFall.AddDate(0, 0, 1)
	require.Equal(t, 25, afterFall.Day())
	require.Equal(t, 12, afterFall.Hour())
	require.Equal(t, 30, afterFall.Minute())

	gapHours := afterSpring.Sub(beforeSpring).Hours()
	require.Equal(t, 23.0, gapHours)
}
