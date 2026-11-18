// Package repaymentbatchrp persists BNPL's durable FC-v2 batch snapshots.
// It never directly writes Financial ledger facts.
package repaymentbatchrp

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type Repo struct{}

func New() *Repo { return &Repo{} }

var _ repo.RepaymentBatch = (*Repo)(nil)

type connQueryer struct{ *postgres.Conn }
type txQueryer struct{ *postgres.Tx }

var _ repo.RepaymentBatchQueryer = (*connQueryer)(nil)
var _ repo.RepaymentBatchTxQueryer = (*txQueryer)(nil)

func (r *Repo) Conn(c repo.Conn) repo.RepaymentBatchQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}
func (r *Repo) Tx(t repo.Tx) repo.RepaymentBatchTxQueryer { return txQueryer{Tx: postgres.MustTx(t)} }

func (q connQueryer) GetByID(ctx context.Context, id uuid.UUID) (repo.RepaymentBatchRow, error) {
	return GetByID(ctx, q.Conn, id)
}
func (q connQueryer) GetByCustomerAndIdempotencyKey(ctx context.Context, id model.CustomerID, key string) (repo.RepaymentBatchRow, error) {
	return GetByCustomerAndIdempotencyKey(ctx, q.Conn, id, key)
}
func (q connQueryer) ListReconciliationDue(ctx context.Context, now time.Time, limit int) ([]repo.RepaymentBatchRow, error) {
	return ListReconciliationDue(ctx, q.Conn, now, limit)
}
func (q txQueryer) GetByID(ctx context.Context, id uuid.UUID) (repo.RepaymentBatchRow, error) {
	return GetByID(ctx, q.Tx, id)
}
func (q txQueryer) GetByCustomerAndIdempotencyKey(ctx context.Context, id model.CustomerID, key string) (repo.RepaymentBatchRow, error) {
	return GetByCustomerAndIdempotencyKey(ctx, q.Tx, id, key)
}
func (q txQueryer) ListReconciliationDue(ctx context.Context, now time.Time, limit int) ([]repo.RepaymentBatchRow, error) {
	return ListReconciliationDue(ctx, q.Tx, now, limit)
}
func (q txQueryer) Create(ctx context.Context, row repo.RepaymentBatchRow, ids []model.InstallmentID) (repo.RepaymentBatchRow, error) {
	return Create(ctx, q.Tx, row, ids)
}
func (q txQueryer) Advance(ctx context.Context, id uuid.UUID, version int64, status model.RepaymentBatchStatus,
	financialID *string, revision int64, nextPoll *time.Time, reason *string, now time.Time) (repo.RepaymentBatchRow, error) {
	return Advance(ctx, q.Tx, id, version, status, financialID, revision, nextPoll, reason, now)
}
func (q txQueryer) ReleaseClaims(ctx context.Context, id uuid.UUID, at time.Time) error {
	return ReleaseClaims(ctx, q.Tx, id, at)
}

func (q txQueryer) BindStart(ctx context.Context, id uuid.UUID, key, fingerprint string, at time.Time) (repo.RepaymentBatchRow, error) {
	return BindStart(ctx, q.Tx, id, key, fingerprint, at)
}

func (q txQueryer) RecordSettlement(
	ctx context.Context,
	settlementID uuid.UUID,
	batchID uuid.UUID,
	financialCollectionPaymentID string,
	snapshotRevision int64,
	currency model.Currency,
	totalAmountMinor uint64,
	postedAt time.Time,
	createdAt time.Time,
	allocations []repo.RepaymentBatchSettlementAllocation,
) error {
	wire := make([]SettlementAllocation, 0, len(allocations))
	for _, alloc := range allocations {
		wire = append(wire, SettlementAllocation{
			ID:               alloc.ID,
			InstallmentID:    alloc.InstallmentID,
			AmountMinor:      alloc.AmountMinor,
			Currency:         alloc.Currency,
			PostingReference: alloc.PostingReference,
		})
	}

	return RecordSettlement(
		ctx,
		q.Tx,
		settlementID,
		batchID,
		financialCollectionPaymentID,
		snapshotRevision,
		currency,
		totalAmountMinor,
		postedAt,
		createdAt,
		wire,
	)
}
