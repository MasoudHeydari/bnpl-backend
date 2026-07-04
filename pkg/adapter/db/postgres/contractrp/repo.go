package contractrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.Contract = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the contract repository.
type Repo struct{}

// New returns a stateless PostgreSQL contract repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.ContractConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.ContractTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.ContractConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(ctx context.Context, id model.ContractID) (model.Contract, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) GetByIDAndCustomerID(
	ctx context.Context,
	id model.ContractID,
	customerID model.CustomerID,
) (model.Contract, error) {
	return GetByIDAndCustomerID(ctx, q.Conn, id, customerID)
}

func (q connQueryer) GetByDecisionID(ctx context.Context, decisionID model.DecisionID) (model.Contract, error) {
	return GetByDecisionID(ctx, q.Conn, decisionID)
}

func (q connQueryer) GetByCheckoutKey(ctx context.Context, checkoutKey string) (model.Contract, error) {
	return GetByCheckoutKey(ctx, q.Conn, checkoutKey)
}

func (q connQueryer) GetPaymentPendingByCreditAccountID(ctx context.Context, creditAccountID model.CreditAccountID) (model.Contract, error) {
	return GetPaymentPendingByCreditAccountID(ctx, q.Conn, creditAccountID)
}

func (q connQueryer) ListActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.Contract, error) {
	return ListActiveByCustomerID(ctx, q.Conn, customerID, page)
}

func (q connQueryer) ListActiveByMerchantProfileID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	page pagination.KeysetRequest,
) ([]model.Contract, error) {
	return ListActiveByMerchantProfileID(ctx, q.Conn, merchantProfileID, page)
}

func (q connQueryer) ListActiveByMerchantProfileIDOffset(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	page pagination.OffsetRequest,
) ([]model.Contract, error) {
	return ListActiveByMerchantProfileIDOffset(ctx, q.Conn, merchantProfileID, page)
}

func (q connQueryer) CountActiveByMerchantProfileID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
) (int64, error) {
	return CountActiveByMerchantProfileID(ctx, q.Conn, merchantProfileID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.ContractTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(ctx context.Context, id model.ContractID) (model.Contract, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) GetByIDAndCustomerID(
	ctx context.Context,
	id model.ContractID,
	customerID model.CustomerID,
) (model.Contract, error) {
	return GetByIDAndCustomerID(ctx, q.Tx, id, customerID)
}

func (q txQueryer) GetByDecisionID(ctx context.Context, decisionID model.DecisionID) (model.Contract, error) {
	return GetByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) GetByCheckoutKey(ctx context.Context, checkoutKey string) (model.Contract, error) {
	return GetByCheckoutKey(ctx, q.Tx, checkoutKey)
}

func (q txQueryer) GetPaymentPendingByCreditAccountID(ctx context.Context, creditAccountID model.CreditAccountID) (model.Contract, error) {
	return GetPaymentPendingByCreditAccountID(ctx, q.Tx, creditAccountID)
}

func (q txQueryer) ListActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.Contract, error) {
	return ListActiveByCustomerID(ctx, q.Tx, customerID, page)
}

func (q txQueryer) ListActiveByMerchantProfileID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	page pagination.KeysetRequest,
) ([]model.Contract, error) {
	return ListActiveByMerchantProfileID(ctx, q.Tx, merchantProfileID, page)
}

func (q txQueryer) ListActiveByMerchantProfileIDOffset(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	page pagination.OffsetRequest,
) ([]model.Contract, error) {
	return ListActiveByMerchantProfileIDOffset(ctx, q.Tx, merchantProfileID, page)
}

func (q txQueryer) CountActiveByMerchantProfileID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
) (int64, error) {
	return CountActiveByMerchantProfileID(ctx, q.Tx, merchantProfileID)
}

func (q txQueryer) LockByID(ctx context.Context, id model.ContractID) (model.Contract, error) {
	return LockByID(ctx, q.Tx, id)
}

func (q txQueryer) LockByDecisionID(ctx context.Context, decisionID model.DecisionID) (model.Contract, error) {
	return LockByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) Create(
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
) (model.Contract, bool, error) {
	return Create(
		ctx,
		q.Tx,
		decisionID,
		checkoutKey,
		customerID,
		creditAccountID,
		merchantProfileID,
		merchantBranchProfileID,
		currency,
		requestedAmountMinor,
		approvedFinancedAmountMinor,
		upfrontAmountMinor,
		installmentCount,
		payableNowAmountMinor,
		remainingPrincipalAmountMinor,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) BindSuccessfulCheckoutPaymentIdentity(
	ctx context.Context,
	contractID model.ContractID,
	invoiceID string,
	paymentIntentID string,
	paymentSessionID string,
	financialPaymentAttemptID *string,
	updatedAt time.Time,
) (model.Contract, error) {
	return BindSuccessfulCheckoutPaymentIdentity(
		ctx,
		q.Tx,
		contractID,
		invoiceID,
		paymentIntentID,
		paymentSessionID,
		financialPaymentAttemptID,
		updatedAt,
	)
}

func (q txQueryer) BindMerchantEntitlementIfAbsent(
	ctx context.Context,
	id model.ContractID,
	merchantEntitlementID string,
	updatedAt time.Time,
) (model.Contract, error) {
	return BindMerchantEntitlementIfAbsent(
		ctx,
		q.Tx,
		id,
		merchantEntitlementID,
		updatedAt,
	)
}

func (q txQueryer) BindReceivableIfAbsent(
	ctx context.Context,
	id model.ContractID,
	receivableID string,
	updatedAt time.Time,
) (model.Contract, error) {
	return BindReceivableIfAbsent(
		ctx,
		q.Tx,
		id,
		receivableID,
		updatedAt,
	)
}

func (q txQueryer) MarkActive(
	ctx context.Context,
	id model.ContractID,
	paymentReference *string,
	paymentID *model.PaymentAttemptID,
	successfulFinancialPaymentAttemptID *string,
	remainingPrincipalAmountMinor uint64,
	activatedAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	return MarkActive(
		ctx,
		q.Tx,
		id,
		paymentReference,
		paymentID,
		successfulFinancialPaymentAttemptID,
		remainingPrincipalAmountMinor,
		activatedAt,
		updatedAt,
	)
}

func (q txQueryer) EnrichSuccessfulFinancialPaymentAttemptID(
	ctx context.Context,
	id model.ContractID,
	successfulFinancialPaymentAttemptID string,
	updatedAt time.Time,
) (model.Contract, error) {
	return EnrichSuccessfulFinancialPaymentAttemptID(
		ctx,
		q.Tx,
		id,
		successfulFinancialPaymentAttemptID,
		updatedAt,
	)
}

func (q txQueryer) EnrichCheckoutPaymentSessionIfAbsent(
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
) (model.Contract, error) {
	return EnrichCheckoutPaymentSessionIfAbsent(
		ctx,
		q.Tx,
		contractID,
		invoiceID,
		paymentIntentID,
		paymentAttemptID,
		paymentSessionID,
		paymentLink,
		paymentExpiresAt,
		reusedIdempotentResult,
		updatedAt,
	)
}

func (q txQueryer) SaveCheckoutPaymentSession(
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
) (model.Contract, error) {
	return SaveCheckoutPaymentSession(
		ctx,
		q.Tx,
		contractID,
		invoiceID,
		paymentIntentID,
		paymentAttemptID,
		paymentSessionID,
		paymentLink,
		paymentExpiresAt,
		reusedIdempotentResult,
		updatedAt,
	)
}

func (q txQueryer) ClaimDuePaymentPending(
	ctx context.Context,
	now time.Time,
	processingGrace time.Duration,
	limit int,
) ([]model.Contract, error) {
	return ClaimDuePaymentPending(
		ctx,
		q.Tx,
		now,
		processingGrace,
		limit,
	)
}

func (q txQueryer) MarkExpired(
	ctx context.Context,
	contractID model.ContractID,
	updatedAt time.Time,
) (model.Contract, error) {
	return MarkExpired(ctx, q.Tx, contractID, updatedAt)
}

func (q txQueryer) MarkCanceledFromPaymentPending(
	ctx context.Context,
	contractID model.ContractID,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	return MarkCanceledFromPaymentPending(
		ctx,
		q.Tx,
		contractID,
		canceledAt,
		updatedAt,
	)
}

func (q txQueryer) MarkCanceling(
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
) (model.Contract, error) {
	return MarkCanceling(
		ctx,
		q.Tx,
		contractID,
		idempotencyKey,
		reason,
		requestFingerprint,
		financialCoreIdempotencyKey,
		tenantID,
		contextID,
		requestedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkCanceledAfterCancellation(
	ctx context.Context,
	contractID model.ContractID,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	return MarkCanceledAfterCancellation(
		ctx,
		q.Tx,
		contractID,
		canceledAt,
		updatedAt,
	)
}

func (q txQueryer) RestoreActiveAfterCancellationRejected(
	ctx context.Context,
	contractID model.ContractID,
	updatedAt time.Time,
) (model.Contract, error) {
	return RestoreActiveAfterCancellationRejected(ctx, q.Tx, contractID, updatedAt)
}

func (q txQueryer) ListDuePaymentPendingContractIDs(
	ctx context.Context,
	now time.Time,
	processingGrace time.Duration,
	limit int,
) ([]model.ContractID, error) {
	return ListDuePaymentPendingContractIDs(
		ctx,
		q.Tx,
		now,
		processingGrace,
		limit,
	)
}

func (q txQueryer) MarkCompleted(
	ctx context.Context,
	contractID model.ContractID,
	completedAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	return MarkCompleted(
		ctx,
		q.Tx,
		contractID,
		completedAt,
		updatedAt,
	)
}

func (q txQueryer) ReduceRemainingPrincipal(
	ctx context.Context,
	contractID model.ContractID,
	amountMinor uint64,
	updatedAt time.Time,
) (model.Contract, error) {
	return ReduceRemainingPrincipal(
		ctx,
		q.Tx,
		contractID,
		amountMinor,
		updatedAt,
	)
}
