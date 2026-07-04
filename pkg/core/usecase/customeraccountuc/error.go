package customeraccountuc

import (
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// contractIDRequiredError returns a public validation error when contract_id
// is missing from the use case input.
//
// Detected locally from input, so cause is intentionally nil.
func contractIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.contract_id_required",
		nil,
		cerr.Details{
			Fields: []string{"contract_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.contract_id_required",
					Fields:     []string{"contract_id"},
				},
			},
		},
	)
}

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

// contractNotFoundError translates a missing customer (or equivalent) into the
// public "contract not found" response for this endpoint.
//
// Callers must not learn whether the customer profile exists; the original
// cause is preserved for logging/tracing only and Fields stay empty because
// this is a resource-level not-found, not a field validation failure.
func contractNotFoundError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeContractNotFound,
		"error.contract_not_found",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_FOUND",
					MessageKey: "error.contract_not_found",
					Fields:     []string{},
				},
			},
		},
	)
}

// invalidPaginationError translates a pagination.Validate failure into a
// client-safe public error.
//
// The validate error is preserved as cause for diagnostics; its raw text must
// never become a public localized message.
func invalidPaginationError(cause error) error {
	fields := paginationViolationFields(cause)
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.pagination_parameters_are_invalid",
		cause,
		cerr.Details{
			Fields: fields,
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.pagination_parameters_are_invalid",
					Fields:     fields,
				},
			},
		},
	)
}

func paginationViolationFields(cause error) []string {
	if cause == nil {
		return []string{}
	}

	msg := strings.ToLower(cause.Error())
	var fields []string
	if strings.Contains(msg, "after_id") {
		fields = append(fields, "after_id")
	}

	if strings.Contains(msg, "after_created_at") {
		fields = append(fields, "after_created_at")
	}

	if strings.Contains(msg, "after_due_at") {
		fields = append(fields, "after_due_at")
	}

	if strings.Contains(msg, "limit") {
		fields = append(fields, "limit")
	}

	if strings.Contains(msg, "page") && !strings.Contains(msg, "pagination") {
		fields = append(fields, "page")
	}

	if len(fields) == 0 {
		return []string{}
	}

	return fields
}
