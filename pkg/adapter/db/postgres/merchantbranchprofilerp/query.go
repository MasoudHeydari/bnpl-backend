package merchantbranchprofilerp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// merchantBranchProfileColumns is the shared projection used by all read queries.
const merchantBranchProfileColumns = `
	id,
	merchant_id,
	branch_id,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier_override,
	max_amount_override,
	max_tenure_months_override,
	jurisdiction,
	created_at,
	updated_at
`

// Create inserts a new branch profile and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.MerchantBranchProfile{}, err
	}

	const query = `
INSERT INTO merchant_branch_profiles (
	merchant_id,
	branch_id,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier_override,
	max_amount_override,
	max_tenure_months_override,
	jurisdiction,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
RETURNING` + merchantBranchProfileColumns

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"create merchant branch profile",
		query,
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

// GetByID returns a branch profile by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileColumns + `FROM merchant_branch_profiles WHERE id = $1`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"get merchant branch profile by id",
		query,
		id,
	)
}

// GetByBranchID returns a branch profile by branch identity id.
func GetByBranchID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	branchID uuid.UUID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileColumns + `FROM merchant_branch_profiles WHERE branch_id = $1`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"get merchant branch profile by branch id",
		query,
		branchID,
	)
}

// ListByMerchantID returns all branch profiles configured for a merchant.
func ListByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID uuid.UUID,
) ([]model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileColumns + `FROM merchant_branch_profiles WHERE merchant_id = $1 ORDER BY created_at ASC`

	rows, err := q.Query(ctx, query, merchantID)
	if err != nil {
		return nil, fmt.Errorf("list merchant branch profiles: %w", postgres.MapError(err))
	}
	defer rows.Close()

	profiles, err := postgres.ValuesInto[model.MerchantBranchProfile](rows.(postgres.Rows))
	if err != nil {
		return nil, fmt.Errorf("list merchant branch profiles: %w", err)
	}

	return profiles, nil
}

// LockByID returns a branch profile by id and locks it for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileColumns + `FROM merchant_branch_profiles WHERE id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"lock merchant branch profile by id",
		query,
		id,
	)
}

// LockByBranchID returns a branch profile by branchID and locks it for update.
func LockByBranchID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	branchID uuid.UUID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileColumns + `FROM merchant_branch_profiles WHERE branch_id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"lock merchant branch profile by branch id",
		query,
		branchID,
	)
}
