package notificationsms

import (
	"fmt"
	"strings"
	"time"
)

const (
	// EventType is the machine-readable outbox / Kafka event type for SMS requests.
	EventType = "notification.sms.requested.v1"

	// EventVersion is the envelope schema version.
	EventVersion = "1"

	// ProducerName identifies BNPL as the producer of SMS notification requests.
	ProducerName = "bnpl-backend"

	// RecipientTypeCustomer identifies an external customer recipient.
	RecipientTypeCustomer = "customer"
)

// Recipient identifies who should receive the SMS.
//
// CustomerID must be the external Customer Reference / Identity identifier
// (ExternalCustomerID), never the BNPL-local customer_profiles.id. Notification
// Service resolves the phone number from this identifier.
type Recipient struct {
	// Type is the recipient classification (currently only "customer").
	Type string `json:"type"`

	// CustomerID is the external customer identifier recognized by Notification Service.
	CustomerID string `json:"customer_id"`
}

// Event is the outbound SMS notification request envelope.
//
// This is the BNPL-side contract published via the transactional outbox. The
// Notification Service consumer contract must treat IdempotencyKey as the
// durable deduplication key. BNPL does not claim end-to-end SMS delivery from
// successful Kafka publication alone.
type Event struct {
	// EventID uniquely identifies this notification request event.
	EventID string `json:"event_id"`

	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`

	// EventVersion is the envelope schema version.
	EventVersion string `json:"event_version"`

	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`

	// OccurredAt is when BNPL produced the request.
	OccurredAt time.Time `json:"occurred_at"`

	// IdempotencyKey is the stable consumer deduplication key.
	IdempotencyKey string `json:"idempotency_key"`

	// Recipient identifies who should receive the SMS.
	Recipient Recipient `json:"recipient"`

	// TemplateID selects the Notification Service message template.
	TemplateID string `json:"template_id"`

	// Variables are non-PII template substitutions (for example payment_url).
	Variables map[string]string `json:"variables"`
}

// NewEvent constructs an SMS notification request envelope.
func NewEvent(
	eventID string,
	idempotencyKey string,
	occurredAt time.Time,
	externalCustomerID string,
	templateID string,
	variables map[string]string,
) Event {
	vars := variables
	if vars == nil {
		vars = map[string]string{}
	}

	return Event{
		EventID:        eventID,
		EventType:      EventType,
		EventVersion:   EventVersion,
		Producer:       ProducerName,
		OccurredAt:     occurredAt.UTC(),
		IdempotencyKey: idempotencyKey,
		Recipient: Recipient{
			Type:       RecipientTypeCustomer,
			CustomerID: externalCustomerID,
		},
		TemplateID: templateID,
		Variables:  vars,
	}
}

// ValidateEvent validates an SMS notification request before outbox insertion.
func ValidateEvent(event Event) error {
	const op = "notificationsms.ValidateEvent"

	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("%s: event id is required", op)
	}

	if event.EventType != EventType {
		return fmt.Errorf("%s: event type must be %q", op, EventType)
	}

	if event.EventVersion != EventVersion {
		return fmt.Errorf("%s: event version must be %q", op, EventVersion)
	}

	if strings.TrimSpace(event.IdempotencyKey) == "" {
		return fmt.Errorf("%s: idempotency key is required", op)
	}

	if event.Recipient.Type != RecipientTypeCustomer {
		return fmt.Errorf("%s: recipient type must be %q", op, RecipientTypeCustomer)
	}

	if strings.TrimSpace(event.Recipient.CustomerID) == "" {
		return fmt.Errorf("%s: recipient customer id is required", op)
	}

	if strings.TrimSpace(event.TemplateID) == "" {
		return fmt.Errorf("%s: template id is required", op)
	}

	if event.Variables == nil {
		return fmt.Errorf("%s: variables must not be nil", op)
	}

	return nil
}
