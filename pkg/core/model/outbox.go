package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// OutboxID uniquely identifies an outbox message.
type OutboxID uuid.UUID

// ParseOutboxID parses an OutboxID from its canonical string form.
func ParseOutboxID(value string) (OutboxID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return OutboxID{}, fmt.Errorf("parse outbox id: %w", err)
	}

	return OutboxID(id), nil
}

func (id OutboxID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id OutboxID) String() string { return uuid.UUID(id).String() }

func (id OutboxID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id OutboxID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *OutboxID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = OutboxID(u)

	return nil
}

func (id *OutboxID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = OutboxID(u)

	return nil
}

func (id OutboxID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// OutboxStatus identifies the delivery state of an outbox message.
type OutboxStatus string

const (
	// PendingOutboxStatus means the message is ready or scheduled for delivery.
	PendingOutboxStatus OutboxStatus = "pending"

	// ProcessingOutboxStatus means a publisher worker has claimed the message.
	ProcessingOutboxStatus OutboxStatus = "processing"

	// PublishedOutboxStatus means the message was delivered successfully.
	PublishedOutboxStatus OutboxStatus = "published"

	// SkippedOutboxStatus means delivery was intentionally skipped (for example
	// external.webhook_dispatcher.provider=noop). It is terminal and must not be read as
	// a successful merchant delivery.
	SkippedOutboxStatus OutboxStatus = "skipped"

	// FailedOutboxStatus means the message exceeded retry limits.
	FailedOutboxStatus OutboxStatus = "failed"
)

var outboxStatusTransitions = newStatusTransitionTable(
	"outbox status",
	map[OutboxStatus][]OutboxStatus{
		PendingOutboxStatus: {
			ProcessingOutboxStatus,
		},
		ProcessingOutboxStatus: {
			PendingOutboxStatus,
			PublishedOutboxStatus,
			SkippedOutboxStatus,
			FailedOutboxStatus,
		},
	},
	[]OutboxStatus{
		PublishedOutboxStatus,
		SkippedOutboxStatus,
		FailedOutboxStatus,
	},
)

// IsValid reports whether the outbox status is known by the domain.
func (s OutboxStatus) IsValid() bool {
	return outboxStatusTransitions.isValid(s)
}

// CanTransitionTo reports whether the outbox status may change to next.
func (s OutboxStatus) CanTransitionTo(next OutboxStatus) bool {
	return outboxStatusTransitions.canTransitionTo(s, next)
}

// ValidateTransitionTo returns an error when the outbox status may not change to next.
func (s OutboxStatus) ValidateTransitionTo(next OutboxStatus) error {
	return outboxStatusTransitions.validateTransitionTo(s, next)
}

// IsTerminal reports whether the outbox status is terminal.
func (s OutboxStatus) IsTerminal() bool {
	return outboxStatusTransitions.isTerminal(s)
}

// OutboxMessage represents a reliable integration message.
//
// Outbox messages are written in the same database transaction as the business
// change that produced them. A publisher worker later delivers pending messages
// to their destination and updates the delivery status.
type OutboxMessage struct {
	// ID uniquely identifies the message.
	ID OutboxID `db:"id" json:"id"`

	// Destination identifies the external destination.
	Destination string `db:"destination" json:"destination,omitempty"`

	// MessageKey is the broker routing key used for ordered delivery.
	MessageKey string `db:"message_key" json:"message_key,omitempty"`

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

	// LockVersion is incremented on every claim or reclaim for lease fencing.
	LockVersion int64 `db:"lock_version" json:"lock_version,omitempty"`

	// LastError stores the last delivery error.
	LastError *string `db:"last_error" json:"last_error,omitempty"`

	// CreatedAt is the message creation time.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// PublishedAt is set after successful delivery.
	PublishedAt *time.Time `db:"published_at" json:"published_at,omitempty"`
}
