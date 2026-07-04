package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CollectionsCaseID uniquely identifies a collections legal/case workflow record.
type CollectionsCaseID uuid.UUID

// ParseCollectionsCaseID parses a CollectionsCaseID from its canonical string form.
func ParseCollectionsCaseID(value string) (CollectionsCaseID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return CollectionsCaseID{}, fmt.Errorf("parse collections case id: %w", err)
	}

	return CollectionsCaseID(id), nil
}

func (id CollectionsCaseID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id CollectionsCaseID) String() string { return uuid.UUID(id).String() }

func (id CollectionsCaseID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id CollectionsCaseID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *CollectionsCaseID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = CollectionsCaseID(u)

	return nil
}

func (id *CollectionsCaseID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CollectionsCaseID(u)

	return nil
}

func (id CollectionsCaseID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// CollectionsCaseStatus is the human/legal workflow status for a case.
type CollectionsCaseStatus string

// Known collections case statuses.
const (
	PendingHumanReviewCollectionsCaseStatus CollectionsCaseStatus = "pending_human_review"
	UnderLegalReviewCollectionsCaseStatus   CollectionsCaseStatus = "under_legal_review"
	LegalActionStartedCollectionsCaseStatus CollectionsCaseStatus = "legal_action_started"
	ResolvedCollectionsCaseStatus           CollectionsCaseStatus = "resolved"
)

// IsValid reports whether the case status is known.
func (s CollectionsCaseStatus) IsValid() bool {
	switch s {
	case PendingHumanReviewCollectionsCaseStatus,
		UnderLegalReviewCollectionsCaseStatus,
		LegalActionStartedCollectionsCaseStatus,
		ResolvedCollectionsCaseStatus:
		return true

	default:
		return false
	}
}

// CollectionsCase is the durable legal/collections workflow record for one
// default episode. Case status is independent of credit restriction recovery.
type CollectionsCase struct {
	// ID uniquely identifies the case.
	ID CollectionsCaseID `db:"id" json:"id"`

	// CustomerID is the BNPL-local customer_profiles.id.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// DefaultEpisodeID links the case to exactly one default episode.
	DefaultEpisodeID DefaultEpisodeID `db:"default_episode_id" json:"default_episode_id"`

	// OpeningDecisionID is the Decision that authorized case opening.
	OpeningDecisionID DecisionID `db:"opening_decision_id" json:"opening_decision_id"`

	// Status is the human/legal workflow status.
	Status CollectionsCaseStatus `db:"status" json:"status"`

	CreatedAt  time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt  time.Time  `db:"updated_at" json:"updated_at"`
	ResolvedAt *time.Time `db:"resolved_at" json:"resolved_at,omitempty"`
}

// CollectionsCaseEventID uniquely identifies an append-only case event.
type CollectionsCaseEventID uuid.UUID

func (id CollectionsCaseEventID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id CollectionsCaseEventID) String() string { return uuid.UUID(id).String() }

func (id CollectionsCaseEventID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id *CollectionsCaseEventID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CollectionsCaseEventID(u)

	return nil
}

func (id CollectionsCaseEventID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// CollectionsCaseEventType classifies append-only case history events.
type CollectionsCaseEventType string

// Known collections case event types.
const (
	CaseOpenedCollectionsCaseEventType              CollectionsCaseEventType = "case_opened"
	CustomerRecoveredCollectionsCaseEventType       CollectionsCaseEventType = "customer_recovered"
	StatusChangedCollectionsCaseEventType           CollectionsCaseEventType = "status_changed"
	PaymentUncertaintyNotedCollectionsCaseEventType CollectionsCaseEventType = "payment_uncertainty_noted"
)

// CollectionsCaseEvent is an append-only historical event for a collections case.
type CollectionsCaseEvent struct {
	ID         CollectionsCaseEventID   `db:"id" json:"id"`
	CaseID     CollectionsCaseID        `db:"case_id" json:"case_id"`
	EventType  CollectionsCaseEventType `db:"event_type" json:"event_type"`
	DecisionID *DecisionID              `db:"decision_id" json:"decision_id,omitempty"`
	Payload    []byte                   `db:"payload" json:"payload,omitempty"`
	OccurredAt time.Time                `db:"occurred_at" json:"occurred_at"`
	ActorType  DecisionActorType        `db:"actor_type" json:"actor_type"`
	ActorID    *uuid.UUID               `db:"actor_id" json:"actor_id,omitempty"`
	CreatedAt  time.Time                `db:"created_at" json:"created_at"`
}
