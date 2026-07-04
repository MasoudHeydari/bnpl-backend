package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MerchantTerminalIssuanceDeliveryID uniquely identifies a
// MerchantTerminalIssuanceDelivery.
type MerchantTerminalIssuanceDeliveryID uuid.UUID

// ParseMerchantTerminalIssuanceDeliveryID parses a
// MerchantTerminalIssuanceDeliveryID from its canonical string form.
func ParseMerchantTerminalIssuanceDeliveryID(value string) (MerchantTerminalIssuanceDeliveryID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantTerminalIssuanceDeliveryID{}, fmt.Errorf("parse merchant terminal issuance delivery id: %w", err)
	}

	return MerchantTerminalIssuanceDeliveryID(id), nil
}

func (id MerchantTerminalIssuanceDeliveryID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantTerminalIssuanceDeliveryID) String() string { return uuid.UUID(id).String() }

func (id MerchantTerminalIssuanceDeliveryID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantTerminalIssuanceDeliveryID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *MerchantTerminalIssuanceDeliveryID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantTerminalIssuanceDeliveryID(u)

	return nil
}

func (id *MerchantTerminalIssuanceDeliveryID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantTerminalIssuanceDeliveryID(u)

	return nil
}

func (id MerchantTerminalIssuanceDeliveryID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// MerchantTerminalIssuanceDelivery is a temporary recoverable copy of an issued
// API key used only to replay the same secret after a lost Provision/Rotate
// response. It is not an authentication source of truth.
//
// After ExpiresAt the ciphertext must never be replayed, even if still present
// physically. A cleanup worker scrubs secret columns and sets PurgedAt, leaving
// an idempotency tombstone so the same Idempotency-Key cannot mint a new key.
type MerchantTerminalIssuanceDelivery struct {
	// ID uniquely identifies this delivery.
	ID MerchantTerminalIssuanceDeliveryID `db:"id" json:"id,omitempty"`

	// MerchantTerminalID references the terminal the issuance belongs to.
	MerchantTerminalID MerchantTerminalID `db:"merchant_terminal_id" json:"merchant_terminal_id,omitempty"`

	// CredentialID references the credential created by the issuance.
	CredentialID MerchantTerminalCredentialID `db:"credential_id" json:"credential_id,omitempty"`

	// IdempotencyKey is the caller-supplied key scoped to the terminal.
	IdempotencyKey string `db:"idempotency_key" json:"idempotency_key,omitempty"`

	// RequestFingerprint is the canonical fingerprint of the issuance request
	// semantics. A reused IdempotencyKey with a different fingerprint is an
	// idempotency conflict.
	RequestFingerprint string `db:"request_fingerprint" json:"request_fingerprint,omitempty"`

	// APIKeyCiphertext is the encrypted API key held only for the replay
	// window. Empty after purge. Excluded from JSON.
	APIKeyCiphertext string `db:"api_key_ciphertext" json:"-"`

	// APIKeyEncryptionKeyID identifies the key that produced APIKeyCiphertext.
	// Empty after purge.
	APIKeyEncryptionKeyID string `db:"api_key_encryption_key_id" json:"api_key_encryption_key_id,omitempty"`

	// ExpiresAt is the deadline after which the ciphertext must not be replayed.
	ExpiresAt time.Time `db:"expires_at" json:"expires_at,omitempty"`

	// ConsumedAt is the first time BNPL extracted the secret to build a
	// response. It does not prove the merchant received the HTTP response and
	// does not block further in-window replays.
	ConsumedAt *time.Time `db:"consumed_at" json:"consumed_at,omitempty"`

	// PurgedAt is when secret-bearing columns were scrubbed. Non-nil marks an
	// idempotency tombstone.
	PurgedAt *time.Time `db:"purged_at" json:"purged_at,omitempty"`

	// CreatedAt is when the delivery was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`
}

// IsPurged reports whether secret material has been scrubbed.
func (d MerchantTerminalIssuanceDelivery) IsPurged() bool {
	return d.PurgedAt != nil
}

// HasReplayableSecret reports whether ciphertext material is still present.
func (d MerchantTerminalIssuanceDelivery) HasReplayableSecret() bool {
	return !d.IsPurged() && d.APIKeyCiphertext != "" && d.APIKeyEncryptionKeyID != ""
}
