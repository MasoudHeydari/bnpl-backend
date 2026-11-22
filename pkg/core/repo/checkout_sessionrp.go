package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutSessionQueryer defines read-only checkout session operations.
//
// The repository also owns the merchant configuration table the hosted
// checkout boundary depends on: pre-registered return destinations. It exists
// only to serve checkout handoffs, so keeping it behind one port avoids
// spreading the boundary across repos.
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

	// GetReturnDestination returns the enabled return destination registered under destinationName.
	GetReturnDestination(
		ctx context.Context,
		merchantID model.ExternalMerchantID,
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
		onlineStoreID model.ExternalOnlineStoreID,
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

	// UpdateStatus transitions a checkout session to status for simple lifecycle
	// moves that do not stamp verification audit columns. Prefer the dedicated
	// MarkVerifyPending / MarkVerifiedByMerchant / MarkCancellationPending
	// methods for post-FC verification transitions.
	UpdateStatus(
		ctx context.Context,
		id model.CheckoutSessionID,
		status model.CheckoutSessionStatus,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkVerifyPending records durable FC success and starts the merchant
	// verification window. Only open/confirming/processing rows match. Rows
	// already in verify_pending/verified_by_merchant/cancellation_pending are
	// reported as Conflict so callers can treat them as idempotent recognition.
	MarkVerifyPending(
		ctx context.Context,
		id model.CheckoutSessionID,
		verificationPendingAt time.Time,
		verificationDeadlineAt time.Time,
		verificationPolicyVersion string,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkVerifiedByMerchant accepts merchant Verify while status is
	// verify_pending and now is strictly before verification_deadline_at.
	MarkVerifiedByMerchant(
		ctx context.Context,
		id model.CheckoutSessionID,
		terminalID model.MerchantTerminalID,
		verifiedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkCancellationPending moves due verify_pending rows to
	// cancellation_pending using the stored deadline (now >= deadline).
	MarkCancellationPending(
		ctx context.Context,
		id model.CheckoutSessionID,
		cancellationPendingAt time.Time,
	) (model.CheckoutSession, error)

	// ClaimDueVerifyPending locks up to limit verify_pending rows whose
	// verification_deadline_at is at or before now (FOR UPDATE SKIP LOCKED).
	ClaimDueVerifyPending(
		ctx context.Context,
		now time.Time,
		limit int,
	) ([]model.CheckoutSession, error)

	// MarkConfirming atomically moves an open checkout session with a linked
	// Decision to confirming, mints confirmation_attempt_id, so Confirm owns it.
	// Any other state is a cerr.Conflict.
	MarkConfirming(
		ctx context.Context,
		id model.CheckoutSessionID,
		attemptID model.ConfirmationAttemptID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// ReopenConfirmingToOpen returns confirming → open and clears
	// confirmation_attempt_id when the row still matches expectedAttemptID.
	ReopenConfirmingToOpen(
		ctx context.Context,
		id model.CheckoutSessionID,
		expectedAttemptID model.ConfirmationAttemptID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkCanceled transitions an open checkout session to canceled, records who
	// canceled it, and stamps canceled_at. Confirming, processing, and terminal
	// sessions are never matched. Rows with a linked contract are never matched;
	// use MarkCanceledSafely after financial-safety evidence is verified.
	MarkCanceled(
		ctx context.Context,
		id model.CheckoutSessionID,
		reason model.CheckoutCancellationReason,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkCanceledSafely transitions an open, confirming, or processing checkout
	// session to canceled after the caller has verified safe local termination
	// evidence. It clears confirmation_attempt_id so a prior confirmation Saga
	// cannot revive the session, and allows a linked contract_id.
	MarkCanceledSafely(
		ctx context.Context,
		id model.CheckoutSessionID,
		reason model.CheckoutCancellationReason,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// MarkExpiredSafely transitions an open, confirming, or processing checkout
	// session to expired after financial-safety evidence is verified. It clears
	// confirmation_attempt_id so a prior confirmation Saga cannot revive the
	// session.
	MarkExpiredSafely(
		ctx context.Context,
		id model.CheckoutSessionID,
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

	// ReplaceCreditKindAndInvalidateEvaluation persists a first Partner credit-kind
	// selection or replaces a prior selection under optimistic concurrency.
	// First selection does not bump evaluation_generation; a genuine replacement
	// clears decision_id and increments generation exactly once. The row must
	// still be open, at expectedGeneration, linked to expectedDecisionID, have
	// contract_id NULL, and have confirmation_attempt_id NULL.
	ReplaceCreditKindAndInvalidateEvaluation(
		ctx context.Context,
		id model.CheckoutSessionID,
		kind model.CreditKind,
		expectedGeneration int,
		expectedDecisionID *model.DecisionID,
		updatedAt time.Time,
	) (model.CheckoutSession, error)

	// UpsertReturnDestination registers or updates a merchant return destination.
	// A nil enabled preserves the stored value on update and defaults to true on
	// insert, so omission never silently disables a destination.
	UpsertReturnDestination(
		ctx context.Context,
		merchantID model.ExternalMerchantID,
		destinationName string,
		returnURL string,
		enabled *bool,
		now time.Time,
	) (model.MerchantReturnDestination, error)

	// BindCustomer attaches the trusted customer identity to a checkout session
	// without allowing a conflicting overwrite.
	BindCustomer(
		ctx context.Context,
		id model.CheckoutSessionID,
		customerID model.CustomerID,
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
