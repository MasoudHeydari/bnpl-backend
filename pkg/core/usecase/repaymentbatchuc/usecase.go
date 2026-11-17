// Package repaymentbatchuc orchestrates FC v2 collections over immutable,
// locally reserved installments. It never decides monetary truth and never
// makes a network call inside a PostgreSQL transaction.
package repaymentbatchuc

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	"github.com/parstis/bnpl-backend/pkg/core/collectionpayment"
	extcollection "github.com/parstis/bnpl-backend/pkg/core/external/collectionpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

const defaultReconciliationInterval = 30*time.Second

// PostedApplier performs the entire local settlement in one caller-owned TX:
// installment marks, charge marks, contract principal, credit exposure and
// immutable credit journal entries. It must be idempotent per FC allocation ID.
type PostedApplier interface {
	ApplyPosted(ctx context.Context, tx repo.Tx, plan collectionpayment.Plan, result extcollection.Result) error
}

// UseCase owns only a batch's local lifecycle; FC owns provider and ledger.
type UseCase struct {
	pool repo.Pool
	batches repo.RepaymentBatch
	financial extcollection.Client
	applier PostedApplier
	clock clock.Clock
}

// New fails fast on incomplete dependencies rather than silently falling
// back to provider-verified (but unposted) payment state.
func New(pool repo.Pool,batches repo.RepaymentBatch,financial extcollection.Client,
	applier PostedApplier,clk clock.Clock)(*UseCase,error){
	if pool==nil || batches==nil || financial==nil || applier==nil || clk==nil {
		return nil,fmt.Errorf("repaymentbatchuc.New: all dependencies are required")
	}
	return &UseCase{pool:pool,batches:batches,financial:financial,applier:applier,clock:clk},nil
}

func (uc *UseCase) now()time.Time{return uc.clock.Now().UTC()}

func (uc *UseCase) load(ctx context.Context,id uuid.UUID)(repo.RepaymentBatchRow,error){
	var row repo.RepaymentBatchRow
	err:=uc.pool.Conn(ctx,func(ctx context.Context,conn repo.Conn)error{
		var loadErr error
		row,loadErr=uc.batches.Conn(conn).GetByID(ctx,id)
		return loadErr
	})
	return row,err
}

func planFromRow(row repo.RepaymentBatchRow)(collectionpayment.Plan,error){
	var plan collectionpayment.Plan
	if err:=json.Unmarshal(row.Snapshot,&plan);err!=nil{
		return collectionpayment.Plan{},fmt.Errorf("repayment batch snapshot decode: %w",err)
	}
	if err:=plan.Validate();err!=nil{return collectionpayment.Plan{},err}
	if plan.ID!=row.ID || plan.CustomerID!=row.CustomerID ||
		plan.ExternalCustomerID!=row.ExternalCustomerID ||
		plan.Currency!=row.Currency || plan.TotalMinor!=row.TotalAmountMinor {
		return collectionpayment.Plan{},fmt.Errorf("repayment batch snapshot identity mismatch")
	}
	return plan,nil
}

// PrepareDurably persists an immutable customer-accepted assessment plus
// installment claims before calling FC. A lost FC response can be recovered
// with GetBySource; no second source identity is ever minted for a retry.
func (uc *UseCase) PrepareDurably(ctx context.Context,plan collectionpayment.Plan,key string)(extcollection.Result,error){
	if err:=plan.Validate();err!=nil{return extcollection.Result{},err}
	if key=="" || len(key)>256{return extcollection.Result{},cerr.InvalidArgument(fmt.Errorf("invalid repayment idempotency key"))}
	payload,err:=json.Marshal(plan)
	if err!=nil{return extcollection.Result{},err}
	now:=uc.now()
	row:=repo.RepaymentBatchRow{
		ID:plan.ID,CustomerID:plan.CustomerID,ExternalCustomerID:plan.ExternalCustomerID,
		IdempotencyKey:key,Snapshot:payload,Currency:plan.Currency,
		TotalAmountMinor:plan.TotalMinor,Status:model.CreatedRepaymentBatchStatus,
		CreatedAt:now,UpdatedAt:now,
	}
	ids:=make([]model.InstallmentID,0,len(plan.Items))
	for _,item:=range plan.Items{ids=append(ids,item.InstallmentID)}

	err=uc.pool.Conn(ctx,func(ctx context.Context,conn repo.Conn)error{
		return conn.Tx(ctx,func(ctx context.Context,tx repo.Tx)error{
			q:=uc.batches.Tx(tx)
			existing,getErr:=q.GetByCustomerAndIdempotencyKey(ctx,plan.CustomerID,key)
			if getErr==nil {
				if existing.ID!=plan.ID || !bytes.Equal(existing.Snapshot,payload){
					return cerr.Conflict(fmt.Errorf("repayment idempotency key reused with different snapshot"))
				}
				row=existing
				return nil
			}
			if !cerr.IsCode(getErr,cerr.CodeResourceNotFound){return getErr}
			var createErr error
			row,createErr=q.Create(ctx,row,ids)
			return createErr
		})
	})
	if err!=nil{return extcollection.Result{},err}

	// Existing state may have an FC identity. Read it, never prepare another
	// logical batch. The original source identity is immutable.
	if row.FinancialCollectionPaymentID!=nil &&
		*row.FinancialCollectionPaymentID!=""{
		return uc.financial.GetByID(ctx,*row.FinancialCollectionPaymentID)
	}

	// Prepare itself is FC-idempotent for the immutable source and key. Calling
	// it again after an unknown outcome cannot create a second collection.
	result,err:=uc.financial.Prepare(ctx,plan,key)
	if err!=nil{
		uc.scheduleReconciliation(ctx,row,nil,0,nil)
		return extcollection.Result{},err
	}
	if result.SourceID!=plan.ID || result.CollectionPaymentID=="" || result.SnapshotRevision<1 {
		uc.scheduleReconciliation(ctx,row,nil,0,nil)
		return extcollection.Result{},fmt.Errorf("FC returned a mismatched/incomplete prepared collection")
	}
	if err:=uc.persistTransition(ctx,row,model.PreparedRepaymentBatchStatus,
		&result.CollectionPaymentID,result.SnapshotRevision,nil,nil);err!=nil{
		return extcollection.Result{},err
	}
	return result,nil
}

// StartPayment saves STARTING before the remote call. It may be retried only
// with the same FC idempotency key and bound provider settings. An ambiguous
// response is recovered by Get, not by minting another payment attempt.
func (uc *UseCase) StartPayment(ctx context.Context,id uuid.UUID,key,providerCode string)(extcollection.Result,error){
	if key=="" || providerCode==""{
		return extcollection.Result{},cerr.InvalidArgument(fmt.Errorf("start payment requires idempotency key and provider"))
	}
	row,err:=uc.load(ctx,id)
	if err!=nil{return extcollection.Result{},err}
	if row.Status.IsTerminal(){return extcollection.Result{},cerr.Conflict(fmt.Errorf("repayment batch is terminal"))}
	if row.FinancialCollectionPaymentID==nil || *row.FinancialCollectionPaymentID=="" || row.SnapshotRevision<1{
		return extcollection.Result{},cerr.Conflict(fmt.Errorf("repayment batch is not prepared in FC"))
	}
	// One durable state transition fences concurrent starts within BNPL.
	if row.Status==model.PreparedRepaymentBatchStatus {
		if err:=uc.persistTransition(ctx,row,model.StartingRepaymentBatchStatus,
			row.FinancialCollectionPaymentID,row.SnapshotRevision,nil,nil);err!=nil{
			return extcollection.Result{},err
		}
		row,err=uc.load(ctx,id)
		if err!=nil{return extcollection.Result{},err}
	}
	if row.Status!=model.StartingRepaymentBatchStatus{
		// Never assume a second Start is safe with different settings.
		return uc.financial.GetByID(ctx,*row.FinancialCollectionPaymentID)
	}
	result,err:=uc.financial.Start(ctx,*row.FinancialCollectionPaymentID,key,providerCode,row.SnapshotRevision)
	if err!=nil{
		uc.scheduleReconciliation(ctx,row,row.FinancialCollectionPaymentID,row.SnapshotRevision,nil)
		return extcollection.Result{},err
	}
	if result.CollectionPaymentID!=*row.FinancialCollectionPaymentID || result.SourceID!=row.ID{
		return extcollection.Result{},fmt.Errorf("FC Start returned a different collection identity")
	}
	// Always let reconciliation settle money and release local exposure.
	uc.scheduleReconciliation(ctx,row,row.FinancialCollectionPaymentID,row.SnapshotRevision,nil)
	return result,nil
}

// Reconcile recovers from a lost Prepare/Start response and applies only FC's
// posted result. It does not trust a browser callback or provider success.
func (uc *UseCase) Reconcile(ctx context.Context,id uuid.UUID)(extcollection.Result,error){
	row,err:=uc.load(ctx,id)
	if err!=nil{return extcollection.Result{},err}
	if row.Status.IsTerminal(){return extcollection.Result{},nil}
	plan,err:=planFromRow(row)
	if err!=nil{return extcollection.Result{},err}
	var result extcollection.Result
	if row.FinancialCollectionPaymentID!=nil && *row.FinancialCollectionPaymentID!=""{
		result,err=uc.financial.GetByID(ctx,*row.FinancialCollectionPaymentID)
	}else{
		result,err=uc.financial.GetBySource(ctx,row.ID)
	}
	if err!=nil {
		uc.scheduleReconciliation(ctx,row,row.FinancialCollectionPaymentID,row.SnapshotRevision,nil)
		return extcollection.Result{},err
	}
	if result.SourceID!=row.ID || result.CollectionPaymentID==""{
		return extcollection.Result{},fmt.Errorf("FC collection identity mismatch")
	}
	if row.FinancialCollectionPaymentID!=nil && *row.FinancialCollectionPaymentID!=result.CollectionPaymentID{
		return extcollection.Result{},fmt.Errorf("FC collection ID changed")
	}
	if result.IsAuthoritativelySettled(){
		if err:=extcollection.VerifyPostedAllocations(plan,result);err!=nil{
			uc.scheduleReconciliation(ctx,row,&result.CollectionPaymentID,result.SnapshotRevision,nil)
			return extcollection.Result{},fmt.Errorf("FC posting mismatch: %w",err)
		}
		err=uc.applyPostedInTransaction(ctx,row,plan,result)
		return result,err
	}
	if result.Status==extcollection.Failed || result.Status==extcollection.Expired{
		// FC terminal failure is safe to release only when no accounting posted.
		if result.PostingOutcome==extcollection.Posted{
			return extcollection.Result{},fmt.Errorf("FC terminal failure conflicts with posted accounting")
		}
		next:=model.FailedRepaymentBatchStatus
		if result.Status==extcollection.Expired{next=model.ExpiredRepaymentBatchStatus}
		err=uc.finishTerminal(ctx,row,next,result)
		return result,err
	}
	next:=model.ReconcilingRepaymentBatchStatus
	if result.Status==extcollection.AccountingPending{next=model.AccountingPendingRepaymentBatchStatus}
	poll:=uc.now().Add(defaultReconciliationInterval)
	err=uc.persistTransition(ctx,row,next,&result.CollectionPaymentID,result.SnapshotRevision,&poll,nil)
	return result,err
}

func (uc *UseCase) applyPostedInTransaction(
	ctx context.Context,row repo.RepaymentBatchRow,plan collectionpayment.Plan,result extcollection.Result,
)error{
	return uc.pool.Conn(ctx,func(ctx context.Context,conn repo.Conn)error{
		return conn.Tx(ctx,func(ctx context.Context,tx repo.Tx)error{
			q:=uc.batches.Tx(tx)
			current,err:=q.GetByID(ctx,row.ID)
			if err!=nil{return err}
			if current.Status==model.PostedRepaymentBatchStatus{return nil}
			if current.Status.IsTerminal(){return cerr.Conflict(fmt.Errorf("batch already terminal"))}

			// Persist local success and all installment/exposure effects inside
			// one transaction. Any application error rolls back the state too.
			_,err=q.Advance(ctx,current.ID,current.Version,model.PostedRepaymentBatchStatus,
				&result.CollectionPaymentID,result.SnapshotRevision,nil,nil,uc.now())
			if err!=nil{return err}
			if err:=uc.applier.ApplyPosted(ctx,tx,plan,result);err!=nil{return err}
			return q.ReleaseClaims(ctx,current.ID)
		})
	})
}

func (uc *UseCase) finishTerminal(ctx context.Context,row repo.RepaymentBatchRow,
	next model.RepaymentBatchStatus,result extcollection.Result)error{
	return uc.pool.Conn(ctx,func(ctx context.Context,conn repo.Conn)error{
		return conn.Tx(ctx,func(ctx context.Context,tx repo.Tx)error{
			q:=uc.batches.Tx(tx)
			current,err:=q.GetByID(ctx,row.ID)
			if err!=nil{return err}
			if current.Status.IsTerminal(){return nil}
			if _,err:=q.Advance(ctx,current.ID,current.Version,next,&result.CollectionPaymentID,
				result.SnapshotRevision,nil,nil,uc.now());err!=nil{return err}
			return q.ReleaseClaims(ctx,current.ID)
		})
	})
}

func (uc *UseCase) persistTransition(ctx context.Context,row repo.RepaymentBatchRow,next model.RepaymentBatchStatus,
	financialID *string,revision int64,poll *time.Time,reason *string)error{
	return uc.pool.Conn(ctx,func(ctx context.Context,conn repo.Conn)error{
		return conn.Tx(ctx,func(ctx context.Context,tx repo.Tx)error{
			_,err:=uc.batches.Tx(tx).Advance(ctx,row.ID,row.Version,next,
				financialID,revision,poll,reason,uc.now())
			return err
		})
	})
}

func (uc *UseCase) scheduleReconciliation(ctx context.Context,row repo.RepaymentBatchRow,
	financialID *string,revision int64,reason *string){
	poll:=uc.now().Add(defaultReconciliationInterval)
	// Best effort on error paths. Persisted STARTING or CREATED still permits
	// operator recovery; successful remote effects must never be assumed failed.
	_ = uc.persistTransition(ctx,row,model.ReconcilingRepaymentBatchStatus,
		financialID,revision,&poll,reason)
}

// ReconcileDue is suitable for an in-process scheduled job. Multiple workers
// may inspect the same row; optimistic versioning and an atomic local applier
// prevent duplicated local principal release.
func(uc *UseCase)ReconcileDue(ctx context.Context,limit int)error{
	if limit<1{return cerr.InvalidArgument(fmt.Errorf("limit must be positive"))}
	var due []repo.RepaymentBatchRow
	err:=uc.pool.Conn(ctx,func(ctx context.Context,conn repo.Conn)error{
		var findErr error
		due,findErr=uc.batches.Conn(conn).ListReconciliationDue(ctx,uc.now(),limit)
		return findErr
	})
	if err!=nil{return err}
	var failures error
	for _,row:=range due{
		if _,err:=uc.Reconcile(ctx,row.ID);err!=nil{
			failures=errors.Join(failures,fmt.Errorf("reconcile batch %s: %w",row.ID,err))
		}
	}
	return failures
}
