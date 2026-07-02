package outboxrp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrInvalidClaimLimit indicates an invalid publisher batch size.
	ErrInvalidClaimLimit = errors.New("invalid outbox claim limit")

	// ErrOutboxMessageNotFound indicates that the requested message was not
	// updated, usually because it does not exist or is not in the expected state.
	ErrOutboxMessageNotFound = errors.New("outbox message not found")
)

// Enqueue inserts a new outbox message.
//
// This function should be called through the transaction-bound repository
// helper so the outbox message commits atomically with the business change.
func Enqueue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	msg model.OutboxMessage,
) error {
	msg = normalizeForInsert(msg)
	const query = `
INSERT INTO integration_outbox (
	id,
	destination,
	event_type,
	aggregate_type,
	payload,
	status,
	attempt_count,
	max_attempts,
	next_attempt_at,
	locked_at,
	locked_by,
	last_error,
	created_at,
	published_at
)
VALUES (
	$1, $2, $3, $4, $5, $6, $7,
	$8, $9, $10, $11, $12, $13, $14
)
`

	_, err := q.Exec(
		ctx,
		query,
		msg.ID,
		msg.Destination,
		msg.EventType,
		msg.AggregateType,
		msg.Payload,
		msg.Status,
		msg.AttemptCount,
		msg.MaxAttempts,
		msg.NextAttemptAt,
		msg.LockedAt,
		msg.LockedBy,
		msg.LastError,
		msg.CreatedAt,
		msg.PublishedAt,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("enqueue outbox message %s: %w", msg.ID, err),
			map[string]any{
				"outbox_id": msg.ID,
			},
		)
	}

	return nil
}

// Create inserts a new outbox message.
//
// It is an alias for Enqueue used by repositories that name transaction-only
// inserts as create operations.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	msg model.OutboxMessage,
) error {
	return Enqueue(ctx, q, msg)
}

// GetByID returns an outbox message by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID uuid.UUID,
) (model.OutboxMessage, error) {
	const query = `
SELECT
	id,
	destination,
	event_type,
	aggregate_type,
	payload,
	status,
	attempt_count,
	max_attempts,
	next_attempt_at,
	locked_at,
	locked_by,
	last_error,
	created_at,
	published_at
FROM integration_outbox
WHERE id = $1
`
	rows, err := q.Query(
		ctx,
		query,
		outboxID,
	)
	if err != nil {
		return model.OutboxMessage{}, fmt.Errorf("get outbox message %s: %w", outboxID, postgres.MapError(err))
	}
	defer rows.Close()

	msg, err := postgres.ValueInto[model.OutboxMessage](rows.(postgres.Rows))
	if err != nil {
		return model.OutboxMessage{}, fmt.Errorf("get outbox message %s: %w", outboxID, err)
	}

	return msg, nil
}

// ClaimPending claims pending messages that are due for publishing.
//
// The query uses FOR UPDATE SKIP LOCKED so multiple publisher workers can poll
// concurrently without processing the same row.
func ClaimPending[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	publisherID string,
	limit int,
	now time.Time,
) ([]model.OutboxMessage, error) {
	if limit <= 0 {
		return nil, cerr.InvalidArgument(
			ErrInvalidClaimLimit,
			map[string]any{
				"limit": limit,
			},
		)
	}

	const query = `
WITH picked AS (
	SELECT id
	FROM integration_outbox
	WHERE status = $1
	  AND next_attempt_at <= $2
	ORDER BY next_attempt_at ASC, created_at ASC
	LIMIT $3
	FOR UPDATE SKIP LOCKED
)
UPDATE integration_outbox AS o
SET
	status = $4,
	locked_at = $2,
	locked_by = $5,
	attempt_count = attempt_count + 1,
	last_error = NULL
FROM picked
WHERE o.id = picked.id
RETURNING
	o.id,
	o.destination,
	o.event_type,
	o.aggregate_type,
	o.payload,
	o.status,
	o.attempt_count,
	o.max_attempts,
	o.next_attempt_at,
	o.locked_at,
	o.locked_by,
	o.last_error,
	o.created_at,
	o.published_at
`

	rows, err := q.Query(
		ctx,
		query,
		model.OutboxStatusPending,
		now,
		limit,
		model.OutboxStatusProcessing,
		publisherID,
	)
	if err != nil {
		return nil, cerr.InternalError(
			fmt.Errorf("claim pending outbox messages: %w", err),
			map[string]any{
				"publisher_id": publisherID,
				"limit":        limit,
			},
		)
	}
	defer rows.Close()

	messages, err := postgres.ValuesInto[model.OutboxMessage](rows.(postgres.Rows))
	if err != nil {
		return nil, fmt.Errorf("claim pending outbox messages: %w", err)
	}

	return messages, nil
}

// MarkPublished marks a processing message as successfully delivered.
func MarkPublished[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID uuid.UUID,
	publishedAt time.Time,
) error {
	const query = `
UPDATE integration_outbox
SET
	status = $1,
	published_at = $2,
	locked_at = NULL,
	locked_by = NULL,
	last_error = NULL
WHERE id = $3
  AND status = $4
`
	affected, err := q.Exec(
		ctx,
		query,
		model.OutboxStatusPublished,
		publishedAt,
		outboxID,
		model.OutboxStatusProcessing,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("mark outbox message %s published: %w", outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrOutboxMessageNotFound,
			map[string]any{
				"outbox_id": outboxID,
				"status":    model.OutboxStatusProcessing,
			},
		)
	}

	return nil
}

// MarkRetry moves a processing message back to pending for a future retry.
func MarkRetry[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID uuid.UUID,
	nextAttemptAt time.Time,
	lastError string,
) error {
	const query = `
UPDATE integration_outbox
SET
	status = $1,
	next_attempt_at = $2,
	locked_at = NULL,
	locked_by = NULL,
	last_error = $3
WHERE id = $4
  AND status = $5
`
	affected, err := q.Exec(
		ctx,
		query,
		model.OutboxStatusPending,
		nextAttemptAt,
		lastError,
		outboxID,
		model.OutboxStatusProcessing,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("schedule outbox message %s retry: %w", outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrOutboxMessageNotFound,
			map[string]any{
				"outbox_id": outboxID,
				"status":    model.OutboxStatusProcessing,
			},
		)
	}

	return nil
}

// ReleaseProcessing moves a processing message back to pending for a future retry.
func ReleaseProcessing[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID uuid.UUID,
	lastError string,
	nextAttemptAt time.Time,
) error {
	return MarkRetry(ctx, q, outboxID, nextAttemptAt, lastError)
}

// MarkFailed marks a processing message as permanently failed.
func MarkFailed[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID uuid.UUID,
	lastError string,
) error {
	const query = `
UPDATE integration_outbox
SET
	status = $1,
	locked_at = NULL,
	locked_by = NULL,
	last_error = $2
WHERE id = $3
  AND status = $4
`

	affected, err := q.Exec(
		ctx,
		query,
		model.OutboxStatusFailed,
		lastError,
		outboxID,
		model.OutboxStatusProcessing,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("mark outbox message %s failed: %w", outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrOutboxMessageNotFound,
			map[string]any{
				"outbox_id": outboxID,
				"status":    model.OutboxStatusProcessing,
			},
		)
	}

	return nil
}

// normalizeForInsert applies application defaults before inserting.
//
// Database defaults protect direct SQL inserts. This normalization protects Go
// callers that pass a zero-value model.
func normalizeForInsert(msg model.OutboxMessage) model.OutboxMessage {
	now := time.Now().UTC()

	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}

	if msg.Status == "" {
		msg.Status = model.OutboxStatusPending
	}

	if msg.MaxAttempts == 0 {
		msg.MaxAttempts = 10
	}

	if msg.NextAttemptAt.IsZero() {
		msg.NextAttemptAt = now
	}

	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = now
	}

	return msg
}
