package installmentchargerp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

var ErrInvalidClaimLimit = fmt.Errorf("invalid claim limit")

const installmentChargeColumns = `
	id,
	installment_id,
	charge_type,
	charge_status,
	amount_minor,
	currency,
	payer_type,
	recipient_type,
	policy_id,
	policy_version,
	assessed_at,
	due_at,
	paid_at,
	metadata,
	created_at,
	updated_at
`

// Create inserts an installment charge row and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	chargeType model.InstallmentChargeType,
	chargeStatus model.InstallmentChargeStatus,
	amountMinor uint64,
	currency model.Currency,
	payerType model.InstallmentChargePayerType,
	recipientType model.InstallmentChargeRecipientType,
	policyID string,
	policyVersion string,
	assessedAt time.Time,
	dueAt *time.Time,
	paidAt *time.Time,
	metadata json.RawMessage,
	createdAt time.Time,
	updatedAt time.Time,
) (model.InstallmentCharge, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"assessed_at": assessedAt,
		"created_at":  createdAt,
		"updated_at":  updatedAt,
	}); err != nil {
		return model.InstallmentCharge{}, err
	}

	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}

	const query = `
INSERT INTO installment_charges (
	installment_id,
	charge_type,
	charge_status,
	amount_minor,
	currency,
	payer_type,
	recipient_type,
	policy_id,
	policy_version,
	assessed_at,
	due_at,
	paid_at,
	metadata,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
RETURNING` + installmentChargeColumns

	return postgres.QueryOne[model.InstallmentCharge](
		ctx,
		q,
		"create installment charge",
		query,
		installmentID,
		chargeType,
		chargeStatus,
		amountMinor,
		currency,
		payerType,
		recipientType,
		policyID,
		policyVersion,
		assessedAt,
		dueAt,
		paidAt,
		metadata,
		createdAt,
		updatedAt,
	)
}

// ListByInstallmentIDs returns charges for the given installment ids.
func ListByInstallmentIDs[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentIDs []model.InstallmentID,
) ([]model.InstallmentCharge, error) {
	const op = "installmentchargerp.ListByInstallmentIDs"

	if len(installmentIDs) == 0 {
		return nil, nil
	}

	const query = `SELECT` + installmentChargeColumns + `
FROM installment_charges
WHERE installment_id = ANY($1)
ORDER BY installment_id ASC, created_at ASC
`

	rows, err := q.Query(ctx, query, installmentIDs)
	if err != nil {
		return nil, fmt.Errorf("%s: list installment charges by installment ids: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.InstallmentCharge](postgres.MustRows(rows))
}

// CancelDueOfferedChargesByAllocationExpiry cancels offered charges whose parent
// offered installment belongs to an expired checkout decision allocation.
//
// Charges on installments already attached to a contract are excluded.
func CancelDueOfferedChargesByAllocationExpiry[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	updatedAt time.Time,
) (int64, error) {
	const query = `
UPDATE installment_charges c
SET
	charge_status = $3,
	updated_at = $2
FROM installments i
INNER JOIN checkout_decision_allocations a ON i.decision_id = a.decision_id
WHERE c.installment_id = i.id
  AND c.charge_status = $4
  AND i.status = $5
  AND i.contract_id IS NULL
  AND a.expires_at <= $1
`
	const op = "installmentchargerp.CancelDueOfferedChargesByAllocationExpiry"

	affected, err := q.Exec(
		ctx,
		query,
		now,
		updatedAt,
		model.CanceledInstallmentChargeStatus,
		model.OfferedInstallmentChargeStatus,
		model.OfferedInstallmentStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: cancel due offered installment charges by allocation expiry: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// CancelOfferedByDecisionIDs cancels offered charges for the exact decision-allocation
// set claimed by the caller.
func CancelOfferedByDecisionIDs[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionIDs []model.DecisionID,
	updatedAt time.Time,
) (int64, error) {
	const op = "installmentchargerp.CancelOfferedByDecisionIDs"

	if len(decisionIDs) == 0 {
		return 0, nil
	}

	const query = `
UPDATE installment_charges c
SET
	charge_status = $2,
	updated_at = $3
FROM installments i
WHERE c.installment_id = i.id
  AND i.decision_id = ANY($1)
  AND c.charge_status = $4
  AND i.status = $5
  AND i.contract_id IS NULL
`

	affected, err := q.Exec(
		ctx,
		query,
		decisionIDs,
		model.CanceledInstallmentChargeStatus,
		updatedAt,
		model.OfferedInstallmentChargeStatus,
		model.OfferedInstallmentStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: cancel offered installment charges by decision ids: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// CancelDueOfferedByAllocationExpiry cancels offered charges for a bounded set
// of expired checkout decision allocations. Allocation rows are locked with
// SKIP LOCKED so concurrent workers claim disjoint offer batches.
func CancelDueOfferedByAllocationExpiry[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.InstallmentCharge, error) {
	if limit <= 0 {
		return nil, ErrInvalidClaimLimit
	}

	const query = `
WITH due_allocations AS (
	SELECT a.decision_id
	FROM checkout_decision_allocations a
	WHERE a.expires_at <= $1
	  AND EXISTS (
		SELECT 1
		FROM installments i
		WHERE i.decision_id = a.decision_id
		  AND i.status = $6
		  AND i.contract_id IS NULL
	  )
	ORDER BY a.decision_id
	FOR UPDATE SKIP LOCKED
	LIMIT $3
)
UPDATE installment_charges c
SET
	charge_status = $4,
	updated_at = $2
FROM installments i, due_allocations d
WHERE c.installment_id = i.id
  AND i.decision_id = d.decision_id
  AND c.charge_status = $5
  AND i.status = $6
  AND i.contract_id IS NULL
RETURNING c.*`

	return postgres.QueryAll[model.InstallmentCharge](
		ctx,
		q,
		"cancel due offered charges by allocation expiry",
		query,
		now,
		updatedAt,
		limit,
		model.CanceledInstallmentChargeStatus,
		model.OfferedInstallmentChargeStatus,
		model.OfferedInstallmentStatus,
	)
}

// CancelOfferedByContractID cancels offered charges for offered installments on a contract.
func CancelOfferedByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	updatedAt time.Time,
) (int64, error) {
	const query = `
UPDATE installment_charges c
SET
	charge_status = $3,
	updated_at = $2
FROM installments i
WHERE c.installment_id = i.id
  AND i.contract_id = $1
  AND c.charge_status = $4
  AND i.status = $5
`
	const op = "installmentchargerp.CancelOfferedByContractID"

	affected, err := q.Exec(
		ctx,
		query,
		contractID,
		updatedAt,
		model.CanceledInstallmentChargeStatus,
		model.OfferedInstallmentChargeStatus,
		model.OfferedInstallmentStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: cancel offered charges by contract id: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// MarkOfferedChargesAppliedByInstallmentIDs marks offered charges applied for installments.
func MarkOfferedChargesAppliedByInstallmentIDs[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentIDs []model.InstallmentID,
	updatedAt time.Time,
) (int64, error) {
	const op = "installmentchargerp.MarkOfferedChargesAppliedByInstallmentIDs"

	if len(installmentIDs) == 0 {
		return 0, nil
	}

	const query = `
UPDATE installment_charges
SET
	charge_status = $3,
	updated_at = $2
WHERE installment_id = ANY($1)
  AND charge_status = $4
`

	affected, err := q.Exec(
		ctx,
		query,
		installmentIDs,
		updatedAt,
		model.AppliedInstallmentChargeStatus,
		model.OfferedInstallmentChargeStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: mark offered installment charges applied: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// MarkChargeIDsApplied marks specific offered charge rows applied by id.
//
// It targets exact charge ids (not an entire installment) so first-installment credits and
// merchant/platform charges transition out of the offered state without being marked paid.
func MarkChargeIDsApplied[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	chargeIDs []model.InstallmentChargeID,
	updatedAt time.Time,
) (int64, error) {
	const op = "installmentchargerp.MarkChargeIDsApplied"

	if len(chargeIDs) == 0 {
		return 0, nil
	}

	const query = `
UPDATE installment_charges
SET
	charge_status = $2,
	updated_at = $3
WHERE id = ANY($1)
  AND charge_status = $4
`

	affected, err := q.Exec(
		ctx,
		query,
		chargeIDs,
		model.AppliedInstallmentChargeStatus,
		updatedAt,
		model.OfferedInstallmentChargeStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: mark charge ids applied: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// MarkSnapshottedChargeIDsPaid settles snapshotted customer components, including consumed discounts and waivers.
func MarkSnapshottedChargeIDsPaid[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	chargeIDs []model.InstallmentChargeID,
	paidAt time.Time,
	updatedAt time.Time,
) (int64, error) {
	const op = "installmentchargerp.MarkSnapshottedChargeIDsPaid"

	if len(chargeIDs) == 0 {
		return 0, nil
	}

	const query = `
UPDATE installment_charges
SET
	charge_status = $3,
	paid_at = $2,
	updated_at = $1
WHERE id = ANY($4)
  AND payer_type = $7
  AND charge_status IN ($5, $6)
`

	affected, err := q.Exec(
		ctx,
		query,
		updatedAt,
		paidAt,
		model.PaidInstallmentChargeStatus,
		chargeIDs,
		model.OfferedInstallmentChargeStatus,
		model.AppliedInstallmentChargeStatus,
		model.CustomerInstallmentChargePayerType,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: mark snapshotted charge ids paid: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// CountUnpaidCustomerDebitChargesByContractID counts unpaid customer debit charges on contract installments.
func CountUnpaidCustomerDebitChargesByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
) (int64, error) {
	const query = `
SELECT COUNT(*) AS count
FROM installment_charges ic
INNER JOIN installments i ON i.id = ic.installment_id
WHERE i.contract_id = $1
  AND ic.payer_type = $2
  AND ic.charge_type NOT IN ($3, $4)
  AND ic.charge_status IN ($5, $6)
`

	type result struct {
		Count int64 `db:"count"`
	}

	value, err := postgres.QueryOne[result](
		ctx,
		q,
		"count unpaid customer debit charges by contract id",
		query,
		contractID,
		model.CustomerInstallmentChargePayerType,
		model.DiscountInstallmentChargeType,
		model.WaiverInstallmentChargeType,
		model.OfferedInstallmentChargeStatus,
		model.AppliedInstallmentChargeStatus,
	)
	if err != nil {
		return 0, err
	}

	return value.Count, nil
}

// LockByIDs returns and locks installment charge rows by id.
func LockByIDs[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	ids []model.InstallmentChargeID,
) ([]model.InstallmentCharge, error) {
	const op = "installmentchargerp.LockByIDs"

	if len(ids) == 0 {
		return nil, nil
	}

	const query = `SELECT` + installmentChargeColumns + `
FROM installment_charges
WHERE id = ANY($1)
FOR UPDATE
`

	rows, err := q.Query(ctx, query, ids)
	if err != nil {
		return nil, fmt.Errorf("%s: lock installment charges by ids: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.InstallmentCharge](postgres.MustRows(rows))
}

func LockByInstallmentIDs[Q postgres.Queryer](ctx context.Context, q Q, ids []model.InstallmentID) ([]model.InstallmentCharge, error) {
	return postgres.QueryAll[model.InstallmentCharge](ctx, q, "lock installment charges", `SELECT`+installmentChargeColumns+` FROM installment_charges WHERE installment_id=ANY($1::uuid[]) ORDER BY id FOR UPDATE`, ids)
}
