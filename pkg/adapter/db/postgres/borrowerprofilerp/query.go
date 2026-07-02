package borrowerprofilerp

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
	// ErrBorrowerProfileNotFound indicates that the borrower profile does not exist.
	ErrBorrowerProfileNotFound = errors.New("borrower profile not found")
)

// Create inserts a new borrower profile and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerID uuid.UUID,
	identityVerificationID *uuid.UUID,
	lastProfileEligibilityDecisionID *uuid.UUID,
	lastCreditLimitID *uuid.UUID,
	jurisdiction model.Jurisdiction,
	status model.BorrowerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerProfile, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.BorrowerProfile{}, err
	}

	const query = `
INSERT INTO borrower_profiles (
	borrower_id,
	identity_verification_id,
	last_profile_eligibility_decision_id,
	last_credit_limit_id,
	jurisdiction,
	status,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING
	id,
	borrower_id,
	identity_verification_id,
	last_profile_eligibility_decision_id,
	last_credit_limit_id,
	jurisdiction,
	status,
	created_at,
	updated_at
`

	return postgres.QueryOne[model.BorrowerProfile](
		ctx,
		q,
		"create borrower profile",
		query,
		borrowerID,
		identityVerificationID,
		lastProfileEligibilityDecisionID,
		lastCreditLimitID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
}

// GetByID returns a borrower profile by id.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) (model.BorrowerProfile, error) {
	const query = `
		SELECT
			id,
			borrower_id,
			identity_verification_id,
			last_profile_eligibility_decision_id,
			last_credit_limit_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM borrower_profiles
		WHERE id = $1
		`

	return getOne(ctx, q, query, id, "get borrower profile by id")
}

// GetByBorrowerID returns a borrower profile by borrower identity id.
func GetByBorrowerID[Q postgres.Queryer](ctx context.Context, q Q, borrowerID uuid.UUID) (model.BorrowerProfile, error) {
	const query = `
		SELECT
			id,
			borrower_id,
			identity_verification_id,
			last_profile_eligibility_decision_id,
			last_credit_limit_id,
			jurisdiction,
			status,
			created_at,
			updated_at
		FROM borrower_profiles
		WHERE borrower_id = $1
		`

	return getOne(ctx, q, query, borrowerID, "get borrower profile by borrower id")
}

// ExistsByBorrowerID reports whether a borrower profile exists for borrowerID.
func ExistsByBorrowerID[Q postgres.Queryer](ctx context.Context, q Q, borrowerID uuid.UUID) (bool, error) {
	const query = `
SELECT EXISTS (
	SELECT 1
	FROM borrower_profiles
	WHERE borrower_id = $1
) AS exists
`

	rows, err := q.Query(
		ctx,
		query,
		borrowerID,
	)
	if err != nil {
		return false, fmt.Errorf("check borrower profile existence: %w", postgres.MapError(err))
	}
	defer rows.Close()

	type result struct {
		Exists bool `db:"exists"`
	}

	value, err := postgres.ValueInto[result](rows.(postgres.Rows))
	if err != nil {
		return false, fmt.Errorf("check borrower profile existence: %w", err)
	}

	return value.Exists, nil
}

// LockByID returns a borrower profile by id and locks it for update.
func LockByID[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) (model.BorrowerProfile, error) {
	const query = `
SELECT
	id,
	borrower_id,
	identity_verification_id,
	last_profile_eligibility_decision_id,
	last_credit_limit_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM borrower_profiles
WHERE id = $1
FOR UPDATE
`

	return getOne(ctx, q, query, id, "lock borrower profile by id")
}

// LockByBorrowerID returns a borrower profile by borrowerID and locks it for update.
func LockByBorrowerID[Q postgres.Queryer](ctx context.Context, q Q, borrowerID uuid.UUID) (model.BorrowerProfile, error) {
	const query = `
SELECT
	id,
	borrower_id,
	identity_verification_id,
	last_profile_eligibility_decision_id,
	last_credit_limit_id,
	jurisdiction,
	status,
	created_at,
	updated_at
FROM borrower_profiles
WHERE borrower_id = $1
FOR UPDATE
`

	return getOne(ctx, q, query, borrowerID, "lock borrower profile by borrower id")
}

// Update persists the full borrower profile state.
func Update[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
	borrowerID uuid.UUID,
	identityVerificationID *uuid.UUID,
	lastProfileEligibilityDecisionID *uuid.UUID,
	lastCreditLimitID *uuid.UUID,
	jurisdiction model.Jurisdiction,
	status model.BorrowerStatus,
	createdAt time.Time,
	updatedAt time.Time,
) error {
	const query = `
UPDATE borrower_profiles
SET
	borrower_id = $2,
	identity_verification_id = $3,
	last_profile_eligibility_decision_id = $4,
	last_credit_limit_id = $5,
	jurisdiction = $6,
	status = $7,
	created_at = $8,
	updated_at = $9
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		id,
		borrowerID,
		identityVerificationID,
		lastProfileEligibilityDecisionID,
		lastCreditLimitID,
		jurisdiction,
		status,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("update borrower profile %s: %w", id, postgres.MapError(err))
	}

	return ensureAffected("update borrower profile", affected, id)
}

// SetIdentityVerificationID records the latest identity verification artifact.
func SetIdentityVerificationID[Q postgres.Queryer](ctx context.Context, q Q, profileID uuid.UUID, identityVerificationID uuid.UUID, updatedAt time.Time) error {
	const query = `
UPDATE borrower_profiles
SET
	identity_verification_id = $2,
	updated_at = $3
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		profileID,
		identityVerificationID,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("set borrower profile identity verification %s: %w", profileID, postgres.MapError(err))
	}

	return ensureAffected("set borrower profile identity verification", affected, profileID)
}

// SetLastProfileEligibilityDecisionID records the latest profile-level eligibility decision.
func SetLastProfileEligibilityDecisionID[Q postgres.Queryer](ctx context.Context, q Q, profileID uuid.UUID, decisionID uuid.UUID, updatedAt time.Time) error {
	const query = `
UPDATE borrower_profiles
SET
	last_profile_eligibility_decision_id = $2,
	updated_at = $3
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		profileID,
		decisionID,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("set borrower profile eligibility decision %s: %w", profileID, postgres.MapError(err))
	}

	return ensureAffected("set borrower profile eligibility decision", affected, profileID)
}

// SetLastCreditLimitID records the current credit limit shortcut for the profile.
func SetLastCreditLimitID[Q postgres.Queryer](ctx context.Context, q Q, profileID uuid.UUID, creditLimitID uuid.UUID, updatedAt time.Time) error {
	const query = `
UPDATE borrower_profiles
SET
	last_credit_limit_id = $2,
	updated_at = $3
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		profileID,
		creditLimitID,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("set borrower profile credit limit %s: %w", profileID, postgres.MapError(err))
	}

	return ensureAffected("set borrower profile credit limit", affected, profileID)
}

func getOne[Q postgres.Queryer](ctx context.Context, q Q, query string, arg any, operation string) (model.BorrowerProfile, error) {
	rows, err := q.Query(ctx, query, arg)
	if err != nil {
		return model.BorrowerProfile{}, fmt.Errorf("%s: %w", operation, postgres.MapError(err))
	}
	defer rows.Close()

	profile, err := postgres.ValueInto[model.BorrowerProfile](rows.(postgres.Rows))
	if err != nil {
		return model.BorrowerProfile{}, fmt.Errorf("%s: %w", operation, err)
	}

	return profile, nil
}

func ensureAffected(operation string, affected int64, id uuid.UUID) error {
	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrBorrowerProfileNotFound,
			map[string]any{
				"operation":  operation,
				"profile_id": id,
			},
		)
	}

	return nil
}
