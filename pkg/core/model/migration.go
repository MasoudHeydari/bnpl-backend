package model

import "time"

// Migration represents a migration record stored in the database.
// ID is expected to be database-assigned, Name should uniquely identify the
// applied migration file, and CreatedAt records when the migration was
// registered.
type Migration struct {
	ID        uint64
	Name      string
	CreatedAt time.Time
}
