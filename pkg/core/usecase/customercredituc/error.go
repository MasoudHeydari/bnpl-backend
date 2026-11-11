package customercredituc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// Public field name for the gateway-injected authenticated mobile. Kept as a
// wire-oriented string so core does not import the REST header package.
const authMobileNumberField = "x-auth-mobile-number"

// authMobileNumberRequiredError returns a public validation error when the
// gateway-injected x-auth-mobile-number header is missing. Clients must not
// invent this header; Envoy injects it from JWT claim auth_mobile_number.
func authMobileNumberRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.auth_mobile_number_required",
		nil,
		cerr.Details{
			Fields: []string{authMobileNumberField},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.auth_mobile_number_required",
					Fields:     []string{authMobileNumberField},
				},
			},
		},
	)
}

// authMobileNumberInvalidError returns a public validation error when the
// gateway mobile is not canonical E.164.
func authMobileNumberInvalidError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.auth_mobile_number_invalid",
		nil,
		cerr.Details{
			Fields: []string{authMobileNumberField},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.auth_mobile_number_invalid",
					Fields:     []string{authMobileNumberField},
				},
			},
		},
	)
}

// customerReferenceNotFoundError returns a public not-found when Customer
// Reference Service has no customer for the authenticated mobile.
func customerReferenceNotFoundError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeResourceNotFound,
		"error.customer_reference_not_found",
		cause,
		cerr.Details{
			Fields: []string{authMobileNumberField},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_FOUND",
					MessageKey: "error.customer_reference_not_found",
					Fields:     []string{authMobileNumberField},
				},
			},
		},
	)
}

// customerReferenceNotActiveError returns a public forbidden when CRS status is
// not ACTIVE (for example suspended).
func customerReferenceNotActiveError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.customer_reference_not_active",
		cause,
		cerr.Details{
			Fields: []string{authMobileNumberField},
			Violations: []cerr.Violation{
				{
					Code:       "FORBIDDEN",
					MessageKey: "error.customer_reference_not_active",
					Fields:     []string{authMobileNumberField},
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
