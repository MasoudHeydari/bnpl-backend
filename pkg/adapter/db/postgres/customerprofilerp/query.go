package customerprofilerp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var (
	// ErrCustomerProfileNotFound indicates that the customer profile does not exist.
	ErrCustomerProfileNotFound = errors.New("customer profile not found")

	customerProfileReturningColumns = `
	id,
	external_customer_id,
	jurisdiction,
	status,
	created_at,
	updated_at
`
)

// Create inserts a new customer profile and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	externalCustomerID model.ExternalCustomerID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CustomerProfile, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.CustomerProfile{}, err
	}

	const query = `
INSERT INTO customer_profiles (
	external_customer_id,
	jurisdiction,
	status,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5)
RETURNING
	id,
	external_customer_id,
	jurisdiction,
	status,
	created_at,
	updated_at
`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"create customer profile",
		query,
		externalCustomerID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
}

// EnsureExists inserts a customer profile when missing and locks the resulting row.
//
// The profile primary key is assigned by the database. Existing rows are not
// updated. Concurrent callers converge on a single row via
// ON CONFLICT (external_customer_id) DO NOTHING followed by FOR UPDATE.
func EnsureExists[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	externalCustomerID model.ExternalCustomerID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CustomerProfile, bool, error) {
	const op = "customerprofilerp.EnsureExists"

	if externalCustomerID.IsZero() {
		return model.CustomerProfile{}, false, errors.New("identity subject id is required")
	}

	if !jurisdiction.IsValid() {
		return model.CustomerProfile{}, false, errors.New("jurisdiction is invalid")
	}

	if !status.IsValid() {
		return model.CustomerProfile{}, false, errors.New("status is invalid")
	}

	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.CustomerProfile{}, false, err
	}

	const insertQuery = `
INSERT INTO customer_profiles (
	external_customer_id,
	jurisdiction,
	status,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (external_customer_id) DO NOTHING
`

	affected, err := q.Exec(
		ctx,
		insertQuery,
		externalCustomerID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return model.CustomerProfile{}, false, fmt.Errorf(
			"%s: ensure customer profile: %w", op, postgres.MapError(err),
		)
	}

	profile, err := LockByExternalCustomerID(ctx, q, externalCustomerID)
	if err != nil {
		return model.CustomerProfile{}, false, err
	}

	return profile, affected > 0, nil
}

// GetByID returns a customer profile by id.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id model.CustomerID) (model.CustomerProfile, error) {
	const query = `
		SELECT
			id,
			external_customer_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM customer_profiles
		WHERE id = $1
		`

	return postgres.QueryOne[model.CustomerProfile](ctx, q, "get customer profile by id", query, id)
}

// GetByExternalCustomerID returns a customer profile by Identity Service subject id.
func GetByExternalCustomerID[Q postgres.Queryer](ctx context.Context, q Q, externalCustomerID model.ExternalCustomerID) (model.CustomerProfile, error) {
	const query = `
		SELECT
			id,
			external_customer_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM customer_profiles
		WHERE external_customer_id = $1
		`

	return postgres.QueryOne[model.CustomerProfile](ctx, q, "get customer profile by identity subject id", query, externalCustomerID)
}

// ExistsByExternalCustomerID reports whether a customer profile exists for externalCustomerID.
func ExistsByExternalCustomerID[Q postgres.Queryer](ctx context.Context, q Q, externalCustomerID model.ExternalCustomerID) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM customer_profiles
	WHERE external_customer_id = $1
) AS exists
`

	type result struct {
		Exists bool `db:"exists"`
	}

	value, err := postgres.QueryOne[result](ctx, q, "check customer profile existence by identity subject id", query, externalCustomerID)
	if err != nil {
		return false, err
	}

	return value.Exists, nil
}

// LockByID returns a customer profile by id and locks it for update.
func LockByID[Q postgres.Queryer](ctx context.Context, q Q, id model.CustomerID) (model.CustomerProfile, error) {
	const query = `
SELECT
	id,
	external_customer_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM customer_profiles
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CustomerProfile](ctx, q, "lock customer profile by id", query, id)
}

// LockByExternalCustomerID returns a customer profile by externalCustomerID and locks it for update.
func LockByExternalCustomerID[Q postgres.Queryer](ctx context.Context, q Q, externalCustomerID model.ExternalCustomerID) (model.CustomerProfile, error) {
	const query = `
SELECT
	id,
	external_customer_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM customer_profiles
WHERE external_customer_id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CustomerProfile](ctx, q, "lock customer profile by identity subject id", query, externalCustomerID)
}

// Update persists the full customer profile state and returns the updated row.
func Update[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CustomerID,
	externalCustomerID model.ExternalCustomerID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	updatedAt time.Time,
) (model.CustomerProfile, error) {
	query := `
UPDATE customer_profiles
SET
	external_customer_id = $2,
	jurisdiction = $3,
	status = $4,
	updated_at = $5
WHERE id = $1
RETURNING
` + customerProfileReturningColumns

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"update customer profile",
		query,
		id,
		externalCustomerID,
		jurisdiction,
		status,
		updatedAt,
	)
}
