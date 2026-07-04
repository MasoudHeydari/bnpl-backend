package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// BorrowerExposureSummaryQueryer defines read-only exposure summary operations.
type BorrowerExposureSummaryQueryer interface {
	// GetByBorrowerProfileIDAndCurrency returns the exposure summary for a
	// borrower profile and currency.
	GetByBorrowerProfileIDAndCurrency(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
	) (model.BorrowerExposureSummary, error)
}

// BorrowerExposureSummaryConnQueryer exposes exposure summary operations on a connection.
type BorrowerExposureSummaryConnQueryer interface {
	BorrowerExposureSummaryQueryer
}

// BorrowerExposureSummaryTxQueryer exposes transaction-scoped exposure summary operations.
type BorrowerExposureSummaryTxQueryer interface {
	BorrowerExposureSummaryQueryer

	// LockByBorrowerProfileIDAndCurrency returns and locks the exposure summary
	// row for update.
	LockByBorrowerProfileIDAndCurrency(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
	) (model.BorrowerExposureSummary, error)

	// EnsureAndLock creates a zero exposure summary when missing and locks it for
	// update within the current transaction.
	EnsureAndLock(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
		updatedAt time.Time,
	) (model.BorrowerExposureSummary, error)

	// AddReservedExposure increases reserved exposure for a borrower profile.
	AddReservedExposure(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
		amountMinor uint64,
		updatedAt time.Time,
	) error

	// SubtractReservedExposure decreases reserved exposure without going below zero.
	SubtractReservedExposure(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
		amountMinor uint64,
		updatedAt time.Time,
	) error
}

// BorrowerExposureSummary persists borrower financed exposure aggregates.
type BorrowerExposureSummary interface {
	Conn(Conn) BorrowerExposureSummaryConnQueryer
	Tx(Tx) BorrowerExposureSummaryTxQueryer
}
