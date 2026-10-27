package paymentattemptrp

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

const paymentAttemptColumns = `
	id,
	contract_id,
	installment_id,
	amount_minor,
	currency,
	purpose,
	tenant_id,
	context_id,
	provider_code,
	success_return_url,
	failure_return_url,
	cancel_return_url,
	state,
	channel,
	idempotency_key,
	request_fingerprint,
	financial_collection_obligation_id,
	financial_payment_intent_id,
	financial_payment_attempt_id,
	financial_payment_session_id,
	payment_url,
	financial_payment_expires_at,
	failure_code,
	reconciliation_reason,
	reconciliation_attempt_count,
	reconciliation_requested_at,
	next_reconciliation_at,
	reconciliation_resolved_at,
	last_reconciliation_error,
	manual_review_required_at,
	submitted_at,
	financial_paid_at,
	applied_at,
	failed_at,
	canceled_at,
	created_at,
	updated_at
`

// paymentAttemptColumnsP qualifies payment_attempts columns with the "p" alias.
// Use this for UPDATE ... FROM CTE queries where unqualified "id" is ambiguous.
const paymentAttemptColumnsP = `
	p.id,
	p.contract_id,
	p.installment_id,
	p.amount_minor,
	p.currency,
	p.purpose,
	p.tenant_id,
	p.context_id,
	p.provider_code,
	p.success_return_url,
	p.failure_return_url,
	p.cancel_return_url,
	p.state,
	p.channel,
	p.idempotency_key,
	p.request_fingerprint,
	p.financial_collection_obligation_id,
	p.financial_payment_intent_id,
	p.financial_payment_attempt_id,
	p.financial_payment_session_id,
	p.payment_url,
	p.financial_payment_expires_at,
	p.failure_code,
	p.reconciliation_reason,
	p.reconciliation_attempt_count,
	p.reconciliation_requested_at,
	p.next_reconciliation_at,
	p.reconciliation_resolved_at,
	p.last_reconciliation_error,
	p.manual_review_required_at,
	p.submitted_at,
	p.financial_paid_at,
	p.applied_at,
	p.failed_at,
	p.canceled_at,
	p.created_at,
	p.updated_at
`

// paymentAttemptHistoryColumns selects only fields required by installment payment history.
const paymentAttemptHistoryColumns = `
	pa.id,
	pa.contract_id,
	pa.installment_id,
	pa.amount_minor,
	pa.currency,
	pa.purpose,
	pa.state,
	pa.applied_at,
	pa.failed_at,
	pa.canceled_at,
	pa.created_at
`

type finalizedInstallmentPaymentRow struct {
	ID            model.PaymentAttemptID            `db:"id"`
	ContractID    model.ContractID                  `db:"contract_id"`
	InstallmentID model.InstallmentID               `db:"installment_id"`
	AmountMinor   uint64                            `db:"amount_minor"`
	Currency      model.Currency                    `db:"currency"`
	Purpose       model.FinancialCorePaymentPurpose `db:"purpose"`
	State         model.PaymentAttemptState         `db:"state"`
	AppliedAt     *time.Time                        `db:"applied_at"`
	FailedAt      *time.Time                        `db:"failed_at"`
	CanceledAt    *time.Time                        `db:"canceled_at"`
	CreatedAt     time.Time                         `db:"created_at"`
}

// ListFinalizedInstallmentPaymentsByCustomerID returns finalized installment
// payment attempts for a customer using keyset pagination (created_at DESC, id DESC).
func ListFinalizedInstallmentPaymentsByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.PaymentAttempt, error) {
	page.Order = pagination.OrderDesc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	args := []any{
		customerID,
		model.SucceededPaymentAttemptState,
		model.FailedPaymentAttemptState,
		model.CanceledPaymentAttemptState,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
	}
	conditions := []string{
		"i.customer_id = $1",
		"pa.state IN ($2, $3, $4)",
		"pa.purpose IN ($5, $6)",
	}

	keysetWhere, keysetArgs, err := postgres.KeysetWhere("pa.created_at", "pa.id", page, len(args)+1)
	if err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	if keysetWhere != "" {
		conditions = append(conditions, keysetWhere)
		args = append(args, keysetArgs...)
	}

	limitSQL, limitArgs := postgres.Limit(page.Limit, len(args)+1)
	args = append(args, limitArgs...)

	query := `SELECT` + paymentAttemptHistoryColumns + `
FROM payment_attempts AS pa
INNER JOIN installments AS i ON i.id = pa.installment_id
WHERE ` + strings.Join(conditions, `
  AND `) + `
ORDER BY pa.created_at DESC, pa.id DESC
` + limitSQL

	rows, err := postgres.QueryAll[finalizedInstallmentPaymentRow](
		ctx,
		q,
		"list finalized installment payments by customer profile id",
		query,
		args...,
	)
	if err != nil {
		return nil, err
	}

	out := make([]model.PaymentAttempt, 0, len(rows))
	for _, row := range rows {
		out = append(out, model.PaymentAttempt{
			ID:            row.ID,
			ContractID:    row.ContractID,
			InstallmentID: row.InstallmentID,
			AmountMinor:   row.AmountMinor,
			Currency:      row.Currency,
			Purpose:       row.Purpose,
			State:         row.State,
			AppliedAt:     row.AppliedAt,
			FailedAt:      row.FailedAt,
			CanceledAt:    row.CanceledAt,
			CreatedAt:     row.CreatedAt,
		})
	}

	return out, nil
}

type installmentIDRow struct {
	InstallmentID model.InstallmentID `db:"installment_id"`
}

// ListInstallmentIDsWithNonTerminalAttempts returns installment IDs among the
// given set that currently have a non-terminal payment attempt for the customer.
func ListInstallmentIDsWithNonTerminalAttempts[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	installmentIDs []model.InstallmentID,
) ([]model.InstallmentID, error) {
	const op = "paymentattemptrp.ListInstallmentIDsWithNonTerminalAttempts"

	if customerID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	if len(installmentIDs) == 0 {
		return nil, nil
	}

	const query = `
SELECT DISTINCT pa.installment_id
FROM payment_attempts AS pa
INNER JOIN installments AS i ON i.id = pa.installment_id
WHERE i.customer_id = $1
  AND pa.installment_id = ANY($2)
  AND pa.state IN ($3, $4, $5)
  AND pa.purpose IN ($6, $7)
ORDER BY pa.installment_id ASC
`

	rows, err := postgres.QueryAll[installmentIDRow](
		ctx,
		q,
		"list installment ids with non-terminal payment attempts",
		query,
		customerID,
		installmentIDs,
		model.CreatedPaymentAttemptState,
		model.SubmittedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
	)
	if err != nil {
		return nil, err
	}

	out := make([]model.InstallmentID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.InstallmentID)
	}

	return out, nil
}

// GetByInstallmentIDAndIdempotencyKey returns a payment attempt by installment and key.
func GetByInstallmentIDAndIdempotencyKey[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	idempotencyKey string,
) (model.PaymentAttempt, error) {
	const query = `SELECT` + paymentAttemptColumns + `
FROM payment_attempts
WHERE installment_id = $1
  AND idempotency_key = $2
`

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"get payment attempt by installment id and idempotency key",
		query,
		installmentID,
		idempotencyKey,
	)
}

// GetByFinancialPaymentIntentID returns a payment attempt by Financial Core intent id.
func GetByFinancialPaymentIntentID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	financialPaymentIntentID string,
) (model.PaymentAttempt, error) {
	const query = `SELECT` + paymentAttemptColumns + `
FROM payment_attempts
WHERE financial_payment_intent_id = $1
`

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"get payment attempt by financial payment intent id",
		query,
		financialPaymentIntentID,
	)
}

// GetByContractIDAndPurpose returns the payment attempt for a contract purpose.
func GetByContractIDAndPurpose[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	purpose model.FinancialCorePaymentPurpose,
) (model.PaymentAttempt, error) {
	const query = `SELECT` + paymentAttemptColumns + `
FROM payment_attempts
WHERE contract_id = $1
  AND purpose = $2
ORDER BY
  CASE WHEN state IN ('created', 'submitted', 'reconciliation_required') THEN 0 ELSE 1 END,
  created_at DESC,
  updated_at DESC
LIMIT 1
`

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"get payment attempt by contract id and purpose",
		query,
		contractID,
		purpose,
	)
}

// CreateCheckout stores a checkout-first-installment payment attempt.
func CreateCheckout[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	installmentID model.InstallmentID,
	amountMinor uint64,
	currency model.Currency,
	channel model.PaymentChannel,
	idempotencyKey string,
	requestFingerprint string,
	tenantID string,
	contextID string,
	providerCode *string,
	successReturnURL string,
	failureReturnURL string,
	cancelReturnURL string,
	createdAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.PaymentAttempt{}, err
	}

	const query = `
INSERT INTO payment_attempts (
	contract_id,
	installment_id,
	amount_minor,
	currency,
	purpose,
	tenant_id,
	context_id,
	provider_code,
	success_return_url,
	failure_return_url,
	cancel_return_url,
	state,
	channel,
	idempotency_key,
	request_fingerprint,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), NULLIF($7, ''), $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"create checkout payment attempt",
		query,
		contractID,
		installmentID,
		amountMinor,
		currency,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		tenantID,
		contextID,
		providerCode,
		successReturnURL,
		failureReturnURL,
		cancelReturnURL,
		model.CreatedPaymentAttemptState,
		channel,
		idempotencyKey,
		requestFingerprint,
		createdAt,
		updatedAt,
	)
}

// GetByID returns a payment attempt row without acquiring a row lock.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
) (model.PaymentAttempt, error) {
	const query = `SELECT` + paymentAttemptColumns + `
FROM payment_attempts
WHERE id = $1
`

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"get payment attempt by id",
		query,
		id,
	)
}

// LockByID returns and locks a payment attempt row for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
) (model.PaymentAttempt, error) {
	const query = `SELECT` + paymentAttemptColumns + `
FROM payment_attempts
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"lock payment attempt by id",
		query,
		id,
	)
}

// Create stores a payment attempt row and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	installmentID model.InstallmentID,
	amountMinor uint64,
	currency model.Currency,
	state model.PaymentAttemptState,
	channel model.PaymentChannel,
	idempotencyKey string,
	requestFingerprint string,
	createdAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.PaymentAttempt{}, err
	}

	const query = `
INSERT INTO payment_attempts (
	contract_id,
	installment_id,
	amount_minor,
	currency,
	purpose,
	state,
	channel,
	idempotency_key,
	request_fingerprint,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"create payment attempt",
		query,
		contractID,
		installmentID,
		amountMinor,
		currency,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		state,
		channel,
		idempotencyKey,
		requestFingerprint,
		createdAt,
		updatedAt,
	)
}

// BindCollectionObligationIfAbsent stores the prepared Financial Core collection obligation id.
func BindCollectionObligationIfAbsent[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	collectionObligationID string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.BindCollectionObligationIfAbsent"
	const query = `
UPDATE payment_attempts
SET financial_collection_obligation_id = $2, updated_at = $3
WHERE id = $1
  AND (financial_collection_obligation_id IS NULL OR financial_collection_obligation_id = $2)
RETURNING` + paymentAttemptColumns
	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"bind financial collection obligation",
		query,
		id,
		collectionObligationID,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt collection obligation already bound to another Financial Core id", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// BindFinancialRequestContextIfAbsent stores immutable Financial Core request context for an attempt.
func BindFinancialRequestContextIfAbsent[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	tenantID string,
	contextID string,
	providerCode *string,
	successReturnURL string,
	failureReturnURL string,
	cancelReturnURL string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	tenant_id = COALESCE(tenant_id, $2),
	context_id = COALESCE(context_id, $3),
	provider_code = COALESCE(provider_code, $4),
	success_return_url = COALESCE(success_return_url, $5),
	failure_return_url = COALESCE(failure_return_url, $6),
	cancel_return_url = COALESCE(cancel_return_url, $7),
	updated_at = $8
WHERE id = $1
  AND (tenant_id IS NULL OR tenant_id = $2)
  AND (context_id IS NULL OR context_id = $3)
  AND (provider_code IS NULL OR provider_code IS NOT DISTINCT FROM $4)
  AND (success_return_url IS NULL OR success_return_url = $5)
  AND (failure_return_url IS NULL OR failure_return_url = $6)
  AND (cancel_return_url IS NULL OR cancel_return_url = $7)
RETURNING` + paymentAttemptColumns

	const op = "paymentattemptrp.BindFinancialRequestContextIfAbsent"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"bind financial request context",
		query,
		id,
		tenantID,
		contextID,
		providerCode,
		successReturnURL,
		failureReturnURL,
		cancelReturnURL,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt financial request context conflicts with persisted values", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// MarkSubmitted persists Financial Core session identifiers and submitted state.
func MarkSubmitted[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID string,
	paymentURL *string,
	financialPaymentExpiresAt *time.Time,
	submittedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	financial_payment_intent_id = $3,
	financial_payment_session_id = $4,
	payment_url = $5,
	financial_payment_expires_at = $6,
	submitted_at = $7,
	updated_at = $8
WHERE id = $1
  AND state = $9
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkSubmitted"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt submitted",
		query,
		id,
		model.SubmittedPaymentAttemptState,
		financialPaymentIntentID,
		financialPaymentSessionID,
		paymentURL,
		financialPaymentExpiresAt,
		submittedAt,
		updatedAt,
		model.CreatedPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt is not created", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}

// MarkSucceeded transitions a submitted attempt to succeeded.
func MarkSucceeded[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	financialPaymentAttemptID *string,
	financialPaidAt time.Time,
	appliedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	financial_payment_attempt_id = $3,
	financial_paid_at = $4,
	applied_at = $5,
	reconciliation_resolved_at = CASE
		WHEN state = $9 THEN $6
		ELSE reconciliation_resolved_at
	END,
	updated_at = $6
WHERE id = $1
  AND state IN ($7, $8, $9)
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkSucceeded"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt succeeded",
		query,
		id,
		model.SucceededPaymentAttemptState,
		financialPaymentAttemptID,
		financialPaidAt,
		appliedAt,
		updatedAt,
		model.SubmittedPaymentAttemptState,
		model.CreatedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt state does not allow success", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}

// EnrichFinancialPaymentSessionIfAbsent fills missing session metadata without changing state.
func EnrichFinancialPaymentSessionIfAbsent[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID string,
	financialPaymentAttemptID *string,
	paymentURL string,
	financialPaymentExpiresAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	financial_payment_intent_id = COALESCE(financial_payment_intent_id, $2),
	financial_payment_session_id = COALESCE(financial_payment_session_id, $3),
	financial_payment_attempt_id = COALESCE(financial_payment_attempt_id, $4),
	payment_url = COALESCE(payment_url, $5),
	financial_payment_expires_at = COALESCE(financial_payment_expires_at, $6),
	updated_at = $7
WHERE id = $1
  AND (financial_payment_intent_id IS NULL OR financial_payment_intent_id = $2)
  AND (financial_payment_session_id IS NULL OR financial_payment_session_id = $3)
  AND ($4::text IS NULL OR financial_payment_attempt_id IS NULL OR financial_payment_attempt_id = $4)
  AND (payment_url IS NULL OR payment_url = $5)
  AND (financial_payment_expires_at IS NULL OR financial_payment_expires_at = $6)
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.EnrichFinancialPaymentSessionIfAbsent"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"enrich financial payment session if absent",
		query,
		id,
		financialPaymentIntentID,
		financialPaymentSessionID,
		financialPaymentAttemptID,
		paymentURL,
		financialPaymentExpiresAt,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment session identity conflicts with payment attempt", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// MarkFailed transitions a submitted attempt to failed.
func MarkFailed[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	failureCode string,
	failedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	failure_code = $3,
	failed_at = $4,
	reconciliation_resolved_at = CASE
		WHEN state = $8 THEN $5
		ELSE reconciliation_resolved_at
	END,
	updated_at = $5
WHERE id = $1
  AND state IN ($6, $7, $8)
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkFailed"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt failed",
		query,
		id,
		model.FailedPaymentAttemptState,
		failureCode,
		failedAt,
		updatedAt,
		model.SubmittedPaymentAttemptState,
		model.CreatedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt state does not allow failure", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}

// MarkCanceled transitions a submitted attempt to canceled.
func MarkCanceled[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	failureCode *string,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	failure_code = $3,
	canceled_at = $4,
	reconciliation_resolved_at = CASE
		WHEN state = $8 THEN $5
		ELSE reconciliation_resolved_at
	END,
	updated_at = $5
WHERE id = $1
  AND state IN ($6, $7, $8)
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkCanceled"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt canceled",
		query,
		id,
		model.CanceledPaymentAttemptState,
		failureCode,
		canceledAt,
		updatedAt,
		model.SubmittedPaymentAttemptState,
		model.CreatedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt state does not allow cancellation", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}

// MarkReconciliationRequired transitions an attempt to reconciliation_required.
func MarkReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	reason model.PaymentReconciliationReason,
	financialPaymentAttemptID *string,
	financialPaidAt *time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	reconciliation_reason = $3,
	financial_payment_attempt_id = $4,
	financial_paid_at = $5,
	reconciliation_requested_at = COALESCE(reconciliation_requested_at, $6),
	next_reconciliation_at = COALESCE(next_reconciliation_at, $6),
	updated_at = $6
WHERE id = $1
  AND state IN ($7, $8)
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkReconciliationRequired"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt reconciliation required",
		query,
		id,
		model.ReconciliationRequiredPaymentAttemptState,
		reason,
		financialPaymentAttemptID,
		financialPaidAt,
		updatedAt,
		model.SubmittedPaymentAttemptState,
		model.CreatedPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt state does not allow reconciliation", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}
