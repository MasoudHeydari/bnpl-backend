package policyassignmentrp

import (
	"context"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const policyAssignmentColumns = `
	id,
	subject_type,
	subject_id,
	policy_family,
	policy_id,
	policy_version,
	reason,
	source,
	valid_from,
	valid_until,
	created_by,
	created_at,
	updated_at
`

func GetActive[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	const query = `SELECT` + policyAssignmentColumns + `
FROM policy_assignments
WHERE subject_type = $1
  AND subject_id = $2
  AND policy_family = $3
  AND valid_from <= $4
  AND (valid_until IS NULL OR valid_until > $4)
ORDER BY valid_from DESC, created_at DESC
LIMIT 1`

	return postgres.QueryOne[model.PolicyAssignment](
		ctx,
		q,
		"get active policy assignment",
		query,
		subjectType,
		subjectID,
		family,
		now,
	)
}

func LockActive[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	now time.Time,
) (model.PolicyAssignment, error) {
	const query = `SELECT` + policyAssignmentColumns + `
FROM policy_assignments
WHERE subject_type = $1
  AND subject_id = $2
  AND policy_family = $3
  AND valid_from <= $4
  AND (valid_until IS NULL OR valid_until > $4)
ORDER BY valid_from DESC, created_at DESC
LIMIT 1
FOR UPDATE`

	return postgres.QueryOne[model.PolicyAssignment](
		ctx,
		q,
		"lock active policy assignment",
		query,
		subjectType,
		subjectID,
		family,
		now,
	)
}

func Create[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	subjectType model.PolicySubjectType,
	subjectID string,
	family model.PolicyFamily,
	policyID string,
	policyVersion string,
	reason string,
	source string,
	validFrom time.Time,
	createdBy string,
	createdAt time.Time,
	updatedAt time.Time,
) (model.PolicyAssignment, error) {
	const op = "policyassignmentrp.Create"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"valid_from": validFrom,
		"created_at": createdAt,
		"updated_at": updatedAt,
	}); err != nil {
		return model.PolicyAssignment{}, err
	}

	const query = `
INSERT INTO policy_assignments (
	subject_type, subject_id, policy_family, policy_id, policy_version,
	reason, source, valid_from, created_by, created_at, updated_at
)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
RETURNING` + policyAssignmentColumns

	row, err := postgres.QueryOne[model.PolicyAssignment](
		ctx,
		q,
		"create policy assignment",
		query,
		subjectType,
		subjectID,
		family,
		policyID,
		policyVersion,
		reason,
		source,
		validFrom,
		createdBy,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return model.PolicyAssignment{}, fmt.Errorf("%s: %w", op, err)
	}

	return row, nil
}

func CloseActive[Q postgres.Queryer](
	ctx context.Context,
	q Q,
	id model.PolicyAssignmentID,
	validUntil time.Time,
	updatedAt time.Time,
) (model.PolicyAssignment, error) {
	const op = "policyassignmentrp.CloseActive"

	if err := postgres.RequiredTimes(map[string]time.Time{
		"valid_until": validUntil,
		"updated_at":  updatedAt,
	}); err != nil {
		return model.PolicyAssignment{}, err
	}

	const query = `
UPDATE policy_assignments
SET valid_until = $2, updated_at = $3
WHERE id = $1
  AND valid_until IS NULL
RETURNING` + policyAssignmentColumns

	row, err := postgres.QueryOne[model.PolicyAssignment](
		ctx,
		q,
		"close active policy assignment",
		query,
		id,
		validUntil,
		updatedAt,
	)
	if err != nil && cerr.IsCode(err, cerr.CodeResourceNotFound) {
		return model.PolicyAssignment{}, cerr.Conflict(
			fmt.Errorf("%s: assignment is not active", op),
		)
	}

	return row, err
}
