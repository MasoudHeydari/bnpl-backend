package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MerchantReturnDestinationID is the internal BNPL primary key for a
// pre-registered merchant return destination.
type MerchantReturnDestinationID uuid.UUID

// ParseMerchantReturnDestinationID parses a MerchantReturnDestinationID from its canonical UUID string form.
func ParseMerchantReturnDestinationID(value string) (MerchantReturnDestinationID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantReturnDestinationID{}, fmt.Errorf("parse merchant return destination id: %w", err)
	}

	return MerchantReturnDestinationID(id), nil
}

// UUID returns the underlying uuid.UUID value for boundary conversions.
func (id MerchantReturnDestinationID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical UUID string representation.
func (id MerchantReturnDestinationID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether id is the zero (nil) UUID.
func (id MerchantReturnDestinationID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// MarshalText implements encoding.TextMarshaler.
func (id MerchantReturnDestinationID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *MerchantReturnDestinationID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantReturnDestinationID(u)

	return nil
}

// Scan implements sql.Scanner.
func (id *MerchantReturnDestinationID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantReturnDestinationID(u)

	return nil
}

// Value implements driver.Valuer.
func (id MerchantReturnDestinationID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// MerchantReturnDestination is a return URL registered for a merchant during
// onboarding or configuration.
//
// Checkout requests select a destination by symbolic name only. BNPL never
// accepts an arbitrary return URL from a checkout request, which is what keeps
// the hosted handoff free of open-redirect exposure.
type MerchantReturnDestination struct {
	// ID uniquely identifies this return destination.
	ID MerchantReturnDestinationID `db:"id" json:"id"`

	// MerchantID is the owning external merchant identity
	// (merchant_profiles.merchant_id).
	MerchantID MerchantID `db:"merchant_id" json:"merchant_id"`

	// DestinationName is the symbolic name merchants reference per checkout.
	DestinationName string `db:"destination_name" json:"destination_name"`

	// ReturnURL is the absolute URL the browser is sent to when checkout finishes.
	ReturnURL string `db:"return_url" json:"return_url"`

	// Enabled reports whether this destination may still be selected.
	Enabled bool `db:"enabled" json:"enabled"`

	// CreatedAt is when the destination was registered.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the destination was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
