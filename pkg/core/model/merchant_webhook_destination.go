package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MerchantWebhookDestinationID is the internal BNPL primary key for a
// pre-registered merchant webhook destination.
type MerchantWebhookDestinationID uuid.UUID

// ParseMerchantWebhookDestinationID parses a MerchantWebhookDestinationID from its canonical UUID string form.
func ParseMerchantWebhookDestinationID(value string) (MerchantWebhookDestinationID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantWebhookDestinationID{}, fmt.Errorf("parse merchant webhook destination id: %w", err)
	}

	return MerchantWebhookDestinationID(id), nil
}

// UUID returns the underlying uuid.UUID value for boundary conversions.
func (id MerchantWebhookDestinationID) UUID() uuid.UUID { return uuid.UUID(id) }

// String returns the canonical UUID string representation.
func (id MerchantWebhookDestinationID) String() string { return uuid.UUID(id).String() }

// IsZero reports whether id is the zero (nil) UUID.
func (id MerchantWebhookDestinationID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

// MarshalText implements encoding.TextMarshaler.
func (id MerchantWebhookDestinationID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (id *MerchantWebhookDestinationID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantWebhookDestinationID(u)

	return nil
}

// Scan implements sql.Scanner.
func (id *MerchantWebhookDestinationID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantWebhookDestinationID(u)

	return nil
}

// Value implements driver.Valuer.
func (id MerchantWebhookDestinationID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

const (
	// MinMerchantWebhookSigningSecretLength is the minimum plaintext signing
	// secret size in bytes accepted when a secret is created or rotated.
	MinMerchantWebhookSigningSecretLength = 32

	// MaxMerchantWebhookSigningSecretLength bounds the plaintext signing secret.
	MaxMerchantWebhookSigningSecretLength = 512
)

// MerchantWebhookDestination is the HTTP endpoint registered for merchant
// checkout lifecycle notifications. A merchant has at most one destination.
//
// Checkout requests never supply a webhook URL. BNPL resolves the destination
// from this table so a merchant cannot redirect notifications to an arbitrary
// host — the same open-redirect defense used for browser return destinations.
type MerchantWebhookDestination struct {
	// ID uniquely identifies this webhook destination.
	ID MerchantWebhookDestinationID `db:"id" json:"id"`

	// MerchantID is the owning external merchant identity.
	MerchantID MerchantID `db:"merchant_id" json:"merchant_id"`

	// WebhookURL is the absolute HTTPS/HTTP URL BNPL POSTs lifecycle events to.
	WebhookURL string `db:"webhook_url" json:"webhook_url"`

	// Enabled reports whether this destination may still receive events.
	Enabled bool `db:"enabled" json:"enabled"`

	// SigningSecretCiphertext is the envelope-encrypted shared secret used by the
	// webhook dispatcher to sign request bodies. It must never be echoed to
	// partner APIs.
	SigningSecretCiphertext string `db:"signing_secret_ciphertext" json:"-"`

	// SigningSecretKeyID identifies the key that encrypted SigningSecretCiphertext.
	SigningSecretKeyID string `db:"signing_secret_key_id" json:"-"`

	// CreatedAt is when the destination was registered.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the destination was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
