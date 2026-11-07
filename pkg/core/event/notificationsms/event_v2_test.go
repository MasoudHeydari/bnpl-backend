package notificationsms_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/event/notificationsms"
	defaultnotificationv1 "github.com/parstis/bnpl-backend/pkg/core/policy/defaultnotification/v1"
	reminderv1 "github.com/parstis/bnpl-backend/pkg/core/policy/repaymentreminder/v1"
)

func TestNewEventV2_JSONShapeAndCustomerMapping(t *testing.T) {
	t.Parallel()

	sourceEventID := uuid.NewString()
	correlationID := uuid.NewString()
	externalCustomerID := uuid.NewString()
	installmentID := uuid.NewString()
	idempotencyKey := "repayment:" + installmentID + ":pre_due:2026-11-07"
	expiresAt := time.Date(2026, time.November, 8, 0, 0, 0, 0, time.UTC)

	event := notificationsms.NewEventV2(notificationsms.EventV2Input{
		SourceEventID:      sourceEventID,
		IdempotencyKey:     idempotencyKey,
		CorrelationID:      correlationID,
		NotificationType:   reminderv1.TemplatePreDue,
		ExternalCustomerID: externalCustomerID,
		InstallmentID:      installmentID,
		PaymentURL:         "https://dpay.ir/abc123",
		ExpiresAt:          expiresAt,
	})
	require.NoError(t, notificationsms.ValidateEventV2(event))

	raw, err := json.Marshal(event)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))

	require.EqualValues(t, 2, decoded["schema_version"])
	require.Equal(t, "bnpl-backend", decoded["source_service"])
	require.Equal(t, sourceEventID, decoded["source_event_id"])
	require.Equal(t, idempotencyKey, decoded["idempotency_key"])
	require.Equal(t, reminderv1.TemplatePreDue, decoded["notification_type"])
	require.Equal(t, "transactional", decoded["category"])
	require.Equal(t, correlationID, decoded["correlation_id"])
	require.Equal(t, "2026-11-08T00:00:00Z", decoded["expires_at"])

	targets, ok := decoded["targets"].([]any)
	require.True(t, ok)
	require.Len(t, targets, 1)

	target, ok := targets[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "customer", target["target_key"])
	require.Equal(t, "customer", target["target_type"])
	require.Equal(t, externalCustomerID, target["customer_id"])
	require.Equal(t, "bnpl", target["business_scope"])
	require.Equal(t, "installment", target["business_reference_type"])
	require.Equal(t, installmentID, target["business_reference_id"])
	require.NotContains(t, target, "recipient_address")
	require.NotContains(t, target, "recipient_hash")

	deliveries, ok := target["deliveries"].([]any)
	require.True(t, ok)
	require.Len(t, deliveries, 1)

	delivery, ok := deliveries[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "sms", delivery["delivery_method"])
	require.Equal(t, "sms", delivery["channel"])
	require.Equal(t, reminderv1.TemplatePreDue, delivery["template_code"])

	vars, ok := delivery["variables"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "https://dpay.ir/abc123", vars["payment_url"])
	require.NotContains(t, vars, "recipient_address")
	require.NotContains(t, vars, "recipient_hash")
}

func TestNewEventV2_AllSupportedTemplates(t *testing.T) {
	t.Parallel()

	templates := []string{
		reminderv1.TemplatePreDue,
		reminderv1.TemplateDueToday,
		reminderv1.TemplateGracePeriod,
		reminderv1.TemplateGraceFinalDay,
		defaultnotificationv1.TemplateID,
	}

	for _, template := range templates {
		template := template
		t.Run(template, func(t *testing.T) {
			t.Parallel()

			event := notificationsms.NewEventV2(notificationsms.EventV2Input{
				SourceEventID:      uuid.NewString(),
				IdempotencyKey:     "key-" + template,
				CorrelationID:      uuid.NewString(),
				NotificationType:   template,
				ExternalCustomerID: uuid.NewString(),
				InstallmentID:      uuid.NewString(),
				PaymentURL:         "https://dpay.ir/x",
				ExpiresAt:          time.Now().UTC().Add(time.Hour),
			})
			require.NoError(t, notificationsms.ValidateEventV2(event))
			require.Equal(t, template, event.NotificationType)
			require.Equal(t, template, event.Targets[0].Deliveries[0].TemplateCode)
			require.Equal(t, "installment", event.Targets[0].BusinessReferenceType)
		})
	}
}

func TestDeliveryDeadlineForBusinessDate_NextLocalMidnightUTC(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	businessDate := time.Date(2026, time.November, 7, 0, 0, 0, 0, loc)
	deadline := notificationsms.DeliveryDeadlineForBusinessDate(businessDate)

	expectedLocalNext := time.Date(2026, time.November, 8, 0, 0, 0, 0, loc)
	require.True(t, deadline.Equal(expectedLocalNext.UTC()))
	require.Equal(t, time.UTC, deadline.Location())

	// Must not be a fixed 24h add from an arbitrary wall clock.
	now := time.Date(2026, time.November, 7, 15, 30, 0, 0, loc)
	require.NotEqual(t, now.Add(24*time.Hour).UTC(), deadline)
}

func TestValidateEventV2_RejectsPhoneFields(t *testing.T) {
	t.Parallel()

	event := notificationsms.NewEventV2(notificationsms.EventV2Input{
		SourceEventID:      uuid.NewString(),
		IdempotencyKey:     "k",
		CorrelationID:      uuid.NewString(),
		NotificationType:   reminderv1.TemplateDueToday,
		ExternalCustomerID: uuid.NewString(),
		InstallmentID:      uuid.NewString(),
		PaymentURL:         "https://dpay.ir/x",
		ExpiresAt:          time.Now().UTC().Add(time.Hour),
	})
	event.Targets[0].Deliveries[0].Variables["recipient_address"] = "+989121234567"
	require.Error(t, notificationsms.ValidateEventV2(event))

	delete(event.Targets[0].Deliveries[0].Variables, "recipient_address")
	event.Targets[0].Deliveries[0].Variables["recipient_hash"] = "abc"
	require.Error(t, notificationsms.ValidateEventV2(event))
}

func TestNewEventV2_StableIdentityFields(t *testing.T) {
	t.Parallel()

	input := notificationsms.EventV2Input{
		SourceEventID:      "11111111-1111-1111-1111-111111111111",
		IdempotencyKey:     "repayment:22222222-2222-2222-2222-222222222222:due_today:2026-11-07",
		CorrelationID:      "33333333-3333-3333-3333-333333333333",
		NotificationType:   reminderv1.TemplateDueToday,
		ExternalCustomerID: "44444444-4444-4444-4444-444444444444",
		InstallmentID:      "22222222-2222-2222-2222-222222222222",
		PaymentURL:         "https://dpay.ir/stable",
		ExpiresAt:          time.Date(2026, time.November, 8, 0, 0, 0, 0, time.UTC),
	}

	first := notificationsms.NewEventV2(input)
	second := notificationsms.NewEventV2(input)

	firstRaw, err := json.Marshal(first)
	require.NoError(t, err)
	secondRaw, err := json.Marshal(second)
	require.NoError(t, err)
	require.Equal(t, string(firstRaw), string(secondRaw))
}
