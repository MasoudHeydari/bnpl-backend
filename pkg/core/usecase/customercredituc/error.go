package customercredituc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// identitySubjectIDRequiredError returns a public validation error when the
// authenticated Identity Service subject (IdentitySubjectID) is missing from
// use-case input. REST maps this from x-auth-subject-id; clients must not
// supply an internal customer_id.
//
// There is no underlying technical failure; the use case detected invalid
// input, so the error intentionally has no cause.
func identitySubjectIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.identity_subject_id_required",
		nil,
		cerr.Details{
			Fields: []string{"identity_subject_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.identity_subject_id_required",
					Fields:     []string{"identity_subject_id"},
				},
			},
		},
	)
}

// creditAssessmentUnavailableError returns a public service-unavailable error
// when the credit assessment dependency returns an unusable response.
//
// cause retains diagnostic detail for logging/tracing and must never be copied
// into Message or Details. Fields stay empty because this is a dependency
// failure, not a client field validation issue.
func creditAssessmentUnavailableError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeServiceUnavailable,
		"error.credit_assessment_is_temporarily_unavailable",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "SERVICE_UNAVAILABLE",
					MessageKey: "error.credit_assessment_is_temporarily_unavailable",
					Fields:     []string{},
				},
			},
		},
	)
}

// idempotencyConflictError returns a public conflict when an idempotency key
// was reused with a different request fingerprint.
//
// cause is preserved for diagnostics; public Details only identify the
// idempotency_key field.
func idempotencyConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeIdempotencyConflict,
		"error.idempotency_key_conflicts_with_an_existing_request",
		cause,
		cerr.Details{
			Fields: []string{"idempotency_key"},
			Violations: []cerr.Violation{
				{
					Code:       "IDEMPOTENCY_CONFLICT",
					MessageKey: "error.idempotency_key_conflicts_with_an_existing_request",
					Fields:     []string{"idempotency_key"},
				},
			},
		},
	)
}

// customerProfileChangedConflictError returns a public conflict when the
// customer profile changed during initial limit assignment.
//
// Fields stay empty because this is a resource-state conflict, not a field
// validation failure. cause is preserved for diagnostics only.
func customerProfileChangedConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.request_conflicts_with_the_current_resource_state",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.request_conflicts_with_the_current_resource_state",
					Fields:     []string{},
				},
			},
		},
	)
}

// initialLimitAssignmentConflictError returns a public conflict when a
// concurrent assignment race cannot be resolved to a current limit.
//
// Fields stay empty because this is a resource-state conflict.
func initialLimitAssignmentConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.request_conflicts_with_the_current_resource_state",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.request_conflicts_with_the_current_resource_state",
					Fields:     []string{},
				},
			},
		},
	)
}
