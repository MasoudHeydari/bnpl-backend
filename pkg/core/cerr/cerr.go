package cerr

import (
	"fmt"
	"net/http"
)

type Error struct {
	Err            error
	HTTPStatusCode int
}

func (e *Error) Unwrap() error {
	return e.Err
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%d] %s", e.HTTPStatusCode, e.Err.Error())
}

func BadRequest(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusBadRequest}
}

func Unauthorized(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusUnauthorized}
}

func Forbidden(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusForbidden}
}

func NotFound(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusNotFound}
}

func Conflict(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusConflict}
}

func UnprocessableEntity(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusUnprocessableEntity}
}

func TooManyRequests(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusTooManyRequests}
}

func InternalServerError(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusInternalServerError}
}

func NotImplemented(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusNotImplemented}
}

func BadGateway(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusBadGateway}
}

func GatewayTimeout(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusGatewayTimeout}
}

func ServiceUnavailable(err error) *Error {
	return &Error{Err: err, HTTPStatusCode: http.StatusServiceUnavailable}
}
