// Package memory provides an in-memory implementation of the core cache
// contract.
//
// This adapter is useful for tests, local development, and single-process
// scenarios. It is not a distributed cache and must not be used for
// cross-instance coordination such as production idempotency keys, rate limits,
// or distributed locks.
package memory
