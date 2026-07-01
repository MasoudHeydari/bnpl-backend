package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OutboxStatus identifies the delivery state of an outbox message.
type OutboxStatus string

const (
	// OutboxStatusPending means the message is ready or scheduled for delivery.
	OutboxStatusPending OutboxStatus = "pending"

	// OutboxStatusProcessing means a publisher worker has claimed the message.
	OutboxStatusProcessing OutboxStatus = "processing"

	// OutboxStatusPublished means the message was delivered successfully.
	OutboxStatusPublished OutboxStatus = "published"

	// OutboxStatusFailed means the message exceeded retry limits.
	OutboxStatusFailed OutboxStatus = "failed"
)

// OutboxMessage represents a reliable integration message.
//
// Outbox messages are written in the same database transaction as the business
// change that produced them. A publisher worker later delivers pending messages
// to their destination and updates the delivery status.
type OutboxMessage struct {
	// ID uniquely identifies the message.
	ID uuid.UUID `db:"id" json:"id"`

	// Destination identifies the external destination.
	Destination string `db:"destination" json:"destination,omitempty"`

	// EventType identifies the message type.
	EventType string `db:"event_type" json:"event_type,omitempty"`

	// AggregateType identifies the related aggregate/entity type.
	AggregateType *string `db:"aggregate_type" json:"aggregate_type,omitempty"`

	// Payload contains the serialized event payload.
	Payload json.RawMessage `db:"payload" json:"payload,omitempty"`

	// Status is the current delivery state.
	Status OutboxStatus `db:"status" json:"status,omitempty"`

	// AttemptCount is the number of delivery attempts already performed.
	AttemptCount int `db:"attempt_count" json:"attempt_count,omitempty"`

	// MaxAttempts is the maximum number of allowed delivery attempts.
	MaxAttempts int `db:"max_attempts" json:"max_attempts,omitempty"`

	// NextAttemptAt is the earliest time the message may be retried.
	NextAttemptAt time.Time `db:"next_attempt_at" json:"next_attempt_at"`

	// LockedAt is set when a publisher worker claims this message.
	LockedAt *time.Time `db:"locked_at" json:"locked_at,omitempty"`

	// LockedBy identifies the publisher worker that claimed this message.
	LockedBy *string `db:"locked_by" json:"locked_by,omitempty"`

	// LastError stores the last delivery error.
	LastError *string `db:"last_error" json:"last_error,omitempty"`

	// CreatedAt is the message creation time.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// PublishedAt is set after successful delivery.
	PublishedAt *time.Time `db:"published_at" json:"published_at,omitempty"`
}
