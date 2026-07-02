package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// BorrowerCreditLimitQueryer defines read-only borrower credit limit operations.
type BorrowerCreditLimitQueryer interface {
	// GetByID returns the credit limit identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.BorrowerCreditLimit, error)

	// GetCurrentByBorrowerProfileID returns the active credit limit at a point in time.
	GetCurrentByBorrowerProfileID(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		at time.Time,
	) (model.BorrowerCreditLimit, error)

	// ListByBorrowerProfileID returns all credit limits for a borrower profile.
	ListByBorrowerProfileID(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
	) ([]model.BorrowerCreditLimit, error)
}

// BorrowerCreditLimitConnQueryer exposes borrower credit limit operations on a connection.
type BorrowerCreditLimitConnQueryer interface {
	BorrowerCreditLimitQueryer
}

// BorrowerCreditLimitTxQueryer exposes transaction-scoped borrower credit limit operations.
type BorrowerCreditLimitTxQueryer interface {
	// Create stores a new borrower credit limit inside the current transaction.
	Create(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		decisionID uuid.UUID,
		amountMinor uint64,
		currency model.Currency,
		status model.BorrowerCreditLimitStatus,
		effectiveFrom time.Time,
		expiresAt *time.Time,
		revokedAt *time.Time,
		replacedByCreditLimitID *uuid.UUID,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.BorrowerCreditLimit, error)

	// LockCurrentByBorrowerProfileID returns and locks the active credit limit at a point in time.
	LockCurrentByBorrowerProfileID(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		at time.Time,
	) (model.BorrowerCreditLimit, error)

	// Revoke marks a credit limit as revoked by a later governed decision.
	Revoke(
		ctx context.Context,
		id uuid.UUID,
		revokedAt time.Time,
		updatedAt time.Time,
	) error

	// MarkExpired marks a credit limit as expired.
	MarkExpired(
		ctx context.Context,
		id uuid.UUID,
		updatedAt time.Time,
	) error

	// MarkReplaced links a credit limit to the newer assignment that replaced it.
	MarkReplaced(
		ctx context.Context,
		id uuid.UUID,
		replacedByCreditLimitID uuid.UUID,
		updatedAt time.Time,
	) error
}

// BorrowerCreditLimit persists governed borrower credit limit assignments.
type BorrowerCreditLimit interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) BorrowerCreditLimitConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) BorrowerCreditLimitTxQueryer
}
