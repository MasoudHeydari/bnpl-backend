package model

import "time"

// ValidityWindow defines when a record becomes effective and when it stops
// being valid.
type ValidityWindow struct {
	// EffectiveFrom indicates when the record becomes valid.
	EffectiveFrom time.Time `db:"effective_from" json:"effective_from"`

	// ExpiresAt indicates when the record stops being valid, when applicable.
	ExpiresAt *time.Time `db:"expires_at" json:"expires_at,omitempty"`
}
