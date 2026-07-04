package repo

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// CheckoutDecisionAllocationQueryer defines read-only checkout decision allocation operations.
type CheckoutDecisionAllocationQueryer interface {
	// GetByDecisionID returns the allocation for a decision.
	GetByDecisionID(
		ctx context.Context,
		decisionID model.DecisionID,
	) (model.CheckoutDecisionAllocation, error)
}

// CheckoutDecisionAllocationConnQueryer exposes allocation operations on a connection.
type CheckoutDecisionAllocationConnQueryer interface {
	// CheckoutDecisionAllocationQueryer provides shared checkout decision allocation query operations.
	CheckoutDecisionAllocationQueryer
}

// CheckoutDecisionAllocationTxQueryer exposes transaction-scoped allocation operations.
type CheckoutDecisionAllocationTxQueryer interface {
	// CheckoutDecisionAllocationQueryer provides shared checkout decision allocation query operations.
	CheckoutDecisionAllocationQueryer

	// LockByDecisionID locks the allocation for a decision.
	LockByDecisionID(
		ctx context.Context,
		decisionID model.DecisionID,
	) (model.CheckoutDecisionAllocation, error)

	// Create stores a checkout decision allocation inside the current transaction.
	Create(
		ctx context.Context,
		decisionID model.DecisionID,
		checkoutKey string,
		customerID model.CustomerID,
		merchantProfileID model.MerchantProfileID,
		merchantBranchProfileID *model.MerchantBranchProfileID,
		purchaseChannel model.PurchaseChannel,
		purchaseSourceID uuid.UUID,
		merchantOrderID *string,
		requestedAmountMinor uint64,
		approvedFinancedAmountMinor uint64,
		upfrontAmountMinor uint64,
		availableCreditMinor uint64,
		currency model.Currency,
		installmentCount int,
		expiresAt time.Time,
		createdAt time.Time,
	) (model.CheckoutDecisionAllocation, error)
}

// CheckoutDecisionAllocation persists checkout decision material results.
type CheckoutDecisionAllocation interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) CheckoutDecisionAllocationConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) CheckoutDecisionAllocationTxQueryer
}
