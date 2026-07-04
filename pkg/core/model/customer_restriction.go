package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CustomerRestrictionID uniquely identifies a customer restriction record.
type CustomerRestrictionID uuid.UUID

// ParseCustomerRestrictionID parses a CustomerRestrictionID from its canonical string form.
func ParseCustomerRestrictionID(value string) (CustomerRestrictionID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return CustomerRestrictionID{}, fmt.Errorf("parse customer restriction id: %w", err)
	}

	return CustomerRestrictionID(id), nil
}

func (id CustomerRestrictionID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id CustomerRestrictionID) String() string { return uuid.UUID(id).String() }

func (id CustomerRestrictionID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id CustomerRestrictionID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *CustomerRestrictionID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = CustomerRestrictionID(u)

	return nil
}

func (id *CustomerRestrictionID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = CustomerRestrictionID(u)

	return nil
}

func (id CustomerRestrictionID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// CustomerRestrictionType classifies what business capability is restricted.
type CustomerRestrictionType string

// Known customer restriction types.
const (
	NewCreditUsageCustomerRestrictionType CustomerRestrictionType = "new_credit_usage"
)

// IsValid reports whether the restriction type is known.
func (t CustomerRestrictionType) IsValid() bool {
	switch t {
	case NewCreditUsageCustomerRestrictionType:
		return true

	default:
		return false
	}
}

// CustomerRestrictionReason classifies why a restriction was applied.
type CustomerRestrictionReason string

// Known customer restriction reasons.
const (
	CollectionsDefaultCustomerRestrictionReason CustomerRestrictionReason = "collections_default"
	AdministrativeHoldCustomerRestrictionReason CustomerRestrictionReason = "administrative_hold"
)

// IsValid reports whether the restriction reason is known.
func (r CustomerRestrictionReason) IsValid() bool {
	switch r {
	case CollectionsDefaultCustomerRestrictionReason, AdministrativeHoldCustomerRestrictionReason:
		return true

	default:
		return false
	}
}

// CustomerRestrictionStatus is the lifecycle status of a restriction row.
type CustomerRestrictionStatus string

// Known customer restriction statuses.
const (
	ActiveCustomerRestrictionStatus   CustomerRestrictionStatus = "active"
	ReleasedCustomerRestrictionStatus CustomerRestrictionStatus = "released"
)

// IsValid reports whether the restriction status is known.
func (s CustomerRestrictionStatus) IsValid() bool {
	switch s {
	case ActiveCustomerRestrictionStatus, ReleasedCustomerRestrictionStatus:
		return true

	default:
		return false
	}
}

// CustomerRestriction is a durable, non-deleting restriction on customer capability.
//
// Released restrictions remain in the database for auditability. A later
// independent restriction creates a new row.
type CustomerRestriction struct {
	// ID uniquely identifies the restriction.
	ID CustomerRestrictionID `db:"id" json:"id"`

	// CustomerID is the BNPL-local customer_profiles.id.
	CustomerID CustomerID `db:"customer_id" json:"customer_id"`

	// RestrictionType identifies the restricted capability.
	RestrictionType CustomerRestrictionType `db:"restriction_type" json:"restriction_type"`

	// Reason identifies why the restriction was applied.
	Reason CustomerRestrictionReason `db:"reason" json:"reason"`

	// Status is the current restriction lifecycle status.
	Status CustomerRestrictionStatus `db:"status" json:"status"`

	// DefaultEpisodeID links collections-default restrictions to their episode.
	DefaultEpisodeID *DefaultEpisodeID `db:"default_episode_id" json:"default_episode_id,omitempty"`

	// OpeningDecisionID is the Decision that applied the restriction.
	OpeningDecisionID DecisionID `db:"opening_decision_id" json:"opening_decision_id"`

	// ReleasingDecisionID is the Decision that released the restriction.
	ReleasingDecisionID *DecisionID `db:"releasing_decision_id" json:"releasing_decision_id,omitempty"`

	// AppliedAt is when the restriction became effective.
	AppliedAt time.Time `db:"applied_at" json:"applied_at"`

	// ReleasedAt is when the restriction was released, if applicable.
	ReleasedAt *time.Time `db:"released_at" json:"released_at,omitempty"`

	// CreatedAt is when the row was persisted.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the row was last updated.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
