// Package gobreaker adapts github.com/sony/gobreaker to the project's core
// circuit breaker abstraction.
//
// The adapter keeps circuit breaker state per operation name, so different
// dependencies or operations do not affect each other's circuit state.
package gobreaker
