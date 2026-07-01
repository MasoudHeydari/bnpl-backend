package cache

// Provider identifies a cache infrastructure implementation selected at startup.
type Provider string

const (
	// NoopProvider disables cache storage.
	//
	// Reads always miss and writes are ignored. Use for local development,
	// tests, or environments without Redis.
	NoopProvider Provider = "noop"

	// RedisProvider uses a Redis server for shared cache state.
	//
	// Startup performs a connectivity check before the application accepts
	// traffic.
	RedisProvider Provider = "redis"
)
