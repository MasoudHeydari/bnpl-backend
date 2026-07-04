package serdser

import (
	"net/http"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
)

// StatusCode maps application error codes to REST/HTTP status codes.
//
// HTTP mapping belongs to the REST adapter; the core cerr package intentionally
// has no dependency on net/http.
func StatusCode(code cerr.Code) int {
	switch code {
	case cerr.CodeValidationFailed:
		return http.StatusUnprocessableEntity

	case cerr.CodeInvalidArgument:
		return http.StatusBadRequest

	case cerr.CodeResourceNotFound,
		cerr.CodeDecisionNotFound,
		cerr.CodeContractNotFound,
		cerr.CodeInstallmentNotFound:
		return http.StatusNotFound

	case cerr.CodeDecisionRejected,
		cerr.CodeDecisionExpired:
		return http.StatusUnprocessableEntity

	case cerr.CodeContractAlreadyExists,
		cerr.CodeIdempotencyConflict,
		cerr.CodeConflict:
		return http.StatusConflict

	case cerr.CodePaymentProviderError:
		return http.StatusBadGateway

	case cerr.CodeUnauthorized:
		return http.StatusUnauthorized

	case cerr.CodeForbidden:
		return http.StatusForbidden

	case cerr.CodeServiceUnavailable:
		return http.StatusServiceUnavailable

	case cerr.CodePayloadTooLarge:
		return http.StatusRequestEntityTooLarge

	case cerr.CodeTooManyRequests:
		return http.StatusTooManyRequests

	case cerr.CodeInternalError:
		return http.StatusInternalServerError

	default:
		return http.StatusInternalServerError
	}
}
