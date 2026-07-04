package model

import (
	"bytes"
	"crypto/rand"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"strconv"
	"time"

	"github.com/google/uuid"
)

// MerchantTerminalID is the internal BNPL primary key for a merchant terminal.
type MerchantTerminalID uuid.UUID

// ParseMerchantTerminalID parses a MerchantTerminalID from its canonical string form.
func ParseMerchantTerminalID(value string) (MerchantTerminalID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return MerchantTerminalID{}, fmt.Errorf("parse merchant terminal id: %w", err)
	}

	return MerchantTerminalID(id), nil
}

func (id MerchantTerminalID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id MerchantTerminalID) String() string { return uuid.UUID(id).String() }

func (id MerchantTerminalID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id MerchantTerminalID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *MerchantTerminalID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = MerchantTerminalID(u)

	return nil
}

func (id *MerchantTerminalID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = MerchantTerminalID(u)

	return nil
}

func (id MerchantTerminalID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// TerminalID is the public numeric terminal identifier presented by an
// external partner together with its API key. It is always positive.
//
// It serializes to JSON as a decimal string so JavaScript clients never lose
// precision on values above 2^53.
type TerminalID int64

// ParseTerminalID parses a TerminalID from its canonical decimal string form.
//
// Only a positive decimal representation without leading zeros, sign prefix,
// whitespace, or other decoration is accepted. The canonical String() of the
// parsed value must equal the input exactly.
func ParseTerminalID(value string) (TerminalID, error) {
	const op = "model.ParseTerminalID"

	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: parse terminal id: %w", op, err)
	}

	if n <= 0 {
		return 0, fmt.Errorf("%s: terminal id must be positive", op)
	}

	id := TerminalID(n)
	if id.String() != value {
		return 0, fmt.Errorf("%s: terminal id must be canonical decimal", op)
	}

	return id, nil
}

// GenerateTerminalID returns a uniformly random TerminalID in 1..MaxInt64
// read from r. Pass crypto/rand.Reader in production.
func GenerateTerminalID(r io.Reader) (TerminalID, error) {
	const op = "model.GenerateTerminalID"

	// rand.Int yields [0, MaxInt64-1]; shifting by one yields [1, MaxInt64].
	n, err := rand.Int(r, big.NewInt(math.MaxInt64))
	if err != nil {
		return 0, fmt.Errorf("%s: read random: %w", op, err)
	}

	return TerminalID(n.Int64() + 1), nil
}

// Int64 returns the numeric value of the terminal id.
func (id TerminalID) Int64() int64 { return int64(id) }

// String returns the decimal form of the terminal id.
func (id TerminalID) String() string { return strconv.FormatInt(int64(id), 10) }

// IsZero reports whether the terminal id is unset.
func (id TerminalID) IsZero() bool { return id == 0 }

// MarshalJSON encodes the terminal id as a JSON string.
func (id TerminalID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON decodes a terminal id from a JSON string.
func (id *TerminalID) UnmarshalJSON(data []byte) error {
	const op = "model.TerminalID.UnmarshalJSON"

	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		return nil
	}

	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("%s: terminal id must be a JSON string: %w", op, err)
	}

	parsed, err := ParseTerminalID(s)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	*id = parsed

	return nil
}

// Scan implements sql.Scanner for a postgres bigint.
func (id *TerminalID) Scan(src any) error {
	const op = "model.TerminalID.Scan"

	switch v := src.(type) {
	case int64:
		*id = TerminalID(v)

		return nil

	case nil:
		return fmt.Errorf("%s: terminal id is null", op)

	default:
		return fmt.Errorf("%s: unsupported source type %T", op, src)
	}
}

// Value implements driver.Valuer for a postgres bigint.
func (id TerminalID) Value() (driver.Value, error) {
	if id < 0 {
		return nil, errors.New("model.TerminalID.Value: terminal id must not be negative")
	}

	return int64(id), nil
}

// MerchantTerminalStatus identifies the lifecycle state of a merchant terminal.
type MerchantTerminalStatus string

// Known merchant terminal lifecycle statuses.
const (
	// ActiveMerchantTerminalStatus indicates the terminal may authenticate.
	ActiveMerchantTerminalStatus MerchantTerminalStatus = "active"

	// SuspendedMerchantTerminalStatus indicates the terminal is temporarily
	// blocked from authenticating.
	SuspendedMerchantTerminalStatus MerchantTerminalStatus = "suspended"

	// RevokedMerchantTerminalStatus indicates the terminal is permanently
	// blocked.
	RevokedMerchantTerminalStatus MerchantTerminalStatus = "revoked"
)

// IsValid reports whether the terminal status is known by the domain.
func (s MerchantTerminalStatus) IsValid() bool {
	switch s {
	case ActiveMerchantTerminalStatus,
		SuspendedMerchantTerminalStatus,
		RevokedMerchantTerminalStatus:
		return true

	default:
		return false
	}
}

// IsActive reports whether the terminal may authenticate.
func (s MerchantTerminalStatus) IsActive() bool {
	return s == ActiveMerchantTerminalStatus
}

// MerchantTerminal binds an external partner terminal identity to a merchant.
type MerchantTerminal struct {
	// ID uniquely identifies this terminal record.
	ID MerchantTerminalID `db:"id" json:"id,omitempty"`

	// MerchantID is the merchant that owns the terminal.
	MerchantID MerchantID `db:"merchant_id" json:"merchant_id,omitempty"`

	// TerminalID is the public numeric identifier presented by the partner.
	TerminalID TerminalID `db:"terminal_id" json:"terminal_id,omitempty"`

	// Status is the current lifecycle state of the terminal.
	Status MerchantTerminalStatus `db:"status" json:"status,omitempty"`

	// CreatedAt is when the terminal was created.
	CreatedAt time.Time `db:"created_at" json:"created_at,omitempty"`

	// UpdatedAt is when the terminal was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at,omitempty"`
}
