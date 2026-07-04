//go:build production

package checkoutpayment

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidatePaymentRedirectURL_ProductionRejectsHTTP(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidatePaymentRedirectURL("https://pay.example/ps-1", 0))

	err := ValidatePaymentRedirectURL("http://pay.example/ps-1", 0)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidPaymentRedirectURL))
}

func TestValidateRequiresActionCreateResult_ProductionRejectsHTTPLink(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	result := validRequiresActionResult(now)
	result.PaymentLink = "http://pay.example/ps-1"

	err := ValidateRequiresActionCreateResult(
		result,
		RequiresActionCreateResultCheck{
			ExpectedAmountMinor: 1000,
			ExpectedCurrency:    "IRR",
			Now:                 now,
		},
	)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrInvalidPaymentRedirectURL))
}
