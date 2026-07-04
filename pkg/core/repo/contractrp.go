package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// ContractQueryer defines read-only contract operations.
type ContractQueryer interface {
	// GetByID returns a contract by id.
	GetByID(ctx context.Context, id model.ContractID) (model.Contract, error)

	// GetByIDAndCustomerID returns a contract by id owned by the customer profile.
	//
	// Missing contracts and contracts owned by another customer both map to
	// ResourceNotFound so callers cannot distinguish unauthorized ownership.
	GetByIDAndCustomerID(
		ctx context.Context,
		id model.ContractID,
		customerID model.CustomerID,
	) (model.Contract, error)

	// GetByDecisionID returns a contract by decision id.
	GetByDecisionID(ctx context.Context, decisionID model.DecisionID) (model.Contract, error)

	// GetByCheckoutKey returns a contract by canonical checkout key.
	GetByCheckoutKey(ctx context.Context, checkoutKey string) (model.Contract, error)

	// GetPaymentPendingByCreditAccountID returns the single in-flight checkout contract for a credit account.
	GetPaymentPendingByCreditAccountID(ctx context.Context, creditAccountID model.CreditAccountID) (model.Contract, error)

	// ListActiveByCustomerID returns current contracts (active or canceling) for a
	// customer profile ordered by created_at DESC, id DESC using keyset pagination.
	//
	// An empty result is not an error.
	ListActiveByCustomerID(
		ctx context.Context,
		customerID model.CustomerID,
		page pagination.KeysetRequest,
	) ([]model.Contract, error)

	// ListActiveByMerchantProfileID returns current contracts (active or canceling) for a merchant
	// profile (contracts.merchant_profile_id = merchant_profiles.id) ordered by
	// created_at DESC, id DESC using keyset pagination.
	//
	// An empty result is not an error.
	ListActiveByMerchantProfileID(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		page pagination.KeysetRequest,
	) ([]model.Contract, error)

	// ListActiveByMerchantProfileIDOffset returns one page of current contracts (active or canceling)
	// for a merchant profile ordered by created_at DESC, id DESC using
	// page/limit offset pagination.
	//
	// An empty result is not an error.
	ListActiveByMerchantProfileIDOffset(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		page pagination.OffsetRequest,
	) ([]model.Contract, error)

	// CountActiveByMerchantProfileID returns how many current contracts (active or canceling) belong
	// to the merchant profile. Used to build offset pagination totals.
	CountActiveByMerchantProfileID(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
	) (int64, error)
}

// ContractConnQueryer exposes contract operations on a connection.
type ContractConnQueryer interface {
	// ContractQueryer provides shared contract query operations.
	ContractQueryer
}

// ContractTxQueryer exposes transaction-scoped contract operations.
type ContractTxQueryer interface {
	// ContractQueryer provides shared contract query operations.
	ContractQueryer

	// LockByID returns and locks a contract row for update.
	LockByID(ctx context.Context, id model.ContractID) (model.Contract, error)

	// LockByDecisionID returns and locks a contract row by decision id for update.
	LockByDecisionID(ctx context.Context, decisionID model.DecisionID) (model.Contract, error)

	// Create stores a contract row inside the current transaction.
	Create(
		ctx context.Context,
		decisionID model.DecisionID,
		checkoutKey string,
		customerID model.CustomerID,
		creditAccountID model.CreditAccountID,
		merchantProfileID model.MerchantProfileID,
		merchantBranchProfileID *model.MerchantBranchProfileID,
		currency model.Currency,
		requestedAmountMinor uint64,
		approvedFinancedAmountMinor uint64,
		upfrontAmountMinor uint64,
		installmentCount int,
		payableNowAmountMinor uint64,
		remainingPrincipalAmountMinor uint64,
		status model.ContractStatus,
		expiresAt time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.Contract, bool, error)

	// BindSuccessfulCheckoutPaymentIdentity replaces stale checkout session identifiers with
	// the Financial Core identity that is about to activate the contract. The transaction
	// rolls this update back if activation does not complete.
	BindSuccessfulCheckoutPaymentIdentity(
		ctx context.Context,
		contractID model.ContractID,
		invoiceID string,
		paymentIntentID string,
		paymentSessionID string,
		financialPaymentAttemptID *string,
		updatedAt time.Time,
	) (model.Contract, error)

	// BindMerchantEntitlementIfAbsent stores the Financial Core merchant entitlement id without allowing a conflicting overwrite.
	BindMerchantEntitlementIfAbsent(
		ctx context.Context,
		id model.ContractID,
		merchantEntitlementID string,
		updatedAt time.Time,
	) (model.Contract, error)

	// BindReceivableIfAbsent stores the Financial Core receivable id without allowing a conflicting overwrite.
	BindReceivableIfAbsent(
		ctx context.Context,
		id model.ContractID,
		receivableID string,
		updatedAt time.Time,
	) (model.Contract, error)

	// MarkActive transitions a pre-activation contract to active when the governing checkout policy activation condition is satisfied.
	//
	// successfulFinancialPaymentAttemptID records the final Financial Core provider attempt
	// that settled the checkout payment as optional audit metadata; nil is stored as NULL.
	MarkActive(
		ctx context.Context,
		id model.ContractID,
		paymentReference *string,
		paymentID *model.PaymentAttemptID,
		successfulFinancialPaymentAttemptID *string,
		remainingPrincipalAmountMinor uint64,
		activatedAt time.Time,
		updatedAt time.Time,
	) (model.Contract, error)

	// EnrichSuccessfulFinancialPaymentAttemptID sets the final successful provider-attempt
	// id only when the stored value is currently NULL. It returns a conflict error when a
	// value already exists so callers never blindly overwrite existing audit metadata.
	EnrichSuccessfulFinancialPaymentAttemptID(
		ctx context.Context,
		id model.ContractID,
		successfulFinancialPaymentAttemptID string,
		updatedAt time.Time,
	) (model.Contract, error)

	// EnrichCheckoutPaymentSessionIfAbsent fills missing Financial Core checkout session
	// fields without overwriting non-empty stored identifiers.
	EnrichCheckoutPaymentSessionIfAbsent(
		ctx context.Context,
		contractID model.ContractID,
		invoiceID *string,
		paymentIntentID *string,
		paymentAttemptID *string,
		paymentSessionID *string,
		paymentLink *string,
		paymentExpiresAt *time.Time,
		reusedIdempotentResult *bool,
		updatedAt time.Time,
	) (model.Contract, error)

	// SaveCheckoutPaymentSession persists Financial Core checkout payment session data.
	SaveCheckoutPaymentSession(
		ctx context.Context,
		contractID model.ContractID,
		invoiceID string,
		paymentIntentID string,
		paymentAttemptID string,
		paymentSessionID string,
		paymentLink string,
		paymentExpiresAt time.Time,
		reusedIdempotentResult bool,
		updatedAt time.Time,
	) (model.Contract, error)

	// ClaimDuePaymentPending locks payment_pending contracts eligible for expiry.
	ClaimDuePaymentPending(
		ctx context.Context,
		now time.Time,
		processingGrace time.Duration,
		limit int,
	) ([]model.Contract, error)

	// ListDuePaymentPendingContractIDs returns contract ids eligible for expiry without locking.
	ListDuePaymentPendingContractIDs(
		ctx context.Context,
		now time.Time,
		processingGrace time.Duration,
		limit int,
	) ([]model.ContractID, error)

	// MarkExpired transitions a payment_pending contract to expired.
	MarkExpired(
		ctx context.Context,
		contractID model.ContractID,
		updatedAt time.Time,
	) (model.Contract, error)

	// MarkCanceledFromPaymentPending closes a pre-activation contract after an
	// authoritative checkout-payment cancellation. No credit exposure exists yet.
	MarkCanceledFromPaymentPending(
		ctx context.Context,
		contractID model.ContractID,
		canceledAt time.Time,
		updatedAt time.Time,
	) (model.Contract, error)

	// MarkCanceling transitions an active contract to canceling after a
	// customer-initiated cancellation request is accepted locally.
	//
	// Cancellation metadata, the Financial Core idempotency identity, and the
	// initial synchronous-dispatch lease are persisted atomically with the status
	// change. Exposure is not released here.
	MarkCanceling(
		ctx context.Context,
		contractID model.ContractID,
		idempotencyKey string,
		reason model.ContractCancellationReason,
		requestFingerprint string,
		financialCoreIdempotencyKey string,
		tenantID model.TenantID,
		contextID model.ContextID,
		requestedAt time.Time,
		updatedAt time.Time,
	) (model.Contract, error)

	// MarkCanceledAfterCancellation applies the successful authoritative synchronous
	// Financial Core result and transitions canceling to canceled.
	MarkCanceledAfterCancellation(
		ctx context.Context,
		contractID model.ContractID,
		canceledAt time.Time,
		updatedAt time.Time,
	) (model.Contract, error)

	// RestoreActiveAfterCancellationRejected compensates a local cancellation
	// start after a definitive external rejection. It transitions canceling back
	// to active and clears cancellation-request metadata so a later independent
	// cancellation request can start cleanly.
	RestoreActiveAfterCancellationRejected(
		ctx context.Context,
		contractID model.ContractID,
		updatedAt time.Time,
	) (model.Contract, error)

	// MarkCompleted transitions an active contract to completed.
	MarkCompleted(
		ctx context.Context,
		contractID model.ContractID,
		completedAt time.Time,
		updatedAt time.Time,
	) (model.Contract, error)

	// ReduceRemainingPrincipal decreases remaining contract principal with underflow protection.
	ReduceRemainingPrincipal(
		ctx context.Context,
		contractID model.ContractID,
		amountMinor uint64,
		updatedAt time.Time,
	) (model.Contract, error)
}

// Contract persists checkout BNPL contracts.
type Contract interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) ContractConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) ContractTxQueryer
}
