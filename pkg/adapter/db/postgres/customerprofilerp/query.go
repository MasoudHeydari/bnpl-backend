package customerprofilerp

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var nationalCodePattern = regexp.MustCompile(`^[0-9]{10}$`)

func validNationalCode(s string) bool { return nationalCodePattern.MatchString(s) }

var (
	// ErrCustomerProfileNotFound indicates that the customer profile does not exist.
	ErrCustomerProfileNotFound = errors.New("customer profile not found")

	customerProfileReturningColumns = `
	id,
	external_subject_id,
	external_customer_id,
	credit_owner_id,
	jurisdiction,
	status,
	created_at,
	updated_at
`
)

// Create inserts a new customer profile and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context, q Q,
	externalSubjectID model.ExternalSubjectID,
	externalCustomerID model.ExternalCustomerID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	createdAt time.Time,
	updatedAt time.Time,
	nationalCode string,
) (model.CustomerProfile, error) {
	profile, created, err := EnsureExists(ctx, q, externalSubjectID, externalCustomerID, jurisdiction, status, createdAt, updatedAt, nationalCode)
	if err != nil {
		return model.CustomerProfile{}, err
	}
	if !created {
		return model.CustomerProfile{}, cerr.Conflict(fmt.Errorf("customer profile already exists for external customer id"))
	}
	return profile, nil
}

// EnsureExists inserts a customer profile when missing and locks the resulting row.
//
// The profile primary key is assigned by the database. Existing rows are not
// updated. Concurrent callers converge on a single row via
// ON CONFLICT (external_customer_id) DO NOTHING followed by FOR UPDATE.
func EnsureExists[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	externalSubjectID model.ExternalSubjectID,
	externalCustomerID model.ExternalCustomerID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	createdAt time.Time,
	updatedAt time.Time,
	nationalCode string,
) (model.CustomerProfile, bool, error) {
	const op = "customerprofilerp.EnsureExists"

	if externalSubjectID.IsZero() {
		return model.CustomerProfile{}, false, errors.New("identity subject id is required")
	}

	if externalCustomerID.IsZero() {
		return model.CustomerProfile{}, false, errors.New("external customer id is required")
	}

	if !validNationalCode(nationalCode) {
		return model.CustomerProfile{}, false, cerr.InvalidArgument(fmt.Errorf("national code must contain exactly ten ASCII digits"))
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

	// Upsert the legal person and lock the same owner row for all concurrent
	// accounts of this national code. All callers execute within one local TX.
	const ownerInsert = `INSERT INTO credit_owners (national_code, created_at)
VALUES ($1,$2) ON CONFLICT (national_code) DO NOTHING`
	if _, err := q.Exec(ctx, ownerInsert, nationalCode, createdAt); err != nil {
		return model.CustomerProfile{}, false, fmt.Errorf("%s: create legal person: %w", op, postgres.MapError(err))
	}
	type ownerIDRow struct {
		ID model.CreditOwnerID `db:"id"`
	}
	owner, err := postgres.QueryOne[ownerIDRow](ctx, q, "lock legal person", `SELECT id FROM credit_owners WHERE national_code=$1 FOR UPDATE`, nationalCode)
	if err != nil {
		return model.CustomerProfile{}, false, fmt.Errorf("%s: lock legal person: %w", op, err)
	}

	const insertQuery = `
INSERT INTO customer_profiles (
	external_subject_id,
	external_customer_id,
	credit_owner_id,
	jurisdiction,
	status,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (external_customer_id) DO NOTHING
`

	affected, err := q.Exec(
		ctx,
		insertQuery,
		externalSubjectID,
		externalCustomerID,
		owner.ID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
	if err != nil {
		// The stable Identity subject is already bound to another CRS account.
		// This is an immutable identity conflict, not an invitation to re-key
		// either profile or silently adopt a different legal-person owner.
		if postgres.IsUniqueViolation(err, "customer_profiles_external_subject_id_key") {
			return model.CustomerProfile{}, false, cerr.Conflict(fmt.Errorf("%s: identity subject already belongs to another customer account", op))
		}
		return model.CustomerProfile{}, false, fmt.Errorf(
			"%s: ensure customer profile: %w", op, postgres.MapError(err),
		)
	}

	profile, err := LockByExternalCustomerID(ctx, q, externalCustomerID)
	if err != nil {
		return model.CustomerProfile{}, false, err
	}

	// Neither a reused CRS customer_id nor a reused Identity subject may be
	// reattached to another national code. Never silently rewrite identity.
	if profile.ExternalSubjectID != externalSubjectID || profile.CreditOwnerID != owner.ID || profile.Jurisdiction != jurisdiction {
		return model.CustomerProfile{}, false, cerr.Conflict(fmt.Errorf("%s: immutable customer identity does not match CRS", op))
	}
	return profile, affected > 0, nil
}

// GetByID returns a customer profile by id.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id model.CustomerID) (model.CustomerProfile, error) {
	const query = `
		SELECT
			id,
			external_subject_id,
			external_customer_id,
			credit_owner_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM customer_profiles
		WHERE id = $1
		`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"get customer profile by id",
		query,
		id,
	)
}

// GetByExternalSubjectID returns a customer profile by Identity Service subject id.
func GetByExternalSubjectID[Q postgres.Queryer](ctx context.Context, q Q, externalSubjectID model.ExternalSubjectID) (model.CustomerProfile, error) {
	const query = `
		SELECT
			id,
			external_subject_id,
			external_customer_id,
			credit_owner_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM customer_profiles
		WHERE external_subject_id = $1
		`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"get customer profile by identity subject id",
		query,
		externalSubjectID,
	)
}

// GetByExternalCustomerID returns a customer profile by Customer Reference Service customer id.
func GetByExternalCustomerID[Q postgres.Queryer](ctx context.Context, q Q, externalCustomerID model.ExternalCustomerID) (model.CustomerProfile, error) {
	const query = `
		SELECT
			id,
			external_subject_id,
			external_customer_id,
			credit_owner_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM customer_profiles
		WHERE external_customer_id = $1
		`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"get customer profile by external customer id",
		query,
		externalCustomerID,
	)
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

	value, err := postgres.QueryOne[result](
		ctx,
		q,
		"check customer profile existence by external customer id",
		query,
		externalCustomerID,
	)
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
	external_subject_id,
	external_customer_id,
	credit_owner_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM customer_profiles
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"lock customer profile by id",
		query,
		id,
	)
}

// LockByExternalSubjectID returns a customer profile by Identity subject id and locks it for update.
func LockByExternalSubjectID[Q postgres.Queryer](ctx context.Context, q Q, externalSubjectID model.ExternalSubjectID) (model.CustomerProfile, error) {
	const query = `
SELECT
	id,
	external_subject_id,
	external_customer_id,
	credit_owner_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM customer_profiles
WHERE external_subject_id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"lock customer profile by identity subject id",
		query,
		externalSubjectID,
	)
}

// LockByExternalCustomerID returns a customer profile by externalCustomerID and locks it for update.
func LockByExternalCustomerID[Q postgres.Queryer](ctx context.Context, q Q, externalCustomerID model.ExternalCustomerID) (model.CustomerProfile, error) {
	const query = `
SELECT
	id,
	external_subject_id,
	external_customer_id,
	credit_owner_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM customer_profiles
WHERE external_customer_id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"lock customer profile by external customer id",
		query,
		externalCustomerID,
	)
}

// Update persists the full customer profile state and returns the updated row.
func Update[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.CustomerID,
	externalSubjectID model.ExternalSubjectID,
	externalCustomerID model.ExternalCustomerID,
	jurisdiction model.Jurisdiction,
	status model.CustomerStatus,
	updatedAt time.Time,
) (model.CustomerProfile, error) {
	query := `
UPDATE customer_profiles
SET
	status = $5,
	updated_at = $6
WHERE id = $1 AND external_subject_id = $2
    AND external_customer_id = $3 AND jurisdiction = $4
RETURNING
` + customerProfileReturningColumns

	return postgres.QueryOne[model.CustomerProfile](
		ctx,
		q,
		"update customer profile",
		query,
		id,
		externalSubjectID,
		externalCustomerID,
		jurisdiction,
		status,
		updatedAt,
	)
}
