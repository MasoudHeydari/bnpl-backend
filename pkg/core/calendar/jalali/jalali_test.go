package jalali

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestToJalaliKnownDates(t *testing.T) {
	t.Parallel()

	jy, jm, jd := ToJalali(2026, 8, 8)
	require.Equal(t, 1405, jy)
	require.Equal(t, 5, jm)
	require.Equal(t, 17, jd)
}

func TestAddMonthsClamped_PreservesJalaliDayAcrossHalfYearBoundary(t *testing.T) {
	t.Parallel()

	// 2026-08-08 is 1405/05/17. Gregorian month addition drifts to 1405/07/16,
	// but Jalali month addition must keep day 17 through Mehr.
	anchor := time.Date(2026, 8, 8, 13, 43, 37, 98452000, time.UTC)

	require.Equal(t, Date{Year: 1405, Month: 5, Day: 17}, FromTime(anchor))
	require.Equal(t, Date{Year: 1405, Month: 6, Day: 17}, FromTime(AddMonthsClamped(anchor, 1)))
	require.Equal(t, Date{Year: 1405, Month: 7, Day: 17}, FromTime(AddMonthsClamped(anchor, 2)))
	require.Equal(t, Date{Year: 1405, Month: 8, Day: 17}, FromTime(AddMonthsClamped(anchor, 3)))
}

func TestAddMonthsClamped_ClampsDay31IntoShorterMonth(t *testing.T) {
	t.Parallel()

	// 1404/01/31 (Farvardin 31) + 6 months -> 1404/07/30 (Mehr has 30 days).
	farvardin31 := Date{Year: 1404, Month: 1, Day: 31}.ToTime(
		time.Date(1, 1, 1, 12, 0, 0, 0, time.UTC),
		time.UTC,
	)
	require.Equal(t, Date{Year: 1404, Month: 1, Day: 31}, FromTime(farvardin31))

	mehr := AddMonthsClamped(farvardin31, 6)
	require.Equal(t, Date{Year: 1404, Month: 7, Day: 30}, FromTime(mehr))
}

func TestAddMonthsClamped_RecomputesFromOriginalAnchor(t *testing.T) {
	t.Parallel()

	farvardin31 := Date{Year: 1404, Month: 1, Day: 31}.ToTime(
		time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
		time.UTC,
	)

	// +7 months from Farvardin 31 -> Aban 30, not chained from clamped Mehr 30.
	aban := AddMonthsClamped(farvardin31, 7)
	require.Equal(t, Date{Year: 1404, Month: 8, Day: 30}, FromTime(aban))
}

func TestRoundTripSelectedDates(t *testing.T) {
	t.Parallel()

	cases := []struct{ gy, gm, gd int }{
		{2026, 8, 8},
		{2026, 3, 10},
		{2024, 3, 20},
		{2025, 3, 21},
	}
	for _, tc := range cases {
		jy, jm, jd := ToJalali(tc.gy, tc.gm, tc.gd)
		gy, gm, gd := ToGregorian(jy, jm, jd)
		require.Equal(t, tc.gy, gy)
		require.Equal(t, tc.gm, gm)
		require.Equal(t, tc.gd, gd)
	}
}
