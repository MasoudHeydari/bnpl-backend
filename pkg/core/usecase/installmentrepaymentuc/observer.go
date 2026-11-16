package installmentrepaymentuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records operational flow checkpoints for customer-initiated
// installment repayment. Implementations must not modify domain state or
// return errors that affect business execution.
//
// Checkpoints here describe payment submission only: a payment attempt exists
// and a Financial Core payment session with a payment URL was obtained. Payment
// success is never observed by this use case; it is observed by
// financialcorepaymentresultuc after the authoritative Financial Core result is
// applied and committed.
type Observer interface {
	// RepaymentRequested records an accepted installment repayment request.
	RepaymentRequested(
		ctx context.Context,
		contractID model.ContractID,
		installmentID model.InstallmentID,
	)

	// PaymentAttemptCreated records a committed new payment attempt.
	PaymentAttemptCreated(
		ctx context.Context,
		contractID model.ContractID,
		paymentAttemptID model.PaymentAttemptID,
	)

	// PaymentAttemptReplayed records an idempotent replay of an existing payment attempt.
	PaymentAttemptReplayed(
		ctx context.Context,
		contractID model.ContractID,
		paymentAttemptID model.PaymentAttemptID,
	)

	// FinancialCoreRequestStarted records the start of a Financial Core
	// checkout payment session request.
	FinancialCoreRequestStarted(
		ctx context.Context,
		contractID model.ContractID,
		paymentAttemptID model.PaymentAttemptID,
	)

	// FinancialCoreResponseReceived records a validated Financial Core
	// checkout payment session response (payment submitted, not succeeded).
	FinancialCoreResponseReceived(
		ctx context.Context,
		contractID model.ContractID,
		paymentAttemptID model.PaymentAttemptID,
	)

	// FinancialCoreFailed records a failed or timed-out Financial Core
	// checkout payment session request.
	FinancialCoreFailed(
		ctx context.Context,
		contractID model.ContractID,
		paymentAttemptID model.PaymentAttemptID,
		err error,
	)

	// PaymentURLObtained records a committed payment session whose payment URL
	// is available to the customer (payment submitted, not succeeded).
	PaymentURLObtained(
		ctx context.Context,
		contractID model.ContractID,
		paymentAttemptID model.PaymentAttemptID,
	)
}
