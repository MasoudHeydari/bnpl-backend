package observer

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

// FinancialCorePaymentResult records inbound Financial Core payment-result
// processing flow checkpoints.
//
// Payment succeeded is distinct from payment submitted: submission is observed
// by the contract and installment repayment observers (payment URL obtained),
// while success is observed here only after the authoritative result was
// applied and committed.
type FinancialCorePaymentResult struct {
	logger log.Logger
}

// NewFinancialCorePaymentResult constructs a Financial Core payment-result observer.
func NewFinancialCorePaymentResult(logger log.Logger) (*FinancialCorePaymentResult, error) {
	const op = "observer.NewFinancialCorePaymentResult"

	if logger == nil {
		return nil, fmt.Errorf("%s: logger is required", op)
	}

	return &FinancialCorePaymentResult{logger: logger}, nil
}

var _ financialcorepaymentresultuc.Observer = (*FinancialCorePaymentResult)(nil)

// resultBaseFields returns the identity fields shared by payment-result
// checkpoints. A zero contract ID is omitted.
func resultBaseFields(eventID string, contractID model.ContractID) []log.Field {
	fields := []log.Field{field.EventID(eventID)}
	if !contractID.IsZero() {
		fields = append(fields, field.ContractID(contractID))
	}

	return fields
}

// ResultReceived implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ResultReceived(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	status model.FinancialPaymentResultStatus,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentResultReceived,
		append(resultBaseFields(eventID, contractID), field.Status(string(status)))...,
	)
}

// ResultValidated implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ResultValidated(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	status model.FinancialPaymentResultStatus,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentResultValidated,
		append(resultBaseFields(eventID, contractID), field.Status(string(status)))...,
	)
}

// ResultValidationFailed implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ResultValidationFailed(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.PaymentResultValidationFailed,
		append(resultBaseFields(eventID, contractID), field.ReasonCode(reasonCode))...,
	)
}

// InboxDuplicateDetected implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) InboxDuplicateDetected(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	inboxResult string,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentInboxDuplicateDetected,
		append(resultBaseFields(eventID, contractID), field.Outcome(inboxResult))...,
	)
}

// ApplicationStarted implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ApplicationStarted(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentApplicationStarted,
		resultBaseFields(eventID, contractID)...,
	)
}

// ApplicationSucceeded implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ApplicationSucceeded(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentApplicationSucceeded,
		append(resultBaseFields(eventID, contractID), field.Status("succeeded"))...,
	)
}

// ApplicationAlreadyApplied implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ApplicationAlreadyApplied(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
) {
	o.logger.WithContext(ctx).Info(
		event.PaymentApplicationAlreadyApplied,
		resultBaseFields(eventID, contractID)...,
	)
}

// ApplicationConflict implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ApplicationConflict(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	reasonCode string,
) {
	o.logger.WithContext(ctx).Warn(
		event.PaymentApplicationConflict,
		append(resultBaseFields(eventID, contractID), field.ReasonCode(reasonCode))...,
	)
}

// TerminalFailure implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) TerminalFailure(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	status model.FinancialPaymentResultStatus,
) {
	o.logger.WithContext(ctx).Warn(
		event.PaymentTerminalFailure,
		append(resultBaseFields(eventID, contractID), field.Status(string(status)))...,
	)
}

// ContractActivated implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) ContractActivated(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
) {
	o.logger.WithContext(ctx).Info(
		event.ContractActivated,
		append(resultBaseFields(eventID, contractID), field.Outcome("financial_core_payment_result"))...,
	)
}

// InstallmentRepaymentApplied implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) InstallmentRepaymentApplied(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	installmentID model.InstallmentID,
) {
	fields := resultBaseFields(eventID, contractID)
	if !installmentID.IsZero() {
		fields = append(fields, field.InstallmentID(installmentID))
	}

	o.logger.WithContext(ctx).Info(event.InstallmentRepaymentApplied, fields...)
}

// InstallmentRepaymentFailed implements the corresponding Observer checkpoint.
func (o *FinancialCorePaymentResult) InstallmentRepaymentFailed(
	ctx context.Context,
	eventID string,
	contractID model.ContractID,
	installmentID model.InstallmentID,
	status model.FinancialPaymentResultStatus,
) {
	fields := append(resultBaseFields(eventID, contractID), field.Status(string(status)))
	if !installmentID.IsZero() {
		fields = append(fields, field.InstallmentID(installmentID))
	}

	o.logger.WithContext(ctx).Warn(event.InstallmentRepaymentFailed, fields...)
}
