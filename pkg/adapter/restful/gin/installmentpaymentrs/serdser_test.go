package installmentpaymentrs

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
)

func TestToCreatePaymentAttemptResponse_HidesFinancialCoreIDs(t *testing.T) {
	t.Parallel()

	intent := "fpi_secret"
	session := "fps_secret"
	redirect := "https://pay.example/session/abc"
	expiresAt := time.Date(
		2026,
		8,
		9,
		12,
		30,
		0,
		0,
		time.UTC,
	)
	attemptID := model.PaymentAttemptID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	raw, err := json.Marshal(toCreatePaymentAttemptResponse(installmentrepaymentuc.CreatePaymentAttemptOutput{
		Result: installmentrepaymentuc.CreatePaymentAttemptResultQueued,
		PaymentAttempt: model.PaymentAttempt{
			ID:                        attemptID,
			InstallmentID:             installmentID,
			AmountMinor:               25_000_000,
			Currency:                  model.Currency("IRR"),
			State:                     model.SubmittedPaymentAttemptState,
			Channel:                   model.GatewayPaymentChannel,
			FinancialPaymentIntentID:  &intent,
			FinancialPaymentSessionID: &session,
			PaymentURL:                &redirect,
			FinancialPaymentExpiresAt: &expiresAt,
		},
	}))
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	attempt, ok := decoded["payment_attempt"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, attemptID.String(), attempt["id"])
	require.Equal(t, installmentID.String(), attempt["installment_id"])
	require.Equal(t, redirect, attempt["redirect_url"])
	require.Equal(t, "2026-08-09T12:30:00Z", attempt["expires_at"])
	require.NotContains(t, attempt, "financial_payment_intent_id")
	require.NotContains(t, attempt, "financial_payment_session_id")
	require.NotContains(t, attempt, "payment_url")
	require.NotContains(t, attempt, "provider_code")

	for _, term := range []string{
		"provider_code",
		"financial_payment_",
		"payment_intent_id",
		"payment_session_id",
		"invoice_id",
	} {
		require.Falsef(t, strings.Contains(string(raw), term), "public JSON must not contain %q", term)
	}
}

func TestCreatePaymentAttemptRequest_HasNoProviderCode(t *testing.T) {
	t.Parallel()

	raw, err := json.Marshal(createPaymentAttemptRequest{Channel: "gateway"})
	require.NoError(t, err)
	require.JSONEq(t, `{"channel":"gateway"}`, string(raw))
}
