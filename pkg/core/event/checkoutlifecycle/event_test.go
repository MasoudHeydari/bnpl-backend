package checkoutlifecycle

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewEvent_CompletedIncludesCommercialFields(t *testing.T) {
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
	event, err := NewEvent(
		"session:bnpl.checkout.completed.v1:completed",
		EventTypeCompleted,
		completedAt,
		EventData{
			CheckoutSessionID: "11111111-1111-1111-1111-111111111111",
			MerchantOrderID:   "DAMZ-ORDER-1001",
			MerchantID:        "22222222-2222-2222-2222-222222222222",
			Status:            "completed",
			AmountMinor:       120_000_000,
			Currency:          "IRR",
			CompletedAt:       &completedAt,
			MerchantState:     "cart-42",
		},
	)
	require.NoError(t, err)

	raw, err := json.Marshal(event)
	require.NoError(t, err)

	require.Contains(t, string(raw), `"amount_minor":120000000`)
	require.Contains(t, string(raw), `"currency":"IRR"`)
	require.Contains(t, string(raw), `"completed_at":"2026-10-05T10:30:12Z"`)

	for _, term := range []string{
		"provider_code",
		"financial_payment_",
		"payment_intent_id",
		"payment_session_id",
		"invoice_id",
		"available_credit",
		"installment",
	} {
		require.Falsef(t, strings.Contains(string(raw), term), "webhook payload must not contain %q", term)
	}
}

func TestNewEvent_NonCompletedOmitsCompletedAt(t *testing.T) {
	t.Parallel()

	event, err := NewEvent(
		"session:bnpl.checkout.canceled.v1:canceled",
		EventTypeCanceled,
		time.Date(
			2026,
			10,
			5,
			10,
			30,
			12,
			0,
			time.UTC,
		),
		EventData{
			CheckoutSessionID: "11111111-1111-1111-1111-111111111111",
			MerchantOrderID:   "DAMZ-ORDER-1001",
			MerchantID:        "22222222-2222-2222-2222-222222222222",
			Status:            "canceled",
			AmountMinor:       120_000_000,
			Currency:          "IRR",
		},
	)
	require.NoError(t, err)

	raw, err := json.Marshal(event)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "completed_at")
	require.Contains(t, string(raw), `"amount_minor":120000000`)
}
