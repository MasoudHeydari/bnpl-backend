package cerr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestError_ErrorAndUnwrap(t *testing.T) {
	t.Parallel()

	cause := errors.New("underlying")
	err := ValidationFailed(cause)

	require.Equal(t, "[VALIDATION_FAILED] underlying", err.Error())
	require.Same(t, cause, err.Unwrap())
	require.Equal(t, DefaultMessageKey(CodeValidationFailed), err.MessageKey)
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

func TestNewPublicSeparatesPublicContractFromCause(t *testing.T) {
	t.Parallel()

	cause := errors.New(`ERROR: column "display_name" does not exist (SQLSTATE 42703)`)
	details := Details{
		Fields: []string{"merchant_id"},
		Violations: []Violation{{
			Code:       "NOT_FOUND",
			MessageKey: "error.merchant_profile_not_found",
			Fields:     []string{},
		}},
	}

	err := NewPublic(
		CodeResourceNotFound,
		"error.merchant_profile_not_found",
		cause,
		details,
	)

	require.Equal(t, CodeResourceNotFound, err.Code)
	require.Equal(t, MessageKey("error.merchant_profile_not_found"), err.MessageKey)
	require.Equal(t, details, err.Details)
	require.Same(t, cause, err.Unwrap())
	require.Contains(t, err.Error(), "SQLSTATE 42703")
}

func TestLegacyArbitraryMetadataIsNotPublicDetails(t *testing.T) {
	t.Parallel()

	err := ValidationFailed(
		errors.New("invalid"),
		map[string]any{
			"constraint": "merchant_profiles_merchant_id_key",
		},
	)

	require.Empty(t, err.Details.Fields)
	require.Empty(t, err.Details.Violations)
}

func TestLegacyTypedDetailsRemainSupported(t *testing.T) {
	t.Parallel()

	details := Details{
		Fields: []string{"merchant_id"},
	}
	err := InvalidArgument(errors.New("invalid"), details)

	require.Equal(t, details, err.Details)
}

func TestConstructorsSetCodeAndSafeDefaultMessageKey(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")

	tests := []struct {
		name string
		got  *Error
		code Code
	}{
		{"ValidationFailed", ValidationFailed(cause), CodeValidationFailed},
		{"InvalidArgument", InvalidArgument(cause), CodeInvalidArgument},
		{"ResourceNotFound", ResourceNotFound(cause), CodeResourceNotFound},
		{"DecisionNotFound", DecisionNotFound(cause), CodeDecisionNotFound},
		{"DecisionRejected", DecisionRejected(cause), CodeDecisionRejected},
		{"DecisionExpired", DecisionExpired(cause), CodeDecisionExpired},
		{"ContractNotFound", ContractNotFound(cause), CodeContractNotFound},
		{"ContractAlreadyExists", ContractAlreadyExists(cause), CodeContractAlreadyExists},
		{"InstallmentNotFound", InstallmentNotFound(cause), CodeInstallmentNotFound},
		{"IdempotencyConflict", IdempotencyConflict(cause), CodeIdempotencyConflict},
		{"PaymentProviderError", PaymentProviderError(cause), CodePaymentProviderError},
		{"Conflict", Conflict(cause), CodeConflict},
		{"ServiceUnavailable", ServiceUnavailable(cause), CodeServiceUnavailable},
		{"InternalError", InternalError(cause), CodeInternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.code, tt.got.Code)
			require.Equal(t, DefaultMessageKey(tt.code), tt.got.MessageKey)
			require.Same(t, cause, tt.got.Unwrap())
		})
	}
}
