package cerr

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestError_ErrorAndUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("underlying")
	err := ValidationFailed(cause)

	require.Equal(t, "[VALIDATION_FAILED] underlying", err.Error())
	require.Same(t, cause, err.Unwrap())
}

func TestIsCode(t *testing.T) {
	t.Parallel()

	err := ResourceNotFound(errors.New("missing"))
	wrapped := errors.Join(errors.New("outer"), err)

	require.True(t, IsCode(err, CodeResourceNotFound))
	require.True(t, IsCode(wrapped, CodeResourceNotFound))
	require.False(t, IsCode(err, CodeInvalidArgument))
	require.False(t, IsCode(errors.New("plain"), CodeResourceNotFound))
}

func TestCode_HTTPStatusCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code   Code
		status int
	}{
		{CodeValidationFailed, http.StatusUnprocessableEntity},
		{CodeInvalidArgument, http.StatusBadRequest},
		{CodeResourceNotFound, http.StatusNotFound},
		{CodeDecisionNotFound, http.StatusNotFound},
		{CodeContractNotFound, http.StatusNotFound},
		{CodeInstallmentNotFound, http.StatusNotFound},
		{CodeDecisionRejected, http.StatusUnprocessableEntity},
		{CodeDecisionExpired, http.StatusUnprocessableEntity},
		{CodeContractAlreadyExists, http.StatusConflict},
		{CodeIdempotencyConflict, http.StatusConflict},
		{CodeConflict, http.StatusConflict},
		{CodePaymentProviderError, http.StatusBadGateway},
		{CodeUnauthorized, http.StatusUnauthorized},
		{CodeForbidden, http.StatusForbidden},
		{CodeServiceUnavailable, http.StatusServiceUnavailable},
		{CodeInternalError, http.StatusInternalServerError},
		{Code("UNKNOWN"), http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(string(tt.code), func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.status, tt.code.HTTPStatusCode())
		})
	}
}

func TestConstructorsSetCodeErrAndData(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")
	data := map[string]string{"field": "value"}

	tests := []struct {
		name string
		got  *Error
		code Code
	}{
		{"ValidationFailed", ValidationFailed(cause, data), CodeValidationFailed},
		{"InvalidArgument", InvalidArgument(cause, data), CodeInvalidArgument},
		{"ResourceNotFound", ResourceNotFound(cause, data), CodeResourceNotFound},
		{"DecisionNotFound", DecisionNotFound(cause, data), CodeDecisionNotFound},
		{"DecisionRejected", DecisionRejected(cause, data), CodeDecisionRejected},
		{"DecisionExpired", DecisionExpired(cause, data), CodeDecisionExpired},
		{"ContractNotFound", ContractNotFound(cause, data), CodeContractNotFound},
		{"ContractAlreadyExists", ContractAlreadyExists(cause, data), CodeContractAlreadyExists},
		{"InstallmentNotFound", InstallmentNotFound(cause, data), CodeInstallmentNotFound},
		{"IdempotencyConflict", IdempotencyConflict(cause, data), CodeIdempotencyConflict},
		{"PaymentProviderError", PaymentProviderError(cause, data), CodePaymentProviderError},
		{"Conflict", Conflict(cause, data), CodeConflict},
		{"ServiceUnavailable", ServiceUnavailable(cause, data), CodeServiceUnavailable},
		{"InternalError", InternalError(cause, data), CodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.code, tt.got.Code)
			require.Same(t, cause, tt.got.Err)
			require.Equal(t, data, tt.got.Data)
		})
	}
}

func TestConstructorWithoutData(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")
	err := InternalError(cause)

	require.Nil(t, err.Data)
}
