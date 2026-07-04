package defaultepisoderp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.DefaultEpisode = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the default episode repository.
type Repo struct{}

// New returns a stateless PostgreSQL default episode repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.DefaultEpisodeConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.DefaultEpisodeTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.DefaultEpisodeConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
) (model.DefaultEpisode, error) {
	return GetActiveByCustomerID(ctx, q.Conn, customerID)
}

func (q connQueryer) GetByID(
	ctx context.Context,
	id model.DefaultEpisodeID,
) (model.DefaultEpisode, error) {
	return GetByID(ctx, q.Conn, id)
}

func (q connQueryer) ListTriggersByEpisodeID(
	ctx context.Context,
	episodeID model.DefaultEpisodeID,
) ([]model.DefaultEpisodeTrigger, error) {
	return ListTriggersByEpisodeID(ctx, q.Conn, episodeID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.DefaultEpisodeTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
) (model.DefaultEpisode, error) {
	return GetActiveByCustomerID(ctx, q.Tx, customerID)
}

func (q txQueryer) GetByID(
	ctx context.Context,
	id model.DefaultEpisodeID,
) (model.DefaultEpisode, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) ListTriggersByEpisodeID(
	ctx context.Context,
	episodeID model.DefaultEpisodeID,
) ([]model.DefaultEpisodeTrigger, error) {
	return ListTriggersByEpisodeID(ctx, q.Tx, episodeID)
}

func (q txQueryer) LockActiveByCustomerID(
	ctx context.Context,
	customerID model.CustomerID,
) (model.DefaultEpisode, error) {
	return LockActiveByCustomerID(ctx, q.Tx, customerID)
}

func (q txQueryer) Create(
	ctx context.Context,
	message repo.CreateDefaultEpisode,
) (model.DefaultEpisode, error) {
	return Create(ctx, q.Tx, message)
}

func (q txQueryer) MarkRecovered(
	ctx context.Context,
	episodeID model.DefaultEpisodeID,
	closingDecisionID model.DecisionID,
	recoveredAt time.Time,
	updatedAt time.Time,
) (model.DefaultEpisode, error) {
	return MarkRecovered(ctx, q.Tx, episodeID, closingDecisionID, recoveredAt, updatedAt)
}
