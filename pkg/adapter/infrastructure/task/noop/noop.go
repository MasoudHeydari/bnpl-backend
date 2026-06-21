// Package noop provides a no-op task runtime implementation.
//
// The noop runtime satisfies the core task.Runtime contract while intentionally
// ignoring scheduled task registrations and dispatched commands. It is useful
// for tests, local development, and environments where background work must be
// disabled.
package noop
