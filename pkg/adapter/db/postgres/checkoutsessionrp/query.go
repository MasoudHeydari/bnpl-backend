package checkoutsessionrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const checkoutSessionColumns = `
	id,
	public_token,
	origin,
	merchant_profile_id,
	merchant_branch_profile_id,
	online_store_id,
	merchant_order_id,
	idempotency_key,
	customer_reference,
	customer_phone_ciphertext,
	customer_phone_key_id,
	offer_preparation_mode,
	customer_id,
	return_destination,
	return_url,
	merchant_state,
	requested_amount_minor,
	currency,
	status,
	confirmation_attempt_id,
	decision_id,
	contract_id,
	cancellation_reason,
	evaluation_generation,
	expires_at,
	completed_at,
	canceled_at,
	created_at,
	updated_at
`

const merchantReturnDestinationColumns = `
	id,
	merchant_id,
	destination_name,
	return_url,
	enabled,
	created_at,
	updated_at
`

const merchantWebhookDestinationColumns = `
	id,
	merchant_id,
	webhook_url,
	enabled,
	signing_secret_ciphertext,
	signing_secret_key_id,
	created_at,
	updated_at
`

// Create inserts a checkout session and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.Create"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"expires_at": expiresAt,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.CheckoutSession{}, err
	}

	if err := publicToken.Validate(); err != nil {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: public token: %w", op, err),
			nil,
		)
	}

	if !origin.IsValid() {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: unsupported checkout session origin %q", op, origin),
			map[string]any{"origin": origin},
		)
	}

	if (customerPhoneCiphertext == nil) != (customerPhoneKeyID == nil) {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: customer phone ciphertext and key id must be set together", op),
			nil,
		)
	}

	if !offerPreparationMode.IsValid() {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: unsupported offer preparation mode %q", op, offerPreparationMode),
			nil,
		)
	}

	const query = `
INSERT INTO checkout_sessions (
	merchant_profile_id,
	merchant_branch_profile_id,
	online_store_id,
	merchant_order_id,
	idempotency_key,
	public_token,
	origin,
	customer_reference,
	customer_phone_ciphertext,
	customer_phone_key_id,
	offer_preparation_mode,
	customer_id,
	return_destination,
	return_url,
	merchant_state,
	requested_amount_minor,
	currency,
	status,
	expires_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"create checkout session",
		query,
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
		offerPreparationMode,
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
	if err != nil {
		// Concurrent creates lose on one of these unique indexes. Map only those
		// constraints so the use case can reconcile to the winning row without
		// swallowing unrelated ValidationFailed/Conflict errors from earlier
		// steps in the same transaction.
		if postgres.IsUniqueViolation(err, "uq_checkout_sessions_idempotency_key") {
			return model.CheckoutSession{}, cerr.Conflict(
				fmt.Errorf("%s: idempotency key already exists", op),
			)
		}

		if postgres.IsUniqueViolation(err, "uq_checkout_sessions_active_merchant_order") {
			return model.CheckoutSession{}, cerr.Conflict(
				fmt.Errorf("%s: merchant order already has an active checkout attempt", op),
			)
		}

		return model.CheckoutSession{}, err
	}

	return row, nil
}

// GetByID returns a checkout session by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE id = $1
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"get checkout session by id",
		query,
		id,
	)
}

// GetByPublicToken returns a checkout session by its public token.
func GetByPublicToken[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE public_token = $1
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"get checkout session by public token",
		query,
		token,
	)
}

// LockByPublicToken returns and locks a checkout session by its public token.
func LockByPublicToken[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	token model.CheckoutSessionToken,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE public_token = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"lock checkout session by public token",
		query,
		token,
	)
}

// LockByID returns and locks a checkout session by id.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"lock checkout session by id",
		query,
		id,
	)
}

// GetByMerchantIdempotencyKey returns a checkout session by create-request idempotency key.
func GetByMerchantIdempotencyKey[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
	idempotencyKey string,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE merchant_profile_id = $1
  AND idempotency_key = $2
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"get checkout session by merchant idempotency key",
		query,
		merchantProfileID,
		idempotencyKey,
	)
}

// LockByMerchantIdempotencyKey returns and locks a checkout session by create-request idempotency key.
func LockByMerchantIdempotencyKey[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
	idempotencyKey string,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE merchant_profile_id = $1
  AND idempotency_key = $2
FOR UPDATE
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"lock checkout session by merchant idempotency key",
		query,
		merchantProfileID,
		idempotencyKey,
	)
}

// GetByMerchantOrderID returns the current checkout attempt for a merchant order.
//
// Completed history wins even if a newer canceled/expired attempt exists:
// no later attempt may silently refinance an already fulfilled merchant order.
// Otherwise return the active attempt or most recent terminal attempt.
func GetByMerchantOrderID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
	merchantOrderID string,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE merchant_profile_id = $1
  AND merchant_order_id = $2
ORDER BY (status = 'completed') DESC, (status IN ('open', 'confirming', 'processing')) DESC, created_at DESC, id DESC
LIMIT 1
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"get checkout session by merchant order id",
		query,
		merchantProfileID,
		merchantOrderID,
	)
}

// LockByMerchantOrderID locks the completed order if one exists, otherwise
// the active attempt or most recent terminal attempt.
func LockByMerchantOrderID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
	merchantOrderID string,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE merchant_profile_id = $1
  AND merchant_order_id = $2
ORDER BY (status = 'completed') DESC, (status IN ('open', 'confirming', 'processing')) DESC, created_at DESC, id DESC
LIMIT 1
FOR UPDATE
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"lock checkout session by merchant order id",
		query,
		merchantProfileID,
		merchantOrderID,
	)
}

// UpdateStatus transitions a checkout session to status and stamps completed_at
// when the new status is completed. Cancellation goes through MarkCanceled so
// the reason and canceled_at are always recorded together.
func UpdateStatus[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	status model.CheckoutSessionStatus,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	const query = `
UPDATE checkout_sessions
SET status = $2::text,
	updated_at = $3::timestamptz,
	completed_at = CASE
		WHEN $2::text = 'completed' THEN COALESCE(completed_at, $3::timestamptz)
		ELSE completed_at
	END
WHERE id = $1
RETURNING` + checkoutSessionColumns

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"update checkout session status",
		query,
		id,
		status,
		updatedAt,
	)
}

// BindCustomer attaches the trusted customer identity without allowing a
// conflicting overwrite.
func BindCustomer[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	customerID model.CustomerID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.BindCustomer"

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	const query = `
UPDATE checkout_sessions
SET customer_id = $2, updated_at = $3
WHERE id = $1
  AND (customer_id IS NULL OR customer_id = $2)
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"bind checkout session customer",
		query,
		id,
		customerID,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(
			fmt.Errorf("%s: checkout session is already bound to another customer", op),
		)
	}

	return row, err
}






// LinkDecision attaches the personalized checkout Decision to a checkout session.
func LinkDecision[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	decisionID model.DecisionID,
	expectedGeneration int,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	const query = `
UPDATE checkout_sessions
SET decision_id = $2, updated_at = $3
WHERE id = $1
  AND status = 'open'
  AND evaluation_generation = $4
  AND contract_id IS NULL
  AND (decision_id IS NULL OR decision_id = $2)
RETURNING` + checkoutSessionColumns

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"link checkout session decision",
		query,
		id,
		decisionID,
		updatedAt,
		expectedGeneration,
	)
}

// ClampExpiresAt shortens expires_at to the earlier of the current value and
// expiresAt. Rows whose expires_at is already at or before expiresAt are
// returned unchanged (matched by id alone after the conditional update).
func ClampExpiresAt[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	expiresAt time.Time,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"expires_at": expiresAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.CheckoutSession{}, err
	}

	const query = `
UPDATE checkout_sessions
SET expires_at = LEAST(expires_at, $2::timestamptz),
	updated_at = $3::timestamptz
WHERE id = $1
RETURNING` + checkoutSessionColumns

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"clamp checkout session expires_at",
		query,
		id,
		expiresAt,
		updatedAt,
	)
}

// LinkContract attaches the BNPL contract created from the accepted offer.
func LinkContract[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	contractID model.ContractID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.LinkContract"

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	const query = `
UPDATE checkout_sessions
SET contract_id = $2, updated_at = $3
WHERE id = $1
  AND (contract_id IS NULL OR contract_id = $2)
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"link checkout session contract",
		query,
		id,
		contractID,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(
			fmt.Errorf("%s: checkout session is already linked to another contract", op),
		)
	}

	return row, err
}

// GetReturnDestination returns the enabled return destination registered under destinationName.
func GetReturnDestination[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
	destinationName string,
) (model.MerchantReturnDestination, error) {
	const query = `SELECT` + merchantReturnDestinationColumns + `
FROM merchant_return_destinations
WHERE merchant_id = $1
  AND destination_name = $2
  AND enabled = true
`

	return postgres.QueryOne[model.MerchantReturnDestination](
		ctx,
		q,
		"get merchant return destination",
		query,
		merchantID,
		destinationName,
	)
}

// GetByContractID returns the checkout session linked to contractID.
func GetByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE contract_id = $1
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"get checkout session by contract id",
		query,
		contractID,
	)
}

// LockByContractID returns and locks the checkout session linked to contractID.
func LockByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
) (model.CheckoutSession, error) {
	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE contract_id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"lock checkout session by contract id",
		query,
		contractID,
	)
}

// ExpireOpenDue transitions open checkout sessions whose lifetime elapsed to
// expired and returns the rows it expired.
//
// Only open rows without an accepted Contract are touched. A checkout may
// become open again on a definite payment failure, but its accepted Contract
// and possible late FC outcome prohibit expiry of that checkout. Confirming
// and processing sessions are also exempt. SKIP LOCKED lets
// concurrent workers expire disjoint batches without blocking customer requests.
func ExpireOpenDue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	limit int,
) ([]model.CheckoutSession, error) {
	const op = "checkoutsessionrp.ExpireOpenDue"

	if err := postgres.RequiredTime("now", now); err != nil {
		return nil, err
	}

	if limit <= 0 {
		return nil, cerr.InvalidArgument(
			fmt.Errorf("%s: limit must be positive", op),
			map[string]any{"limit": limit},
		)
	}

	const query = `
UPDATE checkout_sessions
SET status = $1, updated_at = $2
WHERE id IN (
	SELECT id
	FROM checkout_sessions
	WHERE status = $3
	  AND expires_at <= $2
	  AND contract_id IS NULL
	ORDER BY expires_at ASC, id ASC
	LIMIT $4
	FOR UPDATE SKIP LOCKED
)
RETURNING` + checkoutSessionColumns

	return postgres.QueryAll[model.CheckoutSession](
		ctx,
		q,
		"expire due open checkout sessions",
		query,
		model.ExpiredCheckoutSessionStatus,
		now,
		model.OpenCheckoutSessionStatus,
		limit,
	)
}

// ReopenStaleConfirmingWithoutContract is the legacy-named recovery scanner:
// it locks stale unlinked confirming and eligible processing sessions for
// inspection. It NEVER reopens a claim; TTL is not proof the owner has stopped.
// Processing candidates are limited to FC-recognizable Contracts.
func ReopenStaleConfirmingWithoutContract[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	staleBefore time.Time,
	limit int,
) ([]model.CheckoutSession, error) {
	const op = "checkoutsessionrp.ReopenStaleConfirmingWithoutContract"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"now":          now,
		"stale_before": staleBefore,
	}); err != nil {
		return nil, err
	}

	if limit <= 0 {
		return nil, cerr.InvalidArgument(
			fmt.Errorf("%s: limit must be positive", op),
			map[string]any{"limit": limit},
		)
	}

	_ = now // retained for RequiredTimes / call-site symmetry with other sweeps

	const query = `SELECT` + checkoutSessionColumns + `
FROM checkout_sessions
WHERE ((status = $1 AND contract_id IS NULL)
   OR (status = 'processing' AND contract_id IN (
        SELECT id FROM contracts
        WHERE status IN ('payment_pending', 'active')
   )))
  AND updated_at <= $2
ORDER BY (status = 'processing') DESC, updated_at ASC, id ASC
LIMIT $3
FOR UPDATE SKIP LOCKED
`

	return postgres.QueryAll[model.CheckoutSession](
		ctx,
		q,
		"list stale confirming checkout sessions for recovery",
		query,
		model.ConfirmingCheckoutSessionStatus,
		staleBefore,
		limit,
	)
}

// MarkConfirming moves an open checkout session that has a linked Decision to
// confirming so Confirm owns it through contract/payment initialization.
//
// The conditional UPDATE is the arbiter between concurrent Confirm calls and
// cancel/expiry: exactly one caller observes status = 'open'.
func MarkConfirming[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	attemptID model.ConfirmationAttemptID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.MarkConfirming"

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	if attemptID.IsZero() {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: confirmation attempt id is required", op),
		)
	}

	const query = `
UPDATE checkout_sessions
SET status = $2, confirmation_attempt_id = $3, updated_at = $4
WHERE id = $1
  AND status = $5
  AND decision_id IS NOT NULL
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"mark checkout session confirming",
		query,
		id,
		model.ConfirmingCheckoutSessionStatus,
		attemptID,
		updatedAt,
		model.OpenCheckoutSessionStatus,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(
			fmt.Errorf("%s: checkout session is not open with a linked decision", op),
		)
	}

	return row, err
}

// ReopenConfirmingToOpen returns confirming → open, clears the active
// confirmation attempt, and invalidates the offer generation atomically when
// no contract is linked.
//
// Generation invalidation is local state only. It does not evaluate a new
// offer, so the transaction does not perform network I/O. A stale Confirm
// still carrying the superseded generation cannot claim a new attempt for the
// rejected terms.
func ReopenConfirmingToOpen[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	expectedAttemptID model.ConfirmationAttemptID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.ReopenConfirmingToOpen"

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	if expectedAttemptID.IsZero() {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: expected confirmation attempt id is required", op),
		)
	}

	const query = `
UPDATE checkout_sessions
SET status = $2,
	confirmation_attempt_id = NULL,
	evaluation_generation = evaluation_generation + 1,
	decision_id = NULL,
	updated_at = $3
WHERE id = $1
  AND status = $4
  AND contract_id IS NULL
  AND confirmation_attempt_id = $5
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"reopen confirming checkout session to open",
		query,
		id,
		model.OpenCheckoutSessionStatus,
		updatedAt,
		model.ConfirmingCheckoutSessionStatus,
		expectedAttemptID,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(
			fmt.Errorf("%s: checkout session is not confirming for the expected attempt", op),
		)
	}

	return row, err
}

// MarkCanceled transitions an open checkout session to canceled, records who
// canceled it, and stamps canceled_at.
//
// Confirming and processing sessions are deliberately not matched: Confirm or
// Financial Core owns their outcome and canceling them could race a successful
// payment. Unavailable sessions are already terminal.
func MarkCanceled[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	reason model.CheckoutCancellationReason,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.MarkCanceled"

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	if !reason.IsValid() {
		return model.CheckoutSession{}, cerr.InvalidArgument(
			fmt.Errorf("%s: unsupported cancellation reason %q", op, reason),
			map[string]any{"reason": reason},
		)
	}

	const query = `
UPDATE checkout_sessions
SET status = $2, cancellation_reason = $3, updated_at = $4, canceled_at = $4
WHERE id = $1
  AND status = $5
  AND contract_id IS NULL
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"mark checkout session canceled",
		query,
		id,
		model.CanceledCheckoutSessionStatus,
		reason,
		updatedAt,
		model.OpenCheckoutSessionStatus,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(
			fmt.Errorf("%s: checkout session is not in a cancelable state", op),
		)
	}

	return row, err
}

// BumpEvaluationGenerationCAS starts a fresh evaluation for an open checkout
// session if nobody else advanced it first.
//
// It increments evaluation_generation and clears decision_id so the next
// evaluation links a new Decision instead of reusing an expired offer. Prior
// Decisions stay immutable history under their own idempotency keys.
//
// OTP challenges live in otp_challenges keyed by session id; a generation bump
// invalidates prior codes via context_version mismatch without mutating that
// table here.
//
// After a Contract has been created and linked, the accepted Decision is
// immutable: this UPDATE requires contract_id IS NULL so payment retry cannot
// detach Decision A and attach Decision B while Contract A remains linked.
//
// The update is a compare-and-swap on (status = open, evaluation_generation,
// decision_id, contract_id IS NULL). A concurrent bump, Decision link, Confirm,
// cancel, contract link, or expiry changes one of those and this call reports a
// conflict instead of clobbering the newer state.
func BumpEvaluationGenerationCAS[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CheckoutSessionID,
	expectedGeneration int,
	expectedDecisionID *model.DecisionID,
	updatedAt time.Time,
) (model.CheckoutSession, error) {
	const op = "checkoutsessionrp.BumpEvaluationGenerationCAS"

	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.CheckoutSession{}, err
	}

	const query = `
UPDATE checkout_sessions
SET evaluation_generation = evaluation_generation + 1,
	decision_id = NULL,
	updated_at = $2
WHERE id = $1
  AND status = $3
  AND evaluation_generation = $4
  AND decision_id IS NOT DISTINCT FROM $5::uuid
  AND contract_id IS NULL
RETURNING` + checkoutSessionColumns

	row, err := postgres.QueryOne[model.CheckoutSession](
		ctx,
		q,
		"bump checkout session evaluation generation",
		query,
		id,
		updatedAt,
		model.OpenCheckoutSessionStatus,
		expectedGeneration,
		expectedDecisionID,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.CheckoutSession{}, cerr.Conflict(
			fmt.Errorf("%s: checkout session evaluation state changed concurrently", op),
		)
	}

	return row, err
}

// UpsertReturnDestination registers or updates a merchant return destination.
func UpsertReturnDestination[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
	destinationName string,
	returnURL string,
	enabled *bool,
	now time.Time,
) (model.MerchantReturnDestination, error) {
	if err := postgres.RequiredTime("updated_at", now); err != nil {
		return model.MerchantReturnDestination{}, err
	}

	const query = `
INSERT INTO merchant_return_destinations (
	merchant_id,
	destination_name,
	return_url,
	enabled,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, COALESCE($4::boolean, true), $5, $5)
ON CONFLICT (merchant_id, destination_name) DO UPDATE
SET return_url = EXCLUDED.return_url,
	enabled = COALESCE($4::boolean, merchant_return_destinations.enabled),
	updated_at = EXCLUDED.updated_at
RETURNING` + merchantReturnDestinationColumns

	return postgres.QueryOne[model.MerchantReturnDestination](
		ctx,
		q,
		"upsert merchant return destination",
		query,
		merchantID,
		destinationName,
		returnURL,
		enabled,
		now,
	)
}

// UpsertWebhookDestination registers or updates the merchant's single webhook
// destination. The signing secret is persisted only as envelope ciphertext plus
// the key id that produced it.
func UpsertWebhookDestination[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
	webhookURL string,
	signingSecretCiphertext string,
	signingSecretKeyID string,
	enabled bool,
	now time.Time,
) (model.MerchantWebhookDestination, error) {
	if err := postgres.RequiredTime("updated_at", now); err != nil {
		return model.MerchantWebhookDestination{}, err
	}

	const query = `
INSERT INTO merchant_webhook_destinations (
	merchant_id,
	webhook_url,
	signing_secret_ciphertext,
	signing_secret_key_id,
	enabled,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $6)
ON CONFLICT (merchant_id) DO UPDATE
SET webhook_url = EXCLUDED.webhook_url,
	signing_secret_ciphertext = CASE
		WHEN EXCLUDED.signing_secret_ciphertext = ''
			THEN merchant_webhook_destinations.signing_secret_ciphertext
		ELSE EXCLUDED.signing_secret_ciphertext
	END,
	signing_secret_key_id = CASE
		WHEN EXCLUDED.signing_secret_ciphertext = ''
			THEN merchant_webhook_destinations.signing_secret_key_id
		ELSE EXCLUDED.signing_secret_key_id
	END,
	enabled = EXCLUDED.enabled,
	updated_at = EXCLUDED.updated_at
RETURNING` + merchantWebhookDestinationColumns

	return postgres.QueryOne[model.MerchantWebhookDestination](
		ctx,
		q,
		"upsert merchant webhook destination",
		query,
		merchantID,
		webhookURL,
		signingSecretCiphertext,
		signingSecretKeyID,
		enabled,
		now,
	)
}

// GetWebhookDestination returns the merchant's enabled webhook destination.
func GetWebhookDestination[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (model.MerchantWebhookDestination, error) {
	const query = `SELECT` + merchantWebhookDestinationColumns + `
FROM merchant_webhook_destinations
WHERE merchant_id = $1
  AND enabled = true
`

	return postgres.QueryOne[model.MerchantWebhookDestination](
		ctx,
		q,
		"get merchant webhook destination",
		query,
		merchantID,
	)
}

// FindWebhookDestination returns the merchant's webhook destination regardless
// of its enabled flag.
func FindWebhookDestination[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (model.MerchantWebhookDestination, error) {
	const query = `SELECT` + merchantWebhookDestinationColumns + `
FROM merchant_webhook_destinations
WHERE merchant_id = $1
`

	return postgres.QueryOne[model.MerchantWebhookDestination](
		ctx,
		q,
		"find merchant webhook destination",
		query,
		merchantID,
	)
}

