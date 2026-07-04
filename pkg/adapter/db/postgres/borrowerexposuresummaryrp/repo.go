package borrowerexposuresummaryrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core borrower exposure summary repository contract.
var _ repo.BorrowerExposureSummary = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the borrower exposure summary repository.
type Repo struct{}

// New returns a stateless PostgreSQL borrower exposure summary repository.
func New() *Repo {
	return &Repo{}
}

func (r *Repo) Conn(c repo.Conn) repo.BorrowerExposureSummaryConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

func (r *Repo) Tx(tx repo.Tx) repo.BorrowerExposureSummaryTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.BorrowerExposureSummaryConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByBorrowerProfileIDAndCurrency(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
) (model.BorrowerExposureSummary, error) {
	return GetByBorrowerProfileIDAndCurrency(ctx, q.Conn, borrowerProfileID, currency)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.BorrowerExposureSummaryTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByBorrowerProfileIDAndCurrency(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
) (model.BorrowerExposureSummary, error) {
	return GetByBorrowerProfileIDAndCurrency(ctx, q.Tx, borrowerProfileID, currency)
}

func (q txQueryer) LockByBorrowerProfileIDAndCurrency(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
) (model.BorrowerExposureSummary, error) {
	return LockByBorrowerProfileIDAndCurrency(ctx, q.Tx, borrowerProfileID, currency)
}

func (q txQueryer) EnsureAndLock(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	updatedAt time.Time,
) (model.BorrowerExposureSummary, error) {
	return EnsureAndLock(ctx, q.Tx, borrowerProfileID, currency, updatedAt)
}

func (q txQueryer) AddReservedExposure(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	amountMinor uint64,
	updatedAt time.Time,
) error {
	return AddReservedExposure(ctx, q.Tx, borrowerProfileID, currency, amountMinor, updatedAt)
}

func (q txQueryer) SubtractReservedExposure(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	amountMinor uint64,
	updatedAt time.Time,
) error {
	return SubtractReservedExposure(ctx, q.Tx, borrowerProfileID, currency, amountMinor, updatedAt)
}
