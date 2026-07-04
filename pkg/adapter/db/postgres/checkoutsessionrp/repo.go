package checkoutsessionrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CheckoutSession = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the checkout session repository.
type Repo struct{}

// New returns a stateless PostgreSQL checkout session repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.CheckoutSessionConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.CheckoutSessionTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.CheckoutSessionConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(
	ctx context.Context,
	id model.CheckoutSessionID,
) (model.CheckoutSession, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) GetByPublicToken(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	return GetByPublicToken(ctx, q.Conn, token)
}

func (q connQueryer) GetByMerchantIdempotencyKey(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	idempotencyKey string,
) (model.CheckoutSession, error) {
	return GetByMerchantIdempotencyKey(ctx, q.Conn, merchantProfileID, idempotencyKey)
}

func (q connQueryer) GetByMerchantOrderID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	merchantOrderID string,
) (model.CheckoutSession, error) {
	return GetByMerchantOrderID(ctx, q.Conn, merchantProfileID, merchantOrderID)
}

func (q connQueryer) GetByContractID(
	ctx context.Context,
	contractID model.ContractID,
) (model.CheckoutSession, error) {
	return GetByContractID(ctx, q.Conn, contractID)
}

func (q connQueryer) GetWebhookDestination(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantWebhookDestination, error) {
	return GetWebhookDestination(ctx, q.Conn, merchantID)
}

func (q connQueryer) FindWebhookDestination(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantWebhookDestination, error) {
	return FindWebhookDestination(ctx, q.Conn, merchantID)
}

func (q connQueryer) GetReturnDestination(
	ctx context.Context,
	merchantID model.MerchantID,
	destinationName string,
) (model.MerchantReturnDestination, error) {
	return GetReturnDestination(ctx, q.Conn, merchantID, destinationName)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.CheckoutSessionTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(
	ctx context.Context,
	id model.CheckoutSessionID,
) (model.CheckoutSession, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) GetByPublicToken(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	return GetByPublicToken(ctx, q.Tx, token)
}

func (q txQueryer) GetByMerchantIdempotencyKey(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	idempotencyKey string,
) (model.CheckoutSession, error) {
	return GetByMerchantIdempotencyKey(ctx, q.Tx, merchantProfileID, idempotencyKey)
}

func (q txQueryer) GetByMerchantOrderID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	merchantOrderID string,
) (model.CheckoutSession, error) {
	return GetByMerchantOrderID(ctx, q.Tx, merchantProfileID, merchantOrderID)
}

func (q txQueryer) GetByContractID(
	ctx context.Context,
	contractID model.ContractID,
) (model.CheckoutSession, error) {
	return GetByContractID(ctx, q.Tx, contractID)
}

func (q txQueryer) GetWebhookDestination(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantWebhookDestination, error) {
	return GetWebhookDestination(ctx, q.Tx, merchantID)
}

func (q txQueryer) FindWebhookDestination(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantWebhookDestination, error) {
	return FindWebhookDestination(ctx, q.Tx, merchantID)
}

func (q txQueryer) GetReturnDestination(
	ctx context.Context,
	merchantID model.MerchantID,
	destinationName string,
) (model.MerchantReturnDestination, error) {
	return GetReturnDestination(ctx, q.Tx, merchantID, destinationName)
}

func (q txQueryer) LockByID(
	ctx context.Context,
	id model.CheckoutSessionID,
) (model.CheckoutSession, error) {
	return LockByID(ctx, q.Tx, id)
}

func (q txQueryer) LockByPublicToken(
	ctx context.Context,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	return LockByPublicToken(ctx, q.Tx, token)
}

func (q txQueryer) LockByMerchantIdempotencyKey(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	idempotencyKey string,
) (model.CheckoutSession, error) {
	return LockByMerchantIdempotencyKey(ctx, q.Tx, merchantProfileID, idempotencyKey)
}

func (q txQueryer) LockByMerchantOrderID(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	merchantOrderID string,
) (model.CheckoutSession, error) {
	return LockByMerchantOrderID(ctx, q.Tx, merchantProfileID, merchantOrderID)
}

func (q txQueryer) Create(
	ctx context.Context,
	merchantProfileID model.MerchantProfileID,
	merchantBranchProfileID model.MerchantBranchProfileID,
	onlineStoreID model.OnlineStoreID,
	merchantOrderID string,
	idempotencyKey string,
	publicToken model.CheckoutSessionToken,
	origin model.CheckoutSessionOrigin,
	customerReference string,
	customerPhoneCiphertext *string,
	customerPhoneKeyID *string,
	authTenantID *model.TenantID,
	authContextID *model.ContextID,
	customerID *model.CustomerID,
	returnDestination string,
	returnURL string,
	merchantState string,
	requestedAmountMinor uint64,
	currency model.Currency,
	status model.CheckoutSessionStatus,
	expiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return Create(
		ctx,
		q.Tx,
		merchantProfileID,
		merchantBranchProfileID,
		onlineStoreID,
		merchantOrderID,
		idempotencyKey,
		publicToken,
		origin,
		customerReference,
		customerPhoneCiphertext,
		customerPhoneKeyID,
		authTenantID,
		authContextID,
		customerID,
		returnDestination,
		returnURL,
		merchantState,
		requestedAmountMinor,
		currency,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

func (q txQueryer) UpdateStatus(
	ctx context.Context,
	id model.CheckoutSessionID,
	status model.CheckoutSessionStatus,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return UpdateStatus(ctx, q.Tx, id, status, updatedAt)
}

func (q txQueryer) BindCustomer(
	ctx context.Context,
	id model.CheckoutSessionID,
	customerID model.CustomerID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return BindCustomer(ctx, q.Tx, id, customerID, updatedAt)
}

func (q txQueryer) ConsumeOTPRateLimit(
	ctx context.Context,
	scope model.CheckoutOTPRateLimitScope,
	scopeKey string,
	window time.Duration,
	maxSends int,
	now time.Time,
) error {
	return ConsumeOTPRateLimit(ctx, q.Tx, scope, scopeKey, window, maxSends, now)
}

func (q txQueryer) IssueOTP(
	ctx context.Context,
	id model.CheckoutSessionID,
	issueID model.OTPIssueID,
	verifier string,
	evaluationGeneration int,
	expiresAt time.Time,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return IssueOTP(ctx, q.Tx, id, issueID, verifier, evaluationGeneration, expiresAt, updatedAt)
}

func (q txQueryer) MarkOTPNotificationAccepted(
	ctx context.Context,
	id model.CheckoutSessionID,
	issueID model.OTPIssueID,
	acceptedAt time.Time,
	requestID string,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return MarkOTPNotificationAccepted(ctx, q.Tx, id, issueID, acceptedAt, requestID, updatedAt)
}

func (q txQueryer) ClearOTPIssue(
	ctx context.Context,
	id model.CheckoutSessionID,
	issueID model.OTPIssueID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return ClearOTPIssue(ctx, q.Tx, id, issueID, updatedAt)
}

func (q txQueryer) RecordOTPFailure(
	ctx context.Context,
	id model.CheckoutSessionID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return RecordOTPFailure(ctx, q.Tx, id, updatedAt)
}

func (q txQueryer) MarkOTPVerified(
	ctx context.Context,
	id model.CheckoutSessionID,
	verifiedAt time.Time,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return MarkOTPVerified(ctx, q.Tx, id, verifiedAt, updatedAt)
}

func (q txQueryer) LinkDecision(
	ctx context.Context,
	id model.CheckoutSessionID,
	decisionID model.DecisionID,
	expectedGeneration int,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return LinkDecision(ctx, q.Tx, id, decisionID, expectedGeneration, updatedAt)
}

func (q txQueryer) ClampExpiresAt(
	ctx context.Context,
	id model.CheckoutSessionID,
	expiresAt time.Time,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return ClampExpiresAt(ctx, q.Tx, id, expiresAt, updatedAt)
}

func (q txQueryer) LinkContract(
	ctx context.Context,
	id model.CheckoutSessionID,
	contractID model.ContractID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return LinkContract(ctx, q.Tx, id, contractID, updatedAt)
}

func (q txQueryer) LockByContractID(
	ctx context.Context,
	contractID model.ContractID,
) (model.CheckoutSession, error) {
	return LockByContractID(ctx, q.Tx, contractID)
}

func (q txQueryer) MarkCanceled(
	ctx context.Context,
	id model.CheckoutSessionID,
	reason model.CheckoutCancellationReason,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return MarkCanceled(ctx, q.Tx, id, reason, updatedAt)
}

func (q txQueryer) ExpireOpenDue(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]model.CheckoutSession, error) {
	return ExpireOpenDue(ctx, q.Tx, now, limit)
}

func (q txQueryer) ReopenStaleConfirmingWithoutContract(
	ctx context.Context,
	now time.Time,
	staleBefore time.Time,
	limit int,
) ([]model.CheckoutSession, error) {
	return ReopenStaleConfirmingWithoutContract(ctx, q.Tx, now, staleBefore, limit)
}

func (q txQueryer) MarkConfirming(
	ctx context.Context,
	id model.CheckoutSessionID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return MarkConfirming(ctx, q.Tx, id, updatedAt)
}

func (q txQueryer) BumpEvaluationGenerationCAS(
	ctx context.Context,
	id model.CheckoutSessionID,
	expectedGeneration int,
	expectedDecisionID *model.DecisionID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	return BumpEvaluationGenerationCAS(ctx, q.Tx, id, expectedGeneration, expectedDecisionID, updatedAt)
}

func (q txQueryer) UpsertReturnDestination(
	ctx context.Context,
	merchantID model.MerchantID,
	destinationName string,
	returnURL string,
	enabled *bool,
	now time.Time,
) (model.MerchantReturnDestination, error) {
	return UpsertReturnDestination(ctx, q.Tx, merchantID, destinationName, returnURL, enabled, now)
}

func (q txQueryer) UpsertWebhookDestination(
	ctx context.Context,
	merchantID model.MerchantID,
	webhookURL string,
	signingSecretCiphertext string,
	signingSecretKeyID string,
	enabled bool,
	now time.Time,
) (model.MerchantWebhookDestination, error) {
	return UpsertWebhookDestination(
		ctx, q.Tx, merchantID, webhookURL, signingSecretCiphertext, signingSecretKeyID, enabled, now,
	)
}
