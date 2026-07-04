package paymentattemptrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// GetByIDWithItems returns a payment attempt and its item snapshot without locking.
func GetByIDWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
) (model.PaymentAttempt, error) {
	attempt, err := GetByID(ctx, q, id)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	items, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, items), nil
}

// LockByIDWithItems returns and locks a payment attempt with its snapshotted items.
func LockByIDWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
) (model.PaymentAttempt, error) {
	attempt, err := LockByID(ctx, q, id)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	items, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, items), nil
}

// GetByFinancialPaymentIntentIDWithItems returns a payment attempt and items by intent id.
func GetByFinancialPaymentIntentIDWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	financialPaymentIntentID string,
) (model.PaymentAttempt, error) {
	attempt, err := GetByFinancialPaymentIntentID(ctx, q, financialPaymentIntentID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	items, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, items), nil
}

// GetByInstallmentIDAndIdempotencyKeyWithItems returns a payment attempt and items by idempotency key.
func GetByInstallmentIDAndIdempotencyKeyWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	idempotencyKey string,
) (model.PaymentAttempt, error) {
	attempt, err := GetByInstallmentIDAndIdempotencyKey(ctx, q, installmentID, idempotencyKey)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	items, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, items), nil
}

// GetByContractIDAndPurposeWithItems returns a payment attempt and items by contract purpose.
func GetByContractIDAndPurposeWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	purpose model.FinancialCorePaymentPurpose,
) (model.PaymentAttempt, error) {
	attempt, err := GetByContractIDAndPurpose(ctx, q, contractID, purpose)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	items, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, items), nil
}

// CreateCheckoutWithItems stores a checkout-first-installment attempt and item snapshot.
func CreateCheckoutWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	attempt, err := CreateCheckout(
		ctx,
		q,
		contractID,
		installmentID,
		amountMinor,
		currency,
		channel,
		idempotencyKey,
		requestFingerprint,
		tenantID,
		contextID,
		providerCode,
		successReturnURL,
		failureReturnURL,
		cancelReturnURL,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	if err := createItems(ctx, q, attempt.ID, items, createdAt); err != nil {
		return model.PaymentAttempt{}, err
	}

	persistedItems, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, persistedItems), nil
}

// CreateWithItems stores a payment attempt and its snapshotted obligation items atomically.
func CreateWithItems[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	attempt, err := Create(
		ctx,
		q,
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
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	if err := createItems(ctx, q, attempt.ID, items, createdAt); err != nil {
		return model.PaymentAttempt{}, err
	}

	persistedItems, err := listItemsByPaymentAttemptID(ctx, q, attempt.ID)
	if err != nil {
		return model.PaymentAttempt{}, err
	}

	return attachItems(attempt, persistedItems), nil
}

// HasBlockingAttemptForContract reports whether the contract has a blocking payment attempt.
func HasBlockingAttemptForContract[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM payment_attempts
	WHERE contract_id = $1
	  AND state IN ($2, $3, $4, $5)
) AS exists
`

	type result struct {
		Exists bool `db:"exists"`
	}

	value, err := postgres.QueryOne[result](
		ctx,
		q,
		"has blocking payment attempt for contract",
		query,
		contractID,
		model.CreatedPaymentAttemptState,
		model.SubmittedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		model.ManualReviewRequiredPaymentAttemptState,
	)
	if err != nil {
		return false, err
	}

	return value.Exists, nil
}
