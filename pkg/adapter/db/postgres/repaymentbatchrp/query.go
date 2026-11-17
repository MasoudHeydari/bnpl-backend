package repaymentbatchrp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/repaymentselection"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const columns = `
	id, customer_id, external_customer_id, idempotency_key, snapshot, currency,
	total_amount_minor, status, financial_collection_payment_id, snapshot_revision,
next_reconciliation_at, last_reason_code, version, created_at, updated_at,
	start_idempotency_key, start_fingerprint
`

// GetByID loads the durable snapshot without advancing its lifecycle.
func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) (repo.RepaymentBatchRow, error) {
	const query = `SELECT` + columns + ` FROM repayment_batches WHERE id=$1`
	return postgres.QueryOne[repo.RepaymentBatchRow](ctx, q, "get repayment batch", query, id)
}

// GetByCustomerAndIdempotencyKey supports safe replay without generating a
// second payment collection for the same logical customer request.
func GetByCustomerAndIdempotencyKey[Q postgres.Queryer](
	ctx context.Context, q Q, customerID model.CustomerID, key string,
) (repo.RepaymentBatchRow, error) {
	const query = `SELECT` + columns + ` FROM repayment_batches
	WHERE customer_id=$1 AND idempotency_key=$2`
	return postgres.QueryOne[repo.RepaymentBatchRow](ctx, q, "get batch by idempotency key", query, customerID, key)
}

func ListReconciliationDue[Q postgres.Queryer](
	ctx context.Context, q Q, now time.Time, limit int,
) ([]repo.RepaymentBatchRow, error) {
	if limit < 1 {
		return nil, cerr.InvalidArgument(fmt.Errorf("reconciliation batch size must be positive"))
	}
	const query = `SELECT` + columns + ` FROM repayment_batches
	WHERE next_reconciliation_at <= $1
	AND status IN ('created','prepared','starting','requires_action','reconciling','accounting_pending')
	ORDER BY next_reconciliation_at,id LIMIT $2`
	return postgres.QueryAll[repo.RepaymentBatchRow](ctx, q, "list repayment batches due", query, now, limit)
}

// Create reserves all installment identities in the SAME local database tx
// that stores the snapshot. An overlapping payment attempt cannot reserve the
// same installment in another batch while these claims remain present.
func Create[Q postgres.Queryer](
	ctx context.Context, q Q, row repo.RepaymentBatchRow, installments []model.InstallmentID,
) (repo.RepaymentBatchRow, error) {
	if row.ID == uuid.Nil || row.CustomerID.IsZero() || row.ExternalCustomerID.IsZero() ||
		row.IdempotencyKey == "" || len(row.Snapshot) == 0 || row.TotalAmountMinor == 0 ||
		!row.Currency.IsValid() || row.Status != model.CreatedRepaymentBatchStatus ||
		len(installments) == 0 || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() {
		return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("invalid repayment batch creation"))
	}
	// Lock the customer before taking installment locks. This serializes
	// snapshot claim validation against other customer-scoped BNPL commands,
	// and ensures the final overdue sweep is checked under the same lock.
	type customerLock struct {
		ID model.CustomerID `db:"id"`
	}
	const lockCustomer = `SELECT id FROM customer_profiles WHERE id=$1 FOR UPDATE`
	if _, err := postgres.QueryOne[customerLock](
		ctx, q, "lock repayment customer", lockCustomer, row.CustomerID,
	); err != nil {
		return repo.RepaymentBatchRow{}, err
	}

	const query = `
	INSERT INTO repayment_batches(
		id,customer_id,external_customer_id,idempotency_key,snapshot,currency,
		total_amount_minor,status,next_reconciliation_at,created_at,updated_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
	RETURNING` + columns
	stored, err := postgres.QueryOne[repo.RepaymentBatchRow](
		ctx, q, "create repayment batch", query, row.ID, row.CustomerID, row.ExternalCustomerID,
		row.IdempotencyKey, row.Snapshot, row.Currency, row.TotalAmountMinor, row.Status,
		row.NextReconciliationAt, row.CreatedAt, row.UpdatedAt,
	)
	if err != nil {
		return repo.RepaymentBatchRow{}, err
	}
	// Revalidate the customer-selected snapshot against authoritative BNPL
	// installment and contract rows under locks. FC still verifies its own
	// receivables and registered distribution relationships independently.
	var plan collectionpayment.Plan
	if err := json.Unmarshal(row.Snapshot, &plan); err != nil {
		return repo.RepaymentBatchRow{}, err
	}
	if err := plan.Validate(); err != nil {
		return repo.RepaymentBatchRow{}, err
	}
	if plan.ID != row.ID || plan.CustomerID != row.CustomerID || plan.TotalMinor != row.TotalAmountMinor ||
		plan.Currency != row.Currency || len(plan.Items) != len(installments) {
		return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("batch snapshot identity or total mismatch"))
	}
	type lockedInstallment struct {
		ID                   model.InstallmentID      `db:"id"`
		PrincipalAmountMinor uint64                   `db:"principal_amount_minor"`
		ContractID           model.ContractID         `db:"contract_id"`
		CreditAccountID      model.CreditAccountID    `db:"credit_account_id"`
		CustomerID           model.CustomerID         `db:"customer_id"`
		ExternalMerchantID   model.ExternalMerchantID `db:"external_merchant_id"`
		CreditKind           model.CreditKind         `db:"credit_kind"`
	}
	const lock = `SELECT i.id,i.principal_amount_minor,i.contract_id,
	       c.credit_account_id,c.customer_id,m.external_merchant_id,a.credit_kind
	FROM installments i
	JOIN contracts c ON c.id=i.contract_id
	JOIN merchant_profiles m ON m.id=c.merchant_profile_id
	JOIN credit_accounts a ON a.id=c.credit_account_id
	WHERE i.id=$1 AND i.status IN ('scheduled','due','overdue') AND c.status='active'
	FOR UPDATE OF i,c,a`
	type blockingRow struct {
		Blocked bool `db:"blocked"`
	}
	const blocking = `SELECT EXISTS (
		SELECT 1 FROM payment_attempts
		WHERE installment_id=$1 AND (
			state IN ('created','submitted','reconciliation_required','manual_review_required','succeeded')
			OR (state='expired' AND NOT financial_outcome_resolved
				AND NOT (purpose='installment_repayment'
				AND submitted_at IS NULL AND payment_url IS NULL
				AND financial_payment_session_id IS NULL))
		)
	) AS blocked`

	// Recheck customer-wide overdue inclusion INSIDE the claim transaction.
	// Preflight customer selection can race with activation or due transitions;
	// it is never final authority for a persisted repayment batch.
	seen := make(map[model.InstallmentID]struct{}, len(installments))
	for _, id := range installments {
		seen[id] = struct{}{}
	}
	location, err := time.LoadLocation("Asia/Tehran")
	if err != nil {
		return repo.RepaymentBatchRow{}, fmt.Errorf("load repayment business location: %w", err)
	}
	due, err := installmentrp.ListDueUnpaidByCustomerID(ctx, q, row.CustomerID, row.CreatedAt)
	if err != nil {
		return repo.RepaymentBatchRow{}, err
	}
	if err := repaymentselection.RequireAllOverdueInstallments(
		seen, due, row.CreatedAt, location,
	); err != nil {
		return repo.RepaymentBatchRow{}, err
	}

	// Validate every selected contract's entire installment sequence, not
	// only the selected rows, after the customer lock and before claiming.
	schedules := make(map[model.ContractID]struct{})
	for _, item := range plan.Items {
		if _, checked := schedules[item.ContractID]; checked {
			continue
		}
		schedule, err := installmentrp.LockByContractID(ctx, q, item.ContractID)
		if err != nil {
			return repo.RepaymentBatchRow{}, err
		}
		if err := repaymentselection.RequireConsecutiveInstallments(
			seen, schedule,
		); err != nil {
			return repo.RepaymentBatchRow{}, err
		}
		schedules[item.ContractID] = struct{}{}
	}

	seen = make(map[model.InstallmentID]struct{}, len(installments))
	const claim = `INSERT INTO repayment_batch_installment_claims(
		installment_id,batch_id,created_at
	) VALUES($1,$2,$3)`
	for index, id := range installments {
		if id.IsZero() {
			return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("missing installment id"))
		}
		if _, ok := seen[id]; ok {
			return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("duplicate installment claim"))
		}
		seen[id] = struct{}{}
		item := plan.Items[index]
		if item.InstallmentID != id {
			return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("installment order mismatch"))
		}
		actual, err := postgres.QueryOne[lockedInstallment](ctx, q, "lock selected installment", lock, id)
		if err != nil {
			return repo.RepaymentBatchRow{}, err
		}
		if actual.CustomerID != plan.CustomerID || actual.ContractID != item.ContractID ||
			actual.CreditAccountID != item.CreditAccountID || actual.CreditKind != item.CreditKind ||
			actual.ExternalMerchantID != item.MerchantID {
			return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("batch item does not match an active customer obligation"))
		}
		// A principal component must match the installment's recognized
		// principal in full; no user-selected partial or invented principal.
		var principal uint64
		for _, component := range item.Components {
			if component.Kind == collectionpayment.PrincipalComponent {
				principal += component.GrossMinor
			}
		}
		if principal != actual.PrincipalAmountMinor {
			return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("batch principal differs from installment"))
		}
		blocked, err := postgres.QueryOne[blockingRow](ctx, q, "check active installment payment", blocking, id)
		if err != nil {
			return repo.RepaymentBatchRow{}, err
		}
		if blocked.Blocked {
			return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("an unresolved payment already owns installment"))
		}
		if _, err := q.Exec(ctx, claim, id, stored.ID, row.CreatedAt); err != nil {
			return repo.RepaymentBatchRow{}, fmt.Errorf("reserve installment for batch: %w", postgres.MapError(err))
		}
	}
	return stored, nil
}

// Advance commits a single local orchestration step with optimistic fencing.
// A stale worker never overwrites a newer revision or a different FC identity.
func Advance[Q postgres.Queryer](
	ctx context.Context, q Q, id uuid.UUID, expectedVersion int64, status model.RepaymentBatchStatus,
	financialID *string, revision int64, nextPoll *time.Time, reason *string, at time.Time,
) (repo.RepaymentBatchRow, error) {
	if !status.IsValid() || expectedVersion < 1 || revision < 0 || at.IsZero() {
		return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("invalid batch transition"))
	}
	const query = `
	UPDATE repayment_batches SET
		status=$3,
		financial_collection_payment_id=COALESCE(financial_collection_payment_id,$4),
		snapshot_revision=GREATEST(snapshot_revision,$5),
		next_reconciliation_at=$6,
		last_reason_code=$7,
		version=version+1,
		updated_at=$8
	WHERE id=$1 AND version=$2
	  AND ($4::text IS NULL OR financial_collection_payment_id IS NULL
	       OR financial_collection_payment_id=$4)
	  AND ($5=0 OR snapshot_revision<=$5)
	RETURNING` + columns
	row, err := postgres.QueryOne[repo.RepaymentBatchRow](
		ctx, q, "advance repayment batch", query, id, expectedVersion, status, financialID,
		revision, nextPoll, reason, at,
	)
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("repayment batch state or FC identity changed"))
	}
	return row, err
}

// ReleaseClaims is permitted only after FC's terminal outcome was durably
// recorded and local application has succeeded (for posted) or failure is
// definitive. Unknown outcomes retain claims and prohibit duplicate payment.
func ReleaseClaims[Q postgres.Queryer](ctx context.Context, q Q, id uuid.UUID) error {
	const query = `DELETE FROM repayment_batch_installment_claims
	WHERE batch_id=$1
	  AND EXISTS (
		SELECT 1 FROM repayment_batches WHERE id=$1
		AND status IN ('posted','failed','expired')
	  )`
	_, err := q.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("release batch installment claims: %w", postgres.MapError(err))
	}
	return nil
}

// BindStart is the durable compare-and-set fence before any external Start
// RPC. Another idempotency key/provider/return URL fingerprint is a conflict.
func BindStart[Q postgres.Queryer](
	ctx context.Context, q Q, id uuid.UUID, key, fingerprint string, at time.Time,
) (repo.RepaymentBatchRow, error) {
	if id == uuid.Nil || key == "" || fingerprint == "" || at.IsZero() {
		return repo.RepaymentBatchRow{}, cerr.InvalidArgument(fmt.Errorf("invalid initial collection Start intent"))
	}
	const query = `
UPDATE repayment_batches SET
	status='starting',
	start_idempotency_key=COALESCE(start_idempotency_key,$2),
	start_fingerprint=COALESCE(start_fingerprint,$3),
	next_reconciliation_at=$4,
	version=version+1,
	updated_at=$4
WHERE id=$1
 AND ((status='prepared' AND start_idempotency_key IS NULL)
  OR (status='starting' AND start_idempotency_key=$2 AND start_fingerprint=$3))
RETURNING` + columns
	row, err := postgres.QueryOne[repo.RepaymentBatchRow](ctx, q,
		"bind FC collection start intent", query, id, key, fingerprint, at)
	if cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return repo.RepaymentBatchRow{}, cerr.Conflict(fmt.Errorf("collection Start intent conflicts with existing operation"))
	}
	return row, err
}
