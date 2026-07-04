package paymentattemptrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.PaymentAttempt = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the payment attempt repository.
type Repo struct{}

// New returns a stateless PostgreSQL payment attempt repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.PaymentAttemptConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.PaymentAttemptTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.PaymentAttemptConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByInstallmentIDAndIdempotencyKey(
	ctx context.Context,
	installmentID model.InstallmentID,
	idempotencyKey string,
) (model.PaymentAttempt, error) {
	return GetByInstallmentIDAndIdempotencyKey(ctx, q.Conn, installmentID, idempotencyKey)
}

func (q connQueryer) GetByFinancialPaymentIntentID(
	ctx context.Context,
	financialPaymentIntentID string,
) (model.PaymentAttempt, error) {
	return GetByFinancialPaymentIntentID(ctx, q.Conn, financialPaymentIntentID)
}

func (q connQueryer) ListFinalizedInstallmentPaymentsByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.PaymentAttempt, error) {
	return ListFinalizedInstallmentPaymentsByCustomerID(ctx, q.Conn, customerID, page)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.PaymentAttemptTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByInstallmentIDAndIdempotencyKey(
	ctx context.Context,
	installmentID model.InstallmentID,
	idempotencyKey string,
) (model.PaymentAttempt, error) {
	return GetByInstallmentIDAndIdempotencyKey(ctx, q.Tx, installmentID, idempotencyKey)
}

func (q txQueryer) GetByFinancialPaymentIntentID(
	ctx context.Context,
	financialPaymentIntentID string,
) (model.PaymentAttempt, error) {
	return GetByFinancialPaymentIntentID(ctx, q.Tx, financialPaymentIntentID)
}

func (q txQueryer) ListFinalizedInstallmentPaymentsByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.PaymentAttempt, error) {
	return ListFinalizedInstallmentPaymentsByCustomerID(ctx, q.Tx, customerID, page)
}

func (q txQueryer) GetByIDWithItems(ctx context.Context, id model.PaymentAttemptID) (model.PaymentAttempt, error) {
	return GetByIDWithItems(ctx, q.Tx, id)
}

func (q txQueryer) LockByID(ctx context.Context, id model.PaymentAttemptID) (model.PaymentAttempt, error) {
	return LockByID(ctx, q.Tx, id)
}

func (q txQueryer) LockByIDWithItems(ctx context.Context, id model.PaymentAttemptID) (model.PaymentAttempt, error) {
	return LockByIDWithItems(ctx, q.Tx, id)
}

func (q txQueryer) GetByFinancialPaymentIntentIDWithItems(
	ctx context.Context,
	financialPaymentIntentID string,
) (model.PaymentAttempt, error) {
	return GetByFinancialPaymentIntentIDWithItems(ctx, q.Tx, financialPaymentIntentID)
}

func (q txQueryer) GetByInstallmentIDAndIdempotencyKeyWithItems(
	ctx context.Context,
	installmentID model.InstallmentID,
	idempotencyKey string,
) (model.PaymentAttempt, error) {
	return GetByInstallmentIDAndIdempotencyKeyWithItems(ctx, q.Tx, installmentID, idempotencyKey)
}

func (q txQueryer) GetByContractIDAndPurposeWithItems(
	ctx context.Context,
	contractID model.ContractID,
	purpose model.FinancialCorePaymentPurpose,
) (model.PaymentAttempt, error) {
	return GetByContractIDAndPurposeWithItems(ctx, q.Tx, contractID, purpose)
}

func (q txQueryer) Create(
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
) (model.PaymentAttempt, error) {
	return Create(
		ctx,
		q.Tx,
		contractID,
		installmentID,
		amountMinor,
		currency,
		state,
		channel,
		idempotencyKey,
		requestFingerprint,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) CreateWithItems(
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
) (model.PaymentAttempt, error) {
	return CreateWithItems(
		ctx,
		q.Tx,
		contractID,
		installmentID,
		amountMinor,
		currency,
		state,
		channel,
		idempotencyKey,
		requestFingerprint,
		items,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) CreateCheckoutWithItems(
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
) (model.PaymentAttempt, error) {
	return CreateCheckoutWithItems(
		ctx, q.Tx, contractID, installmentID, amountMinor, currency, channel,
		idempotencyKey, requestFingerprint, tenantID, contextID, providerCode,
		successReturnURL, failureReturnURL, cancelReturnURL, items, createdAt, updatedAt,
	)
}

func (q txQueryer) BindCollectionObligationIfAbsent(
	ctx context.Context,
	id model.PaymentAttemptID,
	collectionObligationID string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return BindCollectionObligationIfAbsent(ctx, q.Tx, id, collectionObligationID, updatedAt)
}

func (q txQueryer) BindFinancialRequestContextIfAbsent(
	ctx context.Context,
	id model.PaymentAttemptID,
	tenantID string,
	contextID string,
	providerCode *string,
	successReturnURL string,
	failureReturnURL string,
	cancelReturnURL string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return BindFinancialRequestContextIfAbsent(
		ctx, q.Tx, id, tenantID, contextID, providerCode, successReturnURL, failureReturnURL, cancelReturnURL, updatedAt,
	)
}

func (q txQueryer) MarkSubmitted(
	ctx context.Context,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID string,
	paymentURL *string,
	financialPaymentExpiresAt *time.Time,
	submittedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkSubmitted(
		ctx,
		q.Tx,
		id,
		financialPaymentIntentID,
		financialPaymentSessionID,
		paymentURL,
		financialPaymentExpiresAt,
		submittedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkSucceeded(
	ctx context.Context,
	id model.PaymentAttemptID,
	financialPaymentAttemptID *string,
	financialPaidAt time.Time,
	appliedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkSucceeded(
		ctx,
		q.Tx,
		id,
		financialPaymentAttemptID,
		financialPaidAt,
		appliedAt,
		updatedAt,
	)
}

func (q txQueryer) EnrichFinancialPaymentIdentityIfAbsent(
	ctx context.Context,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID *string,
	financialPaymentAttemptID *string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return EnrichFinancialPaymentIdentityIfAbsent(
		ctx,
		q.Tx,
		id,
		financialPaymentIntentID,
		financialPaymentSessionID,
		financialPaymentAttemptID,
		updatedAt,
	)
}

func (q txQueryer) EnrichFinancialPaymentSessionIfAbsent(
	ctx context.Context,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID string,
	financialPaymentAttemptID *string,
	paymentURL string,
	financialPaymentExpiresAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return EnrichFinancialPaymentSessionIfAbsent(
		ctx, q.Tx, id, financialPaymentIntentID, financialPaymentSessionID,
		financialPaymentAttemptID, paymentURL, financialPaymentExpiresAt, updatedAt,
	)
}

func (q txQueryer) MarkFailed(
	ctx context.Context,
	id model.PaymentAttemptID,
	failureCode string,
	failedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkFailed(ctx, q.Tx, id, failureCode, failedAt, updatedAt)
}

func (q txQueryer) MarkCanceled(
	ctx context.Context,
	id model.PaymentAttemptID,
	failureCode *string,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkCanceled(ctx, q.Tx, id, failureCode, canceledAt, updatedAt)
}

func (q txQueryer) MarkReconciliationRequired(
	ctx context.Context,
	id model.PaymentAttemptID,
	reason model.PaymentReconciliationReason,
	financialPaymentAttemptID *string,
	financialPaidAt *time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkReconciliationRequired(
		ctx,
		q.Tx,
		id,
		reason,
		financialPaymentAttemptID,
		financialPaidAt,
		updatedAt,
	)
}

func (q txQueryer) ClaimStaleCreatedForReconciliation(
	ctx context.Context,
	cutoff time.Time,
	reason model.PaymentReconciliationReason,
	requestedAt time.Time,
	nextReconciliationAt time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.PaymentAttempt, error) {
	return ClaimStaleCreatedForReconciliation(
		ctx,
		q.Tx,
		cutoff,
		reason,
		requestedAt,
		nextReconciliationAt,
		updatedAt,
		limit,
	)
}

func (q txQueryer) ClaimExpiredSubmittedForReconciliation(
	ctx context.Context,
	now time.Time,
	expiryGrace time.Duration,
	reason model.PaymentReconciliationReason,
	requestedAt time.Time,
	nextReconciliationAt time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.PaymentAttempt, error) {
	return ClaimExpiredSubmittedForReconciliation(
		ctx,
		q.Tx,
		now,
		expiryGrace,
		reason,
		requestedAt,
		nextReconciliationAt,
		updatedAt,
		limit,
	)
}

func (q txQueryer) ClaimDueReconciliationRequired(
	ctx context.Context,
	now time.Time,
	claimLeaseUntil time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.PaymentAttempt, error) {
	return ClaimDueReconciliationRequired(
		ctx,
		q.Tx,
		now,
		claimLeaseUntil,
		updatedAt,
		limit,
	)
}

func (q txQueryer) RecordReconciliationRequest(
	ctx context.Context,
	id model.PaymentAttemptID,
	previousAttemptCount int,
	requestedAt time.Time,
	nextReconciliationAt time.Time,
	lastError *string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return RecordReconciliationRequest(
		ctx, q.Tx, id, previousAttemptCount, requestedAt, nextReconciliationAt, lastError, updatedAt,
	)
}

func (q txQueryer) MarkSubmittedFromReconciliationRequired(
	ctx context.Context,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID string,
	paymentURL *string,
	financialPaymentExpiresAt *time.Time,
	submittedAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkSubmittedFromReconciliationRequired(
		ctx,
		q.Tx,
		id,
		financialPaymentIntentID,
		financialPaymentSessionID,
		paymentURL,
		financialPaymentExpiresAt,
		submittedAt,
		resolvedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkSucceededFromReconciliationRequired(
	ctx context.Context,
	id model.PaymentAttemptID,
	financialPaymentAttemptID *string,
	financialPaidAt time.Time,
	appliedAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkSucceededFromReconciliationRequired(
		ctx,
		q.Tx,
		id,
		financialPaymentAttemptID,
		financialPaidAt,
		appliedAt,
		resolvedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkFailedFromReconciliationRequired(
	ctx context.Context,
	id model.PaymentAttemptID,
	failureCode string,
	failedAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkFailedFromReconciliationRequired(
		ctx,
		q.Tx,
		id,
		failureCode,
		failedAt,
		resolvedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkCanceledFromReconciliationRequired(
	ctx context.Context,
	id model.PaymentAttemptID,
	failureCode *string,
	canceledAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkCanceledFromReconciliationRequired(
		ctx,
		q.Tx,
		id,
		failureCode,
		canceledAt,
		resolvedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkManualReviewRequired(
	ctx context.Context,
	id model.PaymentAttemptID,
	lastError string,
	requiredAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkManualReviewRequired(ctx, q.Tx, id, lastError, requiredAt, updatedAt)
}

func (q txQueryer) ScheduleReconciliationRetry(
	ctx context.Context,
	id model.PaymentAttemptID,
	nextReconciliationAt time.Time,
	lastError *string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return ScheduleReconciliationRetry(ctx, q.Tx, id, nextReconciliationAt, lastError, updatedAt)
}

func (q txQueryer) MarkCanceledFromCreated(
	ctx context.Context,
	id model.PaymentAttemptID,
	failureCode string,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	return MarkCanceledFromCreated(ctx, q.Tx, id, failureCode, canceledAt, updatedAt)
}

func (q txQueryer) HasBlockingAttemptForContract(
	ctx context.Context,
	contractID model.ContractID,
) (bool, error) {
	return HasBlockingAttemptForContract(ctx, q.Tx, contractID)
}
