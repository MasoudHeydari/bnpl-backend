package repaymentreminderrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core repayment reminder repository contract.
var _ repo.RepaymentReminder = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the repayment reminder repository.
type Repo struct{}

// New returns a stateless PostgreSQL repayment reminder repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.RepaymentReminderConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.RepaymentReminderTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.RepaymentReminderConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByCustomerBusinessDate(
	ctx context.Context,
	customerID model.CustomerID,
	businessDate time.Time,
) (model.RepaymentReminder, error) {
	return GetByCustomerBusinessDate(ctx, q.Conn, customerID, businessDate)
}

func (q connQueryer) GetByID(
	ctx context.Context,
	id model.RepaymentReminderID,
) (model.RepaymentReminder, error) {
	return GetByID(ctx, q.Conn, id)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.RepaymentReminderTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByCustomerBusinessDate(
	ctx context.Context,
	customerID model.CustomerID,
	businessDate time.Time,
) (model.RepaymentReminder, error) {
	return GetByCustomerBusinessDate(ctx, q.Tx, customerID, businessDate)
}

func (q txQueryer) GetByID(
	ctx context.Context,
	id model.RepaymentReminderID,
) (model.RepaymentReminder, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) Create(
	ctx context.Context,
	message repo.CreateRepaymentReminder,
) (model.RepaymentReminder, error) {
	return Create(ctx, q.Tx, message)
}

func (q txQueryer) LockByCustomerBusinessDate(
	ctx context.Context,
	customerID model.CustomerID,
	businessDate time.Time,
) (model.RepaymentReminder, error) {
	return LockByCustomerBusinessDate(ctx, q.Tx, customerID, businessDate)
}
