package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// BorrowerProfileQueryer defines read-only borrower profile operations.
type BorrowerProfileQueryer interface {
	// GetByID returns the borrower profile identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.BorrowerProfile, error)

	// GetByBorrowerID returns the BNPL profile for a borrower identity.
	GetByBorrowerID(
		ctx context.Context,
		borrowerID uuid.UUID,
	) (model.BorrowerProfile, error)

	// ExistsByBorrowerID reports whether a borrower identity already has a BNPL profile.
	ExistsByBorrowerID(
		ctx context.Context,
		borrowerID uuid.UUID,
	) (bool, error)
}

// BorrowerProfileConnQueryer exposes borrower profile operations on a connection.
type BorrowerProfileConnQueryer interface {
	BorrowerProfileQueryer
}

// BorrowerProfileTxQueryer exposes transaction-scoped borrower profile operations.
type BorrowerProfileTxQueryer interface {
	// Create stores a new borrower profile inside the current transaction.
	Create(
		ctx context.Context,
		borrowerID uuid.UUID,
		identityVerificationID *uuid.UUID,
		lastProfileEligibilityDecisionID *uuid.UUID,
		lastCreditLimitID *uuid.UUID,
		jurisdiction model.Jurisdiction,
		status model.BorrowerStatus,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.BorrowerProfile, error)

	// LockByID returns the borrower profile and locks it for state changes.
	LockByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.BorrowerProfile, error)

	// LockByBorrowerID returns the borrower profile for borrowerID and locks it for state changes.
	LockByBorrowerID(
		ctx context.Context,
		borrowerID uuid.UUID,
	) (model.BorrowerProfile, error)

	// Update persists the full borrower profile state.
	Update(
		ctx context.Context,
		id uuid.UUID,
		borrowerID uuid.UUID,
		identityVerificationID *uuid.UUID,
		lastProfileEligibilityDecisionID *uuid.UUID,
		lastCreditLimitID *uuid.UUID,
		jurisdiction model.Jurisdiction,
		status model.BorrowerStatus,
		createdAt time.Time,
		updatedAt time.Time,
	) error

	// SetIdentityVerificationID records the latest identity verification artifact.
	SetIdentityVerificationID(
		ctx context.Context,
		profileID uuid.UUID,
		identityVerificationID uuid.UUID,
		updatedAt time.Time,
	) error

	// SetLastProfileEligibilityDecisionID records the latest profile-level eligibility decision.
	SetLastProfileEligibilityDecisionID(
		ctx context.Context,
		profileID uuid.UUID,
		decisionID uuid.UUID,
		updatedAt time.Time,
	) error

	// SetLastCreditLimitID records the current credit limit shortcut for the profile.
	SetLastCreditLimitID(
		ctx context.Context,
		profileID uuid.UUID,
		creditLimitID uuid.UUID,
		updatedAt time.Time,
	) error
}

// BorrowerProfile persists BNPL-specific borrower profile records.
type BorrowerProfile interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) BorrowerProfileConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) BorrowerProfileTxQueryer
}
