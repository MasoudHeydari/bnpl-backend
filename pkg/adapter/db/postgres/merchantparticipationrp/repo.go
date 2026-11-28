package merchantparticipationrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core Merchant participation repository contract.
var _ repo.MerchantParticipation = (*Repo)(nil)

// Repo is the stateless PostgreSQL implementation of Merchant participation.
type Repo struct{}

// New returns a stateless PostgreSQL Merchant participation repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.MerchantParticipationQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.MerchantParticipationTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.MerchantParticipationQueryer = (*connQueryer)(nil)

func (q connQueryer) Get(ctx context.Context, id model.MerchantParticipationRequestID) (model.MerchantParticipation, error) {
	return Get(ctx, q.Conn, id)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.MerchantParticipationTxQueryer = (*txQueryer)(nil)

func (q txQueryer) Get(ctx context.Context, id model.MerchantParticipationRequestID) (model.MerchantParticipation, error) {
	return Get(ctx, q.Tx, id)
}

func (q txQueryer) Insert(ctx context.Context, p model.MerchantParticipation, now time.Time) error {
	return Insert(ctx, q.Tx, p, now)
}

func (q txQueryer) ClaimDue(ctx context.Context, now, leaseUntil time.Time) (model.MerchantParticipation, error) {
	return ClaimDue(ctx, q.Tx, now, leaseUntil)
}

func (q txQueryer) Finish(ctx context.Context, r repo.MerchantProgressDeliveryResult) error {
	return Finish(ctx, q.Tx, r)
}
