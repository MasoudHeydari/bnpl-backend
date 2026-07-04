package merchantbranchprofilerp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
)

// merchantBranchProfileColumns is the shared projection used by all read queries.
const merchantBranchProfileColumns = `
	id,
	merchant_id,
	branch_id,
	branch_type,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier_override,
	max_amount_override_minor,
	max_amount_override_currency,
	max_tenure_months_override,
	jurisdiction,
	offer_preparation_mode,
	created_at,
	updated_at
`

// merchantBranchProfileJoinColumns qualifies the shared projection for source-mapping joins.
const merchantBranchProfileJoinColumns = `
	b.id,
	b.merchant_id,
	b.branch_id,
	b.branch_type,
	b.status,
	b.funding_models,
	b.settlement_models,
	b.currencies,
	b.risk_tier_override,
	b.max_amount_override_minor,
	b.max_amount_override_currency,
	b.max_tenure_months_override,
	b.jurisdiction,
	b.offer_preparation_mode,
	b.created_at,
	b.updated_at
`

// Create inserts a new branch profile and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.MerchantBranchProfile{}, err
	}

	if !offerPreparationMode.IsValid() {
		offerPreparationMode = model.DefaultOfferPreparationMode
	}

	const query = `
INSERT INTO merchant_branch_profiles (
	merchant_id,
	branch_id,
	branch_type,
	status,
	funding_models,
	settlement_models,
	currencies,
	risk_tier_override,
	max_amount_override_minor,
	max_amount_override_currency,
	max_tenure_months_override,
	jurisdiction,
	offer_preparation_mode,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING` + merchantBranchProfileColumns

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"create merchant branch profile",
		query,
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

// UpdateOfferPreparationMode updates the trusted Offer preparation policy for a branch.
func UpdateOfferPreparationMode[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantBranchProfileID,
	mode model.OfferPreparationMode,
	updatedAt time.Time,
) (model.MerchantBranchProfile, error) {
	const op = "merchantbranchprofilerp.UpdateOfferPreparationMode"

	if !mode.IsValid() {
		return model.MerchantBranchProfile{}, fmt.Errorf("%s: invalid offer preparation mode %q", op, mode)
	}

	if err := postgres.RequiredTimes(map[string]time.Time{"updated_at": updatedAt}); err != nil {
		return model.MerchantBranchProfile{}, err
	}

	const query = `
UPDATE merchant_branch_profiles
SET offer_preparation_mode = $2,
	updated_at = $3
WHERE id = $1
RETURNING` + merchantBranchProfileColumns

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"update merchant branch offer preparation mode",
		query,
		id,
		mode,
		updatedAt,
	)
}

// GetByID returns a branch profile by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantBranchProfileID,
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
	branchID model.BranchID,
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

// GetByPosGpsID returns the physical selling point addressed by a POS/GPS terminal.
func GetByPosGpsID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	posGpsID model.PosGpsID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileJoinColumns + `
FROM merchant_pos_terminals t
JOIN merchant_branch_profiles b ON b.id = t.merchant_branch_profile_id
WHERE t.pos_gps_id = $1
  AND t.enabled = true
  AND b.branch_type = 'physical'
`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"get merchant branch profile by pos gps id",
		query,
		posGpsID,
	)
}

// GetByOnlineStoreID returns the online selling point addressed by an online store.
func GetByOnlineStoreID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	onlineStoreID model.OnlineStoreID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileJoinColumns + `
FROM merchant_online_stores s
JOIN merchant_branch_profiles b ON b.id = s.merchant_branch_profile_id
WHERE s.online_store_id = $1
  AND s.enabled = true
  AND b.branch_type = 'online'
`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"get merchant branch profile by online store id",
		query,
		onlineStoreID,
	)
}

// ListByMerchantID returns all branch profiles configured for a merchant.
func ListByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) ([]model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileColumns + `FROM merchant_branch_profiles WHERE merchant_id = $1 ORDER BY created_at ASC`
	const

	//rows, err := postgres.QueryRows(ctx, q, query, merchantID)
	op = "merchantbranchprofilerp.ListByMerchantID"

	rows, err := q.Query(ctx, query, merchantID)
	if err != nil {
		return nil, fmt.Errorf("%s: list merchant branch profiles: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	//profiles, err := postgres.ValuesInto[model.MerchantBranchProfile](rows)
	profiles, err := postgres.ValuesInto[model.MerchantBranchProfile](postgres.MustRows(rows))
	if err != nil {
		return nil, fmt.Errorf("%s: list merchant branch profiles: %w", op, err)
	}

	return profiles, nil
}

// ListByMerchantIDKeyset returns branch profiles for a merchant identity using
// keyset pagination ordered by created_at DESC, id DESC.
func ListByMerchantIDKeyset[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
	page pagination.KeysetRequest,
) ([]model.MerchantBranchProfile, error) {
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	args := []any{merchantID}
	var conditions []string
	conditions = append(conditions, "merchant_id = $1")

	keysetWhere, keysetArgs, err := postgres.KeysetWhere("created_at", "id", page, len(args)+1)
	if err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	if keysetWhere != "" {
		conditions = append(conditions, keysetWhere)
		args = append(args, keysetArgs...)
	}

	limitSQL, limitArgs := postgres.Limit(page.Limit, len(args)+1)
	args = append(args, limitArgs...)

	query := `SELECT` + merchantBranchProfileColumns + `
FROM merchant_branch_profiles
WHERE ` + strings.Join(conditions, `
  AND `) + `
ORDER BY created_at DESC, id DESC
` + limitSQL

	return postgres.QueryAll[model.MerchantBranchProfile](
		ctx,
		q,
		"list merchant branch profiles by merchant id keyset",
		query,
		args...,
	)
}

// ListByMerchantIDOffset returns one page of branch profiles for a merchant
// identity using page/limit offset pagination.
func ListByMerchantIDOffset[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
	page pagination.OffsetRequest,
) ([]model.MerchantBranchProfile, error) {
	page.Order = pagination.OrderDesc
	page.Sort = pagination.DefaultSort
	page.Normalize()

	args := []any{merchantID}
	limitOffsetSQL, limitOffsetArgs := postgres.LimitOffset(page, len(args)+1)
	args = append(args, limitOffsetArgs...)

	query := `SELECT` + merchantBranchProfileColumns + `
FROM merchant_branch_profiles
WHERE merchant_id = $1
ORDER BY created_at DESC, id DESC
` + limitOffsetSQL

	return postgres.QueryAll[model.MerchantBranchProfile](
		ctx,
		q,
		"list merchant branch profiles by merchant id offset",
		query,
		args...,
	)
}

// CountByMerchantID returns the number of branch profiles for a merchant identity.
func CountByMerchantID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantID model.MerchantID,
) (int64, error) {
	const query = `
SELECT COUNT(*) AS count
FROM merchant_branch_profiles
WHERE merchant_id = $1
`

	type countRow struct {
		Count int64 `db:"count"`
	}

	row, err := postgres.QueryOne[countRow](
		ctx,
		q,
		"count merchant branch profiles by merchant id",
		query,
		merchantID,
	)
	if err != nil {
		return 0, err
	}

	return row.Count, nil
}

// LockByID returns a branch profile by id and locks it for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.MerchantBranchProfileID,
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
	branchID model.BranchID,
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

// LockByPosGpsID locks the physical branch and POS mapping used for checkout resolution.
func LockByPosGpsID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	posGpsID model.PosGpsID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileJoinColumns + `
FROM merchant_pos_terminals t
JOIN merchant_branch_profiles b ON b.id = t.merchant_branch_profile_id
WHERE t.pos_gps_id = $1
  AND t.enabled = true
  AND b.branch_type = 'physical'
FOR UPDATE OF b, t
`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"lock merchant branch profile by pos gps id",
		query,
		posGpsID,
	)
}

// LockByOnlineStoreID locks the online branch and store mapping used for checkout resolution.
func LockByOnlineStoreID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	onlineStoreID model.OnlineStoreID,
) (model.MerchantBranchProfile, error) {
	const query = `SELECT` + merchantBranchProfileJoinColumns + `
FROM merchant_online_stores s
JOIN merchant_branch_profiles b ON b.id = s.merchant_branch_profile_id
WHERE s.online_store_id = $1
  AND s.enabled = true
  AND b.branch_type = 'online'
FOR UPDATE OF b, s
`

	return postgres.QueryOne[model.MerchantBranchProfile](
		ctx,
		q,
		"lock merchant branch profile by online store id",
		query,
		onlineStoreID,
	)
}
