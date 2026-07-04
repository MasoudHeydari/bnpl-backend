package borrowerexposurereservationrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const borrowerExposureReservationColumns = `
	id,
	checkout_intent_id,
	decision_id,
	borrower_profile_id,
	amount_minor,
	currency,
	status,
	expires_at,
	created_at,
	updated_at
`

// Create inserts a borrower exposure reservation and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	checkoutIntentID uuid.UUID,
	decisionID uuid.UUID,
	borrowerProfileID uuid.UUID,
	amountMinor uint64,
	currency model.Currency,
	status model.BorrowerExposureReservationStatus,
	expiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerExposureReservation, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"expires_at": expiresAt,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.BorrowerExposureReservation{}, err
	}

	const query = `
INSERT INTO borrower_exposure_reservations (
	checkout_intent_id,
	decision_id,
	borrower_profile_id,
	amount_minor,
	currency,
	status,
	expires_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
RETURNING` + borrowerExposureReservationColumns

	return postgres.QueryOne[model.BorrowerExposureReservation](
		ctx,
		q,
		"create borrower exposure reservation",
		query,
		checkoutIntentID,
		decisionID,
		borrowerProfileID,
		amountMinor,
		currency,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

// GetByID returns a borrower exposure reservation by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	const query = `SELECT` + borrowerExposureReservationColumns + `FROM borrower_exposure_reservations WHERE id = $1`

	return postgres.QueryOne[model.BorrowerExposureReservation](
		ctx,
		q,
		"get borrower exposure reservation by id",
		query,
		id,
	)
}

// GetByDecisionID returns a borrower exposure reservation by decision id.
func GetByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	const query = `SELECT` + borrowerExposureReservationColumns + `FROM borrower_exposure_reservations WHERE decision_id = $1`

	return postgres.QueryOne[model.BorrowerExposureReservation](
		ctx,
		q,
		"get borrower exposure reservation by decision id",
		query,
		decisionID,
	)
}

// GetReservedByCheckoutIntentID returns the live reserved exposure for a checkout intent.
func GetReservedByCheckoutIntentID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	checkoutIntentID uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	const query = `SELECT` + borrowerExposureReservationColumns + `
FROM borrower_exposure_reservations
WHERE checkout_intent_id = $1
  AND status = $2
`

	return postgres.QueryOne[model.BorrowerExposureReservation](
		ctx,
		q,
		"get reserved borrower exposure reservation by checkout intent id",
		query,
		checkoutIntentID,
		model.BorrowerExposureReservationReserved,
	)
}

// ExpireDueByBorrowerProfileIDAndCurrency marks due reserved exposure as expired.
func ExpireDueByBorrowerProfileIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	now time.Time,
	updatedAt time.Time,
) (uint64, error) {
	const query = `
WITH expired AS (
	UPDATE borrower_exposure_reservations
	SET
		status = $5,
		updated_at = $4
	WHERE borrower_profile_id = $1
	  AND currency = $2
	  AND status = $6
	  AND expires_at <= $3
	RETURNING amount_minor
)
SELECT COALESCE(SUM(amount_minor), 0)::bigint AS expired_amount_minor
FROM expired
`

	type expiredAmountResult struct {
		ExpiredAmountMinor uint64 `db:"expired_amount_minor"`
	}

	result, err := postgres.QueryOne[expiredAmountResult](
		ctx,
		q,
		"expire due borrower exposure reservations",
		query,
		borrowerProfileID,
		currency,
		now,
		updatedAt,
		model.BorrowerExposureReservationExpired,
		model.BorrowerExposureReservationReserved,
	)
	if err != nil {
		return 0, err
	}

	return result.ExpiredAmountMinor, nil
}
