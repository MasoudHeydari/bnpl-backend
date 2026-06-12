// Package model contains core data structures shared across use cases and
// repository contracts. It belongs to the core domain layer and stays free of
// infrastructure concerns.
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
