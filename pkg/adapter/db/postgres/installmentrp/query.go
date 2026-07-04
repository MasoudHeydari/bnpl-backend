package installmentrp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const installmentColumns = `
	id,
	decision_id,
	contract_id,
	customer_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	installment_order,
	currency,
	principal_amount_minor,
	upfront_amount_minor,
	base_fee_amount_minor,
	total_amount_minor,
	due_at,
	grace_period_days,
	status,
	paid_amount_minor,
	paid_at,
	created_at,
	updated_at
`

// Create inserts an installment row and returns the persisted row.
func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
	contractID *model.ContractID,
	customerID model.CustomerID,
	merchantProfileID model.MerchantProfileID,
	merchantBranchProfileID *model.MerchantBranchProfileID,
	installmentOrder int,
	currency model.Currency,
	principalAmountMinor uint64,
	upfrontAmountMinor uint64,
	baseFeeAmountMinor uint64,
	totalAmountMinor uint64,
	dueAt time.Time,
	gracePeriodDays uint,
	status model.InstallmentStatus,
	paidAmountMinor uint64,
	paidAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.Installment, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{
		"due_at":     dueAt,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.Installment{}, err
	}

	const query = `
INSERT INTO installments (
	decision_id,
	contract_id,
	customer_id,
	merchant_profile_id,
	merchant_branch_profile_id,
	installment_order,
	currency,
	principal_amount_minor,
	upfront_amount_minor,
	base_fee_amount_minor,
	total_amount_minor,
	due_at,
	grace_period_days,
	status,
	paid_amount_minor,
	paid_at,
	created_at,
	updated_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
RETURNING` + installmentColumns

	return postgres.QueryOne[model.Installment](
		ctx,
		q,
		"create installment",
		query,
		decisionID,
		contractID,
		customerID,
		merchantProfileID,
		merchantBranchProfileID,
		installmentOrder,
		currency,
		principalAmountMinor,
		upfrontAmountMinor,
		baseFeeAmountMinor,
		totalAmountMinor,
		dueAt,
		gracePeriodDays,
		status,
		paidAmountMinor,
		paidAt,
		createdAt,
		updatedAt,
	)
}

// GetByID returns an installment by id.
func GetByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.InstallmentID,
) (model.Installment, error) {
	const query = `SELECT` + installmentColumns + `
FROM installments
WHERE id = $1
`

	return postgres.QueryOne[model.Installment](
		ctx,
		q,
		"get installment by id",
		query,
		id,
	)
}

// ListByContractID returns installments for a contract ordered by installment_order.
func ListByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
) ([]model.Installment, error) {
	const query = `SELECT` + installmentColumns + `
FROM installments
WHERE contract_id = $1
ORDER BY installment_order ASC
`
	const op = "installmentrp.ListByContractID"

	rows, err := q.Query(ctx, query, contractID)
	if err != nil {
		return nil, fmt.Errorf("%s: list installments by contract id: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Installment](postgres.MustRows(rows))
}

// LockByContractID returns and locks all installments attached to a contract.
func LockByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
) ([]model.Installment, error) {
	const query = `SELECT` + installmentColumns + `
FROM installments
WHERE contract_id = $1
ORDER BY installment_order ASC
FOR UPDATE
`
	const op = "installmentrp.LockByContractID"

	rows, err := q.Query(ctx, query, contractID)
	if err != nil {
		return nil, fmt.Errorf("%s: lock installments by contract id: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Installment](postgres.MustRows(rows))
}

// ListUnpaidByCustomerID returns unpaid contract installments for a
// customer profile using keyset pagination ordered by due_at ASC, id ASC.
//
// Unpaid means status in scheduled, due, or overdue. Pre-contract offers
// (contract_id IS NULL) are excluded.
func ListUnpaidByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	page pagination.KeysetRequest,
) ([]model.Installment, error) {
	page.Order = pagination.OrderAsc
	page.Normalize()
	if err := page.Validate(); err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	args := []any{
		customerID,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		model.ActiveContractStatus,
	}
	conditions := []string{
		"customer_id = $1",
		"contract_id IS NOT NULL",
		"status IN ($2, $3, $4)",
		"EXISTS (SELECT 1 FROM contracts c WHERE c.id = installments.contract_id AND c.status = $5)",
	}

	keysetWhere, keysetArgs, err := postgres.KeysetWhere("due_at", "id", page, len(args)+1)
	if err != nil {
		return nil, cerr.InvalidArgument(err)
	}

	if keysetWhere != "" {
		conditions = append(conditions, keysetWhere)
		args = append(args, keysetArgs...)
	}

	limitSQL, limitArgs := postgres.Limit(page.Limit, len(args)+1)
	args = append(args, limitArgs...)

	query := `SELECT` + installmentColumns + `
FROM installments
WHERE ` + strings.Join(conditions, `
  AND `) + `
ORDER BY due_at ASC, id ASC
` + limitSQL

	return postgres.QueryAll[model.Installment](
		ctx,
		q,
		"list unpaid installments by customer profile id",
		query,
		args...,
	)
}

// ListNextPayablePerContractByCustomerID returns the earliest unpaid installment
// per active contract for the customer (installment_order ASC).
func ListNextPayablePerContractByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
) ([]model.Installment, error) {
	const op = "installmentrp.ListNextPayablePerContractByCustomerID"

	if customerID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `
SELECT` + installmentColumns + `
FROM (
	SELECT DISTINCT ON (contract_id)` + installmentColumns + `
	FROM installments
	WHERE customer_id = $1
	  AND contract_id IS NOT NULL
	  AND status IN ($2, $3, $4)
	  AND EXISTS (
		SELECT 1
		FROM contracts c
		WHERE c.id = installments.contract_id
		  AND c.status = $5
	  )
	ORDER BY contract_id ASC, installment_order ASC, id ASC
) AS next_per_contract
ORDER BY due_at ASC, id ASC
`

	return postgres.QueryAll[model.Installment](
		ctx,
		q,
		"list next payable installments per contract by customer id",
		query,
		customerID,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		model.ActiveContractStatus,
	)
}

// ListDueUnpaidByCustomerID returns unpaid installments that are contractually
// due as of asOf (due_at <= asOf or status due/overdue).
func ListDueUnpaidByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
	asOf time.Time,
) ([]model.Installment, error) {
	const op = "installmentrp.ListDueUnpaidByCustomerID"

	if customerID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `
SELECT` + installmentColumns + `
FROM installments
WHERE customer_id = $1
  AND contract_id IS NOT NULL
  AND status IN ($2, $3, $4)
  AND (
	due_at <= $5
	OR status IN ($3, $4)
  )
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = installments.contract_id
	  AND c.status = $6
  )
ORDER BY due_at ASC, id ASC
`

	return postgres.QueryAll[model.Installment](
		ctx,
		q,
		"list due unpaid installments by customer id",
		query,
		customerID,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		asOf.UTC(),
		model.ActiveContractStatus,
	)
}

// CountUnpaidByCustomerID counts unpaid active-contract installments.
func CountUnpaidByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
) (int, error) {
	const op = "installmentrp.CountUnpaidByCustomerID"

	if customerID.IsZero() {
		return 0, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `
SELECT COUNT(*)::int AS count
FROM installments
WHERE customer_id = $1
  AND contract_id IS NOT NULL
  AND status IN ($2, $3, $4)
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = installments.contract_id
	  AND c.status = $5
  )
`

	type countRow struct {
		Count int `db:"count"`
	}

	row, err := postgres.QueryOne[countRow](
		ctx,
		q,
		"count unpaid installments by customer id",
		query,
		customerID,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		model.ActiveContractStatus,
	)
	if err != nil {
		return 0, err
	}

	return row.Count, nil
}

type collectionsObligationRow struct {
	HasUnresolvedCancellation bool                           `db:"has_unresolved_cancellation"`
	ID                        model.InstallmentID            `db:"id"`
	DecisionID                model.DecisionID               `db:"decision_id"`
	ContractID                *model.ContractID              `db:"contract_id"`
	CustomerID                model.CustomerID               `db:"customer_id"`
	MerchantProfileID         model.MerchantProfileID        `db:"merchant_profile_id"`
	MerchantBranchProfileID   *model.MerchantBranchProfileID `db:"merchant_branch_profile_id"`
	InstallmentOrder          int                            `db:"installment_order"`
	Currency                  model.Currency                 `db:"currency"`
	PrincipalAmountMinor      uint64                         `db:"principal_amount_minor"`
	UpfrontAmountMinor        uint64                         `db:"upfront_amount_minor"`
	BaseFeeAmountMinor        uint64                         `db:"base_fee_amount_minor"`
	TotalAmountMinor          uint64                         `db:"total_amount_minor"`
	DueAt                     time.Time                      `db:"due_at"`
	GracePeriodDays           uint                           `db:"grace_period_days"`
	Status                    model.InstallmentStatus        `db:"status"`
	PaidAmountMinor           uint64                         `db:"paid_amount_minor"`
	PaidAt                    *time.Time                     `db:"paid_at"`
	CreatedAt                 time.Time                      `db:"created_at"`
	UpdatedAt                 time.Time                      `db:"updated_at"`
	HasUnresolvedPayment      bool                           `db:"has_unresolved_payment"`
}

// ListCollectionsObligationsByCustomerID returns economically outstanding debt,
// including unresolved cancellations; inconsistent paid rows belong to the integrity audit.
func ListCollectionsObligationsByCustomerID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	customerID model.CustomerID,
) ([]repo.CollectionsObligation, error) {
	const op = "installmentrp.ListCollectionsObligationsByCustomerID"

	if customerID.IsZero() {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer id is required", op))
	}

	const query = `
SELECT
	EXISTS(SELECT 1 FROM contracts pending_cancel WHERE pending_cancel.id=i.contract_id AND pending_cancel.status='canceling') AS has_unresolved_cancellation,
	i.id,
	i.decision_id,
	i.contract_id,
	i.customer_id,
	i.merchant_profile_id,
	i.merchant_branch_profile_id,
	i.installment_order,
	i.currency,
	i.principal_amount_minor,
	i.upfront_amount_minor,
	i.base_fee_amount_minor,
	i.total_amount_minor,
	i.due_at,
	i.grace_period_days,
	i.status,
	i.paid_amount_minor,
	i.paid_at,
	i.created_at,
	i.updated_at,
	EXISTS (
		SELECT 1
		FROM payment_attempts pa
		WHERE pa.installment_id = i.id
		  AND (pa.state IN ($5, $6, $7, 'manual_review_required') OR (pa.state IN ('failed','canceled') AND NOT pa.financial_outcome_resolved))
	) AS has_unresolved_payment
FROM installments i
WHERE i.customer_id = $1
  AND i.contract_id IS NOT NULL
  AND i.status IN ($2, $3, $4)
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = i.contract_id
	  AND c.status IN ($8, 'canceling')
  )
ORDER BY i.due_at ASC, i.id ASC
`

	rows, err := postgres.QueryAll[collectionsObligationRow](
		ctx,
		q,
		"list collections obligations by customer id",
		query,
		customerID,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		model.CreatedPaymentAttemptState,
		model.SubmittedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		model.ActiveContractStatus,
	)
	if err != nil {
		return nil, err
	}

	ids := make([]model.InstallmentID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	charges, err := installmentchargerp.ListByInstallmentIDs(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	chargesByID := map[model.InstallmentID][]model.InstallmentCharge{}
	for _, ch := range charges {
		chargesByID[ch.InstallmentID] = append(chargesByID[ch.InstallmentID], ch)
	}

	out := make([]repo.CollectionsObligation, 0, len(rows))
	for _, row := range rows {
		out = append(out, repo.CollectionsObligation{
			Installment: model.Installment{
				ID:                      row.ID,
				DecisionID:              row.DecisionID,
				ContractID:              row.ContractID,
				CustomerID:              row.CustomerID,
				MerchantProfileID:       row.MerchantProfileID,
				MerchantBranchProfileID: row.MerchantBranchProfileID,
				InstallmentOrder:        row.InstallmentOrder,
				Currency:                row.Currency,
				PrincipalAmountMinor:    row.PrincipalAmountMinor,
				UpfrontAmountMinor:      row.UpfrontAmountMinor,
				BaseFeeAmountMinor:      row.BaseFeeAmountMinor,
				TotalAmountMinor:        row.TotalAmountMinor,
				DueAt:                   row.DueAt,
				GracePeriodDays:         row.GracePeriodDays,
				Status:                  row.Status,
				PaidAmountMinor:         row.PaidAmountMinor,
				PaidAt:                  row.PaidAt,
				CreatedAt:               row.CreatedAt,
				UpdatedAt:               row.UpdatedAt,
			},
			HasUnresolvedPayment:      row.HasUnresolvedPayment,
			HasUnresolvedCancellation: row.HasUnresolvedCancellation,
		})
	}

	for n := range out {
		out[n].HasUnresolvedAdjustment = model.ClassifyCustomerCash(out[n].Installment, chargesByID[out[n].Installment.ID]).Kind != model.PositiveCashObligation
	}

	return out, nil
}

// ListCollectionsAssessmentCustomerPage returns distinct customers needing
// collections assessment in deterministic keyset order.
func ListCollectionsAssessmentCustomerPage[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	asOf time.Time,
	afterCustomerID model.CustomerID,
	customerLimit int,
) ([]model.CustomerID, error) {
	const op = "installmentrp.ListCollectionsAssessmentCustomerPage"

	if customerLimit <= 0 {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer limit must be positive", op))
	}

	// Candidates include every due obligation for recovery, even inside grace.
	// The domain policy applies the persisted grace boundary for default entry.
	const query = `
SELECT customer_id
FROM (
	SELECT DISTINCT i.customer_id
	FROM installments i
	WHERE i.contract_id IS NOT NULL
	  AND i.status IN ($1, $2, $3)
	  AND i.due_at <= $4
	  AND EXISTS (
		SELECT 1
		FROM contracts c
		WHERE c.id = i.contract_id
		  AND c.status IN ($5, 'canceling')
	  )

	UNION

	SELECT de.customer_id
	FROM default_episodes de
	WHERE de.status = $6
) AS candidates
WHERE ($7::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR customer_id > $7)
ORDER BY customer_id ASC
LIMIT $8
`

	type customerRow struct {
		CustomerID model.CustomerID `db:"customer_id"`
	}

	rows, err := postgres.QueryAll[customerRow](
		ctx,
		q,
		"list collections assessment customer page",
		query,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		asOf.UTC(),
		model.ActiveContractStatus,
		model.ActiveDefaultEpisodeStatus,
		afterCustomerID,
		customerLimit,
	)
	if err != nil {
		return nil, err
	}

	out := make([]model.CustomerID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.CustomerID)
	}

	return out, nil
}

// LockByID returns and locks an installment row for update.
func LockByID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
) (model.Installment, error) {
	const query = `SELECT` + installmentColumns + `
FROM installments
WHERE id = $1
FOR UPDATE
`

	return postgres.QueryOne[model.Installment](
		ctx,
		q,
		"lock installment by id",
		query,
		installmentID,
	)
}

// ListByDecisionID returns installments for a decision.
func ListByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) ([]model.Installment, error) {
	const query = `SELECT` + installmentColumns + `
FROM installments
WHERE decision_id = $1
ORDER BY installment_order ASC
`
	const op = "installmentrp.ListByDecisionID"

	rows, err := q.Query(ctx, query, decisionID)
	if err != nil {
		return nil, fmt.Errorf("%s: list installments by decision id: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Installment](postgres.MustRows(rows))
}

// LockByDecisionID returns and locks installments for a decision ordered by installment_order.
func LockByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
) ([]model.Installment, error) {
	const query = `SELECT` + installmentColumns + `
FROM installments
WHERE decision_id = $1
ORDER BY installment_order ASC
FOR UPDATE
`
	const op = "installmentrp.LockByDecisionID"

	rows, err := q.Query(ctx, query, decisionID)
	if err != nil {
		return nil, fmt.Errorf("%s: lock installments by decision id: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Installment](postgres.MustRows(rows))
}

// AttachContractToOfferedByDecisionID sets contract_id on offered installments for a decision.
func AttachContractToOfferedByDecisionID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionID model.DecisionID,
	contractID model.ContractID,
	updatedAt time.Time,
) (int64, error) {
	const query = `
UPDATE installments
SET
	contract_id = $3,
	updated_at = $2
WHERE decision_id = $1
  AND status = $4
`
	const op = "installmentrp.AttachContractToOfferedByDecisionID"

	affected, err := q.Exec(
		ctx,
		query,
		decisionID,
		updatedAt,
		contractID,
		model.OfferedInstallmentStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: attach contract to offered installments: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

// Database invariant: installment_complete_settlement requires every customer
// component finalized before this statement; paid is terminal.
// MarkPaid marks an installment paid.
func MarkPaid[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	contractID model.ContractID,
	paidAmountMinor uint64,
	paidAt time.Time,
	updatedAt time.Time,
) (model.Installment, error) {
	const query = `
UPDATE installments
SET
	status = $7,
	paid_amount_minor = $8,
	paid_at = $9,
	updated_at = $10
WHERE id = $1
  AND contract_id = $2
  AND status IN ($3, $4, $5, $6)
RETURNING` + installmentColumns

	return postgres.QueryOne[model.Installment](
		ctx,
		q,
		"mark installment paid",
		query,
		installmentID,
		contractID,
		model.OfferedInstallmentStatus,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		model.PaidInstallmentStatus,
		paidAmountMinor,
		paidAt,
		updatedAt,
	)
}

// UpdateOfferedDueAt finalizes an offered installment due date before activation.
func UpdateOfferedDueAt[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	dueAt time.Time,
	updatedAt time.Time,
) (model.Installment, error) {
	if err := postgres.RequiredTimes(map[string]time.Time{"due_at": dueAt, "updated_at": updatedAt}); err != nil {
		return model.Installment{}, err
	}

	const query = `
UPDATE installments
SET due_at = $3, updated_at = $4
WHERE id = $1
  AND status = $2
RETURNING` + installmentColumns

	return postgres.QueryOne[model.Installment](
		ctx,
		q,
		"update offered installment due at",
		query,
		installmentID,
		model.OfferedInstallmentStatus,
		dueAt,
		updatedAt,
	)
}

// MarkScheduledOrDue transitions an offered installment to scheduled or due.
func MarkScheduledOrDue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	contractID model.ContractID,
	nextStatus model.InstallmentStatus,
	updatedAt time.Time,
) (model.Installment, error) {
	const query = `
UPDATE installments
SET
	status = $4,
	updated_at = $5
WHERE id = $1
  AND contract_id = $2
  AND status = $3
RETURNING` + installmentColumns

	return postgres.QueryOne[model.Installment](
		ctx,
		q,
		"mark installment scheduled or due",
		query,
		installmentID,
		contractID,
		model.OfferedInstallmentStatus,
		nextStatus,
		updatedAt,
	)
}

// ExpireDueOfferedByAllocationExpiry expires offered installments for the same
// bounded allocation batch claimed by the charge-cancellation step in the
// current transaction. Allocation-level claiming keeps each offer atomic.
func ExpireDueOfferedByAllocationExpiry[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	updatedAt time.Time,
	limit int,
) ([]model.Installment, error) {
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
		FROM installments offered
		WHERE offered.decision_id = a.decision_id
		  AND offered.status = $5
		  AND offered.contract_id IS NULL
	  )
	ORDER BY a.decision_id
	FOR UPDATE SKIP LOCKED
	LIMIT $3
)
UPDATE installments i
SET
	status = $4,
	updated_at = $2
FROM due_allocations d
WHERE i.decision_id = d.decision_id
  AND i.status = $5
  AND i.contract_id IS NULL
RETURNING i.*`

	return postgres.QueryAll[model.Installment](
		ctx,
		q,
		"expire due offered installments by allocation expiry",
		query,
		now,
		updatedAt,
		limit,
		model.ExpiredInstallmentStatus,
		model.OfferedInstallmentStatus,
	)
}

// ExpireOfferedByDecisionIDs expires unattached offered installments for the
// exact decision-allocation set claimed by the caller.
func ExpireOfferedByDecisionIDs[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	decisionIDs []model.DecisionID,
	updatedAt time.Time,
) (int64, error) {
	const op = "installmentrp.ExpireOfferedByDecisionIDs"

	if len(decisionIDs) == 0 {
		return 0, nil
	}

	const query = `
UPDATE installments
SET
	status = $2,
	updated_at = $3
WHERE decision_id = ANY($1)
  AND status = $4
  AND contract_id IS NULL
`

	affected, err := q.Exec(
		ctx,
		query,
		decisionIDs,
		model.ExpiredInstallmentStatus,
		updatedAt,
		model.OfferedInstallmentStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: expire offered installments by decision ids: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

var (
	// ErrInvalidClaimLimit indicates an invalid batch claim limit.
	ErrInvalidClaimLimit = fmt.Errorf("invalid claim limit")
)

// ClaimScheduledDue locks scheduled installments whose due_at has passed.
func ClaimScheduledDue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	limit int,
) ([]model.Installment, error) {
	const op = "installmentrp.ClaimScheduledDue"

	if limit <= 0 {
		return nil, cerr.InvalidArgument(ErrInvalidClaimLimit, map[string]any{"limit": limit})
	}

	const query = `
SELECT` + installmentColumns + `
FROM installments
WHERE status = $1
  AND due_at <= $2
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = installments.contract_id
	  AND c.status = $4
  )
ORDER BY due_at ASC, created_at ASC
LIMIT $3
FOR UPDATE SKIP LOCKED
`

	rows, err := q.Query(
		ctx,
		query,
		model.ScheduledInstallmentStatus,
		now,
		limit,
		model.ActiveContractStatus,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: claim scheduled due installments: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Installment](postgres.MustRows(rows))
}

// MarkDue transitions a scheduled installment to due.
func MarkDue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	updatedAt time.Time,
) (model.Installment, error) {
	const query = `
UPDATE installments
SET
	status = $2,
	updated_at = $3
WHERE id = $1
  AND status = $4
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = installments.contract_id
	  AND c.status = $5
  )
RETURNING` + installmentColumns
	const op = "installmentrp.MarkDue"

	row, err := postgres.QueryOne[model.Installment](
		ctx,
		q,
		"mark installment due",
		query,
		installmentID,
		model.DueInstallmentStatus,
		updatedAt,
		model.ScheduledInstallmentStatus,
		model.ActiveContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Installment{}, cerr.Conflict(
				fmt.Errorf("%s: installment is no longer eligible to become due", op),
				map[string]any{"installment_id": installmentID},
			)
		}

		return model.Installment{}, err
	}

	return row, nil
}

// ClaimDueOverdueEligible locks due installments whose grace period has elapsed.
func ClaimDueOverdueEligible[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	now time.Time,
	limit int,
) ([]model.Installment, error) {
	const op = "installmentrp.ClaimDueOverdueEligible"

	if limit <= 0 {
		return nil, cerr.InvalidArgument(ErrInvalidClaimLimit, map[string]any{"limit": limit})
	}

	const query = `
SELECT` + installmentColumns + `
FROM installments
WHERE status = $1
  AND (((due_at AT TIME ZONE current_setting('TimeZone'))::date + grace_period_days + 1)::timestamp AT TIME ZONE current_setting('TimeZone')) <= $2
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = installments.contract_id
	  AND c.status = $4
  )
ORDER BY due_at ASC, created_at ASC
LIMIT $3
FOR UPDATE SKIP LOCKED
`

	rows, err := q.Query(
		ctx,
		query,
		model.DueInstallmentStatus,
		now,
		limit,
		model.ActiveContractStatus,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: claim due overdue eligible installments: %w", op, postgres.MapError(err))
	}

	defer rows.Close()

	return postgres.ValuesInto[model.Installment](postgres.MustRows(rows))
}

// MarkOverdue transitions a due installment to overdue.
func MarkOverdue[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	installmentID model.InstallmentID,
	updatedAt time.Time,
) (model.Installment, error) {
	const query = `
UPDATE installments
SET
	status = $2,
	updated_at = $3
WHERE id = $1
  AND status = $4
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = installments.contract_id
	  AND c.status = $5
  )
RETURNING` + installmentColumns
	const op = "installmentrp.MarkOverdue"

	row, err := postgres.QueryOne[model.Installment](
		ctx,
		q,
		"mark installment overdue",
		query,
		installmentID,
		model.OverdueInstallmentStatus,
		updatedAt,
		model.DueInstallmentStatus,
		model.ActiveContractStatus,
	)
	if err != nil {
		if cerr.IsCode(err, cerr.CodeResourceNotFound) {
			return model.Installment{}, cerr.Conflict(
				fmt.Errorf("%s: installment is no longer eligible to become overdue", op),
				map[string]any{"installment_id": installmentID},
			)
		}

		return model.Installment{}, err
	}

	return row, nil
}

// ExpireOfferedByContractID marks offered installments expired for a contract.
func ExpireOfferedByContractID[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	contractID model.ContractID,
	updatedAt time.Time,
) (int64, error) {
	const query = `
UPDATE installments
SET
	status = $3,
	updated_at = $2
WHERE contract_id = $1
  AND status = $4
`
	const op = "installmentrp.ExpireOfferedByContractID"

	affected, err := q.Exec(
		ctx,
		query,
		contractID,
		updatedAt,
		model.ExpiredInstallmentStatus,
		model.OfferedInstallmentStatus,
	)
	if err != nil {
		return 0, fmt.Errorf("%s: expire offered installments by contract id: %w", op, postgres.MapError(err))
	}

	return affected, nil
}

type reminderCandidateRow struct {
	ID                      model.InstallmentID            `db:"id"`
	DecisionID              model.DecisionID               `db:"decision_id"`
	ContractID              *model.ContractID              `db:"contract_id"`
	CustomerID              model.CustomerID               `db:"customer_id"`
	MerchantProfileID       model.MerchantProfileID        `db:"merchant_profile_id"`
	MerchantBranchProfileID *model.MerchantBranchProfileID `db:"merchant_branch_profile_id"`
	InstallmentOrder        int                            `db:"installment_order"`
	Currency                model.Currency                 `db:"currency"`
	PrincipalAmountMinor    uint64                         `db:"principal_amount_minor"`
	UpfrontAmountMinor      uint64                         `db:"upfront_amount_minor"`
	BaseFeeAmountMinor      uint64                         `db:"base_fee_amount_minor"`
	TotalAmountMinor        uint64                         `db:"total_amount_minor"`
	DueAt                   time.Time                      `db:"due_at"`
	GracePeriodDays         uint                           `db:"grace_period_days"`
	Status                  model.InstallmentStatus        `db:"status"`
	PaidAmountMinor         uint64                         `db:"paid_amount_minor"`
	PaidAt                  *time.Time                     `db:"paid_at"`
	CreatedAt               time.Time                      `db:"created_at"`
	UpdatedAt               time.Time                      `db:"updated_at"`
	ExternalCustomerID      model.ExternalCustomerID       `db:"external_customer_id"`
	HasUnresolvedPayment    bool                           `db:"has_unresolved_payment"`
}

// ListReminderCandidatePage returns all eligible reminder installments for the
// next customerLimit customers after afterCustomerID (exclusive keyset).
func ListReminderCandidatePage[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	dueAtFrom time.Time,
	dueAtTo time.Time,
	businessDate time.Time,
	afterCustomerID model.CustomerID,
	customerLimit int,
) ([]repo.ReminderCandidate, error) {
	const op = "installmentrp.ListReminderCandidatePage"

	if customerLimit <= 0 {
		return nil, cerr.InternalError(fmt.Errorf("%s: customer limit must be positive", op))
	}

	if !dueAtTo.After(dueAtFrom) {
		return nil, cerr.InternalError(fmt.Errorf("%s: due_at window is empty", op))
	}

	// Page by distinct customers so a customer's full candidate set is never
	// truncated mid-group. Exclude customers already reminded for businessDate
	// so repeated runs advance past processed customer-days. Longer persisted
	// grace snapshots extend the operational lookback instead of being shortened.
	const query = `
WITH page_customers AS (
	SELECT DISTINCT i.customer_id
	FROM installments i
	WHERE i.contract_id IS NOT NULL
	  AND i.status IN ($1, $2, $3)
	  AND (EXISTS (SELECT 1 FROM default_episodes de WHERE de.customer_id=i.customer_id AND de.status='active') OR i.due_at >= $8 OR (((i.due_at AT TIME ZONE current_setting('TimeZone'))::date + i.grace_period_days + 1)::timestamp AT TIME ZONE current_setting('TimeZone')) >= $8)
	  AND i.due_at < $9
	  AND ($10::uuid = '00000000-0000-0000-0000-000000000000'::uuid OR i.customer_id > $10)
	  AND EXISTS (
		SELECT 1
		FROM contracts c
		WHERE c.id = i.contract_id
		  AND (c.status = $4 OR (c.status='canceling' AND EXISTS(SELECT 1 FROM default_episodes de WHERE de.customer_id=i.customer_id AND de.status='active')))
	  )
	  AND NOT EXISTS (
		SELECT 1
		FROM repayment_reminders rr
		WHERE rr.customer_id = i.customer_id
		  AND rr.business_date = $11::date
	  )
	ORDER BY i.customer_id ASC
	LIMIT $12
)
SELECT
	i.id,
	i.decision_id,
	i.contract_id,
	i.customer_id,
	i.merchant_profile_id,
	i.merchant_branch_profile_id,
	i.installment_order,
	i.currency,
	i.principal_amount_minor,
	i.upfront_amount_minor,
	i.base_fee_amount_minor,
	i.total_amount_minor,
	i.due_at,
	i.grace_period_days,
	i.status,
	i.paid_amount_minor,
	i.paid_at,
	i.created_at,
	i.updated_at,
	cp.external_customer_id,
	EXISTS (
		SELECT 1
		FROM payment_attempts pa
		WHERE pa.installment_id = i.id
		  AND (pa.state IN ($5, $6, $7, 'manual_review_required') OR (pa.state IN ('failed','canceled') AND NOT pa.financial_outcome_resolved))
	) AS has_unresolved_payment
FROM installments i
INNER JOIN page_customers pc ON pc.customer_id = i.customer_id
INNER JOIN customer_profiles cp ON cp.id = i.customer_id
WHERE i.contract_id IS NOT NULL
  AND i.status IN ($1, $2, $3)
  AND (EXISTS (SELECT 1 FROM default_episodes de WHERE de.customer_id=i.customer_id AND de.status='active') OR i.due_at >= $8 OR (((i.due_at AT TIME ZONE current_setting('TimeZone'))::date + i.grace_period_days + 1)::timestamp AT TIME ZONE current_setting('TimeZone')) >= $8)
  AND i.due_at < $9
  AND EXISTS (
	SELECT 1
	FROM contracts c
	WHERE c.id = i.contract_id
	  AND (c.status = $4 OR (c.status='canceling' AND EXISTS(SELECT 1 FROM default_episodes de WHERE de.customer_id=i.customer_id AND de.status='active')))
  )
ORDER BY i.customer_id ASC, i.due_at ASC, i.id ASC
`

	y, m, d := businessDate.Date()
	businessDateUTC := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)

	rows, err := postgres.QueryAll[reminderCandidateRow](
		ctx,
		q,
		"list repayment reminder candidate page",
		query,
		model.ScheduledInstallmentStatus,
		model.DueInstallmentStatus,
		model.OverdueInstallmentStatus,
		model.ActiveContractStatus,
		model.CreatedPaymentAttemptState,
		model.SubmittedPaymentAttemptState,
		model.ReconciliationRequiredPaymentAttemptState,
		dueAtFrom.UTC(),
		dueAtTo.UTC(),
		afterCustomerID,
		businessDateUTC,
		customerLimit,
	)
	if err != nil {
		return nil, err
	}

	out := make([]repo.ReminderCandidate, 0, len(rows))
	for _, row := range rows {
		out = append(out, repo.ReminderCandidate{
			Installment: model.Installment{
				ID:                      row.ID,
				DecisionID:              row.DecisionID,
				ContractID:              row.ContractID,
				CustomerID:              row.CustomerID,
				MerchantProfileID:       row.MerchantProfileID,
				MerchantBranchProfileID: row.MerchantBranchProfileID,
				InstallmentOrder:        row.InstallmentOrder,
				Currency:                row.Currency,
				PrincipalAmountMinor:    row.PrincipalAmountMinor,
				UpfrontAmountMinor:      row.UpfrontAmountMinor,
				BaseFeeAmountMinor:      row.BaseFeeAmountMinor,
				TotalAmountMinor:        row.TotalAmountMinor,
				DueAt:                   row.DueAt,
				GracePeriodDays:         row.GracePeriodDays,
				Status:                  row.Status,
				PaidAmountMinor:         row.PaidAmountMinor,
				PaidAt:                  row.PaidAt,
				CreatedAt:               row.CreatedAt,
				UpdatedAt:               row.UpdatedAt,
			},
			ExternalCustomerID:   row.ExternalCustomerID,
			HasUnresolvedPayment: row.HasUnresolvedPayment,
		})
	}

	return out, nil
}
