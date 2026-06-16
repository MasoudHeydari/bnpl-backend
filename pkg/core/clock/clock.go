// Package clock defines the core abstraction for time-related operations.
//
// Time is a business dependency. Domain and application code must depend on
// Clock instead of calling time.Now directly to improve testability and ensure
// business rules use the configured business time zone.
package clock
