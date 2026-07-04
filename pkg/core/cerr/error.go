package cerr

import (
	"errors"
	"fmt"
)

// Code identifies a stable error category.
//
// Codes are part of the public application contract and may be consumed by
// transports, clients, and monitoring systems. A transport is responsible for
// mapping a Code to its protocol-specific status (for example HTTP or gRPC).
type Code string

// MessageKey identifies a stable, language-neutral message in the presentation catalog.
//
// The core carries only semantic keys. Transport adapters decide how those keys
// are localized for a particular protocol/request.
type MessageKey string

const (
	// CodeValidationFailed indicates that business validation rules failed.
	CodeValidationFailed Code = "VALIDATION_FAILED"

	// CodeInvalidArgument indicates invalid client input.
	CodeInvalidArgument Code = "INVALID_ARGUMENT"

	// CodeResourceNotFound indicates that the requested resource does not exist.
	CodeResourceNotFound Code = "RESOURCE_NOT_FOUND"

	// CodeDecisionNotFound indicates that the requested decision does not exist.
	CodeDecisionNotFound Code = "DECISION_NOT_FOUND"

	// CodeDecisionRejected indicates that a decision exists
	// but its outcome does not permit the requested operation.
	CodeDecisionRejected Code = "DECISION_REJECTED"

	// CodeDecisionExpired indicates that a decision exists
	// but is no longer valid for use.
	CodeDecisionExpired Code = "DECISION_EXPIRED"

	// CodeContractNotFound indicates that the requested contract does not exist.
	CodeContractNotFound Code = "CONTRACT_NOT_FOUND"

	// CodeContractAlreadyExists indicates that a contract already exists
	// for the supplied business identifier.
	CodeContractAlreadyExists Code = "CONTRACT_ALREADY_EXISTS"

	// CodeInstallmentNotFound indicates that the requested installment does not exist.
	CodeInstallmentNotFound Code = "INSTALLMENT_NOT_FOUND"

	// CodeIdempotencyConflict indicates that an idempotency key
	// has already been used with a different request payload.
	CodeIdempotencyConflict Code = "IDEMPOTENCY_CONFLICT"

	// CodePaymentProviderError indicates a failure returned by an external payment provider.
	CodePaymentProviderError Code = "PAYMENT_PROVIDER_ERROR"

	// CodeUnauthorized indicates that authentication is missing or invalid.
	CodeUnauthorized Code = "UNAUTHORIZED"

	// CodeForbidden indicates that the caller is authenticated
	// but not permitted to perform the requested operation.
	CodeForbidden Code = "FORBIDDEN"

	// CodeInternalError indicates an unexpected server-side failure.
	CodeInternalError Code = "INTERNAL_ERROR"

	// CodeConflict indicates that the requested operation conflicts with
	// the current state of the target resource.
	CodeConflict Code = "CONFLICT"

	// CodeServiceUnavailable indicates that a required dependency is
	// temporarily unavailable.
	CodeServiceUnavailable Code = "SERVICE_UNAVAILABLE"

	// CodePayloadTooLarge indicates that the request body exceeds the
	// transport safety limit configured by the serving application.
	CodePayloadTooLarge Code = "PAYLOAD_TOO_LARGE"

	// CodeTooManyRequests indicates that the caller exceeded a rate or attempt
	// budget for the requested operation.
	CodeTooManyRequests Code = "TOO_MANY_REQUESTS"
)

// Details is the transport-agnostic, client-safe structured context associated
// with an application error.
//
// Details must never contain secrets, SQL/driver diagnostics, stack traces,
// internal host names, provider payloads, or any other infrastructure detail.
type Details struct {
	// Fields lists client-safe field names implicated by the error.
	Fields []string `json:"fields"`

	// Violations lists structured field-level or rule-level failures.
	Violations []Violation `json:"violations"`

	// RetryAfterSeconds suggests how long clients should wait before retrying,
	// when the failure is expected to be transient.
	RetryAfterSeconds *int `json:"retry_after_seconds,omitempty"`
}

// MessageParams contains language-neutral template data used by a transport
// localizer. Values must be client-safe because they may influence public text.
type MessageParams map[string]any

// Violation describes a client-safe field-level or rule-level failure.
type Violation struct {
	// Code is a stable machine-readable identifier for this violation.
	Code string `json:"code"`

	// MessageKey selects the language-neutral template for this violation.
	MessageKey MessageKey `json:"-"`

	// MessageParams supplies language-neutral template data for localization.
	MessageParams MessageParams `json:"-"`

	// Fields lists client-safe field names implicated by this violation.
	Fields []string `json:"fields"`
}

// Error represents an application-level error.
//
// MessageKey and Details are the only client-facing semantic parts of Error.
// The underlying cause is deliberately private so transport serializers cannot
// accidentally expose infrastructure diagnostics by reading cause.Error(). The
// cause remains available to logging/tracing through Error(), errors.Is/errors.As,
// and Unwrap.
type Error struct {
	// Code is the stable application error classification.
	Code Code

	// MessageKey selects the language-neutral public message template.
	MessageKey MessageKey

	// MessageParams supplies language-neutral template data for localization.
	MessageParams MessageParams

	// Details carries client-safe structured context for the error.
	Details Details

	cause error
}

// NewPublic creates an application error with an explicitly client-safe,
// language-neutral message key and details while retaining cause for internal
// diagnostics.
//
// messageKey identifies presentation text; the core never stores a translated
// sentence. details are part of the public error contract and must already be
// sanitized by the caller.
func NewPublic(code Code, messageKey MessageKey, cause error, details Details) *Error {
	return NewPublicWithParams(code, messageKey, nil, cause, details)
}

// NewPublicWithParams is NewPublic with template data for a dynamic public
// message. Params remain language-neutral and are rendered only by the
// presentation localizer.
func NewPublicWithParams(
	code Code,
	messageKey MessageKey,
	messageParams MessageParams,
	cause error,
	details Details,
) *Error {
	if messageKey == "" {
		messageKey = DefaultMessageKey(code)
	}

	return &Error{
		Code:          code,
		MessageKey:    messageKey,
		MessageParams: messageParams,
		Details:       details,
		cause:         cause,
	}
}

// newError creates an error using the safe default message for code.
//
// The variadic argument is retained for source compatibility with older call
// sites. Only cerr.Details (or *cerr.Details) is accepted as public metadata;
// arbitrary maps/structs are intentionally ignored so internal metadata cannot
// become API output by accident. New code should prefer NewPublic whenever it
// needs custom client-facing details.
func newError(code Code, cause error, legacyData ...any) *Error {
	var details Details

	for _, data := range legacyData {
		switch d := data.(type) {
		case Details:
			details = d

		case *Details:
			if d != nil {
				details = *d
			}
		}
	}

	return NewPublic(code, DefaultMessageKey(code), cause, details)
}

// Error returns a diagnostic string suitable for server-side logs.
// It is not a public/client-facing message.
func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}

	if e.cause == nil {
		return fmt.Sprintf("[%s] %s", e.Code, e.MessageKey)
	}

	return fmt.Sprintf("[%s] %s", e.Code, e.cause.Error())
}

// Unwrap returns the private underlying cause for errors.Is/errors.As.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}

	return e.cause
}

// IsCode reports whether err or any wrapped error is a cerr.Error with code.
func IsCode(err error, code Code) bool {
	var ce *Error
	return errors.As(err, &ce) && ce.Code == code
}

// DefaultMessageKey returns the stable language-neutral fallback key for code.
func DefaultMessageKey(code Code) MessageKey {
	switch code {
	case CodeValidationFailed:
		return "error.default.validation_failed"

	case CodeInvalidArgument:
		return "error.default.invalid_argument"

	case CodeResourceNotFound:
		return "error.default.resource_not_found"

	case CodeDecisionNotFound:
		return "error.default.decision_not_found"

	case CodeDecisionRejected:
		return "error.default.decision_rejected"

	case CodeDecisionExpired:
		return "error.default.decision_expired"

	case CodeContractNotFound:
		return "error.default.contract_not_found"

	case CodeContractAlreadyExists:
		return "error.default.contract_already_exists"

	case CodeInstallmentNotFound:
		return "error.default.installment_not_found"

	case CodeIdempotencyConflict:
		return "error.default.idempotency_conflict"

	case CodePaymentProviderError:
		return "error.default.payment_provider_error"

	case CodeUnauthorized:
		return "error.default.unauthorized"

	case CodeForbidden:
		return "error.default.forbidden"

	case CodeConflict:
		return "error.default.conflict"

	case CodeServiceUnavailable:
		return "error.default.service_unavailable"

	case CodePayloadTooLarge:
		return "error.default.payload_too_large"

	case CodeTooManyRequests:
		return "error.default.too_many_requests"

	case CodeInternalError:
		return "error.default.internal"

	default:
		return "error.default.internal"
	}
}

// ValidationFailed indicates that business validation rules failed.
func ValidationFailed(err error, data ...any) *Error {
	return newError(CodeValidationFailed, err, data...)
}

// InvalidArgument indicates invalid client input.
func InvalidArgument(err error, data ...any) *Error {
	return newError(CodeInvalidArgument, err, data...)
}

// Unauthorized indicates that authentication is missing or invalid.
func Unauthorized(err error, data ...any) *Error {
	return newError(CodeUnauthorized, err, data...)
}

// Forbidden indicates that the caller is authenticated
// but not permitted to perform the requested operation.
func Forbidden(err error, data ...any) *Error {
	return newError(CodeForbidden, err, data...)
}

// ResourceNotFound indicates that the requested resource does not exist.
func ResourceNotFound(err error, data ...any) *Error {
	return newError(CodeResourceNotFound, err, data...)
}

// DecisionNotFound indicates that the requested decision does not exist.
func DecisionNotFound(err error, data ...any) *Error {
	return newError(CodeDecisionNotFound, err, data...)
}

// DecisionRejected indicates that a decision exists
// but its outcome does not permit the requested operation.
func DecisionRejected(err error, data ...any) *Error {
	return newError(CodeDecisionRejected, err, data...)
}

// DecisionExpired indicates that a decision exists
// but is no longer valid for use.
func DecisionExpired(err error, data ...any) *Error {
	return newError(CodeDecisionExpired, err, data...)
}

// ContractNotFound indicates that the requested contract does not exist.
func ContractNotFound(err error, data ...any) *Error {
	return newError(CodeContractNotFound, err, data...)
}

// ContractAlreadyExists indicates that a contract already exists
// for the supplied business identifier.
func ContractAlreadyExists(err error, data ...any) *Error {
	return newError(CodeContractAlreadyExists, err, data...)
}

// InstallmentNotFound indicates that the requested installment does not exist.
func InstallmentNotFound(err error, data ...any) *Error {
	return newError(CodeInstallmentNotFound, err, data...)
}

// IdempotencyConflict indicates that an idempotency key
// has already been used with a different request payload.
func IdempotencyConflict(err error, data ...any) *Error {
	return newError(CodeIdempotencyConflict, err, data...)
}

// PaymentProviderError indicates a failure returned by
// an external payment provider.
func PaymentProviderError(err error, data ...any) *Error {
	return newError(CodePaymentProviderError, err, data...)
}

// Conflict indicates that the requested operation conflicts with
// the current state of the target resource.
func Conflict(err error, data ...any) *Error {
	return newError(CodeConflict, err, data...)
}

// ServiceUnavailable indicates that a required dependency is temporarily unavailable.
func ServiceUnavailable(err error, data ...any) *Error {
	return newError(CodeServiceUnavailable, err, data...)
}

// InternalError indicates an unexpected server-side failure.
//
// InternalError should generally be logged and translated into a
// sanitized transport-level error response.
func InternalError(err error, data ...any) *Error {
	return newError(CodeInternalError, err, data...)
}
