package model

import "time"

// Timestamps stores common creation and update timestamps.
type Timestamps struct {
	// CreatedAt is when the record was created.
	CreatedAt time.Time `db:"created_at" json:"created_at"`

	// UpdatedAt is when the record was last modified.
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}
