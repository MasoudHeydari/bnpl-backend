package merchantparticipation

import (
	"encoding/json"
	"testing"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/stretchr/testify/require"
)

func fixture() messaging.Message {
	const merchantID = "c4f139c9-8a44-42dd-9aad-6a0f60200b6d"
	const eventID = "a5166f33-b950-4355-a835-a8216014d6be"
	return messaging.Message{
		Key:     []byte(merchantID),
		Value:   []byte(`{"event_id":"a5166f33-b950-4355-a835-a8216014d6be","event_type":"merchant_product_participation_requested","schema_version":1,"correlation_id":"c4f139c9-8a44-42dd-9aad-6a0f60200b6d","occurred_at":"2026-11-28T08:30:00Z","actor_subject_id":"f014c11d-be85-47e2-adde-29618dcbb2f6","actor_membership_id":null,"actor_context_id":null,"merchant_id":"c4f139c9-8a44-42dd-9aad-6a0f60200b6d","application_id":"4d16a4fa-8bdc-4e50-9bb7-6d88fd7c1846","request_id":"0bb456eb-945a-401d-b5d7-d51a974a7426","business_line_code":"bnpl","environment":"SANDBOX"}`),
		Headers: map[string][]byte{"x-event-id": []byte(eventID), "x-event-type": []byte(eventType), "x-aggregate-type": []byte("merchant_enrollment"), "x-aggregate-id": []byte(merchantID), "x-schema-version": []byte("1")},
	}
}

func TestDecodeContract(t *testing.T) {
	msg := fixture()
	input, err := decode(msg, sha256.New())
	require.NoError(t, err)
	require.Equal(t, string(msg.Key), input.MerchantID.String())
	require.Equal(t, "SANDBOX", input.Environment)
	require.Equal(t, string(msg.Key), *input.Event.CorrelationID)
	var value map[string]any
	require.NoError(t, json.Unmarshal(msg.Value, &value))
	msg.Value, err = json.MarshalIndent(value, "", "  ")
	require.NoError(t, err)
	repeat, err := decode(msg, sha256.New())
	require.NoError(t, err)
	require.Equal(t, input.Event.PayloadFingerprint, repeat.Event.PayloadFingerprint)
}

func TestDecodeRejectsInvalidContract(t *testing.T) {
	cases := map[string]func(*messaging.Message){
		"key":                  func(m *messaging.Message) { m.Key = []byte("other") },
		"schema header":        func(m *messaging.Message) { m.Headers["x-schema-version"] = []byte("2") },
		"missing event header": func(m *messaging.Message) { delete(m.Headers, "x-event-id") },
		"correlation header":   func(m *messaging.Message) { m.Headers["x-correlation-id"] = []byte("wrong") },
		"live": func(m *messaging.Message) {
			var v map[string]any
			_ = json.Unmarshal(m.Value, &v)
			v["environment"] = "LIVE"
			m.Value, _ = json.Marshal(v)
		},
		"nested payload": func(m *messaging.Message) { m.Value = []byte(`{"payload":{}}`) },
		"trailing data":  func(m *messaging.Message) { m.Value = append(m.Value, []byte(`{}`)...) },
	}
	for name, alter := range cases {
		t.Run(name, func(t *testing.T) {
			msg := fixture()
			alter(&msg)
			_, err := decode(msg, sha256.New())
			require.Error(t, err)
		})
	}
}
