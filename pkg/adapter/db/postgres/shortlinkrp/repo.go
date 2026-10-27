package shortlinkrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core short-link repository contract.
var _ repo.ShortLink = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the short-link repository.
type Repo struct{}

// New returns a stateless PostgreSQL short-link repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.ShortLinkConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.ShortLinkTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.ShortLinkConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetByCode(ctx context.Context, code string) (model.ShortLink, error) {
	return GetByCode(ctx, q.Conn, code)
}

func (q connQueryer) GetByID(ctx context.Context, id model.ShortLinkID) (model.ShortLink, error) {
	return GetByID(ctx, q.Conn, id)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.ShortLinkTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetByCode(ctx context.Context, code string) (model.ShortLink, error) {
	return GetByCode(ctx, q.Tx, code)
}

func (q txQueryer) GetByID(ctx context.Context, id model.ShortLinkID) (model.ShortLink, error) {
	return GetByID(ctx, q.Tx, id)
}

func (q txQueryer) Create(
	ctx context.Context,
	id model.ShortLinkID,
	code string,
	destinationURL string,
	expiresAt *time.Time,
	createdAt time.Time,
	updatedAt time.Time,
) (model.ShortLink, error) {
	return Create(ctx, q.Tx, id, code, destinationURL, expiresAt, createdAt, updatedAt)
}

func (q txQueryer) EnsureAlias(
	ctx context.Context,
	id model.ShortLinkID,
	code string,
	destinationURL string,
	now time.Time,
) (model.ShortLink, error) {
	return EnsureAlias(ctx, q.Tx, id, code, destinationURL, now)
}

func (q txQueryer) Revoke(
	ctx context.Context,
	code string,
	revokedAt time.Time,
) (model.ShortLink, error) {
	return Revoke(ctx, q.Tx, code, revokedAt)
}
