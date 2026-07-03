package repo

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// OutboxQueryer defines publisher-side outbox operations.
//
// These operations are used by outbox publisher workers after the original
// business transaction has already committed.
type OutboxQueryer interface {
	// ClaimPending claims pending messages for a publisher worker.
	//
	// Implementations should select due messages, mark them as processing,
	// set lock metadata, and return the claimed rows.
	ClaimPending(
		ctx context.Context,
		workerID string,
		limit int,
		now time.Time,
	) ([]model.OutboxMessage, error)

	// MarkPublished marks a claimed message as successfully delivered.
	MarkPublished(
		ctx context.Context,
		outboxID uuid.UUID,
		publishedAt time.Time,
	) error

	// MarkRetry schedules a failed delivery for a future retry.
	MarkRetry(
		ctx context.Context,
		outboxID uuid.UUID,
		nextAttemptAt time.Time,
		lastError string,
	) error

	// MarkFailed marks a message as permanently failed.
	MarkFailed(
		ctx context.Context,
		outboxID uuid.UUID,
		lastError string,
	) error
}

// OutboxConnQueryer exposes publisher-side outbox operations on a connection.
type OutboxConnQueryer interface {
	OutboxQueryer
}

// OutboxTxQueryer exposes transaction-scoped outbox operations.
//
// Create is transaction-only by design. Outbox messages should be inserted in
// the same transaction as the business change that produced them.
type OutboxTxQueryer interface {
	// Create stores a new outbox message and returns the persisted row.
	Create(
		ctx context.Context,
		destination string,
		eventType string,
		aggregateType *string,
		payload json.RawMessage,
		status model.OutboxStatus,
		attemptCount int,
		maxAttempts int,
		nextAttemptAt time.Time,
		lockedAt *time.Time,
		lockedBy *string,
		lastError *string,
		createdAt time.Time,
		publishedAt *time.Time,
	) (model.OutboxMessage, error)
}

// Outbox defines repository bindings for reliable integration messages.
type Outbox interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(conn Conn) OutboxConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(tx Tx) OutboxTxQueryer
}
