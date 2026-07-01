// Package noop provides a no-op implementation of the core cache contract.
//
// This adapter never stores data and never communicates with external systems.
// It is useful when caching is disabled, during local development, or in tests
// where cache behavior is not relevant.
//
// Important: noop does not provide real cache guarantees. It must not be used
// for production idempotency, distributed locks, rate limiting, or any workflow
// that depends on persisted cache state.
package noop
