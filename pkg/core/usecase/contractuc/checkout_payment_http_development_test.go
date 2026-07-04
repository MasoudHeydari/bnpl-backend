//go:build development

package contractuc

import (
	"testing"

	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidateExternalPaymentURL_DevelopmentAllowsHTTP(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateExternalPaymentURL("http://financial.example/pay"))
}

func TestValidateCheckoutPaymentResult_DevelopmentAllowsHTTPRedirect(t *testing.T) {
	t.Parallel()

	contractRow := model.Contract{
		PayableNowAmountMinor: 100,
		Currency:              model.Currency("IRR"),
		ExpiresAt:             mustParseRFC3339(t, "2026-07-14T12:30:00Z"),
	}

	result := extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  "payment-intent-123",
		PaymentSessionID: "payment-session-123",
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      100,
		Currency:         "IRR",
		PaymentLink:      "http://financial.example/pay",
		ExpiresAt:        mustParseRFC3339(t, "2026-07-14T12:15:00Z"),
	}

	uc := &UseCase{logger: lognoop.New()}
	err := uc.validateCheckoutPaymentResult(result, contractRow, mustParseRFC3339(t, "2026-07-14T12:00:00Z"))
	require.NoError(t, err)
}
