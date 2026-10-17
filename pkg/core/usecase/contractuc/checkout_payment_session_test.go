package contractuc

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

func TestValidateExternalPaymentURL(t *testing.T) {
	t.Parallel()

	require.NoError(t, validateExternalPaymentURL("https://financial.example/pay"))

	for name, raw := range map[string]string{
		"http":       "http://financial.example/pay",
		"javascript": "javascript:alert(1)",
		"relative":   "/pay",
		"userinfo":   "https://user:pass@financial.example/pay",
		"empty":      "",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			require.Error(t, validateExternalPaymentURL(raw))
		})
	}
}

func TestValidatePaymentBasedCreateContractInput_AllowsMissingTenantOrContext(t *testing.T) {
	t.Parallel()

	generic := CreateContractInput{
		DecisionID:        model.DecisionID(uuid.New()),
		IdentitySubjectID: model.IdentitySubjectID(uuid.New()),
	}
	require.NoError(t, validateCreateContractInput(generic))

	paymentBased := CreateContractInput{
		DecisionID:        model.DecisionID(uuid.New()),
		IdentitySubjectID: model.IdentitySubjectID(uuid.New()),
		TenantID:          model.TenantID{},
		ContextID:         model.ContextID{},
	}
	require.NoError(t, validatePaymentBasedCreateContractInput(paymentBased))
}

func TestValidateCheckoutPaymentResult_RejectsHTTPRedirect(t *testing.T) {
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

	err := validateCheckoutPaymentResult(result, contractRow, mustParseRFC3339(t, "2026-07-14T12:00:00Z"))
	require.Error(t, err)
}

func mustParseRFC3339(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	require.NoError(t, err)

	return parsed
}

func TestValidateCheckoutPaymentResult_ValidatesFinancialCoreExpiryWindow(t *testing.T) {
	t.Parallel()

	now := mustParseRFC3339(t, "2026-07-14T12:00:00Z")
	contractExpiresAt := mustParseRFC3339(t, "2026-07-14T12:30:00Z")
	contractRow := model.Contract{
		PayableNowAmountMinor: 100,
		Currency:              model.IRRCurrency,
		ExpiresAt:             contractExpiresAt,
	}
	base := extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  "payment-intent-123",
		PaymentSessionID: "payment-session-123",
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      100,
		Currency:         "IRR",
		PaymentLink:      "https://financial.example/pay",
		ExpiresAt:        now.Add(10 * time.Minute),
	}

	require.NoError(t, validateCheckoutPaymentResult(base, contractRow, now))

	missing := base
	missing.ExpiresAt = time.Time{}
	require.Error(t, validateCheckoutPaymentResult(missing, contractRow, now))

	expired := base
	expired.ExpiresAt = now
	require.Error(t, validateCheckoutPaymentResult(expired, contractRow, now))

	afterContract := base
	afterContract.ExpiresAt = contractExpiresAt.Add(time.Second)
	require.Error(t, validateCheckoutPaymentResult(afterContract, contractRow, now))
}
