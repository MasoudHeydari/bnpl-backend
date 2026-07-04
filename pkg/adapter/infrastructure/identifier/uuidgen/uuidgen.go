// Package uuidgen provides a UUID-backed implementation of the core identifier
// Generator port for domain entities that require UUID-formatted primary keys.
//
// Trace correlation identifiers continue to use the separate ULID Trace
// provider. Do not wire this adapter into Trace or other opaque-string
// consumers that intentionally accept ULID values.
package uuidgen
