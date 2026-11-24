package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CreditOwnerID is the stable BNPL identifier of a legal person.
// Multiple CustomerProfiles may share this identity; no merchant may supply it.
type CreditOwnerID uuid.UUID

// ParseCreditOwnerID parses the stable internal credit-owner UUID.
func ParseCreditOwnerID(raw string) (CreditOwnerID, error) {
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return CreditOwnerID{}, fmt.Errorf("parse credit owner id: %w", err)
	}
	return CreditOwnerID(parsed), nil
}

// UUID exposes the underlying UUID for persistence boundaries.
func (id CreditOwnerID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical credit owner identifier.
func (id CreditOwnerID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether this credit owner identifier is unset.
func (id CreditOwnerID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// MarshalText encodes the credit owner UUID as canonical text.
func (id CreditOwnerID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

// UnmarshalText decodes a canonical UUID into this identifier.
func (id *CreditOwnerID) UnmarshalText(raw []byte) error {
	var v uuid.UUID
	if err := v.UnmarshalText(raw); err != nil {
		return err
	}
	*id = CreditOwnerID(v)
	return nil
}

// Scan reads this identifier from a database value.
func (id *CreditOwnerID) Scan(src any) error {
	var v uuid.UUID
	if err := v.Scan(src); err != nil {
		return err
	}
	*id = CreditOwnerID(v)
	return nil
}

// Value provides a database driver representation of this identifier.
func (id CreditOwnerID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// CreditOwner represents an Iranian legal person and is never the identity
// used for account-level permissions, installment reads, or repayments.
type CreditOwner struct {
	ID           CreditOwnerID `db:"id" json:"id"`
	NationalCode string        `db:"national_code" json:"-"`
	CreatedAt    time.Time     `db:"created_at" json:"created_at"`
}
