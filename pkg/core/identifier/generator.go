package identifier

// Generator generates opaque unique identifiers for domain and integration use.
//
// Implementations may use ULID, UUID, or another strategy. Callers must treat
// generated values as opaque strings and must not rely on their format.
type Generator interface {
	// Generate returns a new opaque unique identifier.
	Generate() string
}
