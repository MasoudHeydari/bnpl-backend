package repo

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// RepaymentBatchRow holds an immutable JSON assessment snapshot and mutable
// local orchestration metadata. It must not be interpreted as a monetary ledger.
type RepaymentBatchRow struct {
	ID                           uuid.UUID                  `db:"id"`
	CustomerID                   model.CustomerID           `db:"customer_id"`
	ExternalCustomerID           model.ExternalCustomerID   `db:"external_customer_id"`
	IdempotencyKey               string                     `db:"idempotency_key"`
	Snapshot                     []byte                     `db:"snapshot"`
	Currency                     model.Currency             `db:"currency"`
	TotalAmountMinor             uint64                     `db:"total_amount_minor"`
	Status                       model.RepaymentBatchStatus `db:"status"`
	FinancialCollectionPaymentID *string                    `db:"financial_collection_payment_id"`
	SnapshotRevision             int64                      `db:"snapshot_revision"`
	NextReconciliationAt         *time.Time                 `db:"next_reconciliation_at"`
	LastReasonCode               *string                    `db:"last_reason_code"`
	StartIdempotencyKey          *string                    `db:"start_idempotency_key"`
	StartFingerprint             *string                    `db:"start_fingerprint"`
	Version                      int64                      `db:"version"`
	CreatedAt                    time.Time                  `db:"created_at"`
	UpdatedAt                    time.Time                  `db:"updated_at"`
}

// RepaymentBatchQueryer loads batches without a network request.
type RepaymentBatchQueryer interface {
	GetByID(ctx context.Context, id uuid.UUID) (RepaymentBatchRow, error)
	GetByCustomerAndIdempotencyKey(ctx context.Context, customerID model.CustomerID, key string) (RepaymentBatchRow, error)
	ListReconciliationDue(ctx context.Context, now time.Time, limit int) ([]RepaymentBatchRow, error)
}

// RepaymentBatchTxQueryer protects mutations with a database transaction.
// State changes use optimistic version checks and never overwrite a different
// Financial collection identity.
type RepaymentBatchTxQueryer interface {
	RepaymentBatchQueryer
	Create(ctx context.Context, row RepaymentBatchRow, installmentIDs []model.InstallmentID) (RepaymentBatchRow, error)
	Advance(
		ctx context.Context, id uuid.UUID, expectedVersion int64, status model.RepaymentBatchStatus,
		financialID *string, revision int64, nextPoll *time.Time, reason *string, at time.Time) (RepaymentBatchRow, error)
	// BindStart writes the first immutable Start intent and transitions the
	// local row to starting atomically; only identical retries can proceed.
	BindStart(ctx context.Context, id uuid.UUID, key, fingerprint string, at time.Time) (RepaymentBatchRow, error)
	ReleaseClaims(ctx context.Context, id uuid.UUID) error
}

type RepaymentBatch interface {
	Conn(Conn) RepaymentBatchQueryer
	Tx(Tx) RepaymentBatchTxQueryer
}
