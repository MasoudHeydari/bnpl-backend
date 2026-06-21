package cerr

import (
	"errors"
	"fmt"
	"net/http"
)

// Code identifies a stable error category.
//
// Codes are part of the public application contract and may be consumed by:
//
//   - HTTP clients
//   - Frontend applications
//   - External services
//   - Monitoring and alerting systems
//
// Codes should be treated as stable identifiers.
// Changing an existing code is considered a breaking change.
type Code string

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
)

// HTTPStatusCode returns the HTTP status code that should be used
// when this application error code is serialized by a REST adapter.
func (c Code) HTTPStatusCode() int {
	switch c {
	case CodeValidationFailed:
		return http.StatusUnprocessableEntity
	case CodeInvalidArgument:
		return http.StatusBadRequest
	case CodeResourceNotFound,
		CodeDecisionNotFound,
		CodeContractNotFound,
		CodeInstallmentNotFound:
		return http.StatusNotFound
	case CodeDecisionRejected,
		CodeDecisionExpired:
		return http.StatusUnprocessableEntity
	case CodeContractAlreadyExists,
		CodeIdempotencyConflict,
		CodeConflict:
		return http.StatusConflict
	case CodePaymentProviderError:
		return http.StatusBadGateway
	case CodeUnauthorized:
		return http.StatusUnauthorized
	case CodeForbidden:
		return http.StatusForbidden
	case CodeServiceUnavailable:
		return http.StatusServiceUnavailable
	case CodeInternalError:
		return http.StatusInternalServerError
	default:
		return http.StatusInternalServerError
	}
}

// Error represents an application-level error.
//
// Error carries a stable machine-readable code,
// the underlying cause,
// and optional structured metadata.
//
// Error implements the standard error interface
// and supports unwrapping via errors.Unwrap.
type Error struct {
	// Err is the underlying cause.
	Err error

	// Code is a stable machine-readable identifier.
	Code Code

	// Data contains optional structured metadata.
	//
	// Data is intended for transport serialization and should never contain:
	//
	//   - secrets
	//   - access tokens
	//   - SQL statements
	//   - stack traces
	//   - internal infrastructure details
	//
	// Data must be safe to expose to API consumers.
	Data any
}

// newError creates a structured application error.
//
// This function is intentionally unexported.
// Callers should use one of the exported constructors instead.
func newError(code Code, err error, data ...any) *Error {
	var d any
	if len(data) > 0 {
		d = data[0]
	}

	return &Error{
		Err:  err,
		Code: code,
		Data: d,
	}
}

// Error returns the string representation of the error.
//
// The returned value is intended primarily for logs and diagnostics.
func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Err.Error())
}

// Unwrap returns the underlying cause.
func (e *Error) Unwrap() error {
	return e.Err
}

// IsCode reports whether err or any wrapped error
// is a cerr.Error with the specified code.
func IsCode(err error, code Code) bool {
	var ce *Error
	return errors.As(err, &ce) && ce.Code == code
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
