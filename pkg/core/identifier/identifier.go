// Package identifier defines the core port for generating opaque unique IDs.
//
// Implementations may use ULID, UUID, or another strategy. Callers must treat
// generated values as opaque strings and must not rely on their format.
package identifier
