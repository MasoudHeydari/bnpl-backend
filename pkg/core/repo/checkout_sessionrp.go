package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutSessionQueryer defines read-only checkout session operations.
//
// The repository also owns the two merchant configuration tables the hosted
// checkout boundary depends on: pre-registered return destinations and the
// merchant webhook destination. They exist only to serve checkout handoffs, so
// keeping them behind one port avoids spreading the boundary across repos.
type CheckoutSessionQueryer interface {
	// GetByID returns the checkout session identified by id.
	GetByID(ctx context.Context, id model.CheckoutSessionID) (model.CheckoutSession, error)

	// GetByPublicToken returns the checkout session addressed by its public
	// token. Unknown tokens are reported as cerr.ResourceNotFound.
	GetByPublicToken(ctx context.Context, token model.CheckoutSessionToken) (model.CheckoutSession, error)

	// GetByMerchantIdempotencyKey returns the checkout session created by the
	// merchant's create request identified by idempotencyKey.
	GetByMerchantIdempotencyKey(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		idempotencyKey string,
	) (model.CheckoutSession, error)

	// GetByMerchantOrderID returns the current checkout attempt for merchantOrderID:
	// the completed attempt if one exists (to prevent refinancing fulfilled
	// orders), otherwise the active attempt or newest terminal attempt.
	GetByMerchantOrderID(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		merchantOrderID string,
	) (model.CheckoutSession, error)

	// GetByContractID returns the checkout session linked to contractID.
	GetByContractID(ctx context.Context, contractID model.ContractID) (model.CheckoutSession, error)

	// GetWebhookDestination returns the enabled webhook destination registered
	// for the merchant. A merchant has at most one destination.
	GetWebhookDestination(
		ctx context.Context,
		merchantID model.MerchantID,
	) (model.MerchantWebhookDestination, error)

	// FindWebhookDestination returns the merchant's webhook destination whether
	// or not it is enabled, so the dispatcher can distinguish a disabled
	// destination (permanent, auditable) from a missing one. Returns
	// cerr.ResourceNotFound when the merchant has none.
	FindWebhookDestination(
		ctx context.Context,
		merchantID model.MerchantID,
	) (model.MerchantWebhookDestination, error)

	// GetReturnDestination returns the enabled return destination registered under destinationName.
	GetReturnDestination(
		ctx context.Context,
		merchantID model.MerchantID,
		destinationName string,
	) (model.MerchantReturnDestination, error)
}

// CheckoutSessionConnQueryer exposes checkout session operations on a connection.
type CheckoutSessionConnQueryer interface {
	// CheckoutSessionQueryer provides shared checkout session query operations.
	CheckoutSessionQueryer
}

// CheckoutSessionTxQueryer exposes transaction-scoped checkout session operations.
type CheckoutSessionTxQueryer interface {
	// CheckoutSessionQueryer provides shared checkout session query operations.
	CheckoutSessionQueryer

	// LockByID returns and locks a checkout session row for update.
	LockByID(ctx context.Context, id model.CheckoutSessionID) (model.CheckoutSession, error)

	// LockByPublicToken returns and locks a checkout session row addressed by
	// its public token.
	LockByPublicToken(ctx context.Context, token model.CheckoutSessionToken) (model.CheckoutSession, error)

	// LockByContractID returns and locks the checkout session linked to contractID.
	LockByContractID(ctx context.Context, contractID model.ContractID) (model.CheckoutSession, error)

	// LockByMerchantIdempotencyKey returns and locks the checkout session created
	// by the merchant's create request identified by idempotencyKey.
	LockByMerchantIdempotencyKey(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		idempotencyKey string,
	) (model.CheckoutSession, error)

	// LockByMerchantOrderID locks the completed attempt first, then active,
	// otherwise the latest terminal attempt. Completed history must not be hidden.
	LockByMerchantOrderID(
		ctx context.Context,
		merchantProfileID model.MerchantProfileID,
		merchantOrderID string,
	) (model.CheckoutSession, error)

	// Create stores a checkout session inside the current transaction.
	//
	// A duplicate (merchant, idempotencyKey) or a second active attempt for the
	// same merchant order is reported as a cerr.Conflict.
	//
	// customerPhoneCiphertext and customerPhoneKeyID carry the envelope-encrypted
	// customer phone of an external partner session. Both are nil together for
	// internal sessions, which never store a phone.
	Create(
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
		offerPreparationMode model.OfferPreparationMode,
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
	) (model.CheckoutSession, error)

	// UpdateStatus transitions a checkout session to status. It stamps
	// completed_at when status is completed. Use MarkCanceled to cancel and
	// MarkConfirming to enter confirming.
	UpdateStatus(
		ctx context.Context,
		id model.CheckoutSessionID,
		status model.CheckoutSessionStatus,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkConfirming atomically moves an open checkout session with a linked
	// Decision to confirming so Confirm owns it. Any other state is a cerr.Conflict.
	MarkConfirming(
		ctx context.Context,
		id model.CheckoutSessionID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkCanceled transitions an open checkout session to canceled, records who
	// canceled it, and stamps canceled_at. Confirming, processing, and terminal
	// sessions are never matched.
	MarkCanceled(
		ctx context.Context,
		id model.CheckoutSessionID,
		reason model.CheckoutCancellationReason,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// ExpireOpenDue expires up to limit open checkout sessions whose expires_at is
	// at or before now and returns them. Confirming and processing sessions are
	// skipped.
	ExpireOpenDue(ctx context.Context, now time.Time, limit int) ([]model.CheckoutSession, error)

	// ReopenStaleConfirmingWithoutContract is the legacy-named recovery scan.
	// It locks stale unlinked confirming claims and eligible processing sessions;
	// the use case must never reopen a claim solely because the Contract has
	// not yet committed. The operation itself makes no state transition.
	ReopenStaleConfirmingWithoutContract(
		ctx context.Context,
		now time.Time,
		staleBefore time.Time,
		limit int,
	) ([]model.CheckoutSession, error)

	// BumpEvaluationGenerationCAS increments the evaluation generation and clears
	// the Decision link so the next evaluation starts fresh. Rows that already
	// have a Contract linked are never matched: accepted financing terms stay
	// immutable for payment retry. It is a compare-and-swap: the row must still
	// be open, at expectedGeneration, linked to expectedDecisionID (nil means no
	// Decision linked yet), and have contract_id NULL. A lost race is reported as
	// a cerr.Conflict.
	BumpEvaluationGenerationCAS(
		ctx context.Context,
		id model.CheckoutSessionID,
		expectedGeneration int,
		expectedDecisionID *model.DecisionID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// UpsertReturnDestination registers or updates a merchant return destination.
	// A nil enabled preserves the stored value on update and defaults to true on
	// insert, so omission never silently disables a destination.
	UpsertReturnDestination(
		ctx context.Context,
		merchantID model.MerchantID,
		destinationName string,
		returnURL string,
		enabled *bool,
		now time.Time,
	) (model.MerchantReturnDestination, error)

	// UpsertWebhookDestination registers or updates the merchant's single webhook
	// destination. The signing secret is stored envelope-encrypted. An empty
	// ciphertext on update preserves the stored secret rather than wiping it.
	UpsertWebhookDestination(
		ctx context.Context,
		merchantID model.MerchantID,
		webhookURL string,
		signingSecretCiphertext string,
		signingSecretKeyID string,
		enabled bool,
		now time.Time,
	) (model.MerchantWebhookDestination, error)

	// BindCustomer attaches the trusted customer identity to a checkout session
	// without allowing a conflicting overwrite.
	BindCustomer(
		ctx context.Context,
		id model.CheckoutSessionID,
		customerID model.CustomerID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// ConsumeOTPRateLimit consumes one cross-checkout OTP send from the durable
	// fixed-window bucket identified by scope and scopeKey. It must run in the
	// same transaction as IssueOTP. Exceeding the budget returns
	// cerr.CodeTooManyRequests without mutating the counter past the max.
	ConsumeOTPRateLimit(
		ctx context.Context,
		scope model.CheckoutOTPRateLimitScope,
		scopeKey string,
		window time.Duration,
		maxSends int,
		now time.Time,
	) error

	// IssueOTP stores a newly issued phone OTP challenge on an open checkout
	// session under a fresh opaque issueID, binds it to evaluationGeneration,
	// and increments the per-checkout send count. It resets every other OTP
	// field (accepted/verified/request id cleared, attempt count zeroed) so each
	// issue starts a clean attempt budget.
	IssueOTP(
		ctx context.Context,
		id model.CheckoutSessionID,
		issueID model.OTPIssueID,
		verifier string,
		evaluationGeneration int,
		expiresAt time.Time,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkOTPNotificationAccepted records that Notification durably accepted
	// submission of the OTP issuance identified by issueID. Stale issue IDs
	// must not update a newer challenge.
	MarkOTPNotificationAccepted(
		ctx context.Context,
		id model.CheckoutSessionID,
		issueID model.OTPIssueID,
		acceptedAt time.Time,
		requestID string,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// ClearOTPIssue discards the OTP issuance identified by issueID after a
	// definite Notification rejection. Stale failure handlers must not clear a
	// newer issuance.
	ClearOTPIssue(
		ctx context.Context,
		id model.CheckoutSessionID,
		issueID model.OTPIssueID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// RecordOTPFailure atomically increments the failed-attempt counter of the
	// issued OTP.
	RecordOTPFailure(
		ctx context.Context,
		id model.CheckoutSessionID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkOTPVerified records proof of phone possession and clears the verifier
	// and its expiry so the code cannot be replayed.
	MarkOTPVerified(
		ctx context.Context,
		id model.CheckoutSessionID,
		verifiedAt time.Time,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// LinkDecision attaches the personalized checkout Decision to a checkout session.
	LinkDecision(
		ctx context.Context,
		id model.CheckoutSessionID,
		decisionID model.DecisionID,
		expectedGeneration int,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// ClampExpiresAt shortens the session lifetime to expiresAt when the stored
	// expires_at is later. Used so an External checkout cannot outlive the
	// initially prepared offer. A no-op when expiresAt is not earlier.
	ClampExpiresAt(
		ctx context.Context,
		id model.CheckoutSessionID,
		expiresAt time.Time,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// LinkContract attaches the BNPL contract created from the accepted offer.
	LinkContract(
		ctx context.Context,
		id model.CheckoutSessionID,
		contractID model.ContractID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)
}

// CheckoutSession persists merchant checkout handoffs and the merchant
// configuration the handoff boundary depends on.
type CheckoutSession interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) CheckoutSessionConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) CheckoutSessionTxQueryer
}
