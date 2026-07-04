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

	// GetByExternalCustomerID returns the BNPL profile for an external Identity Service
	// subject ID (customer_profiles.external_customer_id / x-auth-subject-id).
	GetByExternalCustomerID(
		ctx context.Context,
		externalCustomerID model.ExternalCustomerID,
	) (model.CustomerProfile, error)

	// ExistsByExternalCustomerID reports whether an external Identity Service subject
	// already has a BNPL profile (lookup by customer_profiles.external_customer_id).
	ExistsByExternalCustomerID(
		ctx context.Context,
		externalCustomerID model.ExternalCustomerID,
	) (bool, error)
}

// CustomerProfileConnQueryer exposes customer profile operations on a connection.
type CustomerProfileConnQueryer interface {
	// CustomerProfileQueryer provides shared customer profile query operations.
	CustomerProfileQueryer
}

// CustomerProfileTxQueryer exposes transaction-scoped customer profile operations.
type CustomerProfileTxQueryer interface {
	// GetByExternalCustomerID reads the profile without locking in the current snapshot.
	GetByExternalCustomerID(ctx context.Context, externalCustomerID model.ExternalCustomerID) (model.CustomerProfile, error)

	// Create stores a new customer profile inside the current transaction.
	Create(
		ctx context.Context,
		externalCustomerID model.ExternalCustomerID,
		jurisdiction model.Jurisdiction,
		status model.CustomerStatus,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.CustomerProfile, error)

	// EnsureExists inserts a customer profile when missing for the external
	// Identity Service subject ID (external_customer_id) and locks the row.
	//
	// The profile primary key (id) is assigned by the database. Existing
	// profiles are left unchanged. The boolean is true when this call inserted
	// the row.
	EnsureExists(
		ctx context.Context,
		externalCustomerID model.ExternalCustomerID,
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

	// LockByExternalCustomerID returns the customer profile for an external Identity
	// Service subject ID (customer_profiles.external_customer_id) and locks it for
	// state changes.
	LockByExternalCustomerID(
		ctx context.Context,
		externalCustomerID model.ExternalCustomerID,
	) (model.CustomerProfile, error)

	// Update persists the full customer profile state and returns the updated row.
	Update(
		ctx context.Context,
		id model.CustomerID,
		externalCustomerID model.ExternalCustomerID,
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
