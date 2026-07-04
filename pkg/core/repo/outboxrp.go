package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CreateOutboxMessage carries fields required to persist a new outbox row.
type CreateOutboxMessage struct {
	// Destination is the messaging topic or channel to publish to.
	Destination string

	// MessageKey is the optional routing key for ordered delivery.
	MessageKey string

	// EventType identifies the integration event type.
	EventType string

	// AggregateType optionally names the aggregate that produced the event.
	AggregateType *string

	// Payload is the serialized event body.
	Payload json.RawMessage

	// CreatedAt is the time the outbox row should record as created.
	CreatedAt time.Time
}

// OutboxQueryer defines publisher-side outbox operations.
//
// These operations are used by outbox publisher workers after the original
// business transaction has already committed.
type OutboxQueryer interface {
	// ClaimPending claims pending or stale processing messages for a publisher worker.
	ClaimPending(
		ctx context.Context,
		workerID string,
		limit int,
		now time.Time,
		processingLeaseTimeout time.Duration,
	) ([]model.OutboxMessage, error)

	// MarkPublished marks a claimed message as successfully delivered.
	MarkPublished(
		ctx context.Context,
		outboxID model.OutboxID,
		workerID string,
		lockVersion int64,
		publishedAt time.Time,
	) error

	// MarkSkipped marks a claimed message as intentionally not delivered.
	// Used when a configured provider (for example webhook noop) skips I/O.
	MarkSkipped(
		ctx context.Context,
		outboxID model.OutboxID,
		workerID string,
		lockVersion int64,
		skippedAt time.Time,
		reason string,
	) error

	// MarkRetry schedules a failed delivery for retry.
	MarkRetry(
		ctx context.Context,
		outboxID model.OutboxID,
		workerID string,
		lockVersion int64,
		nextAttemptAt time.Time,
		lastError string,
	) error

	// MarkFailed marks a message as permanently failed.
	MarkFailed(
		ctx context.Context,
		outboxID model.OutboxID,
		workerID string,
		lockVersion int64,
		lastError string,
	) error
}

// OutboxConnQueryer exposes publisher-side outbox operations on a connection.
type OutboxConnQueryer interface {
	// OutboxQueryer provides shared publisher-side outbox operations.
	OutboxQueryer
}

// OutboxTxQueryer exposes transaction-scoped outbox operations.
//
// Create is transaction-only by design. Outbox messages should be inserted in
// the same transaction as the business change that produced them.
type OutboxTxQueryer interface {
	// Create stores a new outbox message and returns the persisted row.
	Create(ctx context.Context, message CreateOutboxMessage) (model.OutboxMessage, error)
}

// Outbox defines repository bindings for reliable integration messages.
type Outbox interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(conn Conn) OutboxConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(tx Tx) OutboxTxQueryer
}
