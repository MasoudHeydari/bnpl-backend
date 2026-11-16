package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
)

// PaymentAttemptReconciliation records payment attempt reconciliation
// scheduling flow checkpoints.
type PaymentAttemptReconciliation struct {
	logger log.Logger
}

// NewPaymentAttemptReconciliation constructs a payment attempt reconciliation observer.
func NewPaymentAttemptReconciliation(logger log.Logger) (*PaymentAttemptReconciliation, error) {
	const op = "observer.NewPaymentAttemptReconciliation"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &PaymentAttemptReconciliation{logger: logger}, nil
}

var _ paymentattemptreconciliationuc.Observer = (*PaymentAttemptReconciliation)(nil)

// ReconciliationRequested implements the corresponding Observer checkpoint.
func (o *PaymentAttemptReconciliation) ReconciliationRequested(
	ctx context.Context,
	paymentAttemptID model.PaymentAttemptID,
	generation int,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentReconciliationRetryScheduled,
		field.PaymentAttemptID(paymentAttemptID),
		field.AttemptCount(generation),
	)
}

// ManualReviewRequired implements the corresponding Observer checkpoint.
func (o *PaymentAttemptReconciliation) ManualReviewRequired(
	ctx context.Context,
	paymentAttemptID model.PaymentAttemptID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.PaymentReconciliationFailed,
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome("manual_review_required"),
		field.ReasonCode(reasonCode),
	)
}

// AttemptClosedLocally implements the corresponding Observer checkpoint.
func (o *PaymentAttemptReconciliation) AttemptClosedLocally(
	ctx context.Context,
	paymentAttemptID model.PaymentAttemptID,
	outcome string,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentTerminalFailure,
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome(outcome),
	)
}
