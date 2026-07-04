package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// PolicyAssignmentImportQueryer defines durable bulk-import reads.
type PolicyAssignmentImportQueryer interface {
	GetByID(ctx context.Context, id model.PolicyAssignmentImportID) (model.PolicyAssignmentImport, error)
}

// PolicyAssignmentImportConnQueryer exposes import reads on a connection.
type PolicyAssignmentImportConnQueryer interface{ PolicyAssignmentImportQueryer }

// PolicyAssignmentImportTxQueryer exposes durable batch/item mutations.
type PolicyAssignmentImportTxQueryer interface {
	PolicyAssignmentImportQueryer

	Create(
		ctx context.Context,
		importType string,
		sourceFile string,
		totalCount int,
		createdBy string,
		createdAt time.Time,
	) (model.PolicyAssignmentImport, error)

	CreateItem(
		ctx context.Context,
		importID model.PolicyAssignmentImportID,
		rowNumber int,
		subjectID string,
		createdAt time.Time,
	) (model.PolicyAssignmentImportItem, error)

	MarkItemSucceeded(
		ctx context.Context,
		id model.PolicyAssignmentImportItemID,
		updatedAt time.Time,
	) (model.PolicyAssignmentImportItem, error)

	MarkItemFailed(
		ctx context.Context,
		id model.PolicyAssignmentImportItemID,
		errorCode string,
		errorMessage string,
		updatedAt time.Time,
	) (model.PolicyAssignmentImportItem, error)

	Complete(
		ctx context.Context,
		id model.PolicyAssignmentImportID,
		status model.PolicyAssignmentImportStatus,
		successCount int,
		failureCount int,
		completedAt time.Time,
	) (model.PolicyAssignmentImport, error)
}

// PolicyAssignmentImport persists durable bulk-import execution evidence.
type PolicyAssignmentImport interface {
	Conn(Conn) PolicyAssignmentImportConnQueryer
	Tx(Tx) PolicyAssignmentImportTxQueryer
}
