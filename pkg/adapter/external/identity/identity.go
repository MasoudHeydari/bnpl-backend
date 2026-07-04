// Package identity selects Identity Service client implementations.
//
// Concrete clients live in subpackages such as grpc and noop. This package
// defines provider identifiers used by configuration. Identity owns only
// subject resolution; durable notification submission is a separate service.
package identity
