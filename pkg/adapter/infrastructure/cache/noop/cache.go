package noop

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cache"
)

// Verify Cache implements the core cache contract at compile time.
var _ cache.Cache = (*Cache)(nil)

// Cache is a no-op cache implementation.
//
// All write operations succeed without storing data. Read operations always
// behave as cache misses.
type Cache struct{}

// New constructs a no-op cache adapter.
func New() *Cache {
	return &Cache{}
}

// Get always returns cache.ErrMiss.
func (c *Cache) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, cache.ErrMiss
}

// Set accepts the value and performs no action.
func (c *Cache) Set(_ context.Context, _ string, _ []byte, _ time.Duration) error {
	return nil
}

// SetNX accepts the value but does not store it.
//
// It returns true to keep disabled-cache flows non-blocking. This does not
// provide idempotency or lock semantics because no state is retained.
func (c *Cache) SetNX(_ context.Context, _ string, _ []byte, _ time.Duration) (bool, error) {
	return true, nil
}

// Delete performs no action.
func (c *Cache) Delete(_ context.Context, _ ...string) error {
	return nil
}

// Exists always reports that the key does not exist.
func (c *Cache) Exists(_ context.Context, _ string) (bool, error) {
	return false, nil
}

// TTL always returns cache.ErrMiss.
func (c *Cache) TTL(_ context.Context, _ string) (time.Duration, error) {
	return 0, cache.ErrMiss
}

// Increment returns delta as if a new key were created.
//
// Consecutive calls do not accumulate values because no state is retained.
func (c *Cache) Increment(_ context.Context, _ string, delta int64, _ time.Duration) (int64, error) {
	return delta, nil
}

// Ping always succeeds because the noop implementation has no external
// dependency.
func (c *Cache) Ping(context.Context) error {
	return nil
}

// Close releases no resources and always succeeds.
func (c *Cache) Close() error {
	return nil
}
