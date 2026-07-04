package financialcorepaymentreconciliationuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

func eventIDRequiredError() error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_id_required",
		nil,
		cerr.Details{
			Fields: []string{"event_id"},
			Violations: []cerr.Violation{
				{
					Code:       "REQUIRED",
					MessageKey: "error.event_id_required",
					Fields:     []string{"event_id"},
				},
			},
		},
	)
}

func unsupportedEventTypeError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_type_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"event_type"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_type_not_supported",
					Fields:     []string{"event_type"},
				},
			},
		},
	)
}

func unsupportedEventVersionError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.event_version_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"event_version"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.event_version_not_supported",
					Fields:     []string{"event_version"},
				},
			},
		},
	)
}

func unsupportedSourceServiceError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.source_service_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"source_service"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.source_service_not_supported",
					Fields:     []string{"source_service"},
				},
			},
		},
	)
}

func unsupportedReconciliationResultStatusError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.reconciliation_result_status_not_supported",
		cause,
		cerr.Details{
			Fields: []string{"status"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.reconciliation_result_status_not_supported",
					Fields:     []string{"status"},
				},
			},
		},
	)
}

func inboxNotReceivableConflictError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.inbox_event_is_not_receivable",
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CONFLICT",
					MessageKey: "error.inbox_event_is_not_receivable",
					Fields:     []string{},
				},
			},
		},
	)
}
