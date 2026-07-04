package repo

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// PolicyAssignmentQueryer defines runtime policy-assignment lookups.
type PolicyAssignmentQueryer interface {
	// GetActive returns the active assignment for subject/family at now.
	GetActive(
		ctx context.Context,
		subjectType model.PolicySubjectType,
		subjectID string,
		family model.PolicyFamily,
		now time.Time,
	) (model.PolicyAssignment, error)
}

// PolicyAssignmentConnQueryer exposes read operations on a connection.
type PolicyAssignmentConnQueryer interface{ PolicyAssignmentQueryer }

// PolicyAssignmentTxQueryer exposes assignment-history mutations in a transaction.
type PolicyAssignmentTxQueryer interface {
	PolicyAssignmentQueryer

	// LockActive returns and row-locks the current active assignment.
	LockActive(
		ctx context.Context,
		subjectType model.PolicySubjectType,
		subjectID string,
		family model.PolicyFamily,
		now time.Time,
	) (model.PolicyAssignment, error)

	// Create appends a new assignment-history interval.
	Create(
		ctx context.Context,
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
	) (model.PolicyAssignment, error)

	// CloseActive closes an exact active assignment interval without deleting history.
	CloseActive(
		ctx context.Context,
		id model.PolicyAssignmentID,
		validUntil time.Time,
		updatedAt time.Time,
	) (model.PolicyAssignment, error)
}

// PolicyAssignment persists generic subject-to-policy assignment history.
type PolicyAssignment interface {
	Conn(Conn) PolicyAssignmentConnQueryer
	Tx(Tx) PolicyAssignmentTxQueryer
}
