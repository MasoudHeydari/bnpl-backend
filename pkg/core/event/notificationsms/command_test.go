package notificationsms_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
)

func TestNewCommand_MatchesNotificationContract(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	event := notificationsms.NewCommand(notificationsms.CommandInput{
		SourceEventID:      "33333333-3333-4333-8333-333333333333",
		IdempotencyKey:     "repayment:44444444-4444-4444-8444-444444444444:pre_due:2026-11-20",
		NotificationType:   "bnpl.repayment.pre_due.v1",
		ExternalCustomerID: "55555555-5555-4555-8555-555555555555",
		InstallmentID:      "44444444-4444-4444-8444-444444444444",
		PaymentURL:         "https://pay.example/fictional",
		ExpiresAt:          expires,
	})

	require.NoError(t, notificationsms.ValidateCommand(event))

	raw, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.EqualValues(t, 1, decoded["schema_version"])
	require.Equal(t, "bnpl-backend", decoded["source_service"])
	require.EqualValues(t, 5, decoded["priority"])
	require.NotContains(t, decoded, "correlation_id")
	require.NotContains(t, decoded, "event_type")
	require.NotContains(t, decoded, "recipient")

	targets := decoded["targets"].([]any)
	target := targets[0].(map[string]any)
	require.Equal(t, "BNPL_INSTALLMENT", target["business_reference_type"])
	require.NotContains(t, target, "business_scope")

	delivery := target["deliveries"].([]any)[0].(map[string]any)
	require.Equal(t, "upstream_selected", delivery["selection_source"])
	require.Equal(t, "bnpl.repayment.pre_due.v1", delivery["template_code"])
}

func TestValidateCommand_RejectsNonHTTPSPaymentURL(t *testing.T) {
	t.Parallel()

	event := notificationsms.NewCommand(notificationsms.CommandInput{
		SourceEventID:      "33333333-3333-4333-8333-333333333333",
		IdempotencyKey:     "key",
		NotificationType:   "bnpl.repayment.pre_due.v1",
		ExternalCustomerID: "55555555-5555-4555-8555-555555555555",
		InstallmentID:      "44444444-4444-4444-8444-444444444444",
		PaymentURL:         "dpay.ir/s/abc",
		ExpiresAt:          time.Now().UTC().Add(time.Hour),
	})
	require.Error(t, notificationsms.ValidateCommand(event))
}
