package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// MerchantBranchProfileQueryer defines read-only merchant branch profile operations.
type MerchantBranchProfileQueryer interface {
	// GetByID returns the branch profile identified by id.
	GetByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.MerchantBranchProfile, error)

	// GetByBranchID returns the branch profile for a branch identity.
	GetByBranchID(
		ctx context.Context,
		branchID uuid.UUID,
	) (model.MerchantBranchProfile, error)

	// ListByMerchantID returns all branch profiles configured for a merchant.
	ListByMerchantID(
		ctx context.Context,
		merchantID uuid.UUID,
	) ([]model.MerchantBranchProfile, error)
}

// MerchantBranchProfileConnQueryer exposes branch profile operations on a connection.
type MerchantBranchProfileConnQueryer interface {
	MerchantBranchProfileQueryer
}

// MerchantBranchProfileTxQueryer exposes transaction-scoped branch profile operations.
type MerchantBranchProfileTxQueryer interface {
	MerchantBranchProfileQueryer

	// Create stores a new branch profile inside the current transaction.
	Create(
		ctx context.Context,
		merchantID uuid.UUID,
		branchID uuid.UUID,
		status model.BranchStatus,
		fundingModels []model.FundingModel,
		settlementModels []model.SettlementModel,
		currencies []model.Currency,
		riskTierOverride *model.RiskTier,
		maxAmountOverride *model.Money,
		maxTenureMonthsOverride *uint,
		jurisdiction model.Jurisdiction,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.MerchantBranchProfile, error)

	// LockByID returns the branch profile and locks it for state changes.
	LockByID(
		ctx context.Context,
		id uuid.UUID,
	) (model.MerchantBranchProfile, error)

	// LockByBranchID returns the branch profile for branchID and locks it for
	// state changes.
	LockByBranchID(
		ctx context.Context,
		branchID uuid.UUID,
	) (model.MerchantBranchProfile, error)
}

// MerchantBranchProfile persists per-branch BNPL configuration records.
type MerchantBranchProfile interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) MerchantBranchProfileConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) MerchantBranchProfileTxQueryer
}
