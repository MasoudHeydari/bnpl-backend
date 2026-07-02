package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// BorrowerIdentityVerificationQueryer defines read-only verification operations.
type BorrowerIdentityVerificationQueryer interface {
	// GetByID returns the verification artifact identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.BorrowerIdentityVerification, error)

	// GetLatestByBorrowerID returns the newest verification artifact for a borrower.
	GetLatestByBorrowerID(
		ctx context.Context,
		borrowerID uuid.UUID,
	) (model.BorrowerIdentityVerification, error)

	// FindByNationalIDHash returns a verification artifact by hashed national identifier.
	FindByNationalIDHash(
		ctx context.Context,
		nationalIDHash string,
	) (model.BorrowerIdentityVerification, error)
}

// BorrowerIdentityVerificationConnQueryer exposes verification operations on a connection.
type BorrowerIdentityVerificationConnQueryer interface {
	BorrowerIdentityVerificationQueryer
}

// BorrowerIdentityVerificationTxQueryer exposes transaction-scoped verification operations.
type BorrowerIdentityVerificationTxQueryer interface {
	BorrowerIdentityVerificationQueryer

	// Create stores a new verification artifact inside the current transaction.
	Create(
		ctx context.Context,
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
	) (model.BorrowerIdentityVerification, error)

	// LockByID returns the verification artifact and locks it for state changes.
	LockByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.BorrowerIdentityVerification, error)

	// Update persists the full verification artifact state.
	Update(
		ctx context.Context,
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
	) error
}

// BorrowerIdentityVerification persists borrower verification artifacts.
type BorrowerIdentityVerification interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) BorrowerIdentityVerificationConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) BorrowerIdentityVerificationTxQueryer
}
