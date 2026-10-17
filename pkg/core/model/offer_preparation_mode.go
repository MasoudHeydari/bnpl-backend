package model

import (
	"database/sql/driver"
	"fmt"
)

// OfferPreparationMode controls when Partner Create prepares the financing offer.
//
// on_create prepares during Partner Create; on_start defers preparation to the
// customer's POST .../start call.
type OfferPreparationMode string

const (
	// OfferPreparationOnCreate prepares the offer during Partner Create.
	OfferPreparationOnCreate OfferPreparationMode = "on_create"

	// OfferPreparationOnStart defers offer preparation until customer START.
	OfferPreparationOnStart OfferPreparationMode = "on_start"
)

// DefaultOfferPreparationMode is the trusted merchant default when unset.
const DefaultOfferPreparationMode = OfferPreparationOnStart

// ParseOfferPreparationMode parses mode from persisted or config text.
func ParseOfferPreparationMode(value string) (OfferPreparationMode, error) {
	mode := OfferPreparationMode(value)
	if !mode.IsValid() {
		return "", fmt.Errorf("parse offer preparation mode: unsupported value %q", value)
	}

	return mode, nil
}

// IsValid reports whether mode is a supported preparation mode.
func (m OfferPreparationMode) IsValid() bool {
	switch m {
	case OfferPreparationOnCreate, OfferPreparationOnStart:
		return true

	default:
		return false
	}
}

// String returns the canonical persisted representation.
func (m OfferPreparationMode) String() string { return string(m) }

// Scan implements sql.Scanner.
func (m *OfferPreparationMode) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*m = DefaultOfferPreparationMode

		return nil

	case string:
		parsed, err := ParseOfferPreparationMode(v)
		if err != nil {
			return err
		}

		*m = parsed

		return nil

	case []byte:
		parsed, err := ParseOfferPreparationMode(string(v))
		if err != nil {
			return err
		}

		*m = parsed

		return nil

	default:
		return fmt.Errorf("scan offer preparation mode: unsupported type %T", src)
	}
}

// Value implements driver.Valuer.
func (m OfferPreparationMode) Value() (driver.Value, error) {
	if !m.IsValid() {
		return nil, fmt.Errorf("value offer preparation mode: unsupported value %q", m)
	}

	return string(m), nil
}
