package model

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// ShortLinkID uniquely identifies a short link.
type ShortLinkID uuid.UUID

// ParseShortLinkID parses a ShortLinkID from its canonical string form.
func ParseShortLinkID(value string) (ShortLinkID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return ShortLinkID{}, fmt.Errorf("parse short link id: %w", err)
	}

	return ShortLinkID(id), nil
}

func (id ShortLinkID) UUID() uuid.UUID { return uuid.UUID(id) }

func (id ShortLinkID) String() string { return uuid.UUID(id).String() }

func (id ShortLinkID) IsZero() bool { return uuid.UUID(id) == uuid.Nil }

func (id ShortLinkID) MarshalText() ([]byte, error) { return uuid.UUID(id).MarshalText() }

func (id *ShortLinkID) UnmarshalText(data []byte) error {
	var u uuid.UUID
	if err := u.UnmarshalText(data); err != nil {
		return err
	}

	*id = ShortLinkID(u)

	return nil
}

func (id *ShortLinkID) Scan(src any) error {
	var u uuid.UUID
	if err := u.Scan(src); err != nil {
		return err
	}

	*id = ShortLinkID(u)

	return nil
}

func (id ShortLinkID) Value() (driver.Value, error) { return uuid.UUID(id).Value() }

// ShortLink is a reusable opaque redirect mapping from a public code to an
// HTTPS destination. It is intentionally free of BNPL domain concepts.
type ShortLink struct {
	// ID uniquely identifies the short link.
	ID ShortLinkID `db:"id" json:"id"`

	// Code is the public opaque path segment (cryptographically unpredictable
	// for generated codes, or a configured static alias).
	Code string `db:"code" json:"code"`

	// DestinationURL is the HTTPS location browsers are redirected to.
	DestinationURL string `db:"destination_url" json:"destination_url"`

	// ExpiresAt is the optional absolute expiry instant. Nil means no expiry.
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`

	// RevokedAt is set when the link is intentionally disabled.
	RevokedAt *time.Time `db:"revoked_at" json:"revoked_at,omitempty"`

	// CreatedAt is when the row was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the row was last updated.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// IsRevoked reports whether the short link has been revoked.
func (s ShortLink) IsRevoked() bool {
	return s.RevokedAt != nil && !s.RevokedAt.IsZero()
}

// IsExpired reports whether the short link has expired at now.
func (s ShortLink) IsExpired(now time.Time) bool {
	if s.ExpiresAt == nil || s.ExpiresAt.IsZero() {
		return false
	}

	return !now.UTC().Before(s.ExpiresAt.UTC())
}

// IsActive reports whether the short link may be resolved at now.
func (s ShortLink) IsActive(now time.Time) bool {
	return !s.IsRevoked() && !s.IsExpired(now)
}
