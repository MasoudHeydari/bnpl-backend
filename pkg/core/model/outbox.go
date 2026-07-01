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
	ID uuid.UUID

	// Destination identifies the external destination.
	Destination string

	// EventType identifies the message type.
	EventType string

	// AggregateType identifies the related aggregate/entity type.
	AggregateType *string

	// Payload contains the serialized event payload.
	Payload json.RawMessage

	// Status is the current delivery state.
	Status OutboxStatus

	// AttemptCount is the number of delivery attempts already performed.
	AttemptCount int

	// MaxAttempts is the maximum number of allowed delivery attempts.
	MaxAttempts int

	// NextAttemptAt is the earliest time the message may be retried.
	NextAttemptAt time.Time

	// LockedAt is set when a publisher worker claims this message.
	LockedAt *time.Time

	// LockedBy identifies the publisher worker that claimed this message.
	LockedBy *string

	// LastError stores the last delivery error.
	LastError *string

	// CreatedAt is the message creation time.
	CreatedAt time.Time

	// PublishedAt is set after successful delivery.
	PublishedAt *time.Time
}
