package model

import "time"

// Migration represents a migration record stored in the database.
// ID is expected to be database-assigned, Name should uniquely identify the
// applied migration file, and CreatedAt records when the migration was
// registered.
type Migration struct {
	// ID is the database-assigned identifier of the migration record.
	ID uint64 `db:"id" json:"id,omitempty"`

	// Name uniquely identifies the applied migration file.
	Name string `db:"name" json:"name,omitempty"`

	// CreatedAt records when the migration was registered.
	CreatedAt time.Time `db:"created_at" json:"created_at"`
}
