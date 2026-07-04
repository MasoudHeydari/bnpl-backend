package merchantterminaluc

import (
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// validationError builds a single-field public validation error.
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

// publicError builds a public error without field details.
func publicError(code cerr.Code, messageKey cerr.MessageKey, cause error) error {
	return cerr.NewPublic(
		code,
		messageKey,
		cause,
		cerr.Details{
			Fields:     []string{},
			Violations: []cerr.Violation{},
		},
	)
}

func merchantIDRequiredError() error {
	return validationError("REQUIRED", "merchant_id", "error.merchant_id_required")
}

func idempotencyKeyRequiredError() error {
	return validationError("REQUIRED", "idempotency_key", "error.terminal_idempotency_key_required")
}

func idempotencyKeyTooLongError() error {
	return validationError("MAX_LENGTH", "idempotency_key", "error.terminal_idempotency_key_too_long")
}

func credentialIDRequiredError() error {
	return validationError("REQUIRED", "credential_id", "error.terminal_credential_id_required")
}

func gracePeriodInvalidError() error {
	return validationError("INVALID", "grace_period", "error.terminal_grace_period_invalid")
}

func gracePeriodTooLongError() error {
	return validationError("MAX_LENGTH", "grace_period", "error.terminal_grace_period_too_long")
}

// invalidCredentialsError is the single public failure of Authenticate. It is
// identical for unknown terminals, inactive terminals, malformed keys, and wrong
// keys so a caller cannot probe which part was wrong.
func invalidCredentialsError(cause error) error {
	return publicError(cerr.CodeUnauthorized, "error.invalid_terminal_credentials", cause)
}

func merchantProfileNotFoundError(cause error) error {
	return publicError(cerr.CodeResourceNotFound, "error.merchant_profile_not_found", cause)
}

func merchantNotActiveError(cause error) error {
	return publicError(cerr.CodeConflict, "error.merchant_is_not_active_for_terminal_provisioning", cause)
}

func terminalNotFoundError(cause error) error {
	return publicError(cerr.CodeResourceNotFound, "error.merchant_terminal_not_found", cause)
}

func terminalNotActiveError(cause error) error {
	return publicError(cerr.CodeConflict, "error.merchant_terminal_is_not_active", cause)
}

func terminalAlreadyProvisionedError(cause error) error {
	return publicError(cerr.CodeConflict, "error.merchant_terminal_already_provisioned", cause)
}

func issuanceReplayUnavailableError(cause error) error {
	return publicError(cerr.CodeIdempotencyConflict, "error.terminal_issuance_replay_unavailable", cause)
}

// idempotencyConflictError reports an Idempotency-Key reused for a request whose
// semantics differ from the one that first used the key.
func idempotencyConflictError(cause error) error {
	return publicError(cerr.CodeIdempotencyConflict, "error.terminal_idempotency_key_conflict", cause)
}

func provisionContendedError(cause error) error {
	return publicError(
		cerr.CodeConflict,
		"error.terminal_could_not_be_provisioned_due_to_a_concurrent_conflict",
		cause,
	)
}

func credentialNotFoundError(cause error) error {
	return publicError(cerr.CodeResourceNotFound, "error.terminal_credential_not_found", cause)
}

func credentialNotRetirableError(cause error) error {
	return publicError(cerr.CodeConflict, "error.terminal_credential_cannot_be_retired", cause)
}

// credentialRotationInProgressError refuses a rotation while the previous
// credential is still inside its grace period.
func credentialRotationInProgressError(cause error) error {
	const messageKey cerr.MessageKey = "error.terminal_credential_rotation_in_progress"

	return cerr.NewPublic(
		cerr.CodeConflict,
		messageKey,
		cause,
		cerr.Details{
			Fields: []string{},
			Violations: []cerr.Violation{
				{
					Code:       "CREDENTIAL_ROTATION_IN_PROGRESS",
					MessageKey: messageKey,
					Fields:     []string{},
				},
			},
		},
	)
}
