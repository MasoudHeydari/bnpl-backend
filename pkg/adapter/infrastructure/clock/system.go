package clock

import (
	"time"

	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
)

// Verify System implements the core Clock contract at compile time.
var _ coreclock.Clock = (*System)(nil)

// System is the production Clock implementation backed by the real system
// clock.
//
// The system location controls the location of Now. The business location
// controls calendar operations such as Today, StartOfDay, EndOfDay, AddDate,
// IsToday, and DaysBetween.
//
// In production, the system location is usually UTC while the business location
// follows the market/legal calendar, for example Asia/Tehran.
type System struct {
	location         *time.Location
	businessLocation *time.Location
}

// NewSystem constructs a production clock.
//
// When location is nil, UTC is used. When businessLocation is nil, it falls
// back to location.
func NewSystem(location, businessLocation *time.Location) *System {
	if location == nil {
		location = time.UTC
	}

	if businessLocation == nil {
		businessLocation = location
	}

	return &System{
		location:         location,
		businessLocation: businessLocation,
	}
}

// BusinessLocation returns the location used for business calendar rules.
func (s *System) BusinessLocation() *time.Location {
	return s.businessLocation
}

// Now returns the current instant in the configured system location.
func (s *System) Now() time.Time {
	return time.Now().In(s.location)
}

// Today returns the start of the current business date.
func (s *System) Today() time.Time {
	return s.StartOfDay(s.Now())
}

// Add returns t shifted by d in the business location.
func (s *System) Add(t time.Time, d time.Duration) time.Time {
	return t.In(s.businessLocation).Add(d)
}

// AddDate returns t shifted by the given calendar interval in the business
// location.
func (s *System) AddDate(t time.Time, years, months, days int) time.Time {
	return t.In(s.businessLocation).AddDate(years, months, days)
}

// StartOfDay returns midnight of t's calendar date in the business location.
func (s *System) StartOfDay(t time.Time) time.Time {
	t = t.In(s.businessLocation)

	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, s.businessLocation)
}

// EndOfDay returns the final instant of t's calendar date in the business
// location.
func (s *System) EndOfDay(t time.Time) time.Time {
	return s.StartOfDay(t).AddDate(0, 0, 1).Add(-time.Nanosecond)
}

// IsToday reports whether t falls on the current business date.
func (s *System) IsToday(t time.Time) bool {
	return s.StartOfDay(t).Equal(s.Today())
}

// DaysBetween returns the number of calendar-day boundaries between a and b.
//
// The result is positive when b is after a, negative when b is before a, and
// zero when both instants fall on the same business date.
func (s *System) DaysBetween(a, b time.Time) int {
	startA := s.StartOfDay(a)
	startB := s.StartOfDay(b)

	return int(startB.Sub(startA).Hours() / 24)
}

// IsAfter reports whether a occurs after b.
func (s *System) IsAfter(a, b time.Time) bool {
	return a.After(b)
}

// IsBefore reports whether a occurs before b.
func (s *System) IsBefore(a, b time.Time) bool {
	return a.Before(b)
}
