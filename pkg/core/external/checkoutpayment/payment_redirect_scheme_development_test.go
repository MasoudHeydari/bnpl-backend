//go:build development

package checkoutpayment

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestValidatePaymentRedirectURL_DevelopmentAllowsHTTP(t *testing.T) {
	t.Parallel()

	require.NoError(t, ValidatePaymentRedirectURL("http://pay.example/ps-1", 0))
	require.NoError(t, ValidatePaymentRedirectURL("https://pay.example/ps-1", 0))
	require.Error(t, ValidatePaymentRedirectURL("https://user:pass@pay.example/ps-1", 0))
	require.Error(t, ValidatePaymentRedirectURL("javascript:alert(1)", 0))
}

func TestValidateRequiresActionCreateResult_DevelopmentAllowsHTTPLink(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	result := validRequiresActionResult(now)
	result.PaymentLink = "http://192.168.1.103:8080/pay/ps-1"

	require.NoError(t, ValidateRequiresActionCreateResult(
		result,
		RequiresActionCreateResultCheck{
			ExpectedAmountMinor: 1000,
			ExpectedCurrency:    "IRR",
			Now:                 now,
		},
	))
}

func TestValidateActionablePaymentSession_DevelopmentAllowsHTTP(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	require.NoError(t, ValidateActionablePaymentSession(
		ActionablePaymentSession{
			PaymentIntentID:  "pi-1",
			PaymentSessionID: "ps-1",
			PaymentURL:       "http://pay.example/ps-1",
			ExpiresAt:        now.Add(10 * time.Minute),
		},
		now,
		0,
		0,
	))
}
