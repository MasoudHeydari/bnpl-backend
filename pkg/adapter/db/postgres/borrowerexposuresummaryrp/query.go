package borrowerexposuresummaryrp

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
	// ErrBorrowerExposureSummaryNotFound indicates that the exposure summary does not exist.
	ErrBorrowerExposureSummaryNotFound = errors.New("borrower exposure summary not found")
)

const borrowerExposureSummaryColumns = `
	borrower_profile_id,
	currency,
	active_exposure_amount_minor,
	reserved_exposure_amount_minor,
	updated_at
`

// GetByBorrowerProfileIDAndCurrency returns the exposure summary for a borrower profile.
func GetByBorrowerProfileIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
) (model.BorrowerExposureSummary, error) {
	const query = `SELECT` + borrowerExposureSummaryColumns + `
FROM borrower_exposure_summaries
WHERE borrower_profile_id = $1
  AND currency = $2
`

	return postgres.QueryOne[model.BorrowerExposureSummary](
		ctx,
		q,
		"get borrower exposure summary",
		query,
		borrowerProfileID,
		currency,
	)
}

// LockByBorrowerProfileIDAndCurrency returns and locks the exposure summary row.
func LockByBorrowerProfileIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
) (model.BorrowerExposureSummary, error) {
	const query = `SELECT` + borrowerExposureSummaryColumns + `
FROM borrower_exposure_summaries
WHERE borrower_profile_id = $1
  AND currency = $2
FOR UPDATE
`

	return postgres.QueryOne[model.BorrowerExposureSummary](
		ctx,
		q,
		"lock borrower exposure summary",
		query,
		borrowerProfileID,
		currency,
	)
}

// EnsureAndLock creates a zero exposure summary when missing and locks it for update.
func EnsureAndLock[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	updatedAt time.Time,
) (model.BorrowerExposureSummary, error) {
	const insertQuery = `
INSERT INTO borrower_exposure_summaries (
	borrower_profile_id,
	currency,
	active_exposure_amount_minor,
	reserved_exposure_amount_minor,
	updated_at
)
VALUES ($1, $2, 0, 0, $3)
ON CONFLICT (borrower_profile_id, currency) DO NOTHING
`

	if _, err := q.Exec(ctx, insertQuery, borrowerProfileID, currency, updatedAt); err != nil {
		return model.BorrowerExposureSummary{}, fmt.Errorf("ensure borrower exposure summary: %w", postgres.MapError(err))
	}

	return LockByBorrowerProfileIDAndCurrency(ctx, q, borrowerProfileID, currency)
}

// AddReservedExposure increases reserved exposure for a borrower profile.
func AddReservedExposure[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	amountMinor uint64,
	updatedAt time.Time,
) error {
	const query = `
UPDATE borrower_exposure_summaries
SET
	reserved_exposure_amount_minor = reserved_exposure_amount_minor + $3,
	updated_at = $4
WHERE borrower_profile_id = $1
  AND currency = $2
`

	affected, err := q.Exec(ctx, query, borrowerProfileID, currency, amountMinor, updatedAt)
	if err != nil {
		return fmt.Errorf("add reserved exposure: %w", postgres.MapError(err))
	}

	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrBorrowerExposureSummaryNotFound,
			map[string]any{
				"borrower_profile_id": borrowerProfileID,
				"currency":            currency,
			},
		)
	}

	return nil
}

// SubtractReservedExposure decreases reserved exposure without going below zero.
func SubtractReservedExposure[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	amountMinor uint64,
	updatedAt time.Time,
) error {
	const query = `
UPDATE borrower_exposure_summaries
SET
	reserved_exposure_amount_minor = GREATEST(reserved_exposure_amount_minor - $3, 0),
	updated_at = $4
WHERE borrower_profile_id = $1
  AND currency = $2
`

	affected, err := q.Exec(ctx, query, borrowerProfileID, currency, amountMinor, updatedAt)
	if err != nil {
		return fmt.Errorf("subtract reserved exposure: %w", postgres.MapError(err))
	}

	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrBorrowerExposureSummaryNotFound,
			map[string]any{
				"borrower_profile_id": borrowerProfileID,
				"currency":            currency,
			},
		)
	}

	return nil
}
