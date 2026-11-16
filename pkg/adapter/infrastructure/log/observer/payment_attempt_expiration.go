package observer

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptexpirationuc"
)

// PaymentAttemptExpiration records local payment-attempt session-creation
// expiration flow checkpoints.
type PaymentAttemptExpiration struct {
	logger log.Logger
}

// NewPaymentAttemptExpiration constructs a payment attempt expiration observer.
func NewPaymentAttemptExpiration(logger log.Logger) (*PaymentAttemptExpiration, error) {
	const op = "observer.NewPaymentAttemptExpiration"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &PaymentAttemptExpiration{logger: logger}, nil
}

var _ paymentattemptexpirationuc.Observer = (*PaymentAttemptExpiration)(nil)

// AttemptExpired implements the corresponding Observer checkpoint.
func (o *PaymentAttemptExpiration) AttemptExpired(
	ctx context.Context,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentTerminalFailure,
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome("expired"),
	)
}

// AttemptCanceled implements the corresponding Observer checkpoint.
func (o *PaymentAttemptExpiration) AttemptCanceled(
	ctx context.Context,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentTerminalFailure,
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome("canceled_retry_exhausted"),
	)
}

// ExpirationCycleCompleted implements the corresponding Observer checkpoint.
func (o *PaymentAttemptExpiration) ExpirationCycleCompleted(
	ctx context.Context,
	expiredCount int,
	failedCount int,
	duration time.Duration,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentAttemptExpirationCompleted,
		field.ProcessedCount(expiredCount),
		field.FailedCount(failedCount),
		field.Duration(duration),
	)
}
