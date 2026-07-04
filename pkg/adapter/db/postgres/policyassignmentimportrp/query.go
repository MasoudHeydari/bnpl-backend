package policyassignmentimportrp

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const importColumns = `
	id, import_type, source_file, status, total_count, success_count,
	failure_count, created_by, created_at, completed_at
`
const itemColumns = `
	id, import_id, row_number, subject_id, status, error_code,
	error_message, created_at, updated_at
`

func GetByID[Q postgres.Queryer](ctx context.Context, q Q, id model.PolicyAssignmentImportID) (model.PolicyAssignmentImport, error) {
	const query = `SELECT` + importColumns + `FROM policy_assignment_imports WHERE id = $1`
	return postgres.QueryOne[model.PolicyAssignmentImport](
		ctx,
		q,
		"get policy assignment import",
		query,
		id,
	)
}

func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	importType string,
	sourceFile string,
	totalCount int,
	createdBy string,
	createdAt time.Time,
) (model.PolicyAssignmentImport, error) {
	if err := postgres.RequiredTime("created_at", createdAt); err != nil {
		return model.PolicyAssignmentImport{}, err
	}

	const query = `
INSERT INTO policy_assignment_imports (
	import_type, source_file, status, total_count, success_count, failure_count, created_by, created_at
)
VALUES ($1,$2,$3,$4,0,0,$5,$6)
RETURNING` + importColumns
	return postgres.QueryOne[model.PolicyAssignmentImport](
		ctx,
		q,
		"create policy assignment import",
		query,
		importType,
		sourceFile,
		model.ProcessingPolicyAssignmentImportStatus,
		totalCount,
		createdBy,
		createdAt,
	)
}

func CreateItem[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	importID model.PolicyAssignmentImportID,
	rowNumber int,
	subjectID string,
	createdAt time.Time,
) (model.PolicyAssignmentImportItem, error) {
	if err := postgres.RequiredTime("created_at", createdAt); err != nil {
		return model.PolicyAssignmentImportItem{}, err
	}

	const query = `
INSERT INTO policy_assignment_import_items (
	import_id, row_number, subject_id, status, created_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$5)
RETURNING` + itemColumns
	return postgres.QueryOne[model.PolicyAssignmentImportItem](
		ctx,
		q,
		"create policy assignment import item",
		query,
		importID,
		rowNumber,
		subjectID,
		model.PendingPolicyAssignmentImportItemStatus,
		createdAt,
	)
}

func MarkItemSucceeded[Q postgres.Queryer](ctx context.Context, q Q, id model.PolicyAssignmentImportItemID, updatedAt time.Time) (model.PolicyAssignmentImportItem, error) {
	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.PolicyAssignmentImportItem{}, err
	}

	const query = `
UPDATE policy_assignment_import_items
SET status = $2, error_code = NULL, error_message = NULL, updated_at = $3
WHERE id = $1 AND status = $4
RETURNING` + itemColumns
	return postgres.QueryOne[model.PolicyAssignmentImportItem](
		ctx,
		q,
		"mark policy assignment import item succeeded",
		query,
		id,
		model.SucceededPolicyAssignmentImportItemStatus,
		updatedAt,
		model.PendingPolicyAssignmentImportItemStatus,
	)
}

func MarkItemFailed[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PolicyAssignmentImportItemID,
	errorCode string,
	errorMessage string,
	updatedAt time.Time,
) (model.PolicyAssignmentImportItem, error) {
	if err := postgres.RequiredTime("updated_at", updatedAt); err != nil {
		return model.PolicyAssignmentImportItem{}, err
	}

	const query = `
UPDATE policy_assignment_import_items
SET status = $2, error_code = $3, error_message = $4, updated_at = $5
WHERE id = $1 AND status = $6
RETURNING` + itemColumns
	return postgres.QueryOne[model.PolicyAssignmentImportItem](
		ctx,
		q,
		"mark policy assignment import item failed",
		query,
		id,
		model.FailedPolicyAssignmentImportItemStatus,
		errorCode,
		errorMessage,
		updatedAt,
		model.PendingPolicyAssignmentImportItemStatus,
	)
}

func Complete[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PolicyAssignmentImportID,
	status model.PolicyAssignmentImportStatus,
	successCount int,
	failureCount int,
	completedAt time.Time,
) (model.PolicyAssignmentImport, error) {
	if err := postgres.RequiredTime("completed_at", completedAt); err != nil {
		return model.PolicyAssignmentImport{}, err
	}

	const query = `
UPDATE policy_assignment_imports
SET status = $2, success_count = $3, failure_count = $4, completed_at = $5
WHERE id = $1 AND status = $6
RETURNING` + importColumns
	return postgres.QueryOne[model.PolicyAssignmentImport](
		ctx,
		q,
		"complete policy assignment import",
		query,
		id,
		status,
		successCount,
		failureCount,
		completedAt,
		model.ProcessingPolicyAssignmentImportStatus,
	)
}
