package audit

import (
	"errors"
	"fmt"
	"time"
)

const (
	// EventType is the machine-readable outbox event type for BNPL audit records.
	EventType = "bnpl.audit_event.v1"

	// EventVersion is the schema version of the audit event envelope.
	EventVersion = "1"

	// ProducerName identifies BNPL as the producer of audit events.
	ProducerName = "bnpl-backend"
)

// Event is the outbound audit envelope published through the transactional outbox.
//
// An audit event records an attributable fact about an action performed on a
// resource. It is evidence for audit and forensic use and must not be consumed as
// a command or used as the source of domain authority.
type Event struct {
	// EventID uniquely identifies this audit event.
	EventID string `json:"event_id"`
	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`
	// EventVersion is the schema version of the audit event envelope.
	EventVersion string `json:"event_version"`
	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`
	// OccurredAt is when the audited action occurred.
	OccurredAt time.Time `json:"occurred_at"`

	// CorrelationID links this event to a request or workflow correlation.
	CorrelationID string `json:"correlation_id,omitempty"`
	// CausationEventID references the upstream event that caused this audit record.
	CausationEventID *string `json:"causation_event_id,omitempty"`
	// TraceID carries the distributed trace identifier for forensic correlation.
	TraceID string `json:"trace_id,omitempty"`

	// Data carries the attributable details of the audited action.
	Data EventData `json:"data"`
}

// EventData carries the attributable details of an audited action.
type EventData struct {
	// Actor identifies the principal responsible for the audited action.
	Actor Actor `json:"actor"`
	// Action is the machine-readable verb describing what was done.
	Action string `json:"action"`
	// Resource identifies the domain object affected by the action.
	Resource Resource `json:"resource"`
	// Outcome is the result of the audited action.
	Outcome Outcome `json:"outcome"`

	// ReasonCode is an optional machine-readable reason supporting Outcome.
	ReasonCode *string `json:"reason_code,omitempty"`
	// Changes lists allowlisted before-and-after field changes, when applicable.
	Changes []FieldChange `json:"changes,omitempty"`
	// Metadata holds optional non-sensitive structured context for the audit record.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// NewEvent constructs a versioned audit event envelope.
//
// The caller supplies the event identity, causal context, occurrence time, and
// event-specific data. The constructor owns the stable envelope fields and always
// normalizes OccurredAt to UTC.
func NewEvent(
	eventID string,
	correlationID string,
	causationEventID *string,
	traceID string,
	occurredAt time.Time,
	data EventData,
) Event {
	return Event{
		EventID:          eventID,
		EventType:        EventType,
		EventVersion:     EventVersion,
		Producer:         ProducerName,
		OccurredAt:       occurredAt.UTC(),
		CorrelationID:    correlationID,
		CausationEventID: causationEventID,
		TraceID:          traceID,
		Data:             data,
	}
}

// ValidateEvent validates an audit event before outbox insertion.
func ValidateEvent(event Event) error {
	const op = "audit.ValidateEvent"

	if event.EventID == "" {
		return errors.New("event id is required")
	}

	if event.EventType != EventType {
		return fmt.Errorf("%s: event type must be %q", op, EventType)
	}

	if event.EventVersion != EventVersion {
		return fmt.Errorf("%s: event version must be %q", op, EventVersion)
	}

	if event.Producer != ProducerName {
		return fmt.Errorf("%s: producer must be %q", op, ProducerName)
	}

	if event.OccurredAt.IsZero() {
		return errors.New("occurred at is required")
	}

	if err := event.Data.Validate(); err != nil {
		return fmt.Errorf("%s: event data: %w", op, err)
	}

	return nil
}

// Validate validates the audited action data independently of its event envelope.
func (d EventData) Validate() error {
	const op = "audit.EventData.Validate"

	if err := d.Actor.Validate(); err != nil {
		return fmt.Errorf("%s: actor: %w", op, err)
	}

	if d.Action == "" {
		return errors.New("action is required")
	}

	if err := d.Resource.Validate(); err != nil {
		return fmt.Errorf("%s: resource: %w", op, err)
	}

	if !d.Outcome.IsValid() {
		return fmt.Errorf("%s: unsupported outcome %q", op, d.Outcome)
	}

	for index, change := range d.Changes {
		if err := change.Validate(); err != nil {
			return fmt.Errorf("%s: change at index %d: %w", op, index, err)
		}
	}

	return nil
}
