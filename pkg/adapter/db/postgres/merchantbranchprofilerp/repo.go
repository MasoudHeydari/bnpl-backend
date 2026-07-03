package merchantbranchprofilerp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
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
	return connQueryer{Conn: c.(*postgres.Conn)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.MerchantBranchProfileTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core merchant branch profile connection queryer contract.
var _ repo.MerchantBranchProfileConnQueryer = (*connQueryer)(nil)

// GetByID returns the branch profile identified by id.
func (q connQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.MerchantBranchProfile, error) {
	return GetByID(ctx, q.Conn, id)
}

// GetByBranchID returns the branch profile for a branch identity.
func (q connQueryer) GetByBranchID(
	ctx context.Context,
	branchID uuid.UUID,
) (model.MerchantBranchProfile, error) {
	return GetByBranchID(ctx, q.Conn, branchID)
}

// ListByMerchantID returns all branch profiles configured for a merchant.
func (q connQueryer) ListByMerchantID(
	ctx context.Context,
	merchantID uuid.UUID,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantID(ctx, q.Conn, merchantID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core merchant branch profile transaction queryer contract.
var _ repo.MerchantBranchProfileTxQueryer = (*txQueryer)(nil)

// GetByID returns the branch profile identified by id.
func (q txQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.MerchantBranchProfile, error) {
	return GetByID(ctx, q.Tx, id)
}

// GetByBranchID returns the branch profile for a branch identity.
func (q txQueryer) GetByBranchID(
	ctx context.Context,
	branchID uuid.UUID,
) (model.MerchantBranchProfile, error) {
	return GetByBranchID(ctx, q.Tx, branchID)
}

// ListByMerchantID returns all branch profiles configured for a merchant.
func (q txQueryer) ListByMerchantID(
	ctx context.Context,
	merchantID uuid.UUID,
) ([]model.MerchantBranchProfile, error) {
	return ListByMerchantID(ctx, q.Tx, merchantID)
}

// Create stores a new branch profile inside the current transaction.
func (q txQueryer) Create(
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
) (model.MerchantBranchProfile, error) {
	return Create(
		ctx,
		q.Tx,
		merchantID,
		branchID,
		status,
		fundingModels,
		settlementModels,
		currencies,
		riskTierOverride,
		maxAmountOverride,
		maxTenureMonthsOverride,
		jurisdiction,
		createdAt,
		updatedAt,
	)
}

// LockByID returns the branch profile and locks it for state changes.
func (q txQueryer) LockByID(
	ctx context.Context,
	id uuid.UUID,
) (model.MerchantBranchProfile, error) {
	return LockByID(ctx, q.Tx, id)
}

// LockByBranchID returns the branch profile for branchID and locks it for state changes.
func (q txQueryer) LockByBranchID(
	ctx context.Context,
	branchID uuid.UUID,
) (model.MerchantBranchProfile, error) {
	return LockByBranchID(ctx, q.Tx, branchID)
}
