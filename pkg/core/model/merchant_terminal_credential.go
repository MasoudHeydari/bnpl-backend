package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MerchantTerminalCredentialID uniquely identifies a MerchantTerminalCredential.
type MerchantTerminalCredentialID uuid.UUID

// ParseMerchantTerminalCredentialID parses a MerchantTerminalCredentialID from
// its canonical string form.
func ParseMerchantTerminalCredentialID(value string) (MerchantTerminalCredentialID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantTerminalCredentialID{}, fmt.Errorf("parse merchant terminal credential id: %w", err)
	}

	return MerchantTerminalCredentialID(id), nil
}

func (id MerchantTerminalCredentialID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantTerminalCredentialID) String() string { return uuid.UUID(id).String() }

func (id MerchantTerminalCredentialID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantTerminalCredentialID) MarshalText() ([]byte, error) {
	return uuid.UUID(id).MarshalText()
}

func (id *MerchantTerminalCredentialID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantTerminalCredentialID(u)

	return nil
}

func (id *MerchantTerminalCredentialID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantTerminalCredentialID(u)

	return nil
}

func (id MerchantTerminalCredentialID) Value() (driver.Value, error) {
	return uuid.UUID(id).Value()
}

// MerchantTerminalCredentialStatus identifies the lifecycle state of a
// terminal API key credential.
type MerchantTerminalCredentialStatus string

// Known merchant terminal credential lifecycle statuses.
const (
	// ActiveMerchantTerminalCredentialStatus indicates the credential is the
	// current key and is accepted.
	ActiveMerchantTerminalCredentialStatus MerchantTerminalCredentialStatus = "active"

	// RetiringMerchantTerminalCredentialStatus indicates the credential has
	// been superseded but is still accepted until RetireAt.
	RetiringMerchantTerminalCredentialStatus MerchantTerminalCredentialStatus = "retiring"

	// RetiredMerchantTerminalCredentialStatus indicates the credential is no
	// longer accepted after its grace period elapsed.
	RetiredMerchantTerminalCredentialStatus MerchantTerminalCredentialStatus = "retired"

	// RevokedMerchantTerminalCredentialStatus indicates the credential was
	// administratively invalidated and is never accepted.
	RevokedMerchantTerminalCredentialStatus MerchantTerminalCredentialStatus = "revoked"
)

// IsValid reports whether the credential status is known by the domain.
func (s MerchantTerminalCredentialStatus) IsValid() bool {
	switch s {
	case ActiveMerchantTerminalCredentialStatus,
		RetiringMerchantTerminalCredentialStatus,
		RetiredMerchantTerminalCredentialStatus,
		RevokedMerchantTerminalCredentialStatus:
		return true

	default:
		return false
	}
}

// MerchantTerminalCredential is a versioned API key credential for a terminal.
//
// Only the keyed hash of the API key is stored (show-once issuance): plaintext
// is returned solely from Provision/Rotate responses and cannot be loaded again.
type MerchantTerminalCredential struct {
	// ID uniquely identifies this credential.
	ID MerchantTerminalCredentialID `db:"id" json:"id,omitempty"`

	// MerchantTerminalID references the terminal this credential belongs to.
	MerchantTerminalID MerchantTerminalID `db:"merchant_terminal_id" json:"merchant_terminal_id,omitempty"`

	// Version is the monotonic credential version within the terminal.
	Version int64 `db:"version" json:"version,omitempty"`

	// APIKeyHash is the hex HMAC-SHA256 of the API key. It is never plaintext
	// and is excluded from JSON output.
	APIKeyHash string `db:"api_key_hash" json:"-"`

	// Status is the current lifecycle state of the credential.
	Status MerchantTerminalCredentialStatus `db:"status" json:"status,omitempty"`

	// CreatedAt is when the credential was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// ActivatedAt is when the credential became usable.
	ActivatedAt time.Time `db:"activated_at" json:"activated_at,omitempty"`

	// RetireAt is the deadline until which a retiring credential is accepted.
	RetireAt *time.Time `db:"retire_at" json:"retire_at,omitempty"`

	// RetiredAt is when the credential was fully retired.
	RetiredAt *time.Time `db:"retired_at" json:"retired_at,omitempty"`

	// UpdatedAt is when the credential was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`
}

// IsAcceptableAt reports whether the credential may authenticate a request at
// now: it is active, or retiring and now is before RetireAt.
func (c MerchantTerminalCredential) IsAcceptableAt(now time.Time) bool {
	switch c.Status {
	case ActiveMerchantTerminalCredentialStatus:
		return true

	case RetiringMerchantTerminalCredentialStatus:
		return c.RetireAt != nil && now.Before(*c.RetireAt)

	default:
		return false
	}
}
