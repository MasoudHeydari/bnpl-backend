package checkoutlifecycle

import (
	"fmt"
	"time"
)

const (
	// EventTypeCompleted is published when a checkout produces an activated contract.
	EventTypeCompleted = "bnpl.checkout.completed.v1"

	// EventTypeCanceled is published when a checkout is abandoned by customer or merchant.
	EventTypeCanceled = "bnpl.checkout.canceled.v1"

	// EventTypeExpired is published when an open handoff lifetime elapses.
	EventTypeExpired = "bnpl.checkout.expired.v1"

	// EventTypeUnavailable is published when financing cannot be offered for a checkout.
	EventTypeUnavailable = "bnpl.checkout.unavailable.v1"

	// EventVersion is the schema version of the checkout lifecycle envelope.
	EventVersion = "1"

	// ProducerName identifies BNPL as the producer of checkout lifecycle events.
	ProducerName = "bnpl-backend"

	// AggregateType names the aggregate that produced the event.
	AggregateType = "checkout_session"
)

// Event is the merchant-facing checkout lifecycle envelope published through
// the transactional outbox.
//
// Payment internals (attempt ids, Financial Core payment ids, risk reason codes)
// are intentionally absent. Merchants learn outcome through these events or the
// status API — never from browser return query parameters.
type Event struct {
	// EventID uniquely identifies this lifecycle event for merchant deduplication.
	EventID string `json:"event_id"`

	// EventType is the machine-readable outbox event type.
	EventType string `json:"event_type"`

	// EventVersion is the schema version of the envelope.
	EventVersion string `json:"event_version"`

	// Producer identifies the service that produced the event.
	Producer string `json:"producer"`

	// OccurredAt is when the lifecycle transition occurred.
	OccurredAt time.Time `json:"occurred_at"`

	// Data carries merchant-safe checkout facts.
	Data EventData `json:"data"`
}

// EventData carries merchant-safe checkout lifecycle details.
//
// It never includes customer credit, installment schedules, provider selection,
// or Financial Core payment identifiers. Merchants verify outcomes with
// event_id, signature, checkout_session_id, merchant_order_id, amount_minor,
// currency, and status.
type EventData struct {
	// CheckoutSessionID is the merchant-visible checkout identifier.
	CheckoutSessionID string `json:"checkout_session_id"`

	// MerchantOrderID is the merchant-owned order reference.
	MerchantOrderID string `json:"merchant_order_id"`

	// MerchantID is the external merchant identity that owns the checkout.
	MerchantID string `json:"merchant_id"`

	// Status is the merchant-facing checkout status after the transition.
	Status string `json:"status"`

	// AmountMinor is the checkout requested amount in minor units.
	AmountMinor uint64 `json:"amount_minor"`

	// Currency is the checkout currency.
	Currency string `json:"currency"`

	// CompletedAt is set only for completed events.
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Reason elaborates cancellation when Status is canceled.
	Reason string `json:"reason,omitempty"`

	// MerchantState echoes the opaque merchant correlation value when present.
	MerchantState string `json:"merchant_state,omitempty"`
}

// NewEvent constructs a versioned checkout lifecycle event envelope.
func NewEvent(eventID string, eventType string, occurredAt time.Time, data EventData) (Event, error) {
	if eventID == "" {
		return Event{}, fmt.Errorf("checkoutlifecycle.NewEvent: event id is required")
	}

	switch eventType {
	case EventTypeCompleted, EventTypeCanceled, EventTypeExpired, EventTypeUnavailable:
		// Allowed lifecycle types.
	default:
		return Event{}, fmt.Errorf("checkoutlifecycle.NewEvent: unsupported event type %q", eventType)
	}

	return Event{
		EventID:      eventID,
		EventType:    eventType,
		EventVersion: EventVersion,
		Producer:     ProducerName,
		OccurredAt:   occurredAt.UTC(),
		Data:         data,
	}, nil
}
