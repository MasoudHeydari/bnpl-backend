package checkoutsessionrs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
)

func TestToOfferResponse_DeniedOmitsFinancingFields(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(toOfferResponse(checkoutsessionuc.Offer{
		Outcome: checkoutsessionuc.DeniedOfferOutcome,
	}))
	require.NoError(t, err)
	require.JSONEq(t, `{"outcome":"denied"}`, string(raw))
}

func TestToOfferResponse_ApprovedKeepsZeroDownPayment(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(toOfferResponse(checkoutsessionuc.Offer{
		Outcome:              checkoutsessionuc.ApprovedOfferOutcome,
		RequestedAmountMinor: 1000,
		Currency:             model.Currency("IRR"),
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Contains(t, decoded, "required_down_payment_amount_minor")
	require.Contains(t, decoded, "approved_financed_amount_minor")
}

func TestToConfirmResponse_ReconfirmCarriesOfferWithoutContract(t *testing.T) {
	t.Parallel()

	session := model.CheckoutSession{
		ID:                   model.CheckoutSessionID(uuid.New()),
		Status:               model.OpenCheckoutSessionStatus,
		EvaluationGeneration: 2,
	}

	resp := toConfirmResponse(checkoutsessionuc.ConfirmOutput{
		ReconfirmRequired: true,
		Session:           session,
		Offer:             checkoutsessionuc.Offer{Outcome: checkoutsessionuc.ApprovedOfferOutcome},
	})

	require.True(t, resp.ReconfirmRequired)
	require.Nil(t, resp.Contract)
	require.Empty(t, resp.RedirectURL)
	require.NotNil(t, resp.Offer)
	require.Equal(t, 2, resp.EvaluationGeneration)
	require.False(t, resp.PaymentRequired)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assertNoExternalPaymentLeak(t, string(raw))
}

func TestToConfirmResponse_PaymentRequiredExposesRedirectOnly(t *testing.T) {
	t.Parallel()

	sessionID := model.CheckoutSessionID(uuid.New())
	contractID := model.ContractID(uuid.New())
	expiresAt := "2026-10-05T14:30:00Z"

	resp := toConfirmResponse(checkoutsessionuc.ConfirmOutput{
		Session: model.CheckoutSession{
			ID:                   sessionID,
			Status:               model.ProcessingCheckoutSessionStatus,
			EvaluationGeneration: 1,
		},
		Contract: model.Contract{
			ID:     contractID,
			Status: model.PaymentPendingContractStatus,
		},
		PaymentRequired: true,
		Payment: contractuc.CheckoutPaymentOutput{
			PaymentLink:            "https://pay.example/session/abc",
			PaymentSessionID:       "fps_secret",
			PaymentIntentID:        "fpi_secret",
			PaymentAttemptID:       uuid.NewString(),
			InvoiceID:              "inv_secret",
			ExpiresAt:              expiresAt,
			ReusedIdempotentResult: true,
		},
	})

	require.False(t, resp.ReconfirmRequired)
	require.True(t, resp.PaymentRequired)
	require.Equal(t, "https://pay.example/session/abc", resp.RedirectURL)
	require.Equal(t, expiresAt, resp.ExpiresAt)
	require.NotNil(t, resp.Contract)
	require.Equal(t, contractID.UUID(), resp.Contract.ID)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assertNoExternalPaymentLeak(t, string(raw))
	require.Contains(t, string(raw), `"redirect_url"`)
	require.Contains(t, string(raw), `"expires_at"`)
	require.NotContains(t, string(raw), `"payment"`)
}

func TestToConfirmResponse_MerchantFundingOmitsRedirect(t *testing.T) {
	t.Parallel()

	resp := toConfirmResponse(checkoutsessionuc.ConfirmOutput{
		Session: model.CheckoutSession{
			ID:                   model.CheckoutSessionID(uuid.New()),
			Status:               model.CompletedCheckoutSessionStatus,
			EvaluationGeneration: 1,
		},
		Contract: model.Contract{
			ID:     model.ContractID(uuid.New()),
			Status: model.ActiveContractStatus,
		},
		PaymentRequired: false,
	})

	require.False(t, resp.PaymentRequired)
	require.Empty(t, resp.RedirectURL)
	require.Empty(t, resp.ExpiresAt)
	require.Equal(t, model.CompletedCheckoutSessionStatus, resp.Status)

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	assertNoExternalPaymentLeak(t, string(raw))
}

func TestConfirmRequestSchema_HasNoProviderCode(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(confirmRequest{EvaluationGeneration: 1})
	require.NoError(t, err)
	require.JSONEq(t, `{"evaluation_generation":1}`, string(raw))
	assertNoExternalPaymentLeak(t, string(raw))
}

func TestToStatusResponse_EnrichedMerchantInquiry(t *testing.T) {
	t.Parallel()

	completedAt := time.Date(
		2026,
		10,
		5,
		10,
		30,
		12,
		0,
		time.UTC,
	)
	token := model.CheckoutSessionToken("cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	raw, err := json.Marshal(toStatusResponse(checkoutsessionuc.GetPartnerStatusOutput{
		CheckoutSessionToken: token,
		MerchantOrderID:      "DAMZ-ORDER-1001",
		Status:               model.CompletedCheckoutSessionStatus,
		AmountMinor:          120_000_000,
		Currency:             model.Currency("IRR"),
		CompletedAt:          &completedAt,
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, token.String(), decoded["checkout_session_id"])
	require.Equal(t, "DAMZ-ORDER-1001", decoded["merchant_order_id"])
	require.Equal(t, "completed", decoded["status"])
	require.EqualValues(t, 120_000_000, decoded["amount_minor"])
	require.Equal(t, "IRR", decoded["currency"])
	require.Equal(t, "2026-10-05T10:30:12Z", decoded["completed_at"])
	require.NotContains(t, decoded, "available_credit_minor")
	require.NotContains(t, decoded, "installments")
	require.NotContains(t, decoded, "offer")
	assertNoExternalPaymentLeak(t, string(raw))
}

func TestToStatusResponse_NonCompletedOmitsCompletedAt(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(toStatusResponse(checkoutsessionuc.GetPartnerStatusOutput{
		CheckoutSessionToken: model.CheckoutSessionToken("cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"),
		MerchantOrderID:      "ORDER-1",
		Status:               model.ProcessingCheckoutSessionStatus,
		AmountMinor:          1_000,
		Currency:             model.Currency("IRR"),
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotContains(t, decoded, "completed_at")
	assertNoExternalPaymentLeak(t, string(raw))
}

func TestToWebhookDestinationResponse_HasNoNameOrSecret(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(toWebhookDestinationResponse(checkoutsessionuc.UpsertWebhookDestinationOutput{
		Destination: model.MerchantWebhookDestination{
			WebhookURL:              "https://merchant.example/hook",
			Enabled:                 true,
			SigningSecretCiphertext: "cipher",
		},
	}))
	require.NoError(t, err)
	require.JSONEq(t, `{"webhook_url":"https://merchant.example/hook","enabled":true}`, string(raw))
}

func assertNoExternalPaymentLeak(t *testing.T, raw string) {
	t.Helper()

	forbidden := []string{
		"provider_code",
		"financial_payment_",
		"payment_intent_id",
		"payment_session_id",
		"invoice_id",
		"payment_link",
		"reused_idempotent_result",
		"financial_core",
	}
	for _, term := range forbidden {
		require.Falsef(
			t,
			strings.Contains(raw, term),
			"public JSON must not contain %q; got %s",
			term,
			raw,
		)
	}
}
