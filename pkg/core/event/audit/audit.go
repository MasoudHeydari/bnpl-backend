// Package audit defines the versioned contract for audit events emitted by BNPL.
//
// The package contains only transport-independent event data, constants, constructors,
// and validation helpers. Persistence in the transactional outbox and publication to
// Kafka are responsibilities of the application and adapter layers.
package audit
