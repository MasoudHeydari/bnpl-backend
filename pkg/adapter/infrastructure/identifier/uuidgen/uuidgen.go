// Package uuidgen provides a UUID-backed implementation of the core identifier
// Generator port for domain entities that require UUID-formatted primary keys.
//
// The tracing provider delegates correlation ID generation to this adapter
// because X-Correlation-Id requires a UUID. Trace and request IDs remain ULIDs
// and must not be switched to UUID generation.
package uuidgen
