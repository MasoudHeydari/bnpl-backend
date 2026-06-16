// Package trace defines core contracts and context helpers for request-scoped
// tracing identifiers.
//
// The core layer does not generate concrete ID formats such as ULID or UUID.
// Concrete generators must live in adapter/infrastructure packages.
package trace
