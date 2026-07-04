package borrowerexposurereservationrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core borrower exposure reservation repository contract.
var _ repo.BorrowerExposureReservation = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the borrower exposure reservation repository.
type Repo struct{}

// New returns a stateless PostgreSQL borrower exposure reservation repository.
func New() *Repo {
	return &Repo{}
}

func (r *Repo) Conn(c repo.Conn) repo.BorrowerExposureReservationConnQueryer {
	return connQueryer{Conn: c.(*postgres.Conn)}
}

func (r *Repo) Tx(tx repo.Tx) repo.BorrowerExposureReservationTxQueryer {
	return txQueryer{Tx: tx.(*postgres.Tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.BorrowerExposureReservationConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) GetByDecisionID(
	ctx context.Context,
	decisionID uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	return GetByDecisionID(ctx, q.Conn, decisionID)
}

func (q connQueryer) GetReservedByCheckoutIntentID(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	return GetReservedByCheckoutIntentID(ctx, q.Conn, checkoutIntentID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.BorrowerExposureReservationTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByID(
	ctx context.Context,
	id uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) GetByDecisionID(
	ctx context.Context,
	decisionID uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	return GetByDecisionID(ctx, q.Tx, decisionID)
}

func (q txQueryer) GetReservedByCheckoutIntentID(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
) (model.BorrowerExposureReservation, error) {
	return GetReservedByCheckoutIntentID(ctx, q.Tx, checkoutIntentID)
}

func (q txQueryer) ExpireDueByBorrowerProfileIDAndCurrency(
	ctx context.Context,
	borrowerProfileID uuid.UUID,
	currency model.Currency,
	now time.Time,
	updatedAt time.Time,
) (uint64, error) {
	return ExpireDueByBorrowerProfileIDAndCurrency(ctx, q.Tx, borrowerProfileID, currency, now, updatedAt)
}

func (q txQueryer) Create(
	ctx context.Context,
	checkoutIntentID uuid.UUID,
	decisionID uuid.UUID,
	borrowerProfileID uuid.UUID,
	amountMinor uint64,
	currency model.Currency,
	status model.BorrowerExposureReservationStatus,
	expiresAt time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.BorrowerExposureReservation, error) {
	return Create(
		ctx,
		q.Tx,
		checkoutIntentID,
		decisionID,
		borrowerProfileID,
		amountMinor,
		currency,
		status,
		expiresAt,
		createdAt,
		updatedAt,
	)
}
