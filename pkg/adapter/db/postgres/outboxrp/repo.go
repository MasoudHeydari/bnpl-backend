package outboxrp

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Verify Repo implements the core outbox repository contract.
var _ repo.Outbox = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the core outbox repository.
//
// Repo is stateless and safe for concurrent reuse.
type Repo struct{}

// New creates a stateless PostgreSQL outbox repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
//
// Publisher workers use this binding to claim and update committed outbox
// messages.
func (r *Repo) Conn(c repo.Conn) repo.OutboxConnQueryer {
	return connQueryer{
		Conn: c.(*postgres.Conn),
	}
}

// Tx binds the repository to a transaction scope.
//
// Producers use this binding to enqueue outbox messages in the same transaction
// as the business change that produced them.
func (r *Repo) Tx(tx repo.Tx) repo.OutboxTxQueryer {
	return txQueryer{
		Tx: tx.(*postgres.Tx),
	}
}

// connQueryer exposes publisher-side outbox operations on a PostgreSQL
// connection.
type connQueryer struct {
	*postgres.Conn
}

// Verify connQueryer implements the core outbox connection queryer contract.
var _ repo.OutboxConnQueryer = (*connQueryer)(nil)

// ClaimPending claims due pending messages for a publisher.
func (q connQueryer) ClaimPending(
	ctx context.Context,
	publisherID string,
	limit int,
	now time.Time,
) ([]model.OutboxMessage, error) {
	return ClaimPending(ctx, q.Conn, publisherID, limit, now)
}

// MarkPublished marks a claimed message as successfully delivered.
func (q connQueryer) MarkPublished(
	ctx context.Context,
	outboxID uuid.UUID,
	publishedAt time.Time,
) error {
	return MarkPublished(ctx, q.Conn, outboxID, publishedAt)
}

// MarkRetry schedules a claimed message for retry.
func (q connQueryer) MarkRetry(
	ctx context.Context,
	outboxID uuid.UUID,
	nextAttemptAt time.Time,
	lastError string,
) error {
	return MarkRetry(ctx, q.Conn, outboxID, nextAttemptAt, lastError)
}

// MarkFailed marks a claimed message as permanently failed.
func (q connQueryer) MarkFailed(
	ctx context.Context,
	outboxID uuid.UUID,
	lastError string,
) error {
	return MarkFailed(ctx, q.Conn, outboxID, lastError)
}

// txQueryer exposes transaction-scoped outbox operations.
type txQueryer struct {
	*postgres.Tx
}

// Verify txQueryer implements the core outbox transaction queryer contract.
var _ repo.OutboxTxQueryer = (*txQueryer)(nil)

// Enqueue stores a new outbox message inside the current transaction.
func (q txQueryer) Enqueue(
	ctx context.Context,
	msg model.OutboxMessage,
) error {
	return Enqueue(ctx, q.Tx, msg)
}
