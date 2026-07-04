package policyassignmentimportrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

var _ repo.PolicyAssignmentImport = (*Repo)(nil)

type Repo struct{}

func New() *Repo { return &Repo{} }
func (r *Repo) Conn(c repo.Conn) repo.PolicyAssignmentImportConnQueryer {
	return connQueryer{Conn: postgres.MustConn(c)}
}
func (r *Repo) Tx(tx repo.Tx) repo.PolicyAssignmentImportTxQueryer {
	return txQueryer{Tx: postgres.MustTx(tx)}
}

type connQueryer struct{ *postgres.Conn }

func (q connQueryer) GetByID(ctx context.Context, id model.PolicyAssignmentImportID) (model.PolicyAssignmentImport, error) {
	return GetByID(ctx, q.Conn, id)
}

type txQueryer struct{ *postgres.Tx }

func (q txQueryer) GetByID(ctx context.Context, id model.PolicyAssignmentImportID) (model.PolicyAssignmentImport, error) {
	return GetByID(ctx, q.Tx, id)
}
func (q txQueryer) Create(
	ctx context.Context,
	importType string,
	sourceFile string,
	totalCount int,
	createdBy string,
	createdAt time.Time,
) (model.PolicyAssignmentImport, error) {
	return Create(
		ctx,
		q.Tx,
		importType,
		sourceFile,
		totalCount,
		createdBy,
		createdAt,
	)
}
func (q txQueryer) CreateItem(
	ctx context.Context,
	importID model.PolicyAssignmentImportID,
	rowNumber int,
	subjectID string,
	createdAt time.Time,
) (model.PolicyAssignmentImportItem, error) {
	return CreateItem(
		ctx,
		q.Tx,
		importID,
		rowNumber,
		subjectID,
		createdAt,
	)
}
func (q txQueryer) MarkItemSucceeded(ctx context.Context, id model.PolicyAssignmentImportItemID, updatedAt time.Time) (model.PolicyAssignmentImportItem, error) {
	return MarkItemSucceeded(ctx, q.Tx, id, updatedAt)
}
func (q txQueryer) MarkItemFailed(
	ctx context.Context,
	id model.PolicyAssignmentImportItemID,
	errorCode string,
	errorMessage string,
	updatedAt time.Time,
) (model.PolicyAssignmentImportItem, error) {
	return MarkItemFailed(
		ctx,
		q.Tx,
		id,
		errorCode,
		errorMessage,
		updatedAt,
	)
}
func (q txQueryer) Complete(
	ctx context.Context,
	id model.PolicyAssignmentImportID,
	status model.PolicyAssignmentImportStatus,
	successCount int,
	failureCount int,
	completedAt time.Time,
) (model.PolicyAssignmentImport, error) {
	return Complete(
		ctx,
		q.Tx,
		id,
		status,
		successCount,
		failureCount,
		completedAt,
	)
}
