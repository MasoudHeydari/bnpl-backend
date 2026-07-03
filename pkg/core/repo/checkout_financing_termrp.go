package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutFinancingTermQueryer defines read-only checkout financing terms operations.
type CheckoutFinancingTermQueryer interface {
	// GetByID returns the financing terms identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.CheckoutFinancingTerm, error)

	// GetByDecisionID returns financing terms for a decision.
	GetByDecisionID(
		ctx context.Context,
		decisionID uuid.UUID,
	) (model.CheckoutFinancingTerm, error)

	// GetByCheckoutIntentID returns financing terms for a checkout intent.
	GetByCheckoutIntentID(
		ctx context.Context,
		checkoutIntentID uuid.UUID,
	) (model.CheckoutFinancingTerm, error)

	// GetOfferedByCheckoutIntentID returns the live offered financing terms for a checkout intent.
	GetOfferedByCheckoutIntentID(
		ctx context.Context,
		checkoutIntentID uuid.UUID,
	) (model.CheckoutFinancingTerm, error)
}

// CheckoutFinancingTermConnQueryer exposes checkout financing terms on a connection.
type CheckoutFinancingTermConnQueryer interface {
	CheckoutFinancingTermQueryer
}

// CheckoutFinancingTermTxQueryer exposes transaction-scoped checkout financing terms operations.
type CheckoutFinancingTermTxQueryer interface {
	CheckoutFinancingTermQueryer

	// Create stores checkout financing terms inside the current transaction.
	Create(
		ctx context.Context,
		checkoutIntentID uuid.UUID,
		decisionID uuid.UUID,
		borrowerProfileID uuid.UUID,
		merchantProfileID uuid.UUID,
		merchantBranchProfileID *uuid.UUID,
		requestedAmountMinor uint64,
		financedAmountMinor uint64,
		upfrontAmountMinor uint64,
		regularInstallmentAmountMinor uint64,
		firstInstallmentAmountMinor uint64,
		regularInstallmentPrincipalAmountMinor uint64,
		firstInstallmentPrincipalAmountMinor uint64,
		firstInstallmentUpfrontAmountMinor uint64,
		firstInstallmentTotalAmountMinor uint64,
		totalFeeAmountMinor uint64,
		totalPayableAmountMinor uint64,
		currency model.Currency,
		installmentCount int,
		status model.CheckoutFinancingTermStatus,
		expiresAt time.Time,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.CheckoutFinancingTerm, error)

	// ExpireDueOfferedByBorrowerProfileIDAndCurrency marks offered financing terms as
	// expired when their offer window has elapsed.
	ExpireDueOfferedByBorrowerProfileIDAndCurrency(
		ctx context.Context,
		borrowerProfileID uuid.UUID,
		currency model.Currency,
		now time.Time,
		updatedAt time.Time,
	) error
}

// CheckoutFinancingTerm persists calculated checkout financing breakdowns.
type CheckoutFinancingTerm interface {
	Conn(Conn) CheckoutFinancingTermConnQueryer
	Tx(Tx) CheckoutFinancingTermTxQueryer
}
