package health

// Check identifies a runtime dependency that participates in readiness
// evaluation.
type Check string

// Supported health dependency checks.
const (
	// DatabaseCheck verifies database availability.
	DatabaseCheck Check = "database"

	// CacheCheck verifies cache availability.
	CacheCheck Check = "cache"

	// MessagingCheck verifies messaging backend availability.
	MessagingCheck Check = "messaging"
)
