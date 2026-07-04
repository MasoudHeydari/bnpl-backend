package otp

import (
	"errors"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func validationError(code string, field string, messageKey cerr.MessageKey) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		messageKey,
		nil,
		cerr.Details{
			Fields: []string{field},
			Violations: []cerr.Violation{
				{
					Code:       code,
					MessageKey: messageKey,
					Fields:     []string{field},
				},
			},
		},
	)
}

func otpRequiredError() error {
	return validationError("REQUIRED", "otp", "error.otp_required")
}

func otpInvalidError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeInvalidArgument,
		"error.otp_invalid",
		cause,
		cerr.Details{
			Fields: []string{"otp"},
			Violations: []cerr.Violation{
				{
					Code:       "INVALID",
					MessageKey: "error.otp_invalid",
					Fields:     []string{"otp"},
				},
			},
		},
	)
}

func otpExpiredError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.otp_expired",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

func otpLockedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeForbidden,
		"error.otp_locked",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

func otpNotRequestedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.otp_not_requested",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

func otpAlreadyConsumedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.otp_already_consumed",
		cause,
		cerr.Details{
			Fields:     []string{"otp"},
			Violations: []cerr.Violation{},
		},
	)
}

func otpIssuanceWindowActiveError(cause error, challenge model.OTPChallenge, now time.Time) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.otp_issuance_window_active",
		cause,
		cerr.Details{
			Fields:            []string{},
			Violations:        []cerr.Violation{},
			RetryAfterSeconds: secondsUntilNextIssue(challenge, now),
		},
	)
}

func otpIssuanceNotAllowedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		"error.otp_issuance_not_allowed",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// contextInvalidatedMessageKey identifies an OTP challenge whose context
// version no longer matches the application binding (for example a refreshed
// checkout offer generation). Callers map this to their reconfirm contract.
const contextInvalidatedMessageKey cerr.MessageKey = "error.otp_context_invalidated"

func otpContextInvalidatedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeConflict,
		contextInvalidatedMessageKey,
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

// IsContextInvalidated reports whether err is an OTP context-version mismatch.
func IsContextInvalidated(err error) bool {
	var appErr *cerr.Error
	if !errors.As(err, &appErr) {
		return false
	}

	return appErr.MessageKey == contextInvalidatedMessageKey
}

func otpSendCooldownError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.otp_send_cooldown",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

func otpSendLimitError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.otp_send_limit_reached",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

func otpRateLimitedError(cause error) error {
	return cerr.NewPublic(
		cerr.CodeTooManyRequests,
		"error.otp_rate_limited",
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}
