package merchantbranchprofilerp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core merchant branch profile repository contract.
var _ repo.MerchantBranchProfile = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the merchant branch profile repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL merchant branch profile repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.MerchantBranchProfileConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.MerchantBranchProfileTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core merchant branch profile connection queryer contract.
var _ repo.MerchantBranchProfileConnQueryer = (*connQueryer)(nil)

// GetByID returns the branch profile identified by id.
func (q connQueryer) GetByID(
	ctx context.Context,
	id model.MerchantBranchProfileID,
) (model.MerchantBranchProfile, error) {
	return GetByID(ctx, q.Conn, id)
}

// GetByBranchID returns the branch profile for a branch identity.
func (q connQueryer) GetByBranchID(
	ctx context.Context,
	branchID model.BranchID,
) (model.MerchantBranchProfile, error) {
	return GetByBranchID(ctx, q.Conn, branchID)
}

// GetByPosGpsID returns the physical selling point for a POS GPS identity.
func (q connQueryer) GetByPosGpsID(
	ctx context.Context,
	posGpsID model.PosGpsID,
) (model.MerchantBranchProfile, error) {
	return GetByPosGpsID(ctx, q.Conn, posGpsID)
}

// GetByOnlineStoreID returns the online selling point for an online-store identity.
func (q connQueryer) GetByOnlineStoreID(
	ctx context.Context,
	onlineStoreID model.OnlineStoreID,
) (model.MerchantBranchProfile, error) {
	return GetByOnlineStoreID(ctx, q.Conn, onlineStoreID)
}

// ListByMerchantID returns all branch profiles configured for a merchant.
func (q connQueryer) ListByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantID(ctx, q.Conn, merchantID)
}

// ListByMerchantIDKeyset returns branch profiles for a merchant using keyset pagination.
func (q connQueryer) ListByMerchantIDKeyset(
	ctx context.Context,
	merchantID model.MerchantID,
	page pagination.KeysetRequest,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantIDKeyset(ctx, q.Conn, merchantID, page)
}

// ListByMerchantIDOffset returns branch profiles for a merchant using offset pagination.
func (q connQueryer) ListByMerchantIDOffset(
	ctx context.Context,
	merchantID model.MerchantID,
	page pagination.OffsetRequest,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantIDOffset(ctx, q.Conn, merchantID, page)
}

// CountByMerchantID returns how many branch profiles belong to the merchant.
func (q connQueryer) CountByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (int64, error) {
	return CountByMerchantID(ctx, q.Conn, merchantID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core merchant branch profile transaction queryer contract.
var _ repo.MerchantBranchProfileTxQueryer = (*txQueryer)(nil)

// GetByID returns the branch profile identified by id.
func (q txQueryer) GetByID(
	ctx context.Context,
	id model.MerchantBranchProfileID,
) (model.MerchantBranchProfile, error) {
	return GetByID(ctx, q.Tx, id)
}

// GetByBranchID returns the branch profile for a branch identity.
func (q txQueryer) GetByBranchID(
	ctx context.Context,
	branchID model.BranchID,
) (model.MerchantBranchProfile, error) {
	return GetByBranchID(ctx, q.Tx, branchID)
}

// GetByPosGpsID returns the physical selling point for a POS GPS identity.
func (q txQueryer) GetByPosGpsID(
	ctx context.Context,
	posGpsID model.PosGpsID,
) (model.MerchantBranchProfile, error) {
	return GetByPosGpsID(ctx, q.Tx, posGpsID)
}

// GetByOnlineStoreID returns the online selling point for an online-store identity.
func (q txQueryer) GetByOnlineStoreID(
	ctx context.Context,
	onlineStoreID model.OnlineStoreID,
) (model.MerchantBranchProfile, error) {
	return GetByOnlineStoreID(ctx, q.Tx, onlineStoreID)
}

// ListByMerchantID returns all branch profiles configured for a merchant.
func (q txQueryer) ListByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantID(ctx, q.Tx, merchantID)
}

// ListByMerchantIDKeyset returns branch profiles for a merchant using keyset pagination.
func (q txQueryer) ListByMerchantIDKeyset(
	ctx context.Context,
	merchantID model.MerchantID,
	page pagination.KeysetRequest,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantIDKeyset(ctx, q.Tx, merchantID, page)
}

// ListByMerchantIDOffset returns branch profiles for a merchant using offset pagination.
func (q txQueryer) ListByMerchantIDOffset(
	ctx context.Context,
	merchantID model.MerchantID,
	page pagination.OffsetRequest,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantIDOffset(ctx, q.Tx, merchantID, page)
}

// CountByMerchantID returns how many branch profiles belong to the merchant.
func (q txQueryer) CountByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (int64, error) {
	return CountByMerchantID(ctx, q.Tx, merchantID)
}

// Create stores a new branch profile inside the current transaction.
func (q txQueryer) Create(
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
	offerPreparationMode model.OfferPreparationMode,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantBranchProfile, error) {
	return Create(
		ctx,
		q.Tx,
		merchantID,
		branchID,
		branchType,
		status,
		fundingModels,
		settlementModels,
		currencies,
		riskTierOverride,
		maxAmountOverrideMinor,
		maxAmountOverrideCurrency,
		maxTenureMonthsOverride,
		jurisdiction,
		offerPreparationMode,
		createdAt,
		updatedAt,
	)
}

// UpdateOfferPreparationMode sets the trusted branch Offer preparation policy.
func (q txQueryer) UpdateOfferPreparationMode(
	ctx context.Context,
	id model.MerchantBranchProfileID,
	mode model.OfferPreparationMode,
	updatedAt time.Time,
) (model.MerchantBranchProfile, error) {
	return UpdateOfferPreparationMode(
		ctx,
		q.Tx,
		id,
		mode,
		updatedAt,
	)
}

// LockByID returns the branch profile and locks it for state changes.
func (q txQueryer) LockByID(
	ctx context.Context,
	id model.MerchantBranchProfileID,
) (model.MerchantBranchProfile, error) {
	return LockByID(ctx, q.Tx, id)
}

// LockByBranchID returns the branch profile for branchID and locks it for state changes.
func (q txQueryer) LockByBranchID(
	ctx context.Context,
	branchID model.BranchID,
) (model.MerchantBranchProfile, error) {
	return LockByBranchID(ctx, q.Tx, branchID)
}

// LockByPosGpsID locks the physical selling point addressed by posGpsID.
func (q txQueryer) LockByPosGpsID(
	ctx context.Context,
	posGpsID model.PosGpsID,
) (model.MerchantBranchProfile, error) {
	return LockByPosGpsID(ctx, q.Tx, posGpsID)
}

// LockByOnlineStoreID locks the online selling point addressed by onlineStoreID.
func (q txQueryer) LockByOnlineStoreID(
	ctx context.Context,
	onlineStoreID model.OnlineStoreID,
) (model.MerchantBranchProfile, error) {
	return LockByOnlineStoreID(ctx, q.Tx, onlineStoreID)
}
