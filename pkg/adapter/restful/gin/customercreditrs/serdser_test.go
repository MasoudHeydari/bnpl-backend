package customercreditrs

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/customercredituc"
)

func TestToCreditSummaryResponse_NotAssigned_StatusOnly(t *testing.T) {
	t.Parallel()

	got := toCreditSummaryResponse(customercredituc.Output{
		AvailableAmountMinor: 0,
		Currency:             model.IRRCurrency,
		Status:               customercredituc.SummaryStatusNotAssigned,
	})

	require.Equal(t, customercredituc.SummaryStatusNotAssigned, got.Status)
	require.Nil(t, got.AvailableAmountMinor)
	require.Empty(t, got.Currency)
	require.Nil(t, got.ExpiresAt)

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, `{"status":"not_assigned","credit_usage":{"status":"unavailable"}}`, string(raw))
}

func TestToCreditSummaryResponse_Expired_StatusAndExpiresAtOnly(t *testing.T) {
	t.Parallel()

	expiresAt := time.Date(
		2025,
		8,
		9,
		11,
		22,
		41,
		952000000,
		time.FixedZone("IRST", 3*3600+1800),
	)
	limitStatus := model.ExpiredCreditAccountStatus

	got := toCreditSummaryResponse(customercredituc.Output{
		AvailableAmountMinor: 0,
		Currency:             model.IRRCurrency,
		Status:               customercredituc.SummaryStatusExpired,
		LimitStatus:          &limitStatus,
		ExpiresAt:            &expiresAt,
	})

	require.Equal(t, customercredituc.SummaryStatusExpired, got.Status)
	require.Equal(t, &expiresAt, got.ExpiresAt)
	require.Nil(t, got.AvailableAmountMinor)
	require.Empty(t, got.Currency)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, "expired", decoded["status"])
	require.Contains(t, decoded, "expires_at")
	require.NotContains(t, decoded, "valid_from")
	require.NotContains(t, decoded, "limit_amount_minor")
	require.NotContains(t, decoded, "used_amount_minor")
	require.NotContains(t, decoded, "available_amount_minor")
	require.NotContains(t, decoded, "currency")
	require.NotContains(t, decoded, "limit_status")
	require.NotContains(t, decoded, "updated_at")
}

func TestToCreditSummaryResponse_Active_IncludesAvailableAmount(t *testing.T) {
	t.Parallel()

	got := toCreditSummaryResponse(customercredituc.Output{
		AvailableAmountMinor: 4_000_000,
		Currency:             model.IRRCurrency,
		Status:               customercredituc.SummaryStatusActive,
	})

	require.Equal(t, customercredituc.SummaryStatusActive, got.Status)
	require.NotNil(t, got.AvailableAmountMinor)
	require.Equal(t, uint64(4_000_000), *got.AvailableAmountMinor)
	require.Equal(t, model.IRRCurrency, got.Currency)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.NotContains(t, decoded, "valid_from")
	require.NotContains(t, decoded, "limit_amount_minor")
	require.NotContains(t, decoded, "used_amount_minor")
}

func TestRestrictedCreditUsageHasSeparatePublicRepresentation(t *testing.T) {
	got := toCreditSummaryResponse(customercredituc.Output{Status: customercredituc.SummaryStatusActive, Currency: model.IRRCurrency, CreditUsage: customercredituc.CreditUsage{Status: "restricted", Reason: "collections_default"}})
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	require.JSONEq(t, `{"status":"active","available_amount_minor":0,"currency":"IRR","credit_usage":{"status":"restricted","reason":"collections_default"}}`, string(raw))
}
