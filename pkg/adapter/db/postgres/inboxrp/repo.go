package inboxrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.Inbox = (*Repo)(nil)

// Repo is the PostgreSQL implementation of the inbox repository.
type Repo struct{}

// New returns a stateless PostgreSQL inbox repository.
func New() *Repo {
	return &Repo{}
}

// Conn binds the repository to a non-transactional connection scope.
func (r *Repo) Conn(c repo.Conn) repo.InboxConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}

// Tx binds the repository to a transaction scope.
func (r *Repo) Tx(tx repo.Tx) repo.InboxTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct {
	*postgres.Conn
}

var _ repo.InboxConnQueryer = (*connQueryer)(nil)

func (q connQueryer) GetBySourceAndEventID(
	ctx context.Context,
	sourceService string,
	eventID string,
) (model.Inbox, error) {
	return GetBySourceAndEventID(ctx, q.Conn, sourceService, eventID)
}

type txQueryer struct {
	*postgres.Tx
}

var _ repo.InboxTxQueryer = (*txQueryer)(nil)

func (q txQueryer) GetBySourceAndEventID(
	ctx context.Context,
	sourceService string,
	eventID string,
) (model.Inbox, error) {
	return GetBySourceAndEventID(ctx, q.Tx, sourceService, eventID)
}

func (q txQueryer) AcquireForProcessing(
	ctx context.Context,
	event model.InboxEvent,
	now time.Time,
) (model.Inbox, model.InboxAcquireResult, error) {
	return AcquireForProcessing(ctx, q.Tx, event, now)
}

func (q txQueryer) MarkProcessed(
	ctx context.Context,
	id model.InboxID,
	processedAt time.Time,
	updatedAt time.Time,
) (model.Inbox, error) {
	return MarkProcessed(ctx, q.Tx, id, processedAt, updatedAt)
}

func (q txQueryer) MarkReconciliationRequired(
	ctx context.Context,
	id model.InboxID,
	failureCode *string,
	failureMessage *string,
	processedAt time.Time,
	updatedAt time.Time,
) (model.Inbox, error) {
	return MarkReconciliationRequired(
		ctx,
		q.Tx,
		id,
		failureCode,
		failureMessage,
		processedAt,
		updatedAt,
	)
}

func (q txQueryer) MarkFailed(
	ctx context.Context,
	id model.InboxID,
	failureCode string,
	failureMessage string,
	failedAt time.Time,
	updatedAt time.Time,
) (model.Inbox, error) {
	return MarkFailed(ctx, q.Tx, id, failureCode, failureMessage, failedAt, updatedAt)
}

func (q txQueryer) RecordPayloadIdentityConflict(
	ctx context.Context,
	event model.InboxPayloadConflictEvent,
	now time.Time,
) (model.InboxPayloadConflict, bool, error) {
	return RecordPayloadIdentityConflict(ctx, q.Tx, event, now)
}
