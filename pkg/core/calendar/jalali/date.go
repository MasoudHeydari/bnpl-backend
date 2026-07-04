package jalali

import "time"

// Date is a civil Jalali calendar date.
type Date struct {
	// Year is the Jalali calendar year.
	Year int
	// Month is the Jalali month in the range 1..12.
	Month int
	// Day is the Jalali day of month (1..31 depending on month/year).
	Day int
}

// FromTime converts t's calendar date in t's location to Jalali.
func FromTime(t time.Time) Date {
	y, m, d := t.Date()
	jy, jm, jd := ToJalali(y, int(m), d)
	return Date{Year: jy, Month: jm, Day: jd}
}

// ToTime converts the Jalali date to a Gregorian instant, preserving the
// wall-clock fields from wall in loc.
func (d Date) ToTime(wall time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}

	gy, gm, gd := ToGregorian(d.Year, d.Month, d.Day)
	return time.Date(
		gy, time.Month(gm), gd,
		wall.Hour(), wall.Minute(), wall.Second(), wall.Nanosecond(),
		loc,
	)
}

// DaysInMonth returns the number of days in the given Jalali month.
func DaysInMonth(year, month int) int {
	switch {
	case month >= 1 && month <= 6:
		return 31

	case month >= 7 && month <= 11:
		return 30

	case month == 12:
		// Esfand has 30 days in leap years and 29 otherwise. Detect leapness by
		// round-tripping day 30 through Gregorian conversion.
		gy, gm, gd := ToGregorian(year, 12, 30)
		jy, jm, jd := ToJalali(gy, gm, gd)
		if jy == year && jm == 12 && jd == 30 {
			return 30
		}

		return 29

	default:
		return 0
	}
}

// AddMonthsClamped shifts t by months on the Jalali calendar while keeping the
// same Jalali day-of-month when possible. If the target month is shorter (for
// example day 31 into a 30-day month, or Esfand in a non-leap year), the day is
// clamped to the last day of that month. The wall-clock time and location of t
// are preserved.
//
// Each call recomputes from the original anchor date; callers should not chain
// month-by-month from a previously clamped result when they need fixed-day
// tenor across a schedule.
func AddMonthsClamped(t time.Time, months int) time.Time {
	loc := t.Location()
	local := t.In(loc)
	anchor := FromTime(local)

	total := anchor.Year*12 + (anchor.Month - 1) + months
	year := total / 12
	month := total%12 + 1
	if month <= 0 {
		// Defensive for large negative month shifts.
		month += 12
		year--
	}

	day := anchor.Day
	last := DaysInMonth(year, month)
	if day > last {
		day = last
	}

	return Date{Year: year, Month: month, Day: day}.ToTime(local, loc)
}

// ToJalali converts a Gregorian civil date to Jalali.
//
// Algorithm adapted from the widely used jalaali calendar conversion
// (same family as jalaali/go-jalaali).
func ToJalali(gy, gm, gd int) (jy, jm, jd int) {
	gdm := []int{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334}
	gy2 := gy - 1600
	gm2 := gm - 1
	gd2 := gd - 1

	gDayNo := 365*gy2 + (gy2+3)/4 - (gy2+99)/100 + (gy2+399)/400
	gDayNo += gdm[gm2] + gd2
	if gm2 > 1 && ((gy%4 == 0 && gy%100 != 0) || (gy%400 == 0)) {
		gDayNo++
	}

	jDayNo := gDayNo - 79
	jNp := jDayNo / 12053
	jDayNo %= 12053
	jy = 979 + 33*jNp + 4*(jDayNo/1461)
	jDayNo %= 1461

	if jDayNo >= 366 {
		jy += (jDayNo - 1) / 365
		jDayNo = (jDayNo - 1) % 365
	}

	var i int
	for i = 0; i < 11 && jDayNo >= daysBeforeMonth(i); i++ {
		jDayNo -= daysBeforeMonth(i)
	}

	jm = i + 1
	jd = jDayNo + 1
	return jy, jm, jd
}

// ToGregorian converts a Jalali civil date to Gregorian.
func ToGregorian(jy, jm, jd int) (gy, gm, gd int) {
	jy2 := jy - 979
	jm2 := jm - 1
	jd2 := jd - 1

	jDayNo := 365*jy2 + (jy2/33)*8 + (jy2%33+3)/4
	for i := 0; i < jm2; i++ {
		jDayNo += daysBeforeMonth(i)
	}

	jDayNo += jd2

	gDayNo := jDayNo + 79

	gy = 1600 + 400*(gDayNo/146097)
	gDayNo %= 146097

	leap := true
	if gDayNo >= 36525 {
		gDayNo--
		gy += 100 * (gDayNo / 36524)
		gDayNo %= 36524
		if gDayNo >= 365 {
			gDayNo++
		} else {
			leap = false
		}
	}

	gy += 4 * (gDayNo / 1461)
	gDayNo %= 1461

	if gDayNo >= 366 {
		leap = false
		gDayNo--
		gy += gDayNo / 365
		gDayNo %= 365
	}

	gdm := []int{31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	if leap {
		gdm[1] = 29
	}

	var i int
	for i = 0; gDayNo >= gdm[i]; i++ {
		gDayNo -= gdm[i]
	}

	gm = i + 1
	gd = gDayNo + 1
	return gy, gm, gd
}

// daysBeforeMonth returns the length of Jalali month i (0-based) for day-number
// accumulation. Esfand is treated as 30 here, matching the classic conversion
// algorithm; DaysInMonth uses round-trip detection for the true Esfand length.
func daysBeforeMonth(monthIndex int) int {
	if monthIndex < 6 {
		return 31
	}

	return 30
}
