// Package trace provides infrastructure implementations for core tracing
// identifier contracts.
//
// The default provider mints ULID-based trace/request identifiers and
// UUID-based correlation identifiers. The core layer treats generated values
// as opaque strings.
package trace
