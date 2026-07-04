package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CustomerProfileQueryer defines read-only customer profile operations.
type CustomerProfileQueryer interface {
	// GetByID returns the customer profile identified by the internal BNPL
	// primary key (customer_profiles.id).
	GetByID(
		ctx context.Context,
		id model.CustomerID,
	) (model.CustomerProfile, error)

	// GetByIdentitySubjectID returns the BNPL profile for an external Identity Service
	// subject ID (customer_profiles.identity_subject_id / x-auth-subject-id).
	GetByIdentitySubjectID(
		ctx context.Context,
		identitySubjectID model.IdentitySubjectID,
	) (model.CustomerProfile, error)

	// ExistsByIdentitySubjectID reports whether an external Identity Service subject
	// already has a BNPL profile (lookup by customer_profiles.identity_subject_id).
	ExistsByIdentitySubjectID(
		ctx context.Context,
		identitySubjectID model.IdentitySubjectID,
	) (bool, error)
}

// CustomerProfileConnQueryer exposes customer profile operations on a connection.
type CustomerProfileConnQueryer interface {
	// CustomerProfileQueryer provides shared customer profile query operations.
	CustomerProfileQueryer
}

// CustomerProfileTxQueryer exposes transaction-scoped customer profile operations.
type CustomerProfileTxQueryer interface {
	// Create stores a new customer profile inside the current transaction.
	Create(
		ctx context.Context,
		identitySubjectID model.IdentitySubjectID,
		jurisdiction model.Jurisdiction,
		status model.CustomerStatus,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.CustomerProfile, error)

	// EnsureExists inserts a customer profile when missing for the external
	// Identity Service subject ID (identity_subject_id) and locks the row.
	//
	// The profile primary key (id) is assigned by the database. Existing
	// profiles are left unchanged. The boolean is true when this call inserted
	// the row.
	EnsureExists(
		ctx context.Context,
		identitySubjectID model.IdentitySubjectID,
		jurisdiction model.Jurisdiction,
		status model.CustomerStatus,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.CustomerProfile, bool, error)

	// LockByID returns the customer profile and locks it for state changes
	// using the internal BNPL primary key (customer_profiles.id).
	LockByID(
		ctx context.Context,
		id model.CustomerID,
	) (model.CustomerProfile, error)

	// LockByIdentitySubjectID returns the customer profile for an external Identity
	// Service subject ID (customer_profiles.identity_subject_id) and locks it for
	// state changes.
	LockByIdentitySubjectID(
		ctx context.Context,
		identitySubjectID model.IdentitySubjectID,
	) (model.CustomerProfile, error)

	// Update persists the full customer profile state and returns the updated row.
	Update(
		ctx context.Context,
		id model.CustomerID,
		identitySubjectID model.IdentitySubjectID,
		jurisdiction model.Jurisdiction,
		status model.CustomerStatus,
		updatedAt time.Time,
	) (model.CustomerProfile, error)
}

// CustomerProfile persists BNPL-specific customer profile records.
type CustomerProfile interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) CustomerProfileConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) CustomerProfileTxQueryer
}
