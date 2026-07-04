package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// InboxQueryer defines read-only inbox operations.
type InboxQueryer interface {
	// GetBySourceAndEventID returns an inbox row by source service and external event id.
	GetBySourceAndEventID(ctx context.Context, sourceService string, eventID string) (model.Inbox, error)
}

// InboxConnQueryer exposes inbox operations on a connection.
type InboxConnQueryer interface {
	// InboxQueryer provides shared inbox query operations.
	InboxQueryer
}

// InboxTxQueryer exposes transaction-scoped inbox operations.
type InboxTxQueryer interface {
	// InboxQueryer provides shared inbox query operations.
	InboxQueryer

	// AcquireForProcessing inserts or locks an inbox row and reports how it should be handled.
	AcquireForProcessing(
		ctx context.Context,
		event model.InboxEvent,
		now time.Time,
	) (model.Inbox, model.InboxAcquireResult, error)

	// MarkProcessed marks an inbox row processed.
	MarkProcessed(
		ctx context.Context,
		id model.InboxID,
		processedAt time.Time,
		updatedAt time.Time,
	) (model.Inbox, error)

	// MarkReconciliationRequired marks an inbox row reconciliation_required.
	MarkReconciliationRequired(
		ctx context.Context,
		id model.InboxID,
		failureCode *string,
		failureMessage *string,
		processedAt time.Time,
		updatedAt time.Time,
	) (model.Inbox, error)

	// MarkFailed marks an inbox row failed.
	MarkFailed(
		ctx context.Context,
		id model.InboxID,
		failureCode string,
		failureMessage string,
		failedAt time.Time,
		updatedAt time.Time,
	) (model.Inbox, error)

	// RecordPayloadIdentityConflict persists a payload identity conflict without mutating terminal inbox rows.
	RecordPayloadIdentityConflict(
		ctx context.Context,
		event model.InboxPayloadConflictEvent,
		now time.Time,
	) (model.InboxPayloadConflict, bool, error)
}

// Inbox persists durable inbound integration events.
type Inbox interface {
	// Conn binds the repository to a non-transactional connection scope.
	Conn(Conn) InboxConnQueryer

	// Tx binds the repository to a transaction scope.
	Tx(Tx) InboxTxQueryer
}
