package outboxpublisheruc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var (
	ErrMissingWorkerID  = errors.New("missing outbox publisher worker id")
	ErrInvalidBatchSize = errors.New("invalid outbox publisher batch size")
)

// PublishDue claims due outbox messages and attempts to publish them.
//
// Messages are processed one-by-one:
//
//   - successful publishes are marked as published
//   - transient failures are scheduled for retry
//   - exhausted retries are marked as permanently failed
//
// The method is safe to execute concurrently from multiple workers when the
// repository coordinates claims so each due message is processed by at most one
// worker at a time.
func (uc *UseCase) PublishDue(ctx context.Context) error {
	if uc.WorkerID == "" {
		return ErrMissingWorkerID
	}
	if uc.BatchSize <= 0 {
		return ErrInvalidBatchSize
	}

	now := uc.clock.Now()

	return uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		outbox := uc.outbox.Conn(conn)

		messages, err := outbox.ClaimPending(
			ctx,
			uc.WorkerID,
			uc.BatchSize,
			now,
		)
		if err != nil {
			return fmt.Errorf("claim pending outbox messages: %w", err)
		}

		var publishErr error
		for _, msg := range messages {
			publishErr = errors.Join(publishErr, uc.publishOne(ctx, outbox, msg))
		}

		return publishErr
	})
}

// publishOne publishes a single outbox message and updates its delivery state.
//
// Any state transition failure is logged and returned so the caller can report
// partial failure while still processing the remaining claimed messages.
func (uc *UseCase) publishOne(
	ctx context.Context,
	outbox repo.OutboxQueryer,
	msg model.OutboxMessage,
) error {
	logger := uc.logger.With(
		"outbox_id", msg.ID.String(),
		"destination", msg.Destination,
		"event_type", msg.EventType,
	)

	err := uc.publisher.Publish(
		ctx,
		msg.Destination,
		messaging.Message{
			Key:   []byte(msg.ID.String()),
			Value: msg.Payload,
			Topic: msg.Destination,
			Headers: map[string][]byte{
				"outbox_id":      []byte(msg.ID.String()),
				"event_type":     []byte(msg.EventType),
				"aggregate_type": optionalStringBytes(msg.AggregateType),
			},
		},
	)
	if err == nil {
		publishedAt := uc.clock.Now()
		if markErr := outbox.MarkPublished(ctx, msg.ID, publishedAt); markErr != nil {
			logger.Error("failed to mark outbox message published", "error", markErr.Error())
			return fmt.Errorf("mark outbox message %s published: %w", msg.ID, markErr)
		}

		logger.Info("outbox message published")
		return nil
	}

	lastError := err.Error()

	// A claimed message already consumed one delivery attempt.
	//
	// Once the configured retry budget is exhausted, the message is
	// moved to the failed state and will no longer be retried.
	if msg.AttemptCount >= msg.MaxAttempts {
		if markErr := outbox.MarkFailed(ctx, msg.ID, lastError); markErr != nil {
			logger.Error(
				"failed to mark outbox message permanently failed",
				"publish_error", lastError,
				"error", markErr.Error(),
			)
			return errors.Join(
				fmt.Errorf("publish outbox message %s: %w", msg.ID, err),
				fmt.Errorf("mark outbox message %s failed: %w", msg.ID, markErr),
			)
		}

		logger.Error("outbox message permanently failed", "error", lastError)
		return fmt.Errorf("publish outbox message %s: %w", msg.ID, err)
	}

	nextAttemptAt := uc.nextAttemptAt(msg.AttemptCount)

	if markErr := outbox.MarkRetry(ctx, msg.ID, nextAttemptAt, lastError); markErr != nil {
		logger.Error(
			"failed to schedule outbox message retry",
			"publish_error", lastError,
			"error", markErr.Error(),
		)
		return errors.Join(
			fmt.Errorf("publish outbox message %s: %w", msg.ID, err),
			fmt.Errorf("schedule outbox message %s retry: %w", msg.ID, markErr),
		)
	}

	logger.Warn(
		"outbox message publish failed; retry scheduled",
		"error", lastError,
		"next_attempt_at", nextAttemptAt,
	)

	return fmt.Errorf("publish outbox message %s: %w", msg.ID, err)
}

// nextAttemptAt calculates the next retry time using exponential backoff.
//
// Delay progression:
//
//	1s -> 2s -> 4s -> 8s -> ...
//
// attemptCount starts at 1 for the first retry because the initial claim has
// already consumed one delivery attempt. The delay is capped at RetryMaxDelay.
func (uc *UseCase) nextAttemptAt(attemptCount int) time.Time {
	delay := uc.RetryBaseDelay

	for i := 1; i < attemptCount; i++ {
		delay *= 2
		if delay >= uc.RetryMaxDelay {
			delay = uc.RetryMaxDelay
			break
		}
	}

	return uc.clock.Add(uc.clock.Now(), delay)
}

func optionalStringBytes(v *string) []byte {
	if v == nil {
		return nil
	}

	return []byte(*v)
}
