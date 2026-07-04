package audit

import (
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	t.Parallel()

	actorID := "operator-123"
	correlationID := "checkout-123"
	causationEventID := "payment-result-123"
	traceID := "trace-123"
	occurredAt := time.Date(2026, time.August, 2, 13, 8, 0, 0, time.FixedZone("IRST", 3*60*60+30*60))

	event := NewEvent(
		"audit-event-123",
		correlationID,
		&causationEventID,
		traceID,
		occurredAt,
		EventData{
			Actor: Actor{
				Type: OperatorActorType,
				ID:   &actorID,
			},
			Action: "contract.activate",
			Resource: Resource{
				Type: "contract",
				ID:   "contract-123",
			},
			Outcome: SucceededOutcome,
		},
	)

	if event.EventType != EventType {
		t.Fatalf("event type = %q, want %q", event.EventType, EventType)
	}

	if event.EventVersion != EventVersion {
		t.Fatalf("event version = %q, want %q", event.EventVersion, EventVersion)
	}

	if event.Producer != ProducerName {
		t.Fatalf("producer = %q, want %q", event.Producer, ProducerName)
	}

	if event.CorrelationID != correlationID {
		t.Fatalf("correlation id = %q, want %q", event.CorrelationID, correlationID)
	}

	if event.TraceID != traceID {
		t.Fatalf("trace id = %q, want %q", event.TraceID, traceID)
	}

	if event.OccurredAt.Location() != time.UTC {
		t.Fatalf("occurred at location = %q, want UTC", event.OccurredAt.Location())
	}

	if !event.OccurredAt.Equal(occurredAt) {
		t.Fatalf("occurred at = %v, want %v", event.OccurredAt, occurredAt)
	}
}

func TestValidateEvent(t *testing.T) {
	t.Parallel()

	service := ProducerName
	event := NewEvent(
		"audit-event-123",
		"corr-123",
		nil,
		"trace-123",
		time.Now(),
		EventData{
			Actor: Actor{
				Type:    ServiceActorType,
				Service: &service,
			},
			Action: "contract.activate",
			Resource: Resource{
				Type: "contract",
				ID:   "contract-123",
			},
			Outcome: SucceededOutcome,
			Changes: []FieldChange{
				{
					Field:  "status",
					Before: "payment_pending",
					After:  "active",
				},
			},
		},
	)

	if err := ValidateEvent(event); err != nil {
		t.Fatalf("validate event: %v", err)
	}
}

func TestActorType_IsValid(t *testing.T) {
	t.Parallel()

	if !SystemActorType.IsValid() {
		t.Fatal("expected system actor type to be valid")
	}

	if ActorType("unknown").IsValid() {
		t.Fatal("expected unknown actor type to be invalid")
	}
}

func TestOutcome_IsValid(t *testing.T) {
	t.Parallel()

	if !SucceededOutcome.IsValid() {
		t.Fatal("expected succeeded outcome to be valid")
	}

	if Outcome("unknown").IsValid() {
		t.Fatal("expected unknown outcome to be invalid")
	}
}
