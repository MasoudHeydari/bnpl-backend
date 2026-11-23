package notificationsms_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
)

func TestNewCreditDebitedCommand_MatchesNotificationContract(t *testing.T) {
	t.Parallel()

	expires := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	event := notificationsms.NewCreditDebitedCommand(notificationsms.CreditDebitedCommandInput{
		SourceEventID:            "11111111-1111-4111-8111-111111111111",
		IdempotencyKey:           "bnpl.credit.debited:11111111-1111-4111-8111-111111111111",
		NotificationType:         notificationsms.NotificationTypeCreditMonthlyDebited,
		ExternalCustomerID:       "55555555-5555-4555-8555-555555555555",
		CheckoutSessionID:        "22222222-2222-4222-8222-222222222222",
		DebitedAmountMinor:       500000,
		RemainingAvailableCredit: 3000000,
		Currency:                 "IRR",
		ExpiresAt:                expires,
	})

	require.NoError(t, notificationsms.ValidateCreditDebitedCommand(event))

	raw, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.EqualValues(t, 1, decoded["schema_version"])
	require.Equal(t, "bnpl-backend", decoded["source_service"])
	require.Equal(t, "bnpl.credit.pay_next_month.debited.v1", decoded["notification_type"])
	require.Equal(t, "transactional", decoded["category"])
	require.EqualValues(t, 5, decoded["priority"])
	require.NotContains(t, decoded, "x-notification-caller")
	require.NotContains(t, decoded, "correlation_id")
	require.NotContains(t, decoded, "phone")
	require.NotContains(t, decoded, "message")

	targets := decoded["targets"].([]any)
	target := targets[0].(map[string]any)
	require.Equal(t, "55555555-5555-4555-8555-555555555555", target["customer_id"])
	require.Equal(t, "BNPL_PURCHASE", target["business_reference_type"])
	require.Equal(t, "cs_22222222-2222-4222-8222-222222222222", target["business_reference_id"])

	delivery := target["deliveries"].([]any)[0].(map[string]any)
	require.Equal(t, "sms", delivery["delivery_method"])
	require.Equal(t, "sms", delivery["channel"])
	require.Equal(t, "upstream_selected", delivery["selection_source"])
	require.Equal(t, "bnpl.credit.pay_next_month.debited.v1", delivery["template_code"])

	vars := delivery["variables"].(map[string]any)
	require.Equal(t, "500000", vars["debited_amount"])
	require.Equal(t, "3000000", vars["remaining_available_credit"])
	require.Equal(t, "IRR", vars["currency"])
	require.NotContains(t, vars, "payment_url")
	require.NotContains(t, vars, "credit_type")
}

func TestNewCreditDebitedCommand_InstallmentTemplate(t *testing.T) {
	t.Parallel()

	event := notificationsms.NewCreditDebitedCommand(notificationsms.CreditDebitedCommandInput{
		SourceEventID:            "11111111-1111-4111-8111-111111111111",
		IdempotencyKey:           notificationsms.CreditDebitedIdempotencyKey("11111111-1111-4111-8111-111111111111"),
		NotificationType:         notificationsms.NotificationTypeCreditInstallmentDebited,
		ExternalCustomerID:       "55555555-5555-4555-8555-555555555555",
		CheckoutSessionID:        "22222222-2222-4222-8222-222222222222",
		DebitedAmountMinor:       1,
		RemainingAvailableCredit: 0,
		Currency:                 "IRR",
		ExpiresAt:                time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	})
	require.NoError(t, notificationsms.ValidateCreditDebitedCommand(event))
	require.Equal(t, notificationsms.NotificationTypeCreditInstallmentDebited, event.NotificationType)
	require.Equal(t, notificationsms.NotificationTypeCreditInstallmentDebited, event.Targets[0].Deliveries[0].TemplateCode)
}

func TestValidateCreditDebitedCommand_RejectsPaymentURLAndZeroDebit(t *testing.T) {
	t.Parallel()

	event := notificationsms.NewCreditDebitedCommand(notificationsms.CreditDebitedCommandInput{
		SourceEventID:            "11111111-1111-4111-8111-111111111111",
		IdempotencyKey:           notificationsms.CreditDebitedIdempotencyKey("11111111-1111-4111-8111-111111111111"),
		NotificationType:         notificationsms.NotificationTypeCreditMonthlyDebited,
		ExternalCustomerID:       "55555555-5555-4555-8555-555555555555",
		CheckoutSessionID:        "22222222-2222-4222-8222-222222222222",
		DebitedAmountMinor:       100,
		RemainingAvailableCredit: 0,
		Currency:                 "IRR",
		ExpiresAt:                time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	})
	event.Targets[0].Deliveries[0].Variables["payment_url"] = "https://pay.example/x"
	require.Error(t, notificationsms.ValidateCreditDebitedCommand(event))

	event = notificationsms.NewCreditDebitedCommand(notificationsms.CreditDebitedCommandInput{
		SourceEventID:            "11111111-1111-4111-8111-111111111111",
		IdempotencyKey:           notificationsms.CreditDebitedIdempotencyKey("11111111-1111-4111-8111-111111111111"),
		NotificationType:         notificationsms.NotificationTypeCreditMonthlyDebited,
		ExternalCustomerID:       "55555555-5555-4555-8555-555555555555",
		CheckoutSessionID:        "22222222-2222-4222-8222-222222222222",
		DebitedAmountMinor:       0,
		RemainingAvailableCredit: 0,
		Currency:                 "IRR",
		ExpiresAt:                time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC),
	})
	require.Error(t, notificationsms.ValidateCreditDebitedCommand(event))
}

func TestValidateCommand_StillRequiresPaymentURL(t *testing.T) {
	t.Parallel()

	event := notificationsms.NewCommand(notificationsms.CommandInput{
		SourceEventID:      "33333333-3333-4333-8333-333333333333",
		IdempotencyKey:     "key",
		NotificationType:   "bnpl.repayment.pre_due.v1",
		ExternalCustomerID: "55555555-5555-4555-8555-555555555555",
		InstallmentID:      "44444444-4444-4444-8444-444444444444",
		PaymentURL:         "https://pay.example/fictional",
		ExpiresAt:          time.Now().UTC().Add(time.Hour),
	})
	require.NoError(t, notificationsms.ValidateCommand(event))

	delete(event.Targets[0].Deliveries[0].Variables, "payment_url")
	require.Error(t, notificationsms.ValidateCommand(event))
}
