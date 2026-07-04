package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// BorrowerExposureReservationQueryer defines read-only exposure reservation operations.
type BorrowerExposureReservationQueryer interface {
	// GetByID returns the reservation identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.BorrowerExposureReservation, error)

	// GetByDecisionID returns the reservation for a decision.
	GetByDecisionID(
		ctx context.Context,
		decisionID uuid.UUID,
	) (model.BorrowerExposureReservation, error)

	// GetReservedByCheckoutIntentID returns the live reserved exposure for a checkout intent.
	GetReservedByCheckoutIntentID(
		ctx context.Context,
		checkoutIntentID uuid.UUID,
	) (model.BorrowerExposureReservation, error)
}

// BorrowerExposureReservationConnQueryer exposes exposure reservation operations on a connection.
type BorrowerExposureReservationConnQueryer interface {
	BorrowerExposureReservationQueryer
}

// BorrowerExposureReservationTxQueryer exposes transaction-scoped exposure reservation operations.
type BorrowerExposureReservationTxQueryer interface {
	BorrowerExposureReservationQueryer

	// Create stores a new exposure reservation inside the current transaction.
	Create(
		ctx context.Context,
		checkoutIntentID uuid.UUID,
		decisionID uuid.UUID,
		borrowerProfileID uuid.UUID,
		amountMinor uint64,
		currency model.Currency,
		status model.BorrowerExposureReservationStatus,
		expiresAt time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.BorrowerExposureReservation, error)

	// ExpireDueByBorrowerProfileIDAndCurrency marks due reserved exposure as expired and
	// returns the total expired amount.
	ExpireDueByBorrowerProfileIDAndCurrency(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
		now time.Time,
		updatedAt time.Time,
	) (expiredAmountMinor uint64, err error)
}

// BorrowerExposureReservation persists financed exposure reservations.
type BorrowerExposureReservation interface {
	Conn(Conn) BorrowerExposureReservationConnQueryer
	Tx(Tx) BorrowerExposureReservationTxQueryer
}
