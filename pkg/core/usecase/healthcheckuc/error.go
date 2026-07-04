package healthcheckuc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// shutdownTimedOutError returns a public service-unavailable error when
// graceful shutdown has exceeded its deadline.
//
// Detected from local lifecycle state, so cause is intentionally nil.
func shutdownTimedOutError() error {
	return cerr.NewPublic(
		cerr.CodeServiceUnavailable,
		"error.application_shutdown_timed_out",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "SERVICE_UNAVAILABLE",
					MessageKey: "error.application_shutdown_timed_out",
					Fields:     []string{},
				},
			},
		},
	)
}

// applicationDrainingError returns a public service-unavailable error when the
// application is draining and must not accept new work.
//
// Detected from local lifecycle state, so cause is intentionally nil.
func applicationDrainingError() error {
	return cerr.NewPublic(
		cerr.CodeServiceUnavailable,
		"error.application_is_draining",
		nil,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "SERVICE_UNAVAILABLE",
					MessageKey: "error.application_is_draining",
					Fields:     []string{},
				},
			},
		},
	)
}
