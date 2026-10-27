package collectionscaserp

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.CollectionsCase = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the collections case repository.
type Repo struct{}

// New returns a stateless PostgreSQL collections case repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.CollectionsCaseConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.CollectionsCaseTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.CollectionsCaseConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByDefaultEpisodeID(
	ctx context.Context,
	episodeID model.DefaultEpisodeID,
) (model.CollectionsCase, error) {
	return GetByDefaultEpisodeID(ctx, q.Conn, episodeID)
}

func (q connQueryer) GetByID(
	ctx context.Context,
	id model.CollectionsCaseID,
) (model.CollectionsCase, error) {
	return GetByID(ctx, q.Conn, id)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.CollectionsCaseTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByDefaultEpisodeID(
	ctx context.Context,
	episodeID model.DefaultEpisodeID,
) (model.CollectionsCase, error) {
	return GetByDefaultEpisodeID(ctx, q.Tx, episodeID)
}

func (q txQueryer) GetByID(
	ctx context.Context,
	id model.CollectionsCaseID,
) (model.CollectionsCase, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) Create(
	ctx context.Context,
	message repo.CreateCollectionsCase,
) (model.CollectionsCase, error) {
	return Create(ctx, q.Tx, message)
}

func (q txQueryer) AppendEvent(
	ctx context.Context,
	message repo.CreateCollectionsCaseEvent,
) (model.CollectionsCaseEvent, error) {
	return AppendEvent(ctx, q.Tx, message)
}
