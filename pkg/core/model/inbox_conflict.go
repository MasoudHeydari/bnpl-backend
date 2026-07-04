package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InboxConflictID uniquely identifies an InboxPayloadConflict row.
type InboxConflictID uuid.UUID

// ParseInboxConflictID parses an InboxConflictID from its canonical string form.
func ParseInboxConflictID(value string) (InboxConflictID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return InboxConflictID{}, fmt.Errorf("parse inbox conflict id: %w", err)
	}

	return InboxConflictID(id), nil
}

func (id InboxConflictID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id InboxConflictID) String() string { return uuid.UUID(id).String() }

func (id InboxConflictID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id InboxConflictID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *InboxConflictID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = InboxConflictID(u)

	return nil
}

func (id *InboxConflictID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = InboxConflictID(u)

	return nil
}

func (id InboxConflictID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// InboxPayloadConflict records a durable payload identity mismatch for a reused inbox event id.
type InboxPayloadConflict struct {
	// ID uniquely identifies this payload conflict record.
	ID InboxConflictID `db:"id" json:"id,omitempty"`

	// InboxID references the inbox row that already claimed the event id.
	InboxID InboxID `db:"inbox_id" json:"inbox_id,omitempty"`

	// SourceService identifies the upstream service that emitted the event.
	SourceService string `db:"source_service" json:"source_service,omitempty"`

	// EventID is the upstream event identity that conflicted on payload fingerprint.
	EventID string `db:"event_id" json:"event_id,omitempty"`

	// ExistingPayloadFingerprint is the fingerprint already stored for the event id.
	ExistingPayloadFingerprint string `db:"existing_payload_fingerprint" json:"existing_payload_fingerprint,omitempty"`

	// IncomingPayloadFingerprint is the fingerprint of the conflicting incoming payload.
	IncomingPayloadFingerprint string `db:"incoming_payload_fingerprint" json:"incoming_payload_fingerprint,omitempty"`

	// IncomingPayload is the conflicting payload body that failed identity checks.
	IncomingPayload json.RawMessage `db:"incoming_payload" json:"incoming_payload,omitempty"`

	// FailureCode classifies why the payload identity check failed.
	FailureCode string `db:"failure_code" json:"failure_code,omitempty"`

	// DetectedAt is when the conflict was detected.
	DetectedAt time.Time `db:"detected_at" json:"detected_at,omitempty"`

	// CreatedAt is when the conflict row was persisted.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`
}

// InboxPayloadConflictEvent carries fields required to persist a payload identity conflict.
type InboxPayloadConflictEvent struct {
	// InboxID references the inbox row that already claimed the event id.
	InboxID InboxID

	// SourceService identifies the upstream service that emitted the event.
	SourceService string

	// EventID is the upstream event identity that conflicted on payload fingerprint.
	EventID string

	// ExistingPayloadFingerprint is the fingerprint already stored for the event id.
	ExistingPayloadFingerprint string

	// IncomingPayloadFingerprint is the fingerprint of the conflicting incoming payload.
	IncomingPayloadFingerprint string

	// IncomingPayload is the conflicting payload body that failed identity checks.
	IncomingPayload json.RawMessage

	// FailureCode classifies why the payload identity check failed.
	FailureCode string
}
