package contractrs

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func TestToCreateContractResponse_PaymentRequiredExposesRedirectOnly(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(toCreateContractResponse(contractuc.CreateContractOutput{
		Result: contractuc.CreateContractResultCreated,
		Contract: model.Contract{
			ID:     model.ContractID(uuid.New()),
			Status: model.PaymentPendingContractStatus,
		},
		PaymentRequired: true,
		Payment: contractuc.CheckoutPaymentOutput{
			PaymentLink:            "https://pay.example/session/abc",
			PaymentSessionID:       "fps_secret",
			PaymentIntentID:        "fpi_secret",
			PaymentAttemptID:       uuid.NewString(),
			InvoiceID:              "inv_secret",
			ExpiresAt:              "2026-10-05T14:30:00Z",
			ReusedIdempotentResult: true,
		},
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, true, decoded["payment_required"])
	require.Equal(t, "https://pay.example/session/abc", decoded["redirect_url"])
	require.Equal(t, "2026-10-05T14:30:00Z", decoded["expires_at"])
	require.NotContains(t, decoded, "payment")
	assertNoExternalPaymentLeak(t, string(raw))
}

func TestToCreateContractResponse_MerchantFundingOmitsRedirect(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(toCreateContractResponse(contractuc.CreateContractOutput{
		Result: contractuc.CreateContractResultCreated,
		Contract: model.Contract{
			ID:     model.ContractID(uuid.New()),
			Status: model.ActiveContractStatus,
		},
		PaymentRequired: false,
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, false, decoded["payment_required"])
	require.NotContains(t, decoded, "redirect_url")
	require.NotContains(t, decoded, "expires_at")
	assertNoExternalPaymentLeak(t, string(raw))
}

func assertNoExternalPaymentLeak(t *testing.T, raw string) {
	t.Helper()

	for _, term := range []string{
		"provider_code",
		"financial_payment_",
		"payment_intent_id",
		"payment_session_id",
		"invoice_id",
		"payment_link",
		"reused_idempotent_result",
	} {
		require.Falsef(t, strings.Contains(raw, term), "public JSON must not contain %q; got %s", term, raw)
	}
}
