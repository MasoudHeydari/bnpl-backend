package customercredituc

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Observer records operational flow checkpoints for customer credit
// assessment, initial limit assignment, and summary reads. Implementations
// must not modify domain state or return errors that affect business
// execution.
//
// Success checkpoints are emitted only after the corresponding database
// transaction committed. Idempotent replays are reported through
// LimitAlreadyAssigned rather than as a new assignment.
type Observer interface {
	// CustomerResolved records an ACTIVE Customer Reference Service resolution.
	CustomerResolved(ctx context.Context, externalCustomerID model.ExternalCustomerID)

	// AssessmentStarted records the start of an initial limit credit assessment.
	AssessmentStarted(ctx context.Context, customerID model.CustomerID)

	// AssessmentCompleted records the assessment outcome before persistence.
	AssessmentCompleted(ctx context.Context, customerID model.CustomerID, outcome model.DecisionOutcome)

	// LimitAssignmentAttempted records the start of an initial limit assignment.
	LimitAssignmentAttempted(ctx context.Context, externalSubjectID model.ExternalSubjectID)

	// LimitAssigned records a committed new credit limit assignment.
	LimitAssigned(ctx context.Context, customerID model.CustomerID, decisionID model.DecisionID)

	// LimitAlreadyAssigned records an assignment request satisfied by an
	// existing usable limit or an idempotent replay of a prior assignment.
	LimitAlreadyAssigned(ctx context.Context, customerID model.CustomerID, decisionID model.DecisionID)

	// LimitAssignmentDenied records a committed (or replayed) denied assignment.
	LimitAssignmentDenied(
		ctx context.Context,
		customerID model.CustomerID,
		decisionID model.DecisionID,
		reasonCode string,
	)

	// AvailabilityRetrieved records a credit summary read with its public status.
	AvailabilityRetrieved(ctx context.Context, customerID model.CustomerID, status string)

	// UsageRestricted records that new-credit usage is restricted for a customer.
	UsageRestricted(ctx context.Context, customerID model.CustomerID, reasonCode string)
}
