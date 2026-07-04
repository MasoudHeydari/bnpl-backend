package checkoutdecisionallocationrp

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const checkoutDecisionAllocationColumns = `
	decision_id,
	checkout_key,
	customer_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	purchase_channel,
	purchase_source_id,
	merchant_order_id,
	requested_amount_minor,
	approved_financed_amount_minor,
	upfront_amount_minor,
	available_credit_minor,
	currency,
	installment_count,
	expires_at,
	created_at
`

// Create inserts a checkout decision allocation and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
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
) (model.CheckoutDecisionAllocation, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"expires_at": expiresAt,
		"created_at": createdAt,
	}); err != nil {
		return model.CheckoutDecisionAllocation{}, err
	}

	const query = `
INSERT INTO checkout_decision_allocations (
	decision_id,
	checkout_key,
	customer_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	purchase_channel,
	purchase_source_id,
	merchant_order_id,
	requested_amount_minor,
	approved_financed_amount_minor,
	upfront_amount_minor,
	available_credit_minor,
	currency,
	installment_count,
	expires_at,
	created_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
RETURNING` + checkoutDecisionAllocationColumns

	return postgres.QueryOne[model.CheckoutDecisionAllocation](
		ctx,
		q,
		"create checkout decision allocation",
		query,
		decisionID,
		checkoutKey,
		customerID,
		merchantProfileID,
		merchantBranchProfileID,
		purchaseChannel,
		purchaseSourceID,
		merchantOrderID,
		requestedAmountMinor,
		approvedFinancedAmountMinor,
		upfrontAmountMinor,
		availableCreditMinor,
		currency,
		installmentCount,
		expiresAt,
		createdAt,
	)
}

// GetByDecisionID returns a checkout decision allocation by decision id.
func GetByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) (model.CheckoutDecisionAllocation, error) {
	const query = `SELECT` + checkoutDecisionAllocationColumns + `
FROM checkout_decision_allocations
WHERE decision_id = $1
`

	return postgres.QueryOne[model.CheckoutDecisionAllocation](
		ctx,
		q,
		"get checkout decision allocation by decision id",
		query,
		decisionID,
	)
}

// LockByDecisionID locks a checkout decision allocation by decision id.
func LockByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) (model.CheckoutDecisionAllocation, error) {
	const query = `SELECT` + checkoutDecisionAllocationColumns + `
FROM checkout_decision_allocations
WHERE decision_id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.CheckoutDecisionAllocation](
		ctx,
		q,
		"lock checkout decision allocation by decision id",
		query,
		decisionID,
	)
}
