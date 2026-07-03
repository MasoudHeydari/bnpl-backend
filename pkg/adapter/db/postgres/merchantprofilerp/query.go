package merchantprofilerp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// merchantProfileColumns is the shared projection used by all read queries.
const merchantProfileColumns = `
	id,
	merchant_id,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier,
	max_amount,
	max_tenure_months,
	requires_branch_mode,
	jurisdiction,
	created_at,
	updated_at
`

// Create inserts a new merchant profile and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID uuid.UUID,
	status model.MerchantStatus,
	fundingModels []model.FundingModel,
	settlementModels []model.SettlementModel,
	currencies []model.Currency,
	riskTier model.RiskTier,
	maxAmount model.Money,
	maxTenureMonths uint,
	requiresBranchMode bool,
	jurisdiction model.Jurisdiction,
	createdAt time.Time,
	updatedAt time.Time,
) (model.MerchantProfile, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.MerchantProfile{}, err
	}

	const query = `
INSERT INTO merchant_profiles (
	merchant_id,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier,
	max_amount,
	max_tenure_months,
	requires_branch_mode,
	jurisdiction,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING` + merchantProfileColumns

	return postgres.QueryOne[model.MerchantProfile](
		ctx,
		q,
		"create merchant profile",
		query,
		merchantID,
		status,
		fundingModels,
		settlementModels,
		currencies,
		riskTier,
		maxAmount,
		maxTenureMonths,
		requiresBranchMode,
		jurisdiction,
		createdAt,
		updatedAt,
	)
}

// GetByID returns a merchant profile by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE id = $1`

	return postgres.QueryOne[model.MerchantProfile](ctx, q, "get merchant profile by id", query, id)
}

// GetByMerchantID returns a merchant profile by merchant identity id.
func GetByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID uuid.UUID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE merchant_id = $1`

	return postgres.QueryOne[model.MerchantProfile](ctx, q, "get merchant profile by merchant id", query, merchantID)
}

// LockByID returns a merchant profile by id and locks it for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantProfile](ctx, q, "lock merchant profile by id", query, id)
}

// LockByMerchantID returns a merchant profile by merchantID and locks it for update.
func LockByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID uuid.UUID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE merchant_id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantProfile](ctx, q, "lock merchant profile by merchant id", query, merchantID)
}
