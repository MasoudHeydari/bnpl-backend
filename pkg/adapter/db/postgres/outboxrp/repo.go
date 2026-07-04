package outboxrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core outbox repository contract.
var _ repo.Outbox = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the core outbox repository.
type Repo struct{}

// New creates a stateless PostgreSQL outbox repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.OutboxConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.OutboxTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.OutboxConnQueryer = (*connQueryer)(nil)

func (q connQueryer) ClaimPending(
	ctx context.Context,
	publisherID string,
	limit int,
	now time.Time,
	processingLeaseTimeout time.Duration,
) ([]model.OutboxMessage, error) {
	return ClaimPending(ctx, q.Conn, publisherID, limit, now, processingLeaseTimeout)
}

func (q connQueryer) MarkPublished(
	ctx context.Context,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
	publishedAt time.Time,
) error {
	return MarkPublished(ctx, q.Conn, outboxID, workerID, lockVersion, publishedAt)
}

func (q connQueryer) MarkSkipped(
	ctx context.Context,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
	skippedAt time.Time,
	reason string,
) error {
	return MarkSkipped(ctx, q.Conn, outboxID, workerID, lockVersion, skippedAt, reason)
}

func (q connQueryer) MarkRetry(
	ctx context.Context,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
	nextAttemptAt time.Time,
	lastError string,
) error {
	return MarkRetry(ctx, q.Conn, outboxID, workerID, lockVersion, nextAttemptAt, lastError)
}

func (q connQueryer) MarkFailed(
	ctx context.Context,
	outboxID model.OutboxID,
	workerID string,
	lockVersion int64,
	lastError string,
) error {
	return MarkFailed(ctx, q.Conn, outboxID, workerID, lockVersion, lastError)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.OutboxTxQueryer = (*txQueryer)(nil)

func (q txQueryer) Create(
	ctx context.Context,
	message repo.CreateOutboxMessage,
) (model.OutboxMessage, error) {
	return Create(ctx, q.Tx, message)
}
