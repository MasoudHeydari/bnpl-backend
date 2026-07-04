package outboxpublisheruc

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Outbox publisher configuration and claim validation errors.
var (
	ErrMissingWorkerID          = errors.New("missing outbox publisher worker id")
	ErrInvalidBatchSize         = errors.New("invalid outbox publisher batch size")
	ErrPublishedButNotCommitted = errors.New("outbox message published but delivery state was not committed")
)

// PublishDue claims due outbox messages, releases the database connection, then
// publishes each message outside DB scope. Each delivery result is persisted in
// its own short database connection scope.
//
// The method is safe to execute concurrently from multiple workers when the
// repository coordinates claims so each due message is processed by at most one
// worker at a time.
func (uc *UseCase) PublishDue(ctx context.Context) error {
	const op = "outboxpublisheruc.UseCase.PublishDue"

	if err := uc.Validate(); err != nil {
		return err
	}

	now := uc.clock.Now()
	var messages []model.OutboxMessage

	if err := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		outbox := uc.outbox.Conn(conn)

		claimed, err := outbox.ClaimPending(
			ctx,
			uc.WorkerID,
			uc.BatchSize,
			now,
			uc.ProcessingLeaseTimeout,
		)
		if err != nil {
			return fmt.Errorf("%s: claim pending outbox messages: %w", op, err)
		}

		messages = claimed
		return nil
	}); err != nil {
		return err
	}

	if len(messages) == 0 {
		return nil
	}

	var resultErr error
	for _, msg := range messages {
		publishErr := uc.publishMessage(ctx, msg)

		persistErr := uc.pool.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
			outbox := uc.outbox.Conn(conn)
			return uc.persistPublishOutcome(ctx, outbox, msg, publishErr)
		})
		resultErr = errors.Join(resultErr, persistErr)
	}

	return resultErr
}

// publishMessage performs only the external publish. It intentionally does not
// hold or require a database connection.
func (uc *UseCase) publishMessage(ctx context.Context, msg model.OutboxMessage) error {
	return uc.publisher.Publish(
		ctx,
		msg.Destination,
		messaging.Message{
			Key:   []byte(msg.MessageKey),
			Value: msg.Payload,
			Topic: msg.Destination,
			Headers: map[string][]byte{
				"outbox_id":      []byte(msg.ID.String()),
				"event_type":     []byte(msg.EventType),
				"aggregate_type": optionalStringBytes(msg.AggregateType),
			},
		},
	)
}

// persistPublishOutcome updates the claimed message state after the external
// publish has completed.
func (uc *UseCase) persistPublishOutcome(
	ctx context.Context,
	outbox repo.OutboxQueryer,
	msg model.OutboxMessage,
	publishErr error,
) error {
	const op = "outboxpublisheruc.UseCase.persistPublishOutcome"

	logger := uc.logger.With(
		"outbox_id",
		msg.ID.String(),
		"destination",
		msg.Destination,
		"event_type",
		msg.EventType,
	)

	if publishErr == nil {
		publishedAt := uc.clock.Now()
		if markErr := outbox.MarkPublished(
			ctx,
			msg.ID,
			uc.WorkerID,
			msg.LockVersion,
			publishedAt,
		); markErr != nil {
			if cerr.IsCode(markErr, cerr.CodeConflict) {
				logger.Warn("outbox lease lost while marking published", "error", markErr.Error())
				return fmt.Errorf(
					"%s: %w: outbox message %s: %w",
					op,
					ErrPublishedButNotCommitted,
					msg.ID,
					markErr,
				)
			}

			logger.Error("failed to mark outbox message published", "error", markErr.Error())
			return fmt.Errorf("%s: mark outbox message %s published: %w", op, msg.ID, markErr)
		}

		logger.Info("outbox message published")
		return nil
	}

	if messaging.IsSkipped(publishErr) {
		skippedAt := uc.clock.Now()
		reason := publishErr.Error()
		if markErr := outbox.MarkSkipped(
			ctx,
			msg.ID,
			uc.WorkerID,
			msg.LockVersion,
			skippedAt,
			reason,
		); markErr != nil {
			if cerr.IsCode(markErr, cerr.CodeConflict) {
				logger.Warn("outbox lease lost while marking skipped", "error", markErr.Error())
				return fmt.Errorf("%s: skip outbox message %s: %w", op, msg.ID, publishErr)
			}

			logger.Error("failed to mark outbox message skipped", "error", markErr.Error())
			return fmt.Errorf("%s: mark outbox message %s skipped: %w", op, msg.ID, markErr)
		}

		logger.Info("outbox message skipped", "reason", reason)
		return nil
	}

	lastError := publishErr.Error()

	// A claimed message already consumed one delivery attempt.
	//
	// Once the configured retry budget is exhausted, or the publisher reports a
	// permanent failure (messaging.NonRetryable, for example a disabled webhook
	// destination or a non-retryable 4xx), the message is moved to the failed
	// state and will no longer be retried.
	if msg.AttemptCount >= msg.MaxAttempts || messaging.IsNonRetryable(publishErr) {
		if markErr := outbox.MarkFailed(
			ctx,
			msg.ID,
			uc.WorkerID,
			msg.LockVersion,
			lastError,
		); markErr != nil {
			if cerr.IsCode(markErr, cerr.CodeConflict) {
				logger.Warn("outbox lease lost while marking failed", "error", markErr.Error())
				return fmt.Errorf("%s: publish outbox message %s: %w", op, msg.ID, publishErr)
			}

			logger.Error(
				"failed to mark outbox message permanently failed",
				"publish_error",
				lastError,
				"error",
				markErr.Error(),
			)
			return errors.Join(
				fmt.Errorf("%s: publish outbox message %s: %w", op, msg.ID, publishErr),
				fmt.Errorf("%s: mark outbox message %s failed: %w", op, msg.ID, markErr),
			)
		}

		logger.Error("outbox message permanently failed", "error", lastError)
		return fmt.Errorf("%s: publish outbox message %s: %w", op, msg.ID, publishErr)
	}

	nextAttemptAt := uc.nextAttemptAt(msg.AttemptCount)

	if markErr := outbox.MarkRetry(
		ctx,
		msg.ID,
		uc.WorkerID,
		msg.LockVersion,
		nextAttemptAt,
		lastError,
	); markErr != nil {
		if cerr.IsCode(markErr, cerr.CodeConflict) {
			logger.Warn("outbox lease lost while scheduling retry", "error", markErr.Error())
			return fmt.Errorf("%s: publish outbox message %s: %w", op, msg.ID, publishErr)
		}

		logger.Error(
			"failed to schedule outbox message retry",
			"publish_error",
			lastError,
			"error",
			markErr.Error(),
		)
		return errors.Join(
			fmt.Errorf("%s: publish outbox message %s: %w", op, msg.ID, publishErr),
			fmt.Errorf("%s: schedule outbox message %s retry: %w", op, msg.ID, markErr),
		)
	}

	logger.Warn(
		"outbox message publish failed; retry scheduled",
		"error",
		lastError,
		"next_attempt_at",
		nextAttemptAt,
	)

	return fmt.Errorf("%s: publish outbox message %s: %w", op, msg.ID, publishErr)
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
	if delay >= uc.RetryMaxDelay {
		delay = uc.RetryMaxDelay
	}

	for i := 1; i < attemptCount && delay < uc.RetryMaxDelay; i++ {
		// Saturate before multiplication so time.Duration never overflows.
		if delay > uc.RetryMaxDelay/2 {
			delay = uc.RetryMaxDelay
			break
		}

		delay *= 2
	}

	return uc.clock.Add(uc.clock.Now(), delay)
}

func optionalStringBytes(v *string) []byte {
	if v == nil {
		return nil
	}

	return []byte(*v)
}
