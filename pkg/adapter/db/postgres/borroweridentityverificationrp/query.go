package borroweridentityverificationrp

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
	// ErrBorrowerIdentityVerificationNotFound indicates that the verification artifact does not exist.
	ErrBorrowerIdentityVerificationNotFound = errors.New("borrower identity verification not found")
)

// Create inserts a new borrower identity verification artifact and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerID uuid.UUID,
	nationalIDHash string,
	phoneNumberHash string,
	birthdate time.Time,
	status model.BorrowerIdentityVerificationStatus,
	provider string,
	providerReferenceID *string,
	verifiedAt *time.Time,
	rejectedAt *time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerIdentityVerification, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"birthdate":  birthdate,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.BorrowerIdentityVerification{}, err
	}

	const query = `
INSERT INTO borrower_identity_verifications (
	borrower_id,
	national_id_hash,
	phone_number_hash,
	birthdate,
	status,
	provider,
	provider_reference_id,
	verified_at,
	rejected_at,
	expires_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING
	id,
	borrower_id,
	national_id_hash,
	phone_number_hash,
	birthdate,
	status,
	provider,
	provider_reference_id,
	verified_at,
	rejected_at,
	expires_at,
	created_at,
	updated_at
`

	return postgres.QueryOne[model.BorrowerIdentityVerification](
		ctx,
		q,
		"create borrower identity verification",
		query,
		borrowerID,
		nationalIDHash,
		phoneNumberHash,
		birthdate,
		status,
		provider,
		providerReferenceID,
		verifiedAt,
		rejectedAt,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

// GetByID returns a verification artifact by id.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) (model.BorrowerIdentityVerification, error) {
	const query = `
SELECT
	id,
	borrower_id,
	national_id_hash,
	phone_number_hash,
	birthdate,
	status,
	provider,
	provider_reference_id,
	verified_at,
	rejected_at,
	expires_at,
	created_at,
	updated_at
FROM borrower_identity_verifications
WHERE id = $1
`

	return getOne(ctx, q, query, id, "get borrower identity verification by id")
}

// GetLatestByBorrowerID returns the newest verification artifact for borrowerID.
func GetLatestByBorrowerID[Q postgres.Queryer](ctx context.Context, q Q, borrowerID uuid.UUID) (model.BorrowerIdentityVerification, error) {
	const query = `
SELECT
	id,
	borrower_id,
	national_id_hash,
	phone_number_hash,
	birthdate,
	status,
	provider,
	provider_reference_id,
	verified_at,
	rejected_at,
	expires_at,
	created_at,
	updated_at
FROM borrower_identity_verifications
WHERE borrower_id = $1
ORDER BY created_at DESC
LIMIT 1
`

	return getOne(ctx, q, query, borrowerID, "get latest borrower identity verification")
}

// FindByNationalIDHash returns a verification artifact by hashed national identifier.
func FindByNationalIDHash[Q postgres.Queryer](ctx context.Context, q Q, nationalIDHash string) (model.BorrowerIdentityVerification, error) {
	const query = `
SELECT
	id,
	borrower_id,
	national_id_hash,
	phone_number_hash,
	birthdate,
	status,
	provider,
	provider_reference_id,
	verified_at,
	rejected_at,
	expires_at,
	created_at,
	updated_at
FROM borrower_identity_verifications
WHERE national_id_hash = $1
ORDER BY created_at DESC
LIMIT 1
`

	return getOne(ctx, q, query, nationalIDHash, "find borrower identity verification by national id hash")
}

// LockByID returns a verification artifact by id and locks it for update.
func LockByID[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) (model.BorrowerIdentityVerification, error) {
	const query = `
SELECT
	id,
	borrower_id,
	national_id_hash,
	phone_number_hash,
	birthdate,
	status,
	provider,
	provider_reference_id,
	verified_at,
	rejected_at,
	expires_at,
	created_at,
	updated_at
FROM borrower_identity_verifications
WHERE id = $1
FOR UPDATE
`

	return getOne(ctx, q, query, id, "lock borrower identity verification by id")
}

// Update persists the full verification artifact state.
func Update[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
	borrowerID uuid.UUID,
	nationalIDHash string,
	phoneNumberHash string,
	birthdate time.Time,
	status model.BorrowerIdentityVerificationStatus,
	provider string,
	providerReferenceID *string,
	verifiedAt *time.Time,
	rejectedAt *time.Time,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) error {
	const query = `
UPDATE borrower_identity_verifications
SET
	borrower_id = $2,
	national_id_hash = $3,
	phone_number_hash = $4,
	birthdate = $5,
	status = $6,
	provider = $7,
	provider_reference_id = $8,
	verified_at = $9,
	rejected_at = $10,
	expires_at = $11,
	created_at = $12,
	updated_at = $13
WHERE id = $1
`

	affected, err := q.Exec(
		ctx,
		query,
		id,
		borrowerID,
		nationalIDHash,
		phoneNumberHash,
		birthdate,
		status,
		provider,
		providerReferenceID,
		verifiedAt,
		rejectedAt,
		expiresAt,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf("update borrower identity verification %s: %w", id, postgres.MapError(err))
	}

	if affected == 0 {
		return cerr.ResourceNotFound(
			ErrBorrowerIdentityVerificationNotFound,
			map[string]any{
				"verification_id": id,
			},
		)
	}

	return nil
}

func getOne[Q postgres.Queryer](ctx context.Context, q Q, query string, arg any, operation string) (model.BorrowerIdentityVerification, error) {
	rows, err := q.Query(ctx, query, arg)
	if err != nil {
		return model.BorrowerIdentityVerification{}, fmt.Errorf("%s: %w", operation, postgres.MapError(err))
	}
	defer rows.Close()

	verification, err := postgres.ValueInto[model.BorrowerIdentityVerification](rows.(postgres.Rows))
	if err != nil {
		return model.BorrowerIdentityVerification{}, fmt.Errorf("%s: %w", operation, err)
	}

	return verification, nil
}
