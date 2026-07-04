package paymentattemptrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// ClaimStaleCreatedForReconciliation transitions stale created attempts to reconciliation_required.
func ClaimStaleCreatedForReconciliation[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	cutoff time.Time,
	reason model.PaymentReconciliationReason,
	requestedAt time.Time,
	nextReconciliationAt time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.PaymentAttempt, error) {
	const query = `
WITH picked AS (
	SELECT id
	FROM payment_attempts
	WHERE state = $1
	  AND purpose = $2
	  AND created_at <= $3
	ORDER BY created_at ASC
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
		"claim stale created payment attempts for reconciliation",
		query,
		model.CreatedPaymentAttemptState,
		model.InstallmentRepaymentFinancialCorePaymentPurpose,
		cutoff,
		limit,
		model.ReconciliationRequiredPaymentAttemptState,
		reason,
		requestedAt,
		nextReconciliationAt,
		updatedAt,
	)
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
