package outboxpublisheruc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records business flow checkpoints for outbox message delivery.
// Implementations must not modify domain state or return errors that affect
// business execution.
//
// Delivery-state checkpoints (published, skipped, failed, retry scheduled) are
// emitted only after the new outbox delivery state was persisted. A message
// that was published but whose state could not be committed is reported as a
// lost lease, never as published. Operational database failures remain on the
// use case logger.
type Observer interface {
	// MessagePublished records a message published and marked as published.
	MessagePublished(ctx context.Context, outboxID model.OutboxID, destination string)

	// MessageSkipped records a message intentionally skipped by the publisher.
	MessageSkipped(ctx context.Context, outboxID model.OutboxID, destination string, reason string)

	// MessageFailed records a message moved to the permanently failed state.
	MessageFailed(
		ctx context.Context,
		outboxID model.OutboxID,
		destination string,
		attemptCount int,
		lastError string,
	)

	// MessageRetryScheduled records a failed delivery with a persisted retry.
	MessageRetryScheduled(
		ctx context.Context,
		outboxID model.OutboxID,
		destination string,
		attemptCount int,
		lastError string,
	)

	// LeaseLost records that this worker lost its claim while persisting the
	// delivery outcome, so another worker may redeliver the message.
	LeaseLost(ctx context.Context, outboxID model.OutboxID, destination string)
}
