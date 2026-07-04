// Package notification selects Notification Service client implementations.
//
// Concrete clients live in subpackages such as grpc and noop. This package
// defines provider identifiers used by configuration. Notification owns only
// durable submission acceptance; OTP generation and verification remain in BNPL.
package notification
