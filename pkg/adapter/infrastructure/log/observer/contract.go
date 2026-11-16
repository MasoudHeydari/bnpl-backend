package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

// Contract records contract origination, payment-session submission,
// cancellation, and expiry flow checkpoints.
type Contract struct {
	logger log.Logger
}

// NewContract constructs a contract observer.
func NewContract(logger log.Logger) (*Contract, error) {
	const op = "observer.NewContract"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &Contract{logger: logger}, nil
}

var _ contractuc.Observer = (*Contract)(nil)

// PreparationStarted implements the corresponding Observer checkpoint.
func (o *Contract) PreparationStarted(ctx context.Context, decisionID model.DecisionID) {
	o.logger.WithContext(ctx).Info(
		event.ContractPreparationStarted,
		field.DecisionID(decisionID),
	)
}

// PaymentPendingCreated implements the corresponding Observer checkpoint.
func (o *Contract) PaymentPendingCreated(
	ctx context.Context,
	contractID model.ContractID,
	decisionID model.DecisionID,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractPaymentPendingCreated,
		field.ContractID(contractID),
		field.DecisionID(decisionID),
	)
}

// ContractReplayed implements the corresponding Observer checkpoint.
func (o *Contract) ContractReplayed(
	ctx context.Context,
	contractID model.ContractID,
	status model.ContractStatus,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractReplayed,
		field.ContractID(contractID),
		field.Status(string(status)),
	)
}

// PaymentAttemptCreated implements the corresponding Observer checkpoint.
func (o *Contract) PaymentAttemptCreated(
	ctx context.Context,
	contractID model.ContractID,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentAttemptCreated,
		field.ContractID(contractID),
		field.PaymentAttemptID(paymentAttemptID),
	)
}

// FinancialCoreRequestStarted implements the corresponding Observer checkpoint.
func (o *Contract) FinancialCoreRequestStarted(
	ctx context.Context,
	contractID model.ContractID,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentFinancialCoreRequestStarted,
		field.ContractID(contractID),
		field.PaymentAttemptID(paymentAttemptID),
	)
}

// FinancialCoreResponseReceived implements the corresponding Observer checkpoint.
func (o *Contract) FinancialCoreResponseReceived(
	ctx context.Context,
	contractID model.ContractID,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentFinancialCoreResponseReceived,
		field.ContractID(contractID),
		field.PaymentAttemptID(paymentAttemptID),
	)
}

// FinancialCoreFailed implements the corresponding Observer checkpoint.
func (o *Contract) FinancialCoreFailed(
	ctx context.Context,
	contractID model.ContractID,
	paymentAttemptID model.PaymentAttemptID,
	err error,
) {
	fields := append(
		[]log.Field{field.ContractID(contractID), field.PaymentAttemptID(paymentAttemptID)},
		fcFailureFields(err)...,
	)

	if isFCTimeout(err) {
		o.logger.WithContext(ctx).Warn(event.PaymentFinancialCoreTimedOut, fields...)
		return
	}

	o.logger.WithContext(ctx).Warn(event.PaymentFinancialCoreFailed, fields...)
}

// PaymentURLObtained implements the corresponding Observer checkpoint.
func (o *Contract) PaymentURLObtained(
	ctx context.Context,
	contractID model.ContractID,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentURLObtained,
		field.ContractID(contractID),
		field.PaymentAttemptID(paymentAttemptID),
		field.Status("submitted"),
	)
}

// ContractActivated implements the corresponding Observer checkpoint.
func (o *Contract) ContractActivated(
	ctx context.Context,
	contractID model.ContractID,
	source string,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractActivated,
		field.ContractID(contractID),
		field.Outcome(source),
	)
}

// CancellationSagaStarted implements the corresponding Observer checkpoint.
func (o *Contract) CancellationSagaStarted(
	ctx context.Context,
	contractID model.ContractID,
	sagaID model.SagaID,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractCancellationSagaStarted,
		field.ContractID(contractID),
		field.SagaID(sagaID.String()),
	)
}

// CancellationSagaCompleted implements the corresponding Observer checkpoint.
func (o *Contract) CancellationSagaCompleted(
	ctx context.Context,
	contractID model.ContractID,
	sagaID model.SagaID,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractCancellationSagaCompleted,
		field.ContractID(contractID),
		field.SagaID(sagaID.String()),
	)
}

// CancellationSagaFailed implements the corresponding Observer checkpoint.
func (o *Contract) CancellationSagaFailed(
	ctx context.Context,
	contractID model.ContractID,
	sagaID model.SagaID,
	err error,
) {
	fields := append(
		[]log.Field{field.ContractID(contractID), field.SagaID(sagaID.String())},
		fcFailureFields(err)...,
	)

	o.logger.WithContext(ctx).Warn(event.ContractCancellationSagaFailed, fields...)
}

// ContractCanceled implements the corresponding Observer checkpoint.
func (o *Contract) ContractCanceled(ctx context.Context, contractID model.ContractID) {
	o.logger.WithContext(ctx).Info(
		event.ContractCanceled,
		field.ContractID(contractID),
	)
}

// ContractExpired implements the corresponding Observer checkpoint.
func (o *Contract) ContractExpired(ctx context.Context, contractID model.ContractID) {
	o.logger.WithContext(ctx).Info(
		event.ContractExpired,
		field.ContractID(contractID),
	)
}

// PaymentPendingExpiryCompleted implements the corresponding Observer checkpoint.
func (o *Contract) PaymentPendingExpiryCompleted(
	ctx context.Context,
	expiredCount int,
	failedCount int,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractPaymentPendingExpiryCompleted,
		field.ProcessedCount(expiredCount),
		field.FailedCount(failedCount),
	)
}
