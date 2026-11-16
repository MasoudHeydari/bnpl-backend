package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

// InstallmentRepayment records customer-initiated installment repayment
// submission flow checkpoints. It never reports payment success.
type InstallmentRepayment struct {
	logger log.Logger
}

// NewInstallmentRepayment constructs an installment repayment observer.
func NewInstallmentRepayment(logger log.Logger) (*InstallmentRepayment, error) {
	const op = "observer.NewInstallmentRepayment"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &InstallmentRepayment{logger: logger}, nil
}

var _ installmentrepaymentuc.Observer = (*InstallmentRepayment)(nil)

// RepaymentRequested implements the corresponding Observer checkpoint.
func (o *InstallmentRepayment) RepaymentRequested(
	ctx context.Context,
	contractID model.ContractID,
	installmentID model.InstallmentID,
) {
	o.logger.WithContext(ctx).Info(
		event.InstallmentRepaymentRequested,
		field.ContractID(contractID),
		field.InstallmentID(installmentID),
	)
}

// PaymentAttemptCreated implements the corresponding Observer checkpoint.
func (o *InstallmentRepayment) PaymentAttemptCreated(
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

// PaymentAttemptReplayed implements the corresponding Observer checkpoint.
func (o *InstallmentRepayment) PaymentAttemptReplayed(
	ctx context.Context,
	contractID model.ContractID,
	paymentAttemptID model.PaymentAttemptID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentAttemptReplayed,
		field.ContractID(contractID),
		field.PaymentAttemptID(paymentAttemptID),
	)
}

// FinancialCoreRequestStarted implements the corresponding Observer checkpoint.
func (o *InstallmentRepayment) FinancialCoreRequestStarted(
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
func (o *InstallmentRepayment) FinancialCoreResponseReceived(
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
func (o *InstallmentRepayment) FinancialCoreFailed(
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
func (o *InstallmentRepayment) PaymentURLObtained(
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
