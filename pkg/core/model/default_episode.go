package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// DefaultEpisodeID uniquely identifies one continuous customer default period.
type DefaultEpisodeID uuid.UUID

// ParseDefaultEpisodeID parses a DefaultEpisodeID from its canonical string form.
func ParseDefaultEpisodeID(value string) (DefaultEpisodeID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return DefaultEpisodeID{}, fmt.Errorf("parse default episode id: %w", err)
	}

	return DefaultEpisodeID(id), nil
}

func (id DefaultEpisodeID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id DefaultEpisodeID) String() string { return uuid.UUID(id).String() }

func (id DefaultEpisodeID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id DefaultEpisodeID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *DefaultEpisodeID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = DefaultEpisodeID(u)

	return nil
}

func (id *DefaultEpisodeID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = DefaultEpisodeID(u)

	return nil
}

func (id DefaultEpisodeID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// DefaultEpisodeStatus is the lifecycle status of a default episode.
type DefaultEpisodeStatus string

// Known default episode statuses.
const (
	ActiveDefaultEpisodeStatus    DefaultEpisodeStatus = "active"
	RecoveredDefaultEpisodeStatus DefaultEpisodeStatus = "recovered"
)

// IsValid reports whether the status is a known enumeration value.
func (s DefaultEpisodeStatus) IsValid() bool {
	switch s {
	case ActiveDefaultEpisodeStatus, RecoveredDefaultEpisodeStatus:
		return true

	default:
		return false
	}
}

// DefaultEpisode is one continuous customer default period.
//
// Historical episodes are retained permanently. Recovery closes an episode;
// a later default creates a new episode identity.
type DefaultEpisode struct {
	// ID uniquely identifies the episode.
	ID DefaultEpisodeID `db:"id" json:"id"`

	// CustomerID is the BNPL-local customer_profiles.id.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// Status is the current episode lifecycle status.
	Status DefaultEpisodeStatus `db:"status" json:"status"`

	// OpeningDecisionID is the immutable restriction Decision that opened the episode.
	OpeningDecisionID DecisionID `db:"opening_decision_id" json:"opening_decision_id"`

	// ClosingDecisionID is the recovery Decision when the episode is recovered.
	ClosingDecisionID *DecisionID `db:"closing_decision_id" json:"closing_decision_id,omitempty"`

	// StartedAt is when the episode became active (detection/persistence time).
	StartedAt time.Time `db:"started_at" json:"started_at"`

	// RecoveredAt is when the episode was recovered, if applicable.
	RecoveredAt *time.Time `db:"recovered_at" json:"recovered_at,omitempty"`

	// CreatedAt is when the row was persisted.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the row was last updated.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// DefaultEpisodeTrigger preserves the installment evidence that justified opening
// a default episode.
type DefaultEpisodeTrigger struct {
	// EpisodeID identifies the owning episode.
	EpisodeID DefaultEpisodeID `db:"episode_id" json:"episode_id"`

	// InstallmentID identifies the triggering installment.
	InstallmentID InstallmentID `db:"installment_id" json:"installment_id"`

	// DueAt is the contractual due instant snapshot.
	DueAt time.Time `db:"due_at" json:"due_at"`

	// GracePeriodDays is the contractual grace snapshot.
	GracePeriodDays uint `db:"grace_period_days" json:"grace_period_days"`

	// OverdueEligibleAt is the first default-eligible instant for the trigger.
	OverdueEligibleAt time.Time `db:"overdue_eligible_at" json:"overdue_eligible_at"`
}
