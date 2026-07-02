package borrowercreditlimitrp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrBorrowerCreditLimitNotFound indicates that the credit limit does not exist.
	ErrBorrowerCreditLimitNotFound = errors.New("borrower credit limit not found")
)

// Create inserts a borrower credit limit assignment and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	decisionID uuid.UUID,
	amountMinor uint64,
	currency model.Currency,
	status model.BorrowerCreditLimitStatus,
	effectiveFrom time.Time,
	expiresAt *time.Time,
	revokedAt *time.Time,
	replacedByCreditLimitID *uuid.UUID,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerCreditLimit, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"effective_from": effectiveFrom,
		"created_at":     createdAt,
		"updated_at":     updatedAt,
	}); err != nil {
		return model.BorrowerCreditLimit{}, err
	}

	const query = `
INSERT INTO borrower_credit_limits (
	borrower_profile_id,
	decision_id,
	amount_minor,
	currency,
	status,
	effective_from,
	expires_at,
	revoked_at,
	replaced_by_credit_limit_id,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING
	id,
	borrower_profile_id,
	decision_id,
	amount_minor,
	currency,
	status,
	effective_from,
	expires_at,
	revoked_at,
	replaced_by_credit_limit_id,
	created_at,
	updated_at
`

	return postgres.QueryOne[model.BorrowerCreditLimit](
		ctx,
		q,
		"create borrower credit limit",
		query,
		borrowerProfileID,
		decisionID,
		amountMinor,
		currency,
		status,
		effectiveFrom,
		expiresAt,
		revokedAt,
		replacedByCreditLimitID,
		createdAt,
		updatedAt,
	)
}

// GetByID returns a borrower credit limit by id.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) (model.BorrowerCreditLimit, error) {
	const query = `
SELECT
	id,
	borrower_profile_id,
	decision_id,
	amount_minor,
	currency,
	status,
	effective_from,
	expires_at,
	revoked_at,
	replaced_by_credit_limit_id,
	created_at,
	updated_at
FROM borrower_credit_limits
WHERE id = $1
`

	return getOne(ctx, q, query, []any{id}, "get borrower credit limit by id")
}

// GetCurrentByBorrowerProfileID returns the active credit limit at the provided time.
func GetCurrentByBorrowerProfileID[Q postgres.Queryer](ctx context.Context, q Q, borrowerProfileID uuid.UUID, at time.Time) (model.BorrowerCreditLimit, error) {
	const query = `
SELECT
	id,
	borrower_profile_id,
	decision_id,
	amount_minor,
	currency,
	status,
	effective_from,
	expires_at,
	revoked_at,
	replaced_by_credit_limit_id,
	created_at,
	updated_at
FROM borrower_credit_limits
WHERE borrower_profile_id = $1
  AND status = $3
  AND effective_from <= $2
  AND (expires_at IS NULL OR expires_at > $2)
  AND revoked_at IS NULL
ORDER BY effective_from DESC, created_at DESC
LIMIT 1
`

	return getOne(ctx, q, query, []any{borrowerProfileID, at, model.ActiveBorrowerCreditLimitStatus}, "get current borrower credit limit")
}

// ListByBorrowerProfileID returns all credit limits for a borrower profile.
func ListByBorrowerProfileID[Q postgres.Queryer](ctx context.Context, q Q, borrowerProfileID uuid.UUID) ([]model.BorrowerCreditLimit, error) {
	const query = `
SELECT
	id,
	borrower_profile_id,
	decision_id,
	amount_minor,
	currency,
	status,
	effective_from,
	expires_at,
	revoked_at,
	replaced_by_credit_limit_id,
	created_at,
	updated_at
FROM borrower_credit_limits
WHERE borrower_profile_id = $1
ORDER BY effective_from DESC, created_at DESC
`

	rows, err := q.Query(
		ctx,
		query,
		borrowerProfileID,
	)
	if err != nil {
		return nil, fmt.Errorf("list borrower credit limits: %w", postgres.MapError(err))
	}
	defer rows.Close()

	limits, err := postgres.ValuesInto[model.BorrowerCreditLimit](rows.(postgres.Rows))
	if err != nil {
		return nil, fmt.Errorf("list borrower credit limits: %w", err)
	}

	return limits, nil
}

// LockCurrentByBorrowerProfileID returns the active credit limit and locks it for update.
func LockCurrentByBorrowerProfileID[Q postgres.Queryer](ctx context.Context, q Q, borrowerProfileID uuid.UUID, at time.Time) (model.BorrowerCreditLimit, error) {
	const query = `
SELECT
	id,
	borrower_profile_id,
	decision_id,
	amount_minor,
	currency,
	status,
	effective_from,
	expires_at,
	revoked_at,
	replaced_by_credit_limit_id,
	created_at,
	updated_at
FROM borrower_credit_limits
WHERE borrower_profile_id = $1
  AND status = $3
  AND effective_from <= $2
  AND (expires_at IS NULL OR expires_at > $2)
  AND revoked_at IS NULL
ORDER BY effective_from DESC, created_at DESC
LIMIT 1
FOR UPDATE
`

	return getOne(ctx, q, query, []any{borrowerProfileID, at, model.ActiveBorrowerCreditLimitStatus}, "lock current borrower credit limit")
}

// Revoke marks a credit limit as revoked.
func Revoke[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID, revokedAt time.Time, updatedAt time.Time) error {
	const query = `
UPDATE borrower_credit_limits
SET
	status = $2,
	revoked_at = $3,
	updated_at = $4
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		id,
		model.RevokedBorrowerCreditLimitStatus,
		revokedAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("revoke borrower credit limit %s: %w", id, postgres.MapError(err))
	}

	return ensureAffected("revoke borrower credit limit", affected, id)
}

// MarkExpired marks a credit limit as expired.
func MarkExpired[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID, updatedAt time.Time) error {
	const query = `
UPDATE borrower_credit_limits
SET
	status = $2,
	updated_at = $3
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		id,
		model.ExpiredBorrowerCreditLimitStatus,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("mark borrower credit limit %s expired: %w", id, postgres.MapError(err))
	}

	return ensureAffected("mark borrower credit limit expired", affected, id)
}

// MarkReplaced links a credit limit to the newer assignment that replaced it.
func MarkReplaced[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID, replacedByCreditLimitID uuid.UUID, updatedAt time.Time) error {
	const query = `
UPDATE borrower_credit_limits
SET
	status = $2,
	replaced_by_credit_limit_id = $3,
	updated_at = $4
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		id,
		model.ReplacedBorrowerCreditLimitStatus,
		replacedByCreditLimitID,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("mark borrower credit limit %s replaced: %w", id, postgres.MapError(err))
	}

	return ensureAffected("mark borrower credit limit replaced", affected, id)
}

func getOne[Q postgres.Queryer](ctx context.Context, q Q, query string, args []any, operation string) (model.BorrowerCreditLimit, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return model.BorrowerCreditLimit{}, fmt.Errorf("%s: %w", operation, postgres.MapError(err))
	}
	defer rows.Close()

	limit, err := postgres.ValueInto[model.BorrowerCreditLimit](rows.(postgres.Rows))
	if err != nil {
		return model.BorrowerCreditLimit{}, fmt.Errorf("%s: %w", operation, err)
	}

	return limit, nil
}

func ensureAffected(operation string, affected int64, id uuid.UUID) error {
	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrBorrowerCreditLimitNotFound,
			map[string]any{
				"operation": operation,
				"limit_id":  id,
			},
		)
	}

	return nil
}
