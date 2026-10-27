package customerrestrictionrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CustomerRestriction = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the customer restriction repository.
type Repo struct{}

// New returns a stateless PostgreSQL customer restriction repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.CustomerRestrictionConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.CustomerRestrictionTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.CustomerRestrictionConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetActiveByCustomerType(
	ctx context.Context,
	customerID model.CustomerID,
	restrictionType model.CustomerRestrictionType,
) (model.CustomerRestriction, error) {
	return GetActiveByCustomerType(ctx, q.Conn, customerID, restrictionType)
}

func (q connQueryer) ListActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
) ([]model.CustomerRestriction, error) {
	return ListActiveByCustomerID(ctx, q.Conn, customerID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.CustomerRestrictionTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetActiveByCustomerType(
	ctx context.Context,
	customerID model.CustomerID,
	restrictionType model.CustomerRestrictionType,
) (model.CustomerRestriction, error) {
	return GetActiveByCustomerType(ctx, q.Tx, customerID, restrictionType)
}

func (q txQueryer) ListActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
) ([]model.CustomerRestriction, error) {
	return ListActiveByCustomerID(ctx, q.Tx, customerID)
}

func (q txQueryer) LockActiveByCustomerType(
	ctx context.Context,
	customerID model.CustomerID,
	restrictionType model.CustomerRestrictionType,
) (model.CustomerRestriction, error) {
	return LockActiveByCustomerType(ctx, q.Tx, customerID, restrictionType)
}

func (q txQueryer) Create(
	ctx context.Context,
	message repo.CreateCustomerRestriction,
) (model.CustomerRestriction, error) {
	return Create(ctx, q.Tx, message)
}

func (q txQueryer) Release(
	ctx context.Context,
	id model.CustomerRestrictionID,
	releasingDecisionID model.DecisionID,
	releasedAt time.Time,
	updatedAt time.Time,
) (model.CustomerRestriction, error) {
	return Release(ctx, q.Tx, id, releasingDecisionID, releasedAt, updatedAt)
}
