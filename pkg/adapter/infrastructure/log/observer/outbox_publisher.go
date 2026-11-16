package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/outboxpublisheruc"
)

// OutboxPublisher records outbox message delivery flow checkpoints.
type OutboxPublisher struct {
	logger log.Logger
}

// NewOutboxPublisher constructs an outbox publisher observer.
func NewOutboxPublisher(logger log.Logger) (*OutboxPublisher, error) {
	const op = "observer.NewOutboxPublisher"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &OutboxPublisher{logger: logger}, nil
}

var _ outboxpublisheruc.Observer = (*OutboxPublisher)(nil)

// MessagePublished implements the corresponding Observer checkpoint.
func (o *OutboxPublisher) MessagePublished(
	ctx context.Context,
	outboxID model.OutboxID,
	destination string,
) {
	o.logger.WithContext(ctx).Info(
		event.OutboxMessagePublished,
		field.OutboxMessageID(outboxID.String()),
		field.Topic(destination),
	)
}

// MessageSkipped implements the corresponding Observer checkpoint.
func (o *OutboxPublisher) MessageSkipped(
	ctx context.Context,
	outboxID model.OutboxID,
	destination string,
	reason string,
) {
	o.logger.WithContext(ctx).Info(
		event.OutboxMessageSkipped,
		field.OutboxMessageID(outboxID.String()),
		field.Topic(destination),
		field.Reason(reason),
	)
}

// MessageFailed implements the corresponding Observer checkpoint.
func (o *OutboxPublisher) MessageFailed(
	ctx context.Context,
	outboxID model.OutboxID,
	destination string,
	attemptCount int,
	lastError string,
) {
	o.logger.WithContext(ctx).Error(
		event.OutboxMessageFailed,
		field.OutboxMessageID(outboxID.String()),
		field.Topic(destination),
		field.AttemptCount(attemptCount),
		field.ErrorMessage(lastError),
	)
}

// MessageRetryScheduled implements the corresponding Observer checkpoint.
func (o *OutboxPublisher) MessageRetryScheduled(
	ctx context.Context,
	outboxID model.OutboxID,
	destination string,
	attemptCount int,
	lastError string,
) {
	o.logger.WithContext(ctx).Warn(
		event.OutboxMessageRetryScheduled,
		field.OutboxMessageID(outboxID.String()),
		field.Topic(destination),
		field.AttemptCount(attemptCount),
		field.ErrorMessage(lastError),
	)
}

// LeaseLost implements the corresponding Observer checkpoint.
func (o *OutboxPublisher) LeaseLost(
	ctx context.Context,
	outboxID model.OutboxID,
	destination string,
) {
	o.logger.WithContext(ctx).Warn(
		event.OutboxLeaseLost,
		field.OutboxMessageID(outboxID.String()),
		field.Topic(destination),
	)
}
