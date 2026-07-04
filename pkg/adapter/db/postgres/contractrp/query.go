package contractrp

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

const contractColumns = `
	id,
	decision_id,
	checkout_key,
	customer_id,
	credit_account_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	currency,
	requested_amount_minor,
	approved_financed_amount_minor,
	upfront_amount_minor,
	installment_count,
	payable_now_amount_minor,
	remaining_principal_amount_minor,
	status,
	payment_reference,
	payment_id,
	financial_invoice_id,
	financial_merchant_entitlement_id,
	financial_receivable_id,
	financial_payment_intent_id,
	financial_payment_attempt_id,
	successful_financial_payment_attempt_id,
	financial_payment_session_id,
	financial_payment_link,
	financial_payment_expires_at,
	financial_payment_reused_idempotent_result,
	expires_at,
	activated_at,
	completed_at,
	canceled_at,
	cancellation_idempotency_key,
	cancellation_reason,
	cancellation_request_fingerprint,
	cancellation_requested_at,
	cancellation_financial_core_idempotency_key,
	cancellation_tenant_id,
	cancellation_context_id,
	cancellation_financial_core_accepted_at,
	created_at,
	updated_at
`

// Create inserts a contract row and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
	checkoutKey string,
	customerID model.CustomerID,
	creditAccountID model.CreditAccountID,
	merchantProfileID model.MerchantProfileID,
	merchantBranchProfileID *model.MerchantBranchProfileID,
	currency model.Currency,
	requestedAmountMinor uint64,
	approvedFinancedAmountMinor uint64,
	upfrontAmountMinor uint64,
	installmentCount int,
	payableNowAmountMinor uint64,
	remainingPrincipalAmountMinor uint64,
	status model.ContractStatus,
	expiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.Contract, bool, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"expires_at": expiresAt,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.Contract{}, false, err
	}

	const query = `
INSERT INTO contracts (
	decision_id,
	checkout_key,
	customer_id,
	credit_account_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	currency,
	requested_amount_minor,
	approved_financed_amount_minor,
	upfront_amount_minor,
	installment_count,
	payable_now_amount_minor,
	remaining_principal_amount_minor,
	status,
	expires_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT DO NOTHING
RETURNING` + contractColumns

	contractRow, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"create contract",
		query,
		decisionID,
		checkoutKey,
		customerID,
		creditAccountID,
		merchantProfileID,
		merchantBranchProfileID,
		currency,
		requestedAmountMinor,
		approvedFinancedAmountMinor,
		upfrontAmountMinor,
		installmentCount,
		payableNowAmountMinor,
		remainingPrincipalAmountMinor,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, false, nil
		}

		return model.Contract{}, false, err
	}

	return contractRow, true, nil
}

// BindMerchantEntitlementIfAbsent stores the Financial Core entitlement without allowing a conflicting overwrite.
func BindMerchantEntitlementIfAbsent[Q postgres.Queryer](
	ctx context.Context, q Q, id model.ContractID, merchantEntitlementID string, updatedAt time.Time,
) (model.Contract, error) {
	const op = "contractrp.BindMerchantEntitlementIfAbsent"
	const query = `
UPDATE contracts
SET financial_merchant_entitlement_id = $2, updated_at = $3
WHERE id = $1
  AND (financial_merchant_entitlement_id IS NULL OR financial_merchant_entitlement_id = $2)
RETURNING` + contractColumns
	row, err := postgres.QueryOne[model.Contract](ctx, q, "bind financial merchant entitlement", query, id, merchantEntitlementID, updatedAt)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.Contract{}, cerr.Conflict(
			fmt.Errorf("%s: contract merchant entitlement already bound to another Financial Core id", op),
			map[string]any{"contract_id": id},
		)
	}

	return row, err
}

// BindReceivableIfAbsent stores the Financial Core receivable id without allowing a conflicting overwrite.
func BindReceivableIfAbsent[Q postgres.Queryer](
	ctx context.Context, q Q, id model.ContractID, receivableID string, updatedAt time.Time,
) (model.Contract, error) {
	const op = "contractrp.BindReceivableIfAbsent"
	const query = `
UPDATE contracts
SET financial_receivable_id = $2, updated_at = $3
WHERE id = $1
  AND (financial_receivable_id IS NULL OR financial_receivable_id = $2)
RETURNING` + contractColumns
	row, err := postgres.QueryOne[model.Contract](ctx, q, "bind financial receivable", query, id, receivableID, updatedAt)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.Contract{}, cerr.Conflict(
			fmt.Errorf("%s: contract receivable already bound to another Financial Core id", op),
			map[string]any{"contract_id": id},
		)
	}

	return row, err
}

// GetByID returns a contract by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ContractID,
) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE id = $1
`

	return postgres.QueryOne[model.Contract](ctx, q, "get contract by id", query, id)
}

// GetByIDAndCustomerID returns a contract owned by the given customer profile.
func GetByIDAndCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ContractID,
	customerID model.CustomerID,
) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE id = $1
  AND customer_id = $2
`

	return postgres.QueryOne[model.Contract](
		ctx,
		q,
		"get contract by id and customer profile id",
		query,
		id,
		customerID,
	)
}

// GetPaymentPendingByCreditAccountID returns the single in-flight contract for a credit account.
func GetPaymentPendingByCreditAccountID[Q postgres.Queryer](ctx context.Context, q Q, creditAccountID model.CreditAccountID) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE credit_account_id = $1
  AND status = 'payment_pending'
LIMIT 1`
	return postgres.QueryOne[model.Contract](ctx, q, "get payment-pending contract by credit account id", query, creditAccountID)
}

// ListActiveByCustomerID returns current contracts (active or canceling) for
// a customer profile using keyset pagination ordered by created_at DESC, id DESC.
func ListActiveByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.Contract, error) {
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	args := []any{customerID, model.ActiveContractStatus, model.CancelingContractStatus}
	var conditions []string
	conditions = append(conditions, "customer_id = $1", "status IN ($2, $3)")

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

	query := `SELECT` + contractColumns + `
FROM contracts
WHERE ` + strings.Join(conditions, `
  AND `) + `
ORDER BY created_at DESC, id DESC
` + limitSQL

	return postgres.QueryAll[model.Contract](
		ctx,
		q,
		"list active contracts by customer profile id",
		query,
		args...,
	)
}

// ListActiveByMerchantProfileID returns current contracts (active or canceling)
// for a merchant profile using keyset pagination ordered by created_at DESC, id DESC.
func ListActiveByMerchantProfileID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
	page pagination.KeysetRequest,
) ([]model.Contract, error) {
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	args := []any{merchantProfileID, model.ActiveContractStatus, model.CancelingContractStatus}
	var conditions []string
	conditions = append(conditions, "merchant_profile_id = $1", "status IN ($2, $3)")

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

	query := `SELECT` + contractColumns + `
FROM contracts
WHERE ` + strings.Join(conditions, `
  AND `) + `
ORDER BY created_at DESC, id DESC
` + limitSQL

	return postgres.QueryAll[model.Contract](
		ctx,
		q,
		"list active contracts by merchant profile id",
		query,
		args...,
	)
}

// ListActiveByMerchantProfileIDOffset returns one page of current contracts
// (active or canceling) for a merchant profile using page/limit offset pagination.
func ListActiveByMerchantProfileIDOffset[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
	page pagination.OffsetRequest,
) ([]model.Contract, error) {
	page.Order = pagination.OrderDesc
	page.Sort = pagination.DefaultSort
	page.Normalize()

	args := []any{merchantProfileID, model.ActiveContractStatus, model.CancelingContractStatus}
	limitOffsetSQL, limitOffsetArgs := postgres.LimitOffset(page, len(args)+1)
	args = append(args, limitOffsetArgs...)

	query := `SELECT` + contractColumns + `
FROM contracts
WHERE merchant_profile_id = $1
  AND status IN ($2, $3)
ORDER BY created_at DESC, id DESC
` + limitOffsetSQL

	return postgres.QueryAll[model.Contract](
		ctx,
		q,
		"list active contracts by merchant profile id offset",
		query,
		args...,
	)
}

// CountActiveByMerchantProfileID returns the number of current contracts
// (active or canceling) for a merchant profile.
func CountActiveByMerchantProfileID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	merchantProfileID model.MerchantProfileID,
) (int64, error) {
	const query = `
SELECT COUNT(*) AS count
FROM contracts
WHERE merchant_profile_id = $1
  AND status IN ($2, $3)
`

	type countRow struct {
		Count int64 `db:"count"`
	}

	row, err := postgres.QueryOne[countRow](
		ctx,
		q,
		"count active contracts by merchant profile id",
		query,
		merchantProfileID,
		model.ActiveContractStatus,
		model.CancelingContractStatus,
	)
	if err != nil {
		return 0, err
	}

	return row.Count, nil
}

// GetByDecisionID returns a contract by decision id.
func GetByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE decision_id = $1
`

	return postgres.QueryOne[model.Contract](ctx, q, "get contract by decision id", query, decisionID)
}

// GetByCheckoutKey returns a contract by canonical checkout key.
func GetByCheckoutKey[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	checkoutKey string,
) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE checkout_key = $1
`

	return postgres.QueryOne[model.Contract](ctx, q, "get contract by checkout key", query, checkoutKey)
}

// LockByID returns and locks a contract row for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ContractID,
) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.Contract](ctx, q, "lock contract by id", query, id)
}

// LockByDecisionID returns and locks a contract row by decision id for update.
func LockByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) (model.Contract, error) {
	const query = `SELECT` + contractColumns + `
FROM contracts
WHERE decision_id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.Contract](ctx, q, "lock contract by decision id", query, decisionID)
}

// BindSuccessfulCheckoutPaymentIdentity stores the Financial Core identity that is
// about to activate a payment-pending contract. It intentionally replaces checkout
// session identifiers from an earlier failed/canceled retry inside the activation
// transaction, while preserving URL/expiry metadata until the synchronous session
// response can enrich them.
func BindSuccessfulCheckoutPaymentIdentity[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	invoiceID string,
	paymentIntentID string,
	paymentSessionID string,
	financialPaymentAttemptID *string,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	financial_invoice_id = $2,
	financial_payment_intent_id = $3,
	financial_payment_session_id = $4,
	financial_payment_attempt_id = $5,
	financial_payment_link = CASE
		WHEN financial_payment_intent_id IS DISTINCT FROM $3 THEN NULL
		ELSE financial_payment_link
	END,
	financial_payment_expires_at = CASE
		WHEN financial_payment_intent_id IS DISTINCT FROM $3 THEN NULL
		ELSE financial_payment_expires_at
	END,
	financial_payment_reused_idempotent_result = CASE
		WHEN financial_payment_intent_id IS DISTINCT FROM $3 THEN false
		ELSE financial_payment_reused_idempotent_result
	END,
	updated_at = $6
WHERE id = $1
  AND status = $7
RETURNING` + contractColumns
	const op = "contractrp.BindSuccessfulCheckoutPaymentIdentity"

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"bind successful checkout payment identity",
		query,
		contractID,
		invoiceID,
		paymentIntentID,
		paymentSessionID,
		financialPaymentAttemptID,
		updatedAt,
		model.PaymentPendingContractStatus,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.Contract{}, cerr.Conflict(
			fmt.Errorf("%s: contract is not payment pending", op),
			map[string]any{"contract_id": contractID},
		)
	}

	return row, err
}

// MarkActive transitions a pre-activation (payment_pending) contract to active.
// The WHERE clause still tolerates any leftover legacy activation_pending rows
// that exist transiently while the unifying migration is being applied.
func MarkActive[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ContractID,
	paymentReference *string,
	paymentID *model.PaymentAttemptID,
	successfulFinancialPaymentAttemptID *string,
	remainingPrincipalAmountMinor uint64,
	activatedAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	status = $2,
	payment_reference = $3,
	payment_id = $4,
	successful_financial_payment_attempt_id = $5,
	remaining_principal_amount_minor = $6,
	activated_at = $7,
	updated_at = $8
WHERE id = $1
  AND status IN ($9, 'activation_pending')
RETURNING` + contractColumns
	const op = "contractrp.MarkActive"

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"mark contract active",
		query,
		id,
		model.ActiveContractStatus,
		paymentReference,
		paymentID,
		successfulFinancialPaymentAttemptID,
		remainingPrincipalAmountMinor,
		activatedAt,
		updatedAt,
		model.PaymentPendingContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract is not in a supported pre-activation state", op),
				map[string]any{
					"contract_id": id,
				},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

// EnrichSuccessfulFinancialPaymentAttemptID sets the final successful provider-attempt id
// only when it is currently NULL, so a later valid duplicate may fill in missing audit
// metadata without overwriting an existing value.
func EnrichSuccessfulFinancialPaymentAttemptID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.ContractID,
	successfulFinancialPaymentAttemptID string,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	successful_financial_payment_attempt_id = $2,
	updated_at = $3
WHERE id = $1
  AND successful_financial_payment_attempt_id IS NULL
RETURNING` + contractColumns
	const op = "contractrp.EnrichSuccessfulFinancialPaymentAttemptID"

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"enrich contract successful financial payment attempt id",
		query,
		id,
		successfulFinancialPaymentAttemptID,
		updatedAt,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract successful financial payment attempt id is already set", op),
				map[string]any{"contract_id": id},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

// SaveCheckoutPaymentSession persists Financial Core checkout payment session data.
func SaveCheckoutPaymentSession[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	invoiceID string,
	paymentIntentID string,
	paymentAttemptID string,
	paymentSessionID string,
	paymentLink string,
	paymentExpiresAt time.Time,
	reusedIdempotentResult bool,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	financial_invoice_id = $2,
	financial_payment_intent_id = $3,
	financial_payment_attempt_id = $4,
	financial_payment_session_id = $5,
	financial_payment_link = $6,
	financial_payment_expires_at = $7,
	financial_payment_reused_idempotent_result = $8,
	updated_at = $9
WHERE id = $1
  AND (
      status = $10
      OR (status IN ($11, $12) AND payment_reference = $3)
  )
RETURNING` + contractColumns
	const op = "contractrp.SaveCheckoutPaymentSession"

	paymentAttemptID = strings.TrimSpace(paymentAttemptID)
	var nullablePaymentAttemptID *string
	if paymentAttemptID != "" {
		nullablePaymentAttemptID = &paymentAttemptID
	}

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"save contract checkout payment session",
		query,
		contractID,
		invoiceID,
		paymentIntentID,
		nullablePaymentAttemptID,
		paymentSessionID,
		paymentLink,
		paymentExpiresAt,
		reusedIdempotentResult,
		updatedAt,
		model.PaymentPendingContractStatus,
		model.ActiveContractStatus,
		model.CompletedContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract cannot accept checkout payment session metadata", op),
				map[string]any{
					"contract_id": contractID,
				},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

// EnrichCheckoutPaymentSessionIfAbsent fills missing Financial Core checkout session
// fields without overwriting non-empty stored identifiers.
func EnrichCheckoutPaymentSessionIfAbsent[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	invoiceID *string,
	paymentIntentID *string,
	paymentAttemptID *string,
	paymentSessionID *string,
	paymentLink *string,
	paymentExpiresAt *time.Time,
	reusedIdempotentResult *bool,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	financial_invoice_id = CASE
		WHEN $2::text IS NOT NULL AND financial_invoice_id IS NULL THEN $2
		ELSE financial_invoice_id
	END,
	financial_payment_intent_id = CASE
		WHEN $3::text IS NOT NULL AND financial_payment_intent_id IS NULL THEN $3
		ELSE financial_payment_intent_id
	END,
	financial_payment_attempt_id = CASE
		WHEN $4::text IS NOT NULL AND financial_payment_attempt_id IS NULL THEN $4
		ELSE financial_payment_attempt_id
	END,
	financial_payment_session_id = CASE
		WHEN $5::text IS NOT NULL AND financial_payment_session_id IS NULL THEN $5
		ELSE financial_payment_session_id
	END,
	financial_payment_link = CASE
		WHEN $6::text IS NOT NULL AND financial_payment_link IS NULL THEN $6
		ELSE financial_payment_link
	END,
	financial_payment_expires_at = CASE
		WHEN $7::timestamptz IS NOT NULL AND financial_payment_expires_at IS NULL THEN $7
		ELSE financial_payment_expires_at
	END,
	financial_payment_reused_idempotent_result = CASE
		WHEN $8::boolean IS NOT NULL THEN $8
		ELSE financial_payment_reused_idempotent_result
	END,
	updated_at = $9
WHERE id = $1
RETURNING` + contractColumns

	return postgres.QueryOne[model.Contract](
		ctx,
		q,
		"enrich contract checkout payment session if absent",
		query,
		contractID,
		invoiceID,
		paymentIntentID,
		paymentAttemptID,
		paymentSessionID,
		paymentLink,
		paymentExpiresAt,
		reusedIdempotentResult,
		updatedAt,
	)
}

var (
	// ErrInvalidClaimLimit indicates an invalid batch claim limit.
	ErrInvalidClaimLimit = fmt.Errorf("invalid claim limit")
)

// ClaimDuePaymentPending locks payment_pending contracts eligible for expiry.
func ClaimDuePaymentPending[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	processingGrace time.Duration,
	limit int,
) ([]model.Contract, error) {
	const op = "contractrp.ClaimDuePaymentPending"

	if limit <= 0 {
		return nil, cerr.InvalidArgument(ErrInvalidClaimLimit, map[string]any{"limit": limit})
	}

	const query = `
SELECT` + contractColumns + `
FROM contracts c
WHERE c.status = $1
  AND (
    CASE
      WHEN c.financial_payment_expires_at IS NOT NULL
           AND c.financial_payment_expires_at < c.expires_at
      THEN c.financial_payment_expires_at
      ELSE c.expires_at
    END + ($2::bigint * interval '1 microsecond')
  ) <= $3
ORDER BY c.expires_at ASC, c.created_at ASC
LIMIT $4
FOR UPDATE SKIP LOCKED
`

	rows, err := q.Query(
		ctx,
		query,
		model.PaymentPendingContractStatus,
		processingGrace.Microseconds(),
		now,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: claim due payment pending contracts: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Contract](postgres.MustRows(rows))
}

// MarkExpired transitions a payment_pending contract to expired.
func MarkExpired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	status = $2,
	updated_at = $3
WHERE id = $1
  AND status = $4
RETURNING` + contractColumns
	const op = "contractrp.MarkExpired"

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"mark contract expired",
		query,
		contractID,
		model.ExpiredContractStatus,
		updatedAt,
		model.PaymentPendingContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract is not payment pending", op),
				map[string]any{"contract_id": contractID},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

// MarkCanceledFromPaymentPending closes a pre-activation contract after an
// authoritative Financial Core checkout-payment cancellation.
func MarkCanceledFromPaymentPending[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	const op = "contractrp.MarkCanceledFromPaymentPending"
	if err := postgres.RequiredTimes(map[string]time.Time{
		"canceled_at": canceledAt,
		"updated_at":  updatedAt,
	}); err != nil {
		return model.Contract{}, err
	}

	const query = `
UPDATE contracts
SET
	status = $2,
	canceled_at = COALESCE(canceled_at, $3),
	updated_at = $4
WHERE id = $1
  AND status = $5
RETURNING` + contractColumns

	row, err := postgres.QueryOne[model.Contract](
		ctx, q, "mark payment pending contract canceled", query,
		contractID, model.CanceledContractStatus, canceledAt, updatedAt, model.PaymentPendingContractStatus,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.Contract{}, cerr.Conflict(
			fmt.Errorf("%s: contract is not payment pending", op),
			map[string]any{"contract_id": contractID},
		)
	}

	return row, err
}

// MarkCanceling transitions an active contract to canceling.
func MarkCanceling[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	idempotencyKey string,
	reason model.ContractCancellationReason,
	requestFingerprint string,
	financialCoreIdempotencyKey string,
	tenantID model.TenantID,
	contextID model.ContextID,
	requestedAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	const op = "contractrp.MarkCanceling"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"cancellation_requested_at": requestedAt,
		"updated_at":                updatedAt,
	}); err != nil {
		return model.Contract{}, err
	}

	const query = `
UPDATE contracts
SET
	status = $2,
	cancellation_idempotency_key = $3,
	cancellation_reason = $4,
	cancellation_request_fingerprint = $5,
	cancellation_requested_at = $6,
	cancellation_financial_core_idempotency_key = $7,
	cancellation_tenant_id = $8,
	cancellation_context_id = $9,
	cancellation_financial_core_accepted_at = NULL,
	updated_at = $10
WHERE id = $1
  AND status = $11
RETURNING` + contractColumns

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"mark contract canceling",
		query,
		contractID,
		model.CancelingContractStatus,
		idempotencyKey,
		reason,
		requestFingerprint,
		requestedAt,
		financialCoreIdempotencyKey,
		tenantID,
		contextID,
		updatedAt,
		model.ActiveContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract is not active", op),
				map[string]any{"contract_id": contractID},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

// MarkCanceledAfterCancellation applies a successful authoritative synchronous
// Financial Core cancellation result. The update is idempotent for a contract
// already canceled by the same request metadata and records canceled_at.
func MarkCanceledAfterCancellation[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	const op = "contractrp.MarkCanceledAfterCancellation"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"canceled_at": canceledAt,
		"updated_at":  updatedAt,
	}); err != nil {
		return model.Contract{}, err
	}

	const query = `
UPDATE contracts
SET
	status = $2,
	canceled_at = COALESCE(canceled_at, $3),
	cancellation_financial_core_accepted_at = COALESCE(cancellation_financial_core_accepted_at, $3),
	updated_at = $4
WHERE id = $1
  AND status = $5
RETURNING` + contractColumns

	row, err := postgres.QueryOne[model.Contract](
		ctx, q, "mark contract canceled after financial core cancellation", query,
		contractID, model.CanceledContractStatus, canceledAt, updatedAt, model.CancelingContractStatus,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.Contract{}, cerr.Conflict(
			fmt.Errorf("%s: contract is not canceling", op),
			map[string]any{"contract_id": contractID},
		)
	}

	return row, err
}

// RestoreActiveAfterCancellationRejected compensates a cancellation start
// after a definitive external rejection. The transition clears cancellation
// request metadata so a later independent cancellation may start cleanly.
func RestoreActiveAfterCancellationRejected[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	updatedAt time.Time,
) (model.Contract, error) {
	const op = "contractrp.RestoreActiveAfterCancellationRejected"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"updated_at": updatedAt,
	}); err != nil {
		return model.Contract{}, err
	}

	const query = `
UPDATE contracts
SET
	status = $2,
	cancellation_idempotency_key = NULL,
	cancellation_reason = NULL,
	cancellation_request_fingerprint = NULL,
	cancellation_requested_at = NULL,
	cancellation_financial_core_idempotency_key = NULL,
	cancellation_tenant_id = NULL,
	cancellation_context_id = NULL,
	cancellation_financial_core_accepted_at = NULL,
	updated_at = $3
WHERE id = $1
  AND status = $4
  AND cancellation_financial_core_accepted_at IS NULL
RETURNING` + contractColumns

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"restore active contract after cancellation rejection",
		query,
		contractID,
		model.ActiveContractStatus,
		updatedAt,
		model.CancelingContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract is not canceling", op),
				map[string]any{"contract_id": contractID},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

type contractIDRow struct {
	ID model.ContractID `db:"id"`
}

// ListDuePaymentPendingContractIDs returns contract ids eligible for expiry without locking.
func ListDuePaymentPendingContractIDs[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	processingGrace time.Duration,
	limit int,
) ([]model.ContractID, error) {
	const op = "contractrp.ListDuePaymentPendingContractIDs"

	if limit <= 0 {
		return nil, cerr.InvalidArgument(ErrInvalidClaimLimit, map[string]any{"limit": limit})
	}

	const query = `
SELECT c.id
FROM contracts c
WHERE c.status = $1
  AND (
    CASE
      WHEN c.financial_payment_expires_at IS NOT NULL
           AND c.financial_payment_expires_at < c.expires_at
      THEN c.financial_payment_expires_at
      ELSE c.expires_at
    END + ($2::bigint * interval '1 microsecond')
  ) <= $3
ORDER BY c.expires_at ASC, c.created_at ASC
LIMIT $4
`

	rows, err := q.Query(
		ctx,
		query,
		model.PaymentPendingContractStatus,
		processingGrace.Microseconds(),
		now,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: list due payment pending contract ids: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	idRows, err := postgres.ValuesInto[contractIDRow](postgres.MustRows(rows))
	if err != nil {
		return nil, fmt.Errorf("%s: list due payment pending contract ids: %w", op, err)
	}

	ids := make([]model.ContractID, len(idRows))
	for i := range idRows {
		ids[i] = idRows[i].ID
	}

	return ids, nil
}

// MarkCompleted transitions an active contract to completed.
func MarkCompleted[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	completedAt time.Time,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	status = $2,
	completed_at = $3,
	updated_at = $4
WHERE id = $1
  AND status = $5
RETURNING` + contractColumns
	const op = "contractrp.MarkCompleted"

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"mark contract completed",
		query,
		contractID,
		model.CompletedContractStatus,
		completedAt,
		updatedAt,
		model.ActiveContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract is not active", op),
				map[string]any{"contract_id": contractID},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}

// ReduceRemainingPrincipal decreases remaining contract principal with underflow protection.
func ReduceRemainingPrincipal[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	amountMinor uint64,
	updatedAt time.Time,
) (model.Contract, error) {
	const query = `
UPDATE contracts
SET
	remaining_principal_amount_minor = GREATEST(remaining_principal_amount_minor - $3, 0),
	updated_at = $4
WHERE id = $1
  AND status = $2
  AND remaining_principal_amount_minor >= $3
RETURNING` + contractColumns
	const op = "contractrp.ReduceRemainingPrincipal"

	row, err := postgres.QueryOne[model.Contract](
		ctx,
		q,
		"reduce contract remaining principal",
		query,
		contractID,
		model.ActiveContractStatus,
		amountMinor,
		updatedAt,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Contract{}, cerr.Conflict(
				fmt.Errorf("%s: contract remaining principal underflow", op),
				map[string]any{
					"contract_id":  contractID,
					"amount_minor": amountMinor,
				},
			)
		}

		return model.Contract{}, err
	}

	return row, nil
}
