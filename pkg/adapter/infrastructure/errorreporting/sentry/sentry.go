// Package sentry adapts the official Sentry Go SDK to the core error reporter.
//
// It captures exceptions and messages with project privacy scrubbing applied
// before events leave the process.
package sentry
