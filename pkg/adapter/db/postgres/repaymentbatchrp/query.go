package repaymentbatchrp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const columns = `
	id, customer_id, external_customer_id, idempotency_key, snapshot, currency,
	total_amount_minor, status, financial_collection_payment_id, snapshot_revision,
	next_reconciliation_at, last_reason_code, version, created_at, updated_at
`

// GetByID loads the durable snapshot without advancing its lifecycle.
func GetByID[Q postgres.Queryer](ctx context.Context,q Q,id uuid.UUID)(repo.RepaymentBatchRow,error){
	const query = `SELECT`+columns+` FROM repayment_batches WHERE id=$1`
	return postgres.QueryOne[repo.RepaymentBatchRow](ctx,q,"get repayment batch",query,id)
}

// GetByCustomerAndIdempotencyKey supports safe replay without generating a
// second payment collection for the same logical customer request.
func GetByCustomerAndIdempotencyKey[Q postgres.Queryer](
	ctx context.Context,q Q,customerID model.CustomerID,key string,
)(repo.RepaymentBatchRow,error){
	const query = `SELECT`+columns+` FROM repayment_batches
	WHERE customer_id=$1 AND idempotency_key=$2`
	return postgres.QueryOne[repo.RepaymentBatchRow](ctx,q,"get batch by idempotency key",query,customerID,key)
}

func ListReconciliationDue[Q postgres.Queryer](
	ctx context.Context,q Q,now time.Time,limit int,
)([]repo.RepaymentBatchRow,error){
	if limit<1{return nil,cerr.InvalidArgument(fmt.Errorf("reconciliation batch size must be positive"))}
	const query = `SELECT`+columns+` FROM repayment_batches
	WHERE next_reconciliation_at <= $1
	AND status IN ('created','prepared','starting','requires_action','reconciling','accounting_pending')
	ORDER BY next_reconciliation_at,id LIMIT $2`
	return postgres.QueryAll[repo.RepaymentBatchRow](ctx,q,"list repayment batches due",query,now,limit)
}

// Create reserves all installment identities in the SAME local database tx
// that stores the snapshot. An overlapping payment attempt cannot reserve the
// same installment in another batch while these claims remain present.
func Create[Q postgres.Queryer](
	ctx context.Context,q Q,row repo.RepaymentBatchRow,installments []model.InstallmentID,
)(repo.RepaymentBatchRow,error){
	if row.ID==uuid.Nil || row.CustomerID.IsZero() || row.ExternalCustomerID.IsZero() ||
		row.IdempotencyKey=="" || len(row.Snapshot)==0 || row.TotalAmountMinor==0 ||
		!row.Currency.IsValid() || row.Status!=model.CreatedRepaymentBatchStatus ||
		len(installments)==0 || row.CreatedAt.IsZero() || row.UpdatedAt.IsZero() {
		return repo.RepaymentBatchRow{},cerr.InvalidArgument(fmt.Errorf("invalid repayment batch creation"))
	}
	const query = `
	INSERT INTO repayment_batches(
		id,customer_id,external_customer_id,idempotency_key,snapshot,currency,
		total_amount_minor,status,created_at,updated_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	RETURNING`+columns
	stored,err:=postgres.QueryOne[repo.RepaymentBatchRow](
		ctx,q,"create repayment batch",query,row.ID,row.CustomerID,row.ExternalCustomerID,
		row.IdempotencyKey,row.Snapshot,row.Currency,row.TotalAmountMinor,row.Status,
		row.CreatedAt,row.UpdatedAt,
	)
	if err!=nil{return repo.RepaymentBatchRow{},err}
	seen:=make(map[model.InstallmentID]struct{},len(installments))
	const claim = `INSERT INTO repayment_batch_installment_claims(
		installment_id,batch_id,created_at
	) VALUES($1,$2,$3)`
	for _,id:=range installments{
		if id.IsZero(){return repo.RepaymentBatchRow{},cerr.InvalidArgument(fmt.Errorf("missing installment id"))}
		if _,ok:=seen[id];ok{return repo.RepaymentBatchRow{},cerr.InvalidArgument(fmt.Errorf("duplicate installment claim"))}
		seen[id]=struct{}{}
		if _,err:=q.Exec(ctx,claim,id,stored.ID,row.CreatedAt);err!=nil{
			return repo.RepaymentBatchRow{},fmt.Errorf("reserve installment for batch: %w",postgres.MapError(err))
		}
	}
	return stored,nil
}

// Advance commits a single local orchestration step with optimistic fencing.
// A stale worker never overwrites a newer revision or a different FC identity.
func Advance[Q postgres.Queryer](
	ctx context.Context,q Q,id uuid.UUID,expectedVersion int64,status model.RepaymentBatchStatus,
	financialID *string,revision int64,nextPoll *time.Time,reason *string,at time.Time,
)(repo.RepaymentBatchRow,error){
	if !status.IsValid() || expectedVersion<1 || revision<0 || at.IsZero(){
		return repo.RepaymentBatchRow{},cerr.InvalidArgument(fmt.Errorf("invalid batch transition"))
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
	RETURNING`+columns
	row,err:=postgres.QueryOne[repo.RepaymentBatchRow](
		ctx,q,"advance repayment batch",query,id,expectedVersion,status,financialID,
		revision,nextPoll,reason,at,
	)
	if cerr.IsCode(err,cerr.CodeResourceNotFound){
		return repo.RepaymentBatchRow{},cerr.Conflict(fmt.Errorf("repayment batch state or FC identity changed"))
	}
	return row,err
}

// ReleaseClaims is permitted only after FC's terminal outcome was durably
// recorded and local application has succeeded (for posted) or failure is
// definitive. Unknown outcomes retain claims and prohibit duplicate payment.
func ReleaseClaims[Q postgres.Queryer](ctx context.Context,q Q,id uuid.UUID)error{
	const query = `DELETE FROM repayment_batch_installment_claims
	WHERE batch_id=$1
	  AND EXISTS (
		SELECT 1 FROM repayment_batches WHERE id=$1
		AND status IN ('posted','failed','expired')
	  )`
	_,err:=q.Exec(ctx,query,id)
	if err!=nil{return fmt.Errorf("release batch installment claims: %w",postgres.MapError(err))}
	return nil
}
