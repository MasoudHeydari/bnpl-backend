// Package redis provides a Redis-backed implementation of the core cache
// contract.
//
// This adapter is suitable for shared cache state across multiple application
// instances, including idempotency keys, rate limiting, short-lived locks,
// sessions, OTP state, and other TTL-based workflows.
package redis
