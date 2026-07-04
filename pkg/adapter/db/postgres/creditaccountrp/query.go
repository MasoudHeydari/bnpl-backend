package creditaccountrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const creditAccountColumns = `
	id,
	customer_id,
	currency,
	limit_amount_minor,
	exposure_amount_minor,
	status,
	last_limit_decision_id,
	effective_from,
	expires_at,
	revoked_at,
	version,
	created_at,
	updated_at
`

const creditTransactionColumns = `
	id,
	credit_account_id,
	type,
	amount_minor,
	currency,
	reason_code,
	reference_type,
	reference_id,
	occurred_at,
	created_at
`

func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id model.CreditAccountID) (model.CreditAccount, error) {
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE id = $1
`
	return postgres.QueryOne[model.CreditAccount](ctx, q, "get credit account by id", query, id)
}

func GetByCustomerIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	currency model.Currency,
) (model.CreditAccount, error) {
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE customer_id = $1
  AND currency = $2
`
	return postgres.QueryOne[model.CreditAccount](ctx, q, "get credit account by customer and currency", query, customerID, currency)
}

func GetByLastLimitDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) (model.CreditAccount, error) {
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE last_limit_decision_id = $1
`
	return postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"get credit account by last limit decision",
		query,
		decisionID,
	)
}

func LockByID[Q postgres.Queryer](ctx context.Context, q Q, id model.CreditAccountID) (model.CreditAccount, error) {
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE id = $1
FOR UPDATE
`
	return postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"lock credit account by id",
		query,
		id,
	)
}

func LockByCustomerIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	currency model.Currency,
) (model.CreditAccount, error) {
	const query = `SELECT` + creditAccountColumns + `
FROM credit_accounts
WHERE customer_id = $1
  AND currency = $2
FOR UPDATE
`
	return postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"lock credit account by customer and currency",
		query,
		customerID,
		currency,
	)
}

func CreateWithLimit[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	currency model.Currency,
	limitAmountMinor uint64,
	decisionID model.DecisionID,
	effectiveFrom time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CreditAccount, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"effective_from": effectiveFrom,
		"created_at":     createdAt,
		"updated_at":     updatedAt,
	}); err != nil {
		return model.CreditAccount{}, err
	}

	const query = `
INSERT INTO credit_accounts (
	customer_id,
	currency,
	limit_amount_minor,
	exposure_amount_minor,
	status,
	last_limit_decision_id,
	effective_from,
	expires_at,
	revoked_at,
	version,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, 0, 'active', $4, $5, $6, NULL, 1, $7, $8)
RETURNING` + creditAccountColumns

	account, err := postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"create credit account",
		query,
		customerID,
		currency,
		limitAmountMinor,
		decisionID,
		effectiveFrom,
		expiresAt,
		createdAt,
		updatedAt,
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, "uq_credit_accounts_customer_currency") {
			return model.CreditAccount{}, cerr.Conflict(
				fmt.Errorf("credit account already exists for customer and currency"),
				map[string]any{"customer_id": customerID, "currency": currency},
			)
		}

		return model.CreditAccount{}, err
	}

	return account, nil
}

func SetLimit[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CreditAccountID,
	limitAmountMinor uint64,
	decisionID model.DecisionID,
	effectiveFrom time.Time,
	expiresAt *time.Time,
	updatedAt time.Time,
) (model.CreditAccount, error) {
	const query = `
UPDATE credit_accounts
SET
	limit_amount_minor = $2,
	status = 'active',
	last_limit_decision_id = $3,
	effective_from = $4,
	expires_at = $5,
	revoked_at = NULL,
	version = version + 1,
	updated_at = $6
WHERE id = $1
RETURNING` + creditAccountColumns

	return postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"set credit account limit",
		query,
		id,
		limitAmountMinor,
		decisionID,
		effectiveFrom,
		expiresAt,
		updatedAt,
	)
}

func ExpireDueByCustomerIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	currency model.Currency,
	now time.Time,
) error {
	const query = `
UPDATE credit_accounts
SET
	status = 'expired',
	version = version + 1,
	updated_at = $3
WHERE customer_id = $1
  AND currency = $2
  AND status = 'active'
  AND expires_at IS NOT NULL
  AND expires_at <= $3
`
	_, err := q.Exec(ctx, query, customerID, currency, now)
	if err != nil {
		return fmt.Errorf("expire due credit account: %w", postgres.MapError(err))
	}

	return nil
}

func AddExposure[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CreditAccountID,
	amountMinor uint64,
	updatedAt time.Time,
) (model.CreditAccount, error) {
	const query = `
UPDATE credit_accounts
SET
	exposure_amount_minor = exposure_amount_minor + $2,
	version = version + 1,
	updated_at = $3
WHERE id = $1
RETURNING` + creditAccountColumns

	return postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"increase credit account exposure",
		query,
		id,
		amountMinor,
		updatedAt,
	)
}

func ReduceExposureIfSufficient[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CreditAccountID,
	amountMinor uint64,
	updatedAt time.Time,
) (model.CreditAccount, error) {
	const query = `
UPDATE credit_accounts
SET
	exposure_amount_minor = exposure_amount_minor - $2,
	version = version + 1,
	updated_at = $3
WHERE id = $1
  AND exposure_amount_minor >= $2
RETURNING` + creditAccountColumns

	account, err := postgres.QueryOne[model.CreditAccount](
		ctx,
		q,
		"decrease credit account exposure",
		query,
		id,
		amountMinor,
		updatedAt,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.CreditAccount{}, cerr.Conflict(
				fmt.Errorf("credit account exposure is insufficient"),
				map[string]any{"credit_account_id": id, "amount_minor": amountMinor},
			)
		}

		return model.CreditAccount{}, err
	}

	return account, nil
}

func CountLimitSetTransactions[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	currency model.Currency,
) (int64, error) {
	const query = `
SELECT COUNT(*) AS count
FROM credit_transactions ct
JOIN credit_accounts ca ON ca.id = ct.credit_account_id
WHERE ca.customer_id = $1
  AND ca.currency = $2
  AND ct.type = 'limit_set'
`
	type result struct {
		Count int64 `db:"count"`
	}
	row, err := postgres.QueryOne[result](
		ctx,
		q,
		"count credit limit set transactions",
		query,
		customerID,
		currency,
	)
	if err != nil {
		return 0, err
	}

	return row.Count, nil
}

func AppendTransaction[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	creditAccountID model.CreditAccountID,
	creditTransactionType model.CreditTransactionType,
	amountMinor uint64,
	currency model.Currency,
	reason model.CreditTransactionReason,
	referenceType model.CreditTransactionReferenceType,
	referenceID uuid.UUID,
	occurredAt time.Time,
	createdAt time.Time,
) (model.CreditTransaction, error) {
	entry := model.CreditTransaction{
		CreditAccountID: creditAccountID,
		Type:            creditTransactionType,
		AmountMinor:     amountMinor,
		Currency:        currency,
		Reason:          reason,
		ReferenceType:   referenceType,
		ReferenceID:     referenceID,
		OccurredAt:      occurredAt,
		CreatedAt:       createdAt,
	}
	if err := entry.Validate(); err != nil {
		return model.CreditTransaction{}, err
	}

	const query = `
INSERT INTO credit_transactions (
	credit_account_id,
	type,
	amount_minor,
	currency,
	reason_code,
	reference_type,
	reference_id,
	occurred_at,
	created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING` + creditTransactionColumns

	row, err := postgres.QueryOne[model.CreditTransaction](
		ctx,
		q,
		"append credit transaction",
		query,
		creditAccountID,
		creditTransactionType,
		amountMinor,
		currency,
		reason,
		referenceType,
		referenceID,
		occurredAt,
		createdAt,
	)
	if err != nil {
		if postgres.IsUniqueViolation(err, "uq_credit_transactions_business_fact") {
			return model.CreditTransaction{}, cerr.IdempotencyConflict(
				fmt.Errorf("credit transaction business fact already recorded"),
				map[string]any{
					"credit_account_id": creditAccountID,
					"type":              creditTransactionType,
					"reference_type":    referenceType,
					"reference_id":      referenceID,
				},
			)
		}

		return model.CreditTransaction{}, err
	}

	return row, nil
}
