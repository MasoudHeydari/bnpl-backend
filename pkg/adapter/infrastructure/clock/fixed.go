package clock

import (
	"time"

	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
)

// Verify Fixed implements the core Clock contract at compile time.
var _ coreclock.Clock = Fixed{}

// Fixed is a deterministic Clock implementation for tests.
//
// Every call to Now returns the same instant, converted to the configured
// business location. This makes timestamp, due-date, overdue, and grace-period
// assertions stable and reproducible.
type Fixed struct {
	Time     time.Time
	Location *time.Location
}

// NewFixed constructs a Fixed clock for tests.
//
// When location is nil, UTC is used.
func NewFixed(t time.Time, location *time.Location) Fixed {
	if location == nil {
		location = time.UTC
	}

	return Fixed{
		Time:     t,
		Location: location,
	}
}

// BusinessLocation returns the location used for business calendar rules.
func (f Fixed) BusinessLocation() *time.Location {
	if f.Location != nil {
		return f.Location
	}

	return time.UTC
}

// Now returns the fixed instant in the configured business location.
func (f Fixed) Now() time.Time {
	return f.Time.In(f.BusinessLocation())
}

// Today returns the start of the current fixed business date.
func (f Fixed) Today() time.Time {
	return f.StartOfDay(f.Now())
}

// Add returns t shifted by d.
//
// The input time is first converted to the business location so tests observe
// the same location behavior as production clock implementations.
func (f Fixed) Add(t time.Time, d time.Duration) time.Time {
	return t.In(f.BusinessLocation()).Add(d)
}

// AddDate returns t shifted by the given calendar interval in the business
// location.
func (f Fixed) AddDate(t time.Time, years, months, days int) time.Time {
	return t.In(f.BusinessLocation()).AddDate(years, months, days)
}

// StartOfDay returns midnight of t's calendar date in the business location.
func (f Fixed) StartOfDay(t time.Time) time.Time {
	t = t.In(f.BusinessLocation())

	return time.Date(
		t.Year(),
		t.Month(),
		t.Day(),
		0,
		0,
		0,
		0,
		f.BusinessLocation(),
	)
}

// EndOfDay returns the final instant of t's calendar date in the business
// location.
func (f Fixed) EndOfDay(t time.Time) time.Time {
	return f.StartOfDay(t).AddDate(0, 0, 1).Add(-time.Nanosecond)
}

// IsToday reports whether t falls on the fixed current business date.
func (f Fixed) IsToday(t time.Time) bool {
	return f.StartOfDay(t).Equal(f.Today())
}

// DaysBetween returns the number of calendar-day boundaries between a and b.
//
// The result is positive when b is after a, negative when b is before a, and
// zero when both instants fall on the same business date.
func (f Fixed) DaysBetween(a, b time.Time) int {
	startA := f.StartOfDay(a)
	startB := f.StartOfDay(b)

	return int(startB.Sub(startA).Hours() / 24)
}

// IsAfter reports whether a occurs after b.
func (f Fixed) IsAfter(a, b time.Time) bool {
	return a.After(b)
}

// IsBefore reports whether a occurs before b.
func (f Fixed) IsBefore(a, b time.Time) bool {
	return a.Before(b)
}
