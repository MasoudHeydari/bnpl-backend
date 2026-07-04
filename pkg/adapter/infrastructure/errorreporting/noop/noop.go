// Package noop implements an error reporter that discards all events.
//
// It satisfies the core error-reporting contract without sending data anywhere.
// Use it when error reporting is disabled or in tests that do not assert on
// reporting side effects.
package noop
