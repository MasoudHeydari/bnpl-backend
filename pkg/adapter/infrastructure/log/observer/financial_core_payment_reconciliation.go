package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
)

// FinancialCorePaymentReconciliation records inbound Financial Core payment
// reconciliation flow checkpoints.
type FinancialCorePaymentReconciliation struct {
	logger log.Logger
}

// NewFinancialCorePaymentReconciliation constructs a reconciliation observer.
func NewFinancialCorePaymentReconciliation(logger log.Logger) (*FinancialCorePaymentReconciliation, error) {
	const op = "observer.NewFinancialCorePaymentReconciliation"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &FinancialCorePaymentReconciliation{logger: logger}, nil
}

var _ financialcorepaymentreconciliationuc.Observer = (*FinancialCorePaymentReconciliation)(nil)

// InboxDuplicateDetected implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentReconciliation) InboxDuplicateDetected(
	ctx context.Context,
	eventID string,
	paymentAttemptID model.PaymentAttemptID,
	inboxResult string,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentInboxDuplicateDetected,
		field.EventID(eventID),
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome(inboxResult),
	)
}

// ReconciliationResolved implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentReconciliation) ReconciliationResolved(
	ctx context.Context,
	eventID string,
	paymentAttemptID model.PaymentAttemptID,
	outcome string,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentReconciliationSucceeded,
		field.EventID(eventID),
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome(outcome),
	)
}

// ReconciliationRetryScheduled implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentReconciliation) ReconciliationRetryScheduled(
	ctx context.Context,
	eventID string,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Warn(
		event.PaymentReconciliationRetryScheduled,
		field.EventID(eventID),
		field.PaymentAttemptID(paymentAttemptID),
	)
}

// ReconciliationFailed implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentReconciliation) ReconciliationFailed(
	ctx context.Context,
	eventID string,
	paymentAttemptID model.PaymentAttemptID,
	outcome string,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentReconciliationFailed,
		field.EventID(eventID),
		field.PaymentAttemptID(paymentAttemptID),
		field.Outcome(outcome),
	)
}

// ReconciliationMismatch implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentReconciliation) ReconciliationMismatch(
	ctx context.Context,
	eventID string,
	paymentAttemptID model.PaymentAttemptID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.PaymentReconciliationMismatch,
		field.EventID(eventID),
		field.PaymentAttemptID(paymentAttemptID),
		field.ReasonCode(reasonCode),
	)
}
