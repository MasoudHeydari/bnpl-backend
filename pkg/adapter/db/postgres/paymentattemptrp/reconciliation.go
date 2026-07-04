package paymentattemptrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// IncrementFinancialSessionCreateAttemptCount reserves one sync CreateCheckoutPayment
// attempt while the row remains created and under MaxFinancialSessionCreateAttempts.
func IncrementFinancialSessionCreateAttemptCount[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	maxAttempts int,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.IncrementFinancialSessionCreateAttemptCount"

	const query = `
UPDATE payment_attempts
SET
	financial_session_create_attempt_count = financial_session_create_attempt_count + 1,
	-- Lease duration matches model.FinancialSessionCreateLeaseDuration (30s):
	-- longer than the 5s Financial Core gRPC deadline, shorter than the local
	-- expiration safety window. Expiry is not financial failure.
	-- The token is the reservation owner. A later verified FC session may
	-- still be stored after this lease expires.
	financial_session_create_lease_until = $2 + interval '30 seconds',
	financial_session_create_lease_token = gen_random_uuid(),
	updated_at = $2
WHERE id = $1
  AND financial_session_create_attempt_count < $4
  AND (
	financial_session_create_lease_until IS NULL
	OR financial_session_create_lease_until <= $2
  )
  AND (
	-- Installment repayment: only created may reserve (no session rediscovery).
	(purpose = $5 AND state = $3)
	OR (
		-- Checkout and other purposes may still recover recoverable uncertainty.
		purpose <> $5
		AND (
			state = $3
			OR (
				state = $6
				AND reconciliation_reason IN (
					'payment_session_creation_timeout',
					'financial_core_status_still_unknown'
				)
			)
		)
	)
  )
RETURNING` + paymentAttemptColumns

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"increment financial session create attempt count",
		query,
		id,
		updatedAt,
		model.CreatedPaymentAttemptState,
		maxAttempts,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt cannot reserve another financial session create attempt", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}

// ClaimExpiredSubmittedForReconciliation transitions expired submitted attempts.
func ClaimExpiredSubmittedForReconciliation[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	expiryGrace time.Duration,
	reason model.PaymentReconciliationReason,
	requestedAt time.Time,
	nextReconciliationAt time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.PaymentAttempt, error) {
	expiryCutoff := now.Add(-expiryGrace)

	const query = `
WITH picked AS (
	SELECT id
	FROM payment_attempts
	WHERE state = $1
	  AND purpose = $2
	  AND financial_payment_expires_at IS NOT NULL
	  AND financial_payment_expires_at <= $3
	ORDER BY financial_payment_expires_at ASC
	LIMIT $4
	FOR UPDATE SKIP LOCKED
)
UPDATE payment_attempts AS p
SET
	state = $5,
	reconciliation_reason = $6,
	reconciliation_attempt_count = 0,
	reconciliation_requested_at = $7,
	next_reconciliation_at = $8,
	updated_at = $9
FROM picked
WHERE p.id = picked.id
RETURNING` + paymentAttemptColumnsP

	return postgres.QueryAll[model.PaymentAttempt](
		ctx,
		q,
		"claim expired submitted payment attempts for reconciliation",
		query,
		model.SubmittedPaymentAttemptState,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		expiryCutoff,
		limit,
		model.ReconciliationRequiredPaymentAttemptState,
		reason,
		requestedAt,
		nextReconciliationAt,
		updatedAt,
	)
}

// ClaimDueReconciliationRequired claims due reconciliation_required attempts for retry.
func ClaimDueReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	claimLeaseUntil time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.PaymentAttempt, error) {
	const query = `
WITH picked AS (
	SELECT id
	FROM payment_attempts
	WHERE state = $1
	  AND purpose = $2
	  AND next_reconciliation_at IS NOT NULL
	  AND next_reconciliation_at <= $3
	ORDER BY next_reconciliation_at ASC, created_at ASC
	LIMIT $4
	FOR UPDATE SKIP LOCKED
)
UPDATE payment_attempts AS p
SET
	next_reconciliation_at = $5,
	updated_at = $6
FROM picked
WHERE p.id = picked.id
RETURNING` + paymentAttemptColumnsP

	return postgres.QueryAll[model.PaymentAttempt](
		ctx,
		q,
		"claim due reconciliation required payment attempts",
		query,
		model.ReconciliationRequiredPaymentAttemptState,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		now,
		limit,
		claimLeaseUntil,
		updatedAt,
	)
}

// RecordReconciliationRequest records a successfully queued reconciliation request.
func RecordReconciliationRequest[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	previousAttemptCount int,
	requestedAt time.Time,
	nextReconciliationAt time.Time,
	lastError *string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	reconciliation_attempt_count = reconciliation_attempt_count + 1,
	reconciliation_requested_at = $3,
	next_reconciliation_at = $4,
	last_reconciliation_error = $5,
	updated_at = $6
WHERE id = $1
  AND state = $7
  AND reconciliation_attempt_count = $2
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.RecordReconciliationRequest"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"record payment attempt reconciliation request",
		query,
		id,
		previousAttemptCount,
		requestedAt,
		nextReconciliationAt,
		lastError,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt reconciliation request was concurrently updated", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// MarkSubmittedFromReconciliationRequired transitions reconciliation_required to submitted.
func MarkSubmittedFromReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	financialPaymentIntentID string,
	financialPaymentSessionID string,
	paymentURL *string,
	financialPaymentExpiresAt *time.Time,
	submittedAt time.Time,
	resolvedAt time.Time,
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
	financial_session_create_lease_until = NULL,
	financial_session_create_lease_token = NULL,
	submitted_at = $7,
	reconciliation_resolved_at = $8,
	updated_at = $9
WHERE id = $1
  AND state = $10
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt submitted from reconciliation required",
		query,
		id,
		model.SubmittedPaymentAttemptState,
		financialPaymentIntentID,
		financialPaymentSessionID,
		paymentURL,
		financialPaymentExpiresAt,
		submittedAt,
		resolvedAt,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
}

// MarkSucceededFromReconciliationRequired transitions reconciliation_required to succeeded.
func MarkSucceededFromReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	financialPaymentAttemptID *string,
	financialPaidAt time.Time,
	appliedAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
 financial_outcome_resolved=true,
	state = $2,
	financial_payment_attempt_id = $3,
	financial_paid_at = $4,
	applied_at = $5,
	reconciliation_resolved_at = $6,
	updated_at = $7
WHERE id = $1
  AND state = $8
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt succeeded from reconciliation required",
		query,
		id,
		model.SucceededPaymentAttemptState,
		financialPaymentAttemptID,
		financialPaidAt,
		appliedAt,
		resolvedAt,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
}

// MarkFailedFromReconciliationRequired transitions reconciliation_required to failed.
func MarkFailedFromReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	failureCode string,
	failedAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
 financial_outcome_resolved=true,
	state = $2,
	failure_code = $3,
	failed_at = $4,
	reconciliation_resolved_at = $5,
	updated_at = $6
WHERE id = $1
  AND state = $7
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkFailedFromReconciliationRequired"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt failed from reconciliation required",
		query,
		id,
		model.FailedPaymentAttemptState,
		failureCode,
		failedAt,
		resolvedAt,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.PaymentAttempt{}, cerr.Conflict(
				fmt.Errorf("%s: payment attempt is not reconciliation required", op),
				map[string]any{"payment_attempt_id": id},
			)
		}

		return model.PaymentAttempt{}, err
	}

	return row, nil
}

// MarkCanceledFromReconciliationRequired transitions reconciliation_required to canceled.
func MarkCanceledFromReconciliationRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	failureCode *string,
	canceledAt time.Time,
	resolvedAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
 financial_outcome_resolved=true,
	state = $2,
	failure_code = $3,
	canceled_at = $4,
	reconciliation_resolved_at = $5,
	updated_at = $6
WHERE id = $1
  AND state = $7
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt canceled from reconciliation required",
		query,
		id,
		model.CanceledPaymentAttemptState,
		failureCode,
		canceledAt,
		resolvedAt,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
}

// MarkCanceledFromCreated transitions created to canceled.
func MarkCanceledFromCreated[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	failureCode string,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	failure_code = $3,
	canceled_at = $4,
	updated_at = $5
WHERE id = $1
  AND state = $6
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt canceled from created",
		query,
		id,
		model.CanceledPaymentAttemptState,
		failureCode,
		canceledAt,
		updatedAt,
		model.CreatedPaymentAttemptState,
	)
}

// CancelSessionCreationRetryExhausted locally cancels an installment repayment
// PaymentAttempt after the synchronous CreateCheckoutPayment budget is exhausted.
// financial_outcome_resolved stays false. Refuses submitted/succeeded and any
// attempt that already exposed a customer-actionable PaymentURL.
func CancelSessionCreationRetryExhausted[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	expectedState model.PaymentAttemptState,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.CancelSessionCreationRetryExhausted"

	const query = `
UPDATE payment_attempts
SET
	state = $2,
	failure_code = $3,
	canceled_at = $4,
	next_reconciliation_at = NULL,
	financial_session_create_lease_until = NULL,
	financial_session_create_lease_token = NULL,
	updated_at = $5
WHERE id = $1
  AND state = $6
  AND purpose = $7
  AND financial_session_create_attempt_count >= $8
  AND NOT financial_outcome_resolved
  AND submitted_at IS NULL
  AND (payment_url IS NULL OR btrim(payment_url) = '')
RETURNING` + paymentAttemptColumns

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"cancel payment attempt session creation retry exhausted",
		query,
		id,
		model.CanceledPaymentAttemptState,
		model.SessionCreationRetryExhaustedFailureCode,
		canceledAt,
		updatedAt,
		expectedState,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		model.MaxFinancialSessionCreateAttempts,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt is not eligible for local session-creation cancellation", op),
			map[string]any{"payment_attempt_id": id, "expected_state": expectedState},
		)
	}

	return row, err
}

// ClearFinancialSessionCreateLease releases a reservation after a sync
// CreateCheckoutPayment invocation returns without MarkSubmitted.
func ClearFinancialSessionCreateLease[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	leaseToken uuid.UUID,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.ClearFinancialSessionCreateLease"

	const query = `
UPDATE payment_attempts
SET
	financial_session_create_lease_until = NULL,
	financial_session_create_lease_token = NULL,
	updated_at = $3
WHERE id = $1
  AND financial_session_create_lease_token = $2
RETURNING` + paymentAttemptColumns

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"clear financial session create lease",
		query,
		id,
		leaseToken,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: financial session create lease owner mismatch", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// ResolveAuthoritativeCancellationOnLocalCancel records a verified terminal
// FC canceled, failed, or expired outcome for a locally canceled session-create
// attempt while retaining its local canceled state. It marks FC outcome resolved.
func ResolveAuthoritativeCancellationOnLocalCancel[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	failureCode *string,
	canceledAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.ResolveAuthoritativeCancellationOnLocalCancel"

	const query = `
UPDATE payment_attempts
SET
	financial_outcome_resolved = true,
	failure_code = COALESCE($2, failure_code),
	canceled_at = COALESCE(canceled_at, $3),
	reconciliation_resolved_at = $4,
	updated_at = $4
WHERE id = $1
  AND state = $5
  AND NOT financial_outcome_resolved
  AND purpose = $6
  AND failure_code = $7
  AND submitted_at IS NULL
  AND (payment_url IS NULL OR btrim(payment_url) = '')
RETURNING` + paymentAttemptColumns

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"resolve authoritative cancellation on local cancel",
		query,
		id,
		failureCode,
		canceledAt,
		updatedAt,
		model.CanceledPaymentAttemptState,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		model.SessionCreationRetryExhaustedFailureCode,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt is not a financially unresolved local cancellation", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// ScheduleReconciliationRetry updates retry metadata for reconciliation_required attempts.
func ScheduleReconciliationRetry[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	nextReconciliationAt time.Time,
	lastError *string,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	next_reconciliation_at = $2,
	last_reconciliation_error = $3,
	updated_at = $4
WHERE id = $1
  AND state = $5
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"schedule payment attempt reconciliation retry",
		query,
		id,
		nextReconciliationAt,
		lastError,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
}

// ClearNextReconciliationAt clears automatic ClaimDue scheduling without changing
// state or inventing a financial outcome. Used after the synchronous
// CreateCheckoutPayment budget is exhausted for session-creation uncertainty.
func ClearNextReconciliationAt[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	next_reconciliation_at = NULL,
	updated_at = $2
WHERE id = $1
  AND state = $3
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"clear payment attempt next reconciliation at",
		query,
		id,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
}

// MarkManualReviewRequired stops automatic reconciliation for an attempt that
// exhausted its retry budget or cannot produce a valid reconciliation request.
func MarkManualReviewRequired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	lastError string,
	requiredAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const query = `
UPDATE payment_attempts
SET
	state = $2,
	last_reconciliation_error = $3,
	manual_review_required_at = $4,
	next_reconciliation_at = NULL,
	updated_at = $5
WHERE id = $1
  AND state = $6
RETURNING` + paymentAttemptColumns
	const op = "paymentattemptrp.MarkManualReviewRequired"

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt manual review required",
		query,
		id,
		model.ManualReviewRequiredPaymentAttemptState,
		lastError,
		requiredAt,
		updatedAt,
		model.ReconciliationRequiredPaymentAttemptState,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt is not reconciliation required", op),
			map[string]any{"payment_attempt_id": id},
		)
	}

	return row, err
}

// ClaimCandidatesForSessionCreationExpiration locks installment repayment
// PaymentAttempts whose local CreateCheckoutPayment window and post-lease
// safety interval have both elapsed. Selection only; callers must revalidate
// with EligibleForSessionCreationExpiration and ValidateTransitionTo before
// MarkExpired. Never contacts Financial Core.
func ClaimCandidatesForSessionCreationExpiration[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	safetyAfterLease time.Duration,
	limit int,
) ([]model.PaymentAttempt, error) {
	sessionDeadline := now.Add(-model.InstallmentRepaymentPaymentSessionTTL)
	leaseSafetyCutoff := now.Add(-safetyAfterLease)

	const query = `
WITH picked AS (
	SELECT id
	FROM payment_attempts
	WHERE purpose = $1
	  AND NOT financial_outcome_resolved
	  AND (financial_payment_session_id IS NULL OR btrim(financial_payment_session_id) = '')
	  AND created_at <= $2
	  AND (
		financial_session_create_lease_until IS NULL
		OR financial_session_create_lease_until <= $3
	  )
	  AND (
		state = $4
		OR (
			state = $5
			AND reconciliation_reason IN ($6, $7)
		)
	  )
	ORDER BY created_at ASC, id ASC
	LIMIT $8
	FOR UPDATE SKIP LOCKED
)
SELECT` + paymentAttemptColumnsP + `
FROM payment_attempts AS p
INNER JOIN picked ON picked.id = p.id
ORDER BY p.created_at ASC, p.id ASC
`

	return postgres.QueryAll[model.PaymentAttempt](
		ctx,
		q,
		"claim payment attempts for session creation expiration",
		query,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		sessionDeadline,
		leaseSafetyCutoff,
		model.CreatedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		model.PaymentSessionCreationTimeoutPaymentReconciliationReason,
		model.FinancialCoreStatusStillUnknownPaymentReconciliationReason,
		limit,
	)
}

// MarkExpired transitions an eligible attempt to expired. financial_outcome_resolved
// stays false. Clears scheduling/lease metadata so automatic FC recovery stops.
func MarkExpired[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PaymentAttemptID,
	expectedState model.PaymentAttemptState,
	expiredAt time.Time,
	updatedAt time.Time,
) (model.PaymentAttempt, error) {
	const op = "paymentattemptrp.MarkExpired"

	const query = `
UPDATE payment_attempts
SET
	state = $2,
	expired_at = $3,
	next_reconciliation_at = NULL,
	financial_session_create_lease_until = NULL,
	financial_session_create_lease_token = NULL,
	updated_at = $4
WHERE id = $1
  AND state = $5
  AND NOT financial_outcome_resolved
  AND (financial_payment_session_id IS NULL OR btrim(financial_payment_session_id) = '')
RETURNING` + paymentAttemptColumns

	row, err := postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt expired",
		query,
		id,
		model.ExpiredPaymentAttemptState,
		expiredAt,
		updatedAt,
		expectedState,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PaymentAttempt{}, cerr.Conflict(
			fmt.Errorf("%s: payment attempt is not eligible for local session creation expiration", op),
			map[string]any{"payment_attempt_id": id, "expected_state": expectedState},
		)
	}

	return row, err
}

// MarkSubmittedFromExpired records a still-usable recovered FC session after local
// expiration without reopening CreateCheckoutPayment for a new customer attempt.
func MarkSubmittedFromExpired[Q postgres.Queryer](
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
	financial_session_create_lease_until = NULL,
	financial_session_create_lease_token = NULL,
	submitted_at = $7,
	updated_at = $8
WHERE id = $1
  AND state = $9
  AND NOT financial_outcome_resolved
RETURNING` + paymentAttemptColumns

	return postgres.QueryOne[model.PaymentAttempt](
		ctx,
		q,
		"mark payment attempt submitted from expired",
		query,
		id,
		model.SubmittedPaymentAttemptState,
		financialPaymentIntentID,
		financialPaymentSessionID,
		paymentURL,
		financialPaymentExpiresAt,
		submittedAt,
		updatedAt,
		model.ExpiredPaymentAttemptState,
	)
}
