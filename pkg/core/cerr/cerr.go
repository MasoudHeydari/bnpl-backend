// Package cerr defines transport-friendly error wrappers used by adapters to
// expose domain and application failures with HTTP semantics.
// It belongs to the core domain support layer because it expresses shared error
// meaning without depending on a specific HTTP framework implementation.
package cerr

import (
	"fmt"
	"net/http"
)

// Error couples an underlying error with an HTTP status code expected by
// transport adapters.
// Instances are immutable after construction when callers do not mutate the
// exported fields directly.
type Error struct {
	Err            error
	HTTPStatusCode int
}

// Unwrap returns the underlying error so callers can use errors.Is and
// errors.As with Error values.
func (e *Error) Unwrap() error {
	return e.Err
}

// Error formats the wrapped error with its associated HTTP status code.
func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.HTTPStatusCode, e.Err.Error())
}

// BadRequest wraps err with HTTP 400 semantics.
func BadRequest(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusBadRequest}
}

// Unauthorized wraps err with HTTP 401 semantics.
func Unauthorized(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusUnauthorized}
}

// Forbidden wraps err with HTTP 403 semantics.
func Forbidden(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusForbidden}
}

// NotFound wraps err with HTTP 404 semantics.
func NotFound(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusNotFound}
}

// Conflict wraps err with HTTP 409 semantics.
func Conflict(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusConflict}
}

// UnprocessableEntity wraps err with HTTP 422 semantics.
func UnprocessableEntity(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusUnprocessableEntity}
}

// TooManyRequests wraps err with HTTP 429 semantics.
func TooManyRequests(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusTooManyRequests}
}

// InternalServerError wraps err with HTTP 500 semantics.
func InternalServerError(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusInternalServerError}
}

// NotImplemented wraps err with HTTP 501 semantics.
func NotImplemented(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusNotImplemented}
}

// BadGateway wraps err with HTTP 502 semantics.
func BadGateway(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusBadGateway}
}

// GatewayTimeout wraps err with HTTP 504 semantics.
func GatewayTimeout(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusGatewayTimeout}
}

// ServiceUnavailable wraps err with HTTP 503 semantics.
func ServiceUnavailable(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusServiceUnavailable}
}
