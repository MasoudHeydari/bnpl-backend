// Package outboxpublisheruc publishes committed outbox messages from the
// integration outbox table to the configured message broker.
//
// The use case implements the Transactional Outbox pattern. Multiple workers
// may execute concurrently; correctness relies on the repository using row-
// level locking and SKIP LOCKED semantics when claiming pending messages.
package outboxpublisheruc
