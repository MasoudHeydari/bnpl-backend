package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// MerchantBranchProfileQueryer defines read-only merchant branch profile operations.
type MerchantBranchProfileQueryer interface {
	// GetByID returns the branch profile identified by id.
	GetByID(
		ctx context.Context,
		id model.MerchantBranchProfileID,
	) (model.MerchantBranchProfile, error)

	// GetByBranchID returns the branch profile for a branch identity.
	GetByBranchID(
		ctx context.Context,
		branchID model.BranchID,
	) (model.MerchantBranchProfile, error)

	// GetByPosGpsID returns the physical selling point addressed by a POS/GPS terminal identity.
	GetByPosGpsID(
		ctx context.Context,
		posGpsID model.PosGpsID,
	) (model.MerchantBranchProfile, error)

	// GetByOnlineStoreID returns the online selling point addressed by an online-store identity.
	GetByOnlineStoreID(
		ctx context.Context,
		onlineStoreID model.OnlineStoreID,
	) (model.MerchantBranchProfile, error)

	// ListByMerchantID returns all branch profiles configured for a merchant
	// identity (merchant_branch_profiles.merchant_id), ordered by created_at ASC.
	//
	// An empty result is not an error.
	ListByMerchantID(
		ctx context.Context,
		merchantID model.MerchantID,
	) ([]model.MerchantBranchProfile, error)

	// ListByMerchantIDKeyset returns branch profiles for a merchant identity
	// ordered by created_at DESC, id DESC using keyset pagination.
	//
	// All branch statuses are included. An empty result is not an error.
	ListByMerchantIDKeyset(
		ctx context.Context,
		merchantID model.MerchantID,
		page pagination.KeysetRequest,
	) ([]model.MerchantBranchProfile, error)

	// ListByMerchantIDOffset returns one page of branch profiles for a merchant
	// identity ordered by created_at DESC, id DESC using page/limit offset
	// pagination.
	//
	// All branch statuses are included. An empty result is not an error.
	ListByMerchantIDOffset(
		ctx context.Context,
		merchantID model.MerchantID,
		page pagination.OffsetRequest,
	) ([]model.MerchantBranchProfile, error)

	// CountByMerchantID returns how many branch profiles belong to the merchant
	// identity. Used to build offset pagination totals.
	CountByMerchantID(
		ctx context.Context,
		merchantID model.MerchantID,
	) (int64, error)
}

// MerchantBranchProfileConnQueryer exposes branch profile operations on a connection.
type MerchantBranchProfileConnQueryer interface {
	// MerchantBranchProfileQueryer provides shared merchant branch profile query operations.
	MerchantBranchProfileQueryer
}

// MerchantBranchProfileTxQueryer exposes transaction-scoped branch profile operations.
type MerchantBranchProfileTxQueryer interface {
	// MerchantBranchProfileQueryer provides shared merchant branch profile query operations.
	MerchantBranchProfileQueryer

	// Create stores a new branch profile inside the current transaction.
	Create(
		ctx context.Context,
		merchantID model.MerchantID,
		branchID model.BranchID,
		branchType model.MerchantBranchType,
		status model.BranchStatus,
		fundingModels []model.FundingModel,
		settlementModels []model.SettlementModel,
		currencies []model.Currency,
		riskTierOverride *model.RiskTier,
		maxAmountOverrideMinor *uint64,
		maxAmountOverrideCurrency *model.Currency,
		maxTenureMonthsOverride *uint,
		jurisdiction model.Jurisdiction,
		createdAt time.Time,
		updatedAt time.Time,
	) (model.MerchantBranchProfile, error)

	// LockByID returns the branch profile and locks it for state changes.
	LockByID(
		ctx context.Context,
		id model.MerchantBranchProfileID,
	) (model.MerchantBranchProfile, error)

	// LockByBranchID returns the branch profile for branchID and locks it for
	// state changes.
	LockByBranchID(
		ctx context.Context,
		branchID model.BranchID,
	) (model.MerchantBranchProfile, error)

	// LockByPosGpsID returns the physical selling point for posGpsID and locks
	// both the source mapping and branch profile for a consistent checkout resolution.
	LockByPosGpsID(
		ctx context.Context,
		posGpsID model.PosGpsID,
	) (model.MerchantBranchProfile, error)

	// LockByOnlineStoreID returns the online selling point for onlineStoreID
	// and locks both the source mapping and branch profile for checkout resolution.
	LockByOnlineStoreID(
		ctx context.Context,
		onlineStoreID model.OnlineStoreID,
	) (model.MerchantBranchProfile, error)
}

// MerchantBranchProfile persists per-branch BNPL configuration records.
type MerchantBranchProfile interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) MerchantBranchProfileConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) MerchantBranchProfileTxQueryer
}
