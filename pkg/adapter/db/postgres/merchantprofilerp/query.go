package merchantprofilerp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// merchantProfileColumns is the shared projection used by all read queries.
const merchantProfileColumns = `
	id,
	merchant_id,
	display_name,
	legal_name,
	logo_url,
	support_phone,
	support_email,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier,
	max_amount_minor,
	max_amount_currency,
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
	if err := postgres.RequiredTimes(
		map[string]time.Time{
			"created_at": createdAt,
			"updated_at": updatedAt,
		},
	); err != nil {
		return model.MerchantProfile{}, err
	}

	const query = `
INSERT INTO merchant_profiles (
	merchant_id,
	display_name,
	legal_name,
	logo_url,
	support_phone,
	support_email,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier,
	max_amount_minor,
	max_amount_currency,
	max_tenure_months,
	requires_branch_mode,
	jurisdiction,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING` + merchantProfileColumns

	return postgres.QueryOne[model.MerchantProfile](
		ctx,
		q,
		"create merchant profile",
		query,
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

// GetByID returns a merchant profile by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantProfileID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE id = $1`

	return postgres.QueryOne[model.MerchantProfile](
		ctx,
		q,
		"get merchant profile by id",
		query,
		id,
	)
}

// GetByMerchantID returns a merchant profile by merchant identity id.
func GetByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE merchant_id = $1`

	return postgres.QueryOne[model.MerchantProfile](
		ctx,
		q,
		"get merchant profile by merchant id",
		query,
		merchantID,
	)
}

// LockByID returns a merchant profile by id and locks it for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantProfileID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantProfile](
		ctx,
		q,
		"lock merchant profile by id",
		query,
		id,
	)
}

// LockByMerchantID returns a merchant profile by merchantID and locks it for update.
func LockByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (model.MerchantProfile, error) {
	const query = `SELECT` + merchantProfileColumns + `FROM merchant_profiles WHERE merchant_id = $1 FOR UPDATE`

	return postgres.QueryOne[model.MerchantProfile](
		ctx,
		q,
		"lock merchant profile by merchant id",
		query,
		merchantID,
	)
}
