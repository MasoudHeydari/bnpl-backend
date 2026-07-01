package clock

import "time"

// Clock is the source of truth for all business time calculations.
//
// Implementations are responsible for providing the current time and applying
// calendar operations in the configured business location. Use cases and domain
// logic must depend on this interface rather than the standard library clock.
type Clock interface {
	// Now returns the current instant in the configured business location.
	Now() time.Time

	// Today returns the beginning of the current business day.
	Today() time.Time

	// BusinessLocation returns the location used for business calendar rules.
	BusinessLocation() *time.Location

	// Add returns t shifted by d while preserving location information.
	Add(t time.Time, d time.Duration) time.Time

	// AddDate returns t shifted by the specified calendar interval.
	AddDate(t time.Time, years, months, days int) time.Time

	// StartOfDay returns midnight of t's calendar date.
	StartOfDay(t time.Time) time.Time

	// EndOfDay returns the final instant of t's calendar date.
	EndOfDay(t time.Time) time.Time

	// IsToday reports whether t falls on the current business date.
	IsToday(t time.Time) bool

	// DaysBetween returns the number of calendar-day boundaries between a and b.
	DaysBetween(a, b time.Time) int

	// IsAfter reports whether a occurs after b.
	IsAfter(a, b time.Time) bool

	// IsBefore reports whether a occurs before b.
	IsBefore(a, b time.Time) bool
}
