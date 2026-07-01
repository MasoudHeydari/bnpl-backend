// Package header defines HTTP header names used by the Gin transport layer.
//
// These constants are transport-specific concerns and must not be referenced by
// the core domain. Other transports such as gRPC or Kafka should define their
// own metadata mappings while reusing the same core trace model.
package header
