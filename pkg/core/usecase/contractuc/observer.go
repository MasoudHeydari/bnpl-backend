package contractuc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// contractActivationSourceMerchantFunding labels activations driven by Financial
// Core merchant-funding recognition.
const contractActivationSourceMerchantFunding = "merchant_funding"

// Observer records operational flow checkpoints for contract origination,
// Financial Core payment-session submission, cancellation, and expiry.
// Implementations must not modify domain state or return errors that affect
// business execution.
//
// Success checkpoints are emitted only after the corresponding database
// transaction committed. Payment submission (a Financial Core payment session
// exists and a payment URL was obtained) is distinct from payment success:
// payment success and contract activation after a Financial Core payment result
// are observed by financialcorepaymentresultuc.
type Observer interface {
	// PreparationStarted records the start of contract preparation for a decision.
	PreparationStarted(ctx context.Context, decisionID model.DecisionID)

	// PaymentPendingCreated records a committed payment_pending contract.
	PaymentPendingCreated(ctx context.Context, contractID model.ContractID, decisionID model.DecisionID)

	// ContractReplayed records an idempotent replay of an existing contract.
	ContractReplayed(ctx context.Context, contractID model.ContractID, status model.ContractStatus)

	// PaymentAttemptCreated records a committed replacement checkout payment attempt.
	PaymentAttemptCreated(
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

	// FinancialCoreResponseReceived records a validated Financial Core checkout
	// payment session response (payment submitted, not succeeded).
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

	// ContractActivated records a committed contract activation driven by an
	// authoritative Financial Core result (merchant funding recognition).
	ContractActivated(ctx context.Context, contractID model.ContractID, source string)

	// CancellationSagaStarted records a committed cancellation saga start.
	CancellationSagaStarted(ctx context.Context, contractID model.ContractID, sagaID model.SagaID)

	// CancellationSagaCompleted records a committed cancellation saga completion.
	CancellationSagaCompleted(ctx context.Context, contractID model.ContractID, sagaID model.SagaID)

	// CancellationSagaFailed records a failed cancellation saga execution attempt.
	CancellationSagaFailed(
		ctx context.Context,
		contractID model.ContractID,
		sagaID model.SagaID,
		err error,
	)

	// ContractCanceled records a committed contract cancellation.
	ContractCanceled(ctx context.Context, contractID model.ContractID)

	// ContractExpired records a committed payment_pending contract expiry.
	ContractExpired(ctx context.Context, contractID model.ContractID)

	// PaymentPendingExpiryCompleted records the end of a payment_pending expiry sweep.
	PaymentPendingExpiryCompleted(ctx context.Context, expiredCount int, failedCount int)
}
