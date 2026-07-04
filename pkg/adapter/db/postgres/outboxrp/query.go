package outboxrp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var (
	// ErrInvalidClaimLimit indicates an invalid publisher batch size.
	ErrInvalidClaimLimit = errors.New("invalid outbox claim limit")

	// ErrOutboxMessageNotFound indicates that the requested message was not
	// updated, usually because it does not exist or is not in the expected state.
	ErrOutboxMessageNotFound = errors.New("outbox message not found")
)

const outboxReturningColumns = `
	id,
	destination,
	message_key,
	event_type,
	aggregate_type,
	payload,
	status,
	attempt_count,
	max_attempts,
	next_attempt_at,
	locked_at,
	locked_by,
	lock_version,
	last_error,
	created_at,
	published_at
`

// ErrOutboxLeaseLost indicates the worker no longer owns the processing lease.
var ErrOutboxLeaseLost = errors.New("outbox processing lease lost")

// Create inserts a new outbox message and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	message repo.CreateOutboxMessage,
) (model.OutboxMessage, error) {
	const op = "outboxrp.Create"

	if err := postgres.RequiredTime("created_at", message.CreatedAt); err != nil {
		return model.OutboxMessage{}, err
	}

	if message.Destination == "" {
		return model.OutboxMessage{}, cerr.InvalidArgument(fmt.Errorf("%s: outbox destination is required", op))
	}

	if message.MessageKey == "" {
		return model.OutboxMessage{}, cerr.InvalidArgument(fmt.Errorf("%s: outbox message key is required", op))
	}

	if message.EventType == "" {
		return model.OutboxMessage{}, cerr.InvalidArgument(fmt.Errorf("%s: outbox event type is required", op))
	}

	if len(message.Payload) == 0 {
		return model.OutboxMessage{}, cerr.InvalidArgument(fmt.Errorf("%s: outbox payload is required", op))
	}

	const query = `
INSERT INTO integration_outbox (
	destination,
	message_key,
	event_type,
	aggregate_type,
	payload,
	status,
	attempt_count,
	next_attempt_at,
	created_at
)
VALUES (
	$1, $2, $3, $4, $5, $6, 0, $7, $8
)
RETURNING` + outboxReturningColumns

	return postgres.QueryOne[model.OutboxMessage](
		ctx,
		q,
		"create outbox message",
		query,
		message.Destination,
		message.MessageKey,
		message.EventType,
		message.AggregateType,
		message.Payload,
		model.PendingOutboxStatus,
		message.CreatedAt,
		message.CreatedAt,
	)
}

// GetByID returns an outbox message by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID model.OutboxID,
) (model.OutboxMessage, error) {
	const query = `
SELECT
	id,
	destination,
	message_key,
	event_type,
	aggregate_type,
	payload,
	status,
	attempt_count,
	max_attempts,
	next_attempt_at,
	locked_at,
	locked_by,
	lock_version,
	last_error,
	created_at,
	published_at
FROM integration_outbox
WHERE id = $1
`

	return postgres.QueryOne[model.OutboxMessage](ctx, q, fmt.Sprintf("get outbox message %s", outboxID), query, outboxID)
}

// ClaimPending claims due pending messages and reclaims stale processing leases.
func ClaimPending[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	publisherID string,
	limit int,
	now time.Time,
	processingLeaseTimeout time.Duration,
) ([]model.OutboxMessage, error) {
	const op = "outboxrp.ClaimPending"

	if limit <= 0 {
		return nil, cerr.InvalidArgument(
			ErrInvalidClaimLimit,
			map[string]any{
				"limit": limit,
			},
		)
	}

	if processingLeaseTimeout <= 0 {
		return nil, cerr.InvalidArgument(errors.New("processing lease timeout must be positive"))
	}

	leaseCutoff := now.Add(-processingLeaseTimeout)

	const expireExhaustedQuery = `
UPDATE integration_outbox AS o
SET
	status = $1,
	locked_at = NULL,
	locked_by = NULL,
	last_error = $2
WHERE o.id IN (
	SELECT id
	FROM integration_outbox
	WHERE status = $3
	  AND locked_at IS NOT NULL
	  AND locked_at <= $4
	  AND attempt_count >= max_attempts
	LIMIT $5
	FOR UPDATE SKIP LOCKED
)
`
	if _, err := q.Exec(
		ctx,
		expireExhaustedQuery,
		model.FailedOutboxStatus,
		"processing lease expired after final attempt",
		model.ProcessingOutboxStatus,
		leaseCutoff,
		limit,
	); err != nil {
		return nil, cerr.InternalError(fmt.Errorf("%s: expire exhausted processing outbox messages: %w", op, err))
	}

	const query = `
WITH picked AS (
	SELECT id
	FROM integration_outbox
	WHERE (
		status = $1
		AND next_attempt_at <= $2
	)
	OR (
		status = $3
		AND locked_at IS NOT NULL
		AND locked_at <= $4
		AND attempt_count < max_attempts
	)
	ORDER BY next_attempt_at ASC, created_at ASC
	LIMIT $5
	FOR UPDATE SKIP LOCKED
)
UPDATE integration_outbox AS o
SET
	status = $6,
	locked_at = $2,
	locked_by = $7,
	lock_version = o.lock_version + 1,
	attempt_count = o.attempt_count + 1,
	last_error = NULL
FROM picked
WHERE o.id = picked.id
RETURNING
	o.id,
	o.destination,
	o.message_key,
	o.event_type,
	o.aggregate_type,
	o.payload,
	o.status,
	o.attempt_count,
	o.max_attempts,
	o.next_attempt_at,
	o.locked_at,
	o.locked_by,
	o.lock_version,
	o.last_error,
	o.created_at,
	o.published_at
`

	rows, err := q.Query(
		ctx,
		query,
		model.PendingOutboxStatus,
		now,
		model.ProcessingOutboxStatus,
		leaseCutoff,
		limit,
		model.ProcessingOutboxStatus,
		publisherID,
	)
	if err != nil {
		return nil, cerr.InternalError(
			fmt.Errorf("%s: claim pending outbox messages: %w", op, err),
			map[string]any{
				"publisher_id": publisherID,
				"limit":        limit,
			},
		)
	}

	defer rows.Close()

	messages, err := postgres.ValuesInto[model.OutboxMessage](postgres.MustRows(rows))
	if err != nil {
		return nil, fmt.Errorf("%s: claim pending outbox messages: %w", op, err)
	}

	return messages, nil
}

// MarkPublished marks a processing message as successfully delivered.
func MarkPublished[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
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
  AND locked_by = $5
  AND lock_version = $6
`
	const op = "outboxrp.MarkPublished"

	affected, err := q.Exec(
		ctx,
		query,
		model.PublishedOutboxStatus,
		publishedAt,
		outboxID,
		model.ProcessingOutboxStatus,
		workerID,
		lockVersion,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("%s: mark outbox message %s published: %w", op, outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.Conflict(
			ErrOutboxLeaseLost,
			map[string]any{
				"outbox_id":    outboxID,
				"worker_id":    workerID,
				"lock_version": lockVersion,
			},
		)
	}

	return nil
}

// MarkSkipped marks a processing message as intentionally not delivered.
func MarkSkipped[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
	skippedAt time.Time,
	reason string,
) error {
	const query = `
UPDATE integration_outbox
SET
	status = $1,
	published_at = $2,
	locked_at = NULL,
	locked_by = NULL,
	last_error = $3
WHERE id = $4
  AND status = $5
  AND locked_by = $6
  AND lock_version = $7
`
	const op = "outboxrp.MarkSkipped"

	affected, err := q.Exec(
		ctx,
		query,
		model.SkippedOutboxStatus,
		skippedAt,
		reason,
		outboxID,
		model.ProcessingOutboxStatus,
		workerID,
		lockVersion,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("%s: mark outbox message %s skipped: %w", op, outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.Conflict(
			ErrOutboxLeaseLost,
			map[string]any{
				"outbox_id":    outboxID,
				"worker_id":    workerID,
				"lock_version": lockVersion,
			},
		)
	}

	return nil
}

// MarkRetry moves a processing message back to pending for a future retry.
func MarkRetry[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
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
  AND locked_by = $6
  AND lock_version = $7
`
	const op = "outboxrp.MarkRetry"

	affected, err := q.Exec(
		ctx,
		query,
		model.PendingOutboxStatus,
		nextAttemptAt,
		lastError,
		outboxID,
		model.ProcessingOutboxStatus,
		workerID,
		lockVersion,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("%s: schedule outbox message %s retry: %w", op, outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.Conflict(
			ErrOutboxLeaseLost,
			map[string]any{
				"outbox_id":    outboxID,
				"worker_id":    workerID,
				"lock_version": lockVersion,
			},
		)
	}

	return nil
}

// ReleaseProcessing moves a processing message back to pending for a future retry.
func ReleaseProcessing[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
	lastError string,
	nextAttemptAt time.Time,
) error {
	return MarkRetry(ctx, q, outboxID, workerID, lockVersion, nextAttemptAt, lastError)
}

// MarkFailed marks a processing message as permanently failed.
func MarkFailed[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
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
  AND locked_by = $5
  AND lock_version = $6
`
	const op = "outboxrp.MarkFailed"

	affected, err := q.Exec(
		ctx,
		query,
		model.FailedOutboxStatus,
		lastError,
		outboxID,
		model.ProcessingOutboxStatus,
		workerID,
		lockVersion,
	)
	if err != nil {
		return cerr.InternalError(
			fmt.Errorf("%s: mark outbox message %s failed: %w", op, outboxID, err),
			map[string]any{
				"outbox_id": outboxID,
			},
		)
	}

	if affected == 0 {
		return cerr.Conflict(
			ErrOutboxLeaseLost,
			map[string]any{
				"outbox_id":    outboxID,
				"worker_id":    workerID,
				"lock_version": lockVersion,
			},
		)
	}

	return nil
}
