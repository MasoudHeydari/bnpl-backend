package merchantuc

import (
	"strings"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// merchantIDRequiredError returns a public validation error when merchant_id
// is missing from the use case input.
//
// There is no underlying technical failure in this case; the use case itself
// has detected invalid input, so the error intentionally has no cause.
func merchantIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.merchant_id_required",
		nil,
		cerr.Details{
			Fields: []string{"merchant_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.merchant_id_required",
					Fields:     []string{"merchant_id"},
				},
			},
		},
	)
}

// merchantProfileNotFoundError translates a lower-level not-found failure into
// the public application error exposed by this use case.
//
// The original cause is preserved for logging, tracing, and errors.Is/errors.As
// inspection, but it must never be serialized into the public API response.
func merchantProfileNotFoundError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeResourceNotFound,
		"error.merchant_profile_not_found",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "NOT_FOUND",
					MessageKey: "error.no_bnpl_merchant_profile_exists_for_this_merchant",
					Fields:     []string{},
				},
			},
		},
	)
}

// invalidPaginationModeError returns a public validation error when the
// requested pagination mode is not keyset or page.
//
// Detected locally from use case input, so cause is intentionally nil.
func invalidPaginationModeError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.pagination_mode_must_be_keyset_or_page",
		nil,
		cerr.Details{
			Fields: []string{"pagination"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.pagination_mode_must_be_keyset_or_page",
					Fields:     []string{"pagination"},
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
