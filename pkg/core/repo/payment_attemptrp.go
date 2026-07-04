package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// PaymentAttemptQueryer defines read-only payment attempt operations.
type PaymentAttemptQueryer interface {
	// GetByInstallmentIDAndIdempotencyKey returns a payment attempt by installment and key.
	GetByInstallmentIDAndIdempotencyKey(
		ctx context.Context,
		installmentID model.InstallmentID,
		idempotencyKey string,
	) (model.PaymentAttempt, error)

	// GetByFinancialPaymentIntentID returns a payment attempt by Financial Core intent id.
	GetByFinancialPaymentIntentID(
		ctx context.Context,
		financialPaymentIntentID string,
	) (model.PaymentAttempt, error)

	// ListFinalizedInstallmentPaymentsByCustomerID returns finalized installment
	// payment attempts owned by the customer, newest first (created_at DESC, id DESC).
	//
	// Only states succeeded, failed, and canceled are included, and only purposes
	// checkout_first_installment and installment_repayment. Ownership is resolved by
	// joining installments.customer_id.
	ListFinalizedInstallmentPaymentsByCustomerID(
		ctx context.Context,
		customerID model.CustomerID,
		page pagination.KeysetRequest,
	) ([]model.PaymentAttempt, error)
}

// PaymentAttemptConnQueryer exposes payment attempt operations on a connection.
type PaymentAttemptConnQueryer interface {
	// PaymentAttemptQueryer provides shared payment attempt query operations.
	PaymentAttemptQueryer
}

// PaymentAttemptTxQueryer exposes transaction-scoped payment attempt operations.
type PaymentAttemptTxQueryer interface {
	// PaymentAttemptQueryer provides shared payment attempt query operations.
	PaymentAttemptQueryer

	// GetByIDWithItems returns a payment attempt and its immutable item snapshot without locking.
	GetByIDWithItems(ctx context.Context, id model.PaymentAttemptID) (model.PaymentAttempt, error)

	// LockByID returns and locks a payment attempt row for update.
	LockByID(ctx context.Context, id model.PaymentAttemptID) (model.PaymentAttempt, error)

	// LockByIDWithItems returns and locks a payment attempt with its snapshotted items.
	LockByIDWithItems(ctx context.Context, id model.PaymentAttemptID) (model.PaymentAttempt, error)

	// GetByFinancialPaymentIntentIDWithItems returns a payment attempt and items by intent id.
	GetByFinancialPaymentIntentIDWithItems(
		ctx context.Context,
		financialPaymentIntentID string,
	) (model.PaymentAttempt, error)

	// GetByInstallmentIDAndIdempotencyKeyWithItems returns a payment attempt and items by idempotency key.
	GetByInstallmentIDAndIdempotencyKeyWithItems(
		ctx context.Context,
		installmentID model.InstallmentID,
		idempotencyKey string,
	) (model.PaymentAttempt, error)

	// GetByContractIDAndPurposeWithItems returns the payment attempt for a contract purpose.
	GetByContractIDAndPurposeWithItems(
		ctx context.Context,
		contractID model.ContractID,
		purpose model.FinancialCorePaymentPurpose,
	) (model.PaymentAttempt, error)

	// Create stores a payment attempt row inside the current transaction.
	Create(
		ctx context.Context,
		contractID model.ContractID,
		installmentID model.InstallmentID,
		amountMinor uint64,
		currency model.Currency,
		state model.PaymentAttemptState,
		channel model.PaymentChannel,
		idempotencyKey string,
		requestFingerprint string,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// CreateWithItems stores a payment attempt and its snapshotted obligation items atomically.
	CreateWithItems(
		ctx context.Context,
		contractID model.ContractID,
		installmentID model.InstallmentID,
		amountMinor uint64,
		currency model.Currency,
		state model.PaymentAttemptState,
		channel model.PaymentChannel,
		idempotencyKey string,
		requestFingerprint string,
		items []model.PaymentAttemptItem,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// CreateCheckoutWithItems stores a checkout-first-installment attempt and request context.
	CreateCheckoutWithItems(
		ctx context.Context,
		contractID model.ContractID,
		installmentID model.InstallmentID,
		amountMinor uint64,
		currency model.Currency,
		channel model.PaymentChannel,
		idempotencyKey string,
		requestFingerprint string,
		tenantID string,
		contextID string,
		providerCode *string,
		successReturnURL string,
		failureReturnURL string,
		cancelReturnURL string,
		items []model.PaymentAttemptItem,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// BindCollectionObligationIfAbsent stores the prepared Financial Core collection obligation id without allowing a conflicting overwrite.
	BindCollectionObligationIfAbsent(
		ctx context.Context,
		id model.PaymentAttemptID,
		collectionObligationID string,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// BindFinancialRequestContextIfAbsent stores the Financial Core scope/provider/return-url context for retries and authoritative status lookups.
	BindFinancialRequestContextIfAbsent(
		ctx context.Context,
		id model.PaymentAttemptID,
		tenantID string,
		contextID string,
		providerCode *string,
		successReturnURL string,
		failureReturnURL string,
		cancelReturnURL string,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkSubmitted persists Financial Core session identifiers and submitted state.
	MarkSubmitted(
		ctx context.Context,
		id model.PaymentAttemptID,
		financialPaymentIntentID string,
		financialPaymentSessionID string,
		paymentURL *string,
		financialPaymentExpiresAt *time.Time,
		submittedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkSucceeded transitions a submitted attempt to succeeded.
	MarkSucceeded(
		ctx context.Context,
		id model.PaymentAttemptID,
		financialPaymentAttemptID *string,
		financialPaidAt time.Time,
		appliedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// EnrichFinancialPaymentIdentityIfAbsent fills missing Financial Core payment identity fields.
	EnrichFinancialPaymentIdentityIfAbsent(
		ctx context.Context,
		id model.PaymentAttemptID,
		financialPaymentIntentID string,
		financialPaymentSessionID *string,
		financialPaymentAttemptID *string,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// EnrichFinancialPaymentSessionIfAbsent fills missing Financial Core session metadata without changing state.
	EnrichFinancialPaymentSessionIfAbsent(
		ctx context.Context,
		id model.PaymentAttemptID,
		financialPaymentIntentID string,
		financialPaymentSessionID string,
		financialPaymentAttemptID *string,
		paymentURL string,
		financialPaymentExpiresAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkFailed transitions a submitted attempt to failed.
	MarkFailed(
		ctx context.Context,
		id model.PaymentAttemptID,
		failureCode string,
		failedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkCanceled transitions a submitted attempt to canceled.
	MarkCanceled(
		ctx context.Context,
		id model.PaymentAttemptID,
		failureCode *string,
		canceledAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkReconciliationRequired transitions an attempt to reconciliation_required.
	MarkReconciliationRequired(
		ctx context.Context,
		id model.PaymentAttemptID,
		reason model.PaymentReconciliationReason,
		financialPaymentAttemptID *string,
		financialPaidAt *time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// ClaimStaleCreatedForReconciliation claims and transitions stale created attempts.
	ClaimStaleCreatedForReconciliation(
		ctx context.Context,
		cutoff time.Time,
		reason model.PaymentReconciliationReason,
		requestedAt time.Time,
		nextReconciliationAt time.Time,
		updatedAt time.Time,
		limit int,
	) ([]model.PaymentAttempt, error)

	// ClaimExpiredSubmittedForReconciliation claims and transitions expired submitted attempts.
	ClaimExpiredSubmittedForReconciliation(
		ctx context.Context,
		now time.Time,
		expiryGrace time.Duration,
		reason model.PaymentReconciliationReason,
		requestedAt time.Time,
		nextReconciliationAt time.Time,
		updatedAt time.Time,
		limit int,
	) ([]model.PaymentAttempt, error)

	// ClaimDueReconciliationRequired claims due reconciliation_required attempts for retry.
	ClaimDueReconciliationRequired(
		ctx context.Context,
		now time.Time,
		claimLeaseUntil time.Time,
		updatedAt time.Time,
		limit int,
	) ([]model.PaymentAttempt, error)

	// RecordReconciliationRequest records a successfully queued reconciliation request
	// and advances retry metadata in the same transaction as the outbox insert.
	RecordReconciliationRequest(
		ctx context.Context,
		id model.PaymentAttemptID,
		previousAttemptCount int,
		requestedAt time.Time,
		nextReconciliationAt time.Time,
		lastError *string,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkSubmittedFromReconciliationRequired transitions reconciliation_required to submitted.
	MarkSubmittedFromReconciliationRequired(
		ctx context.Context,
		id model.PaymentAttemptID,
		financialPaymentIntentID string,
		financialPaymentSessionID string,
		paymentURL *string,
		financialPaymentExpiresAt *time.Time,
		submittedAt time.Time,
		resolvedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkSucceededFromReconciliationRequired transitions reconciliation_required to succeeded.
	MarkSucceededFromReconciliationRequired(
		ctx context.Context,
		id model.PaymentAttemptID,
		financialPaymentAttemptID *string,
		financialPaidAt time.Time,
		appliedAt time.Time,
		resolvedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkFailedFromReconciliationRequired transitions reconciliation_required to failed.
	MarkFailedFromReconciliationRequired(
		ctx context.Context,
		id model.PaymentAttemptID,
		failureCode string,
		failedAt time.Time,
		resolvedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkCanceledFromReconciliationRequired transitions reconciliation_required to canceled.
	MarkCanceledFromReconciliationRequired(
		ctx context.Context,
		id model.PaymentAttemptID,
		failureCode *string,
		canceledAt time.Time,
		resolvedAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkManualReviewRequired stops automatic reconciliation after retry exhaustion or a permanent local inconsistency.
	MarkManualReviewRequired(
		ctx context.Context,
		id model.PaymentAttemptID,
		lastError string,
		requiredAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// ScheduleReconciliationRetry updates retry metadata for reconciliation_required attempts.
	ScheduleReconciliationRetry(
		ctx context.Context,
		id model.PaymentAttemptID,
		nextReconciliationAt time.Time,
		lastError *string,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// MarkCanceledFromCreated transitions created to canceled.
	MarkCanceledFromCreated(
		ctx context.Context,
		id model.PaymentAttemptID,
		failureCode string,
		canceledAt time.Time,
		updatedAt time.Time,
	) (model.PaymentAttempt, error)

	// HasBlockingAttemptForContract reports whether the contract has a blocking payment attempt.
	HasBlockingAttemptForContract(ctx context.Context, contractID model.ContractID) (bool, error)
}

// PaymentAttempt persists local BNPL repayment orchestration records.
type PaymentAttempt interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) PaymentAttemptConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) PaymentAttemptTxQueryer
}
