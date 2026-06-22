// Package outboxrp implements the outbox repository with PostgreSQL.
//
// The repository stores reliable integration messages in the database and
// exposes transaction-bound and connection-bound query helpers. Enqueue is
// intentionally transaction-scoped so producers can insert outbox messages
// atomically with their business changes.
package outboxrp
