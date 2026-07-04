package merchantprofilerp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core merchant profile repository contract.
var _ repo.MerchantProfile = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the merchant profile repository.
// It is stateless and safe for concurrent reuse.
type Repo struct{}

// New returns a stateless PostgreSQL merchant profile repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.MerchantProfileConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.MerchantProfileTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core merchant profile connection queryer contract.
var _ repo.MerchantProfileConnQueryer = (*connQueryer)(nil)

// GetByID returns the merchant profile identified by id.
func (q connQueryer) GetByID(
	ctx context.Context,
	id model.MerchantProfileID,
) (model.MerchantProfile, error) {
	return GetByID(ctx, q.Conn, id)
}

// GetByMerchantID returns the BNPL profile for a merchant identity.
func (q connQueryer) GetByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantProfile, error) {
	return GetByMerchantID(ctx, q.Conn, merchantID)
}

type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core merchant profile transaction queryer contract.
var _ repo.MerchantProfileTxQueryer = (*txQueryer)(nil)

// GetByID returns the merchant profile identified by id.
func (q txQueryer) GetByID(
	ctx context.Context,
	id model.MerchantProfileID,
) (model.MerchantProfile, error) {
	return GetByID(ctx, q.Tx, id)
}

// GetByMerchantID returns the BNPL profile for a merchant identity.
func (q txQueryer) GetByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantProfile, error) {
	return GetByMerchantID(ctx, q.Tx, merchantID)
}

// Create stores a new merchant profile inside the current transaction.
func (q txQueryer) Create(
	ctx context.Context,
	merchantID model.MerchantID,
	displayName string,
	legalName string,
	logoURL string,
	supportPhone string,
	supportEmail string,
	status model.MerchantStatus,
	fundingModels []model.FundingModel,
	settlementModels []model.SettlementModel,
	currencies []model.Currency,
	riskTier model.RiskTier,
	maxAmountMinor uint64,
	maxAmountCurrency model.Currency,
	maxTenureMonths uint,
	requiresBranchMode bool,
	jurisdiction model.Jurisdiction,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantProfile, error) {
	return Create(
		ctx,
		q.Tx,
		merchantID,
		displayName,
		legalName,
		logoURL,
		supportPhone,
		supportEmail,
		status,
		fundingModels,
		settlementModels,
		currencies,
		riskTier,
		maxAmountMinor,
		maxAmountCurrency,
		maxTenureMonths,
		requiresBranchMode,
		jurisdiction,
		createdAt,
		updatedAt,
	)
}

// LockByID returns the merchant profile and locks it for state changes.
func (q txQueryer) LockByID(
	ctx context.Context,
	id model.MerchantProfileID,
) (model.MerchantProfile, error) {
	return LockByID(ctx, q.Tx, id)
}

// LockByMerchantID returns the merchant profile for merchantID and locks it for state changes.
func (q txQueryer) LockByMerchantID(
	ctx context.Context,
	merchantID model.MerchantID,
) (model.MerchantProfile, error) {
	return LockByMerchantID(ctx, q.Tx, merchantID)
}

