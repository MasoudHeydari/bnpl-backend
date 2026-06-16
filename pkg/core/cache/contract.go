package cache

import (
	"context"
	"errors"
	"time"
)

// ErrMiss is returned when a cache key does not exist.
var ErrMiss = errors.New("cache miss")

// Cache defines the application's cache abstraction.
//
// Values are opaque byte slices. Serialization belongs to callers or higher
// level helpers, not to the cache implementation.
//
// Implementations must treat context cancellation and deadlines as authoritative.
type Cache interface {
	// Get returns the value associated with key.
	//
	// It returns ErrMiss when the key does not exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores value under key with ttl.
	//
	// A zero ttl means the key does not expire.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// SetNX stores value only when key does not already exist.
	//
	// It returns true when the value was stored. A zero ttl means the key does
	// not expire.
	SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)

	// Delete removes one or more keys.
	//
	// Deleting a missing key is not an error.
	Delete(ctx context.Context, keys ...string) error

	// Exists reports whether key exists.
	Exists(ctx context.Context, key string) (bool, error)

	// TTL returns the remaining lifetime of key.
	//
	// It returns ErrMiss when the key does not exist. A negative duration may be
	// returned by implementations to indicate that the key exists but has no
	// expiration.
	TTL(ctx context.Context, key string) (time.Duration, error)

	// Increment atomically increases a numeric value by delta.
	//
	// If the key is created and ttl is greater than zero, the ttl must be
	// applied to the new key.
	Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)

	// Ping verifies that the cache backend is reachable and able to serve
	// lightweight requests.
	Ping(ctx context.Context) error

	// Close releases underlying resources.
	//
	// Implementations without external resources may return nil.
	Close() error
}
