package checkoutfinancingtermrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const checkoutFinancingTermColumns = `
	id,
	checkout_intent_id,
	decision_id,
	borrower_profile_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	requested_amount_minor,
	financed_amount_minor,
	upfront_amount_minor,
	regular_installment_amount_minor,
	first_installment_amount_minor,
	regular_installment_principal_amount_minor,
	first_installment_principal_amount_minor,
	first_installment_upfront_amount_minor,
	first_installment_total_amount_minor,
	total_fee_amount_minor,
	total_payable_amount_minor,
	currency,
	installment_count,
	status,
	expires_at,
	created_at,
	updated_at
`

// Create inserts checkout financing terms and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	checkoutIntentID uuid.UUID,
	decisionID uuid.UUID,
	borrowerProfileID uuid.UUID,
	merchantProfileID uuid.UUID,
	merchantBranchProfileID *uuid.UUID,
	requestedAmountMinor uint64,
	financedAmountMinor uint64,
	upfrontAmountMinor uint64,
	regularInstallmentAmountMinor uint64,
	firstInstallmentAmountMinor uint64,
	regularInstallmentPrincipalAmountMinor uint64,
	firstInstallmentPrincipalAmountMinor uint64,
	firstInstallmentUpfrontAmountMinor uint64,
	firstInstallmentTotalAmountMinor uint64,
	totalFeeAmountMinor uint64,
	totalPayableAmountMinor uint64,
	currency model.Currency,
	installmentCount int,
	status model.CheckoutFinancingTermStatus,
	expiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.CheckoutFinancingTerm, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"expires_at": expiresAt,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.CheckoutFinancingTerm{}, err
	}

	const query = `
INSERT INTO checkout_financing_terms (
	checkout_intent_id,
	decision_id,
	borrower_profile_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	requested_amount_minor,
	financed_amount_minor,
	upfront_amount_minor,
	regular_installment_amount_minor,
	first_installment_amount_minor,
	regular_installment_principal_amount_minor,
	first_installment_principal_amount_minor,
	first_installment_upfront_amount_minor,
	first_installment_total_amount_minor,
	total_fee_amount_minor,
	total_payable_amount_minor,
	currency,
	installment_count,
	status,
	expires_at,
	created_at,
	updated_at
)
VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
	$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
)
RETURNING` + checkoutFinancingTermColumns

	return postgres.QueryOne[model.CheckoutFinancingTerm](
		ctx,
		q,
		"create checkout financing terms",
		query,
		checkoutIntentID,
		decisionID,
		borrowerProfileID,
		merchantProfileID,
		merchantBranchProfileID,
		requestedAmountMinor,
		financedAmountMinor,
		upfrontAmountMinor,
		regularInstallmentAmountMinor,
		firstInstallmentAmountMinor,
		regularInstallmentPrincipalAmountMinor,
		firstInstallmentPrincipalAmountMinor,
		firstInstallmentUpfrontAmountMinor,
		firstInstallmentTotalAmountMinor,
		totalFeeAmountMinor,
		totalPayableAmountMinor,
		currency,
		installmentCount,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
}

// GetByID returns checkout financing terms by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	const query = `SELECT` + checkoutFinancingTermColumns + `FROM checkout_financing_terms WHERE id = $1`

	return postgres.QueryOne[model.CheckoutFinancingTerm](
		ctx,
		q,
		"get checkout financing terms by id",
		query,
		id,
	)
}

// GetByDecisionID returns checkout financing terms by decision id.
func GetByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	const query = `SELECT` + checkoutFinancingTermColumns + `FROM checkout_financing_terms WHERE decision_id = $1`

	return postgres.QueryOne[model.CheckoutFinancingTerm](
		ctx,
		q,
		"get checkout financing terms by decision id",
		query,
		decisionID,
	)
}

// GetByCheckoutIntentID returns checkout financing terms by checkout intent id.
func GetByCheckoutIntentID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	checkoutIntentID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	const query = `SELECT` + checkoutFinancingTermColumns + `
FROM checkout_financing_terms
WHERE checkout_intent_id = $1
ORDER BY created_at DESC
LIMIT 1
`

	return postgres.QueryOne[model.CheckoutFinancingTerm](
		ctx,
		q,
		"get checkout financing terms by checkout intent id",
		query,
		checkoutIntentID,
	)
}

// GetOfferedByCheckoutIntentID returns the live offered financing terms for a checkout intent.
func GetOfferedByCheckoutIntentID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	checkoutIntentID uuid.UUID,
) (model.CheckoutFinancingTerm, error) {
	const query = `SELECT` + checkoutFinancingTermColumns + `
FROM checkout_financing_terms
WHERE checkout_intent_id = $1
  AND status = $2
`

	return postgres.QueryOne[model.CheckoutFinancingTerm](
		ctx,
		q,
		"get offered checkout financing terms by checkout intent id",
		query,
		checkoutIntentID,
		model.CheckoutFinancingTermOffered,
	)
}

// ExpireDueOfferedByBorrowerProfileIDAndCurrency marks offered financing terms as expired.
func ExpireDueOfferedByBorrowerProfileIDAndCurrency[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	now time.Time,
	updatedAt time.Time,
) error {
	const query = `
UPDATE checkout_financing_terms
SET
	status = $5,
	updated_at = $4
WHERE borrower_profile_id = $1
  AND currency = $2
  AND status = $6
  AND expires_at <= $3
`

	_, err := q.Exec(
		ctx,
		query,
		borrowerProfileID,
		currency,
		now,
		updatedAt,
		model.CheckoutFinancingTermExpired,
		model.CheckoutFinancingTermOffered,
	)
	if err != nil {
		return fmt.Errorf("expire due offered checkout financing terms: %w", postgres.MapError(err))
	}

	return nil
}
