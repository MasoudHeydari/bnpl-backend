package financialcorepaymentresultuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records operational flow checkpoints for inbound Financial Core
// payment-result processing. Implementations must not modify domain state or
// return errors that affect business execution.
//
// Semantics:
//   - Receipt, validation, and application-start checkpoints describe work in
//     progress and may precede the database commit.
//   - Application, activation, and terminal-outcome checkpoints are emitted only
//     after the database transaction committed.
//   - ContractActivated is emitted only when an authoritative Financial Core
//     payment result was newly applied and committed. Duplicate inbox
//     deliveries and idempotent re-applications never activate a contract.
//   - InboxDuplicateDetected reports a delivery whose inbox row already
//     existed; it is never a new application.
type Observer interface {
	// ResultReceived records an inbound Financial Core payment result.
	ResultReceived(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		status model.FinancialPaymentResultStatus,
	)

	// ResultValidated records a payment result that passed envelope validation.
	ResultValidated(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		status model.FinancialPaymentResultStatus,
	)

	// ResultValidationFailed records a payment result that was rejected as
	// invalid. reasonCode is a stable machine-readable code.
	ResultValidationFailed(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		reasonCode string,
	)

	// InboxDuplicateDetected records a redelivered event whose inbox row
	// already exists. inboxResult is the stable inbox acquire outcome.
	InboxDuplicateDetected(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		inboxResult string,
	)

	// ApplicationStarted records the start of durable payment-result application.
	ApplicationStarted(ctx context.Context, eventID string, contractID model.ContractID)

	// ApplicationSucceeded records a payment result newly applied and committed.
	ApplicationSucceeded(ctx context.Context, eventID string, contractID model.ContractID)

	// ApplicationAlreadyApplied records a payment result whose effect had already
	// been applied; no new application occurred.
	ApplicationAlreadyApplied(ctx context.Context, eventID string, contractID model.ContractID)

	// ApplicationConflict records a committed outcome that requires
	// reconciliation or was rejected as conflicting or unmatched.
	ApplicationConflict(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		reasonCode string,
	)

	// TerminalFailure records a committed failed or canceled payment outcome.
	TerminalFailure(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		status model.FinancialPaymentResultStatus,
	)

	// ContractActivated records a contract activated by an authoritative
	// Financial Core payment result that was applied and committed.
	ContractActivated(ctx context.Context, eventID string, contractID model.ContractID)

	// InstallmentRepaymentApplied records a committed installment repayment
	// payment result.
	InstallmentRepaymentApplied(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		installmentID model.InstallmentID,
	)

	// InstallmentRepaymentFailed records a committed failed or canceled
	// installment repayment payment outcome.
	InstallmentRepaymentFailed(
		ctx context.Context,
		eventID string,
		contractID model.ContractID,
		installmentID model.InstallmentID,
		status model.FinancialPaymentResultStatus,
	)
}
