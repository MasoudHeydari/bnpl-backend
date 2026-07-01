package memory

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cache"
)

// Verify Cache implements the core cache contract at compile time.
var _ cache.Cache = (*Cache)(nil)

type item struct {
	value     []byte
	expiresAt time.Time
}

// Cache stores cache entries in process memory.
//
// Cache is safe for concurrent use by multiple goroutines. Values are copied on
// read and write so callers cannot mutate internal state through shared byte
// slices.
//
// Closing the cache releases all stored entries. Operations after Close behave
// as no-ops or cache misses.
type Cache struct {
	mu     sync.RWMutex
	items  map[string]item
	closed bool
}

// New constructs an empty in-memory cache.
func New() *Cache {
	return &Cache{
		items: make(map[string]item),
	}
}

// Get returns the value associated with key.
//
// It returns cache.ErrMiss when key does not exist, has expired, or the cache
// has been closed.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return nil, cache.ErrMiss
	}
	it, ok := c.items[key]
	c.mu.RUnlock()

	if !ok || it.expired() {
		if ok {
			_ = c.Delete(ctx, key)
		}
		return nil, cache.ErrMiss
	}

	return clone(it.value), nil
}

// Set stores value under key with ttl.
//
// A zero or negative ttl means the key does not expire. The value is copied
// before storage.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.items[key] = item{
		value:     clone(value),
		expiresAt: expiresAt(ttl),
	}

	return nil
}

// SetNX stores value only when key does not already exist.
//
// It returns true when the value was stored. Expired keys are treated as
// missing. A zero or negative ttl means the key does not expire.
func (c *Cache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return false, nil
	}

	it, ok := c.items[key]
	if ok && !it.expired() {
		return false, nil
	}

	c.items[key] = item{
		value:     clone(value),
		expiresAt: expiresAt(ttl),
	}

	return true, nil
}

// Delete removes one or more keys.
//
// Deleting a missing key is not an error. Delete is a no-op after Close.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	for _, key := range keys {
		delete(c.items, key)
	}

	return nil
}

// Exists reports whether key exists and has not expired.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return false, nil
	}
	it, ok := c.items[key]
	c.mu.RUnlock()

	if !ok || it.expired() {
		if ok {
			_ = c.Delete(ctx, key)
		}
		return false, nil
	}

	return true, nil
}

// TTL returns the remaining lifetime of key.
//
// It returns cache.ErrMiss when key does not exist, has expired, or the cache
// has been closed. It returns 0 when key exists without expiration.
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return 0, cache.ErrMiss
	}
	it, ok := c.items[key]
	c.mu.RUnlock()

	if !ok || it.expired() {
		if ok {
			_ = c.Delete(ctx, key)
		}
		return 0, cache.ErrMiss
	}

	if it.expiresAt.IsZero() {
		return 0, nil
	}

	return time.Until(it.expiresAt), nil
}

// Increment atomically increases a numeric value by delta.
//
// If key is missing or expired, it is created with delta as its initial value.
// When the key is created and ttl is greater than zero, ttl is applied to the
// new key. Existing keys keep their original expiration.
func (c *Cache) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, nil
	}

	it, ok := c.items[key]
	if !ok || it.expired() {
		value := delta
		c.items[key] = item{
			value:     []byte(strconv.FormatInt(value, 10)),
			expiresAt: expiresAt(ttl),
		}
		return value, nil
	}

	current, err := strconv.ParseInt(string(it.value), 10, 64)
	if err != nil {
		return 0, err
	}

	current += delta
	it.value = []byte(strconv.FormatInt(current, 10))
	c.items[key] = it

	return current, nil
}

// Ping always succeeds because the in-memory cache is process-local and has no
// external dependency.
func (c *Cache) Ping(context.Context) error {
	return nil
}

// Close releases all entries stored by the cache.
//
// Close is idempotent.
func (c *Cache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	c.items = nil

	return nil
}

func (i item) expired() bool {
	return !i.expiresAt.IsZero() && time.Now().After(i.expiresAt)
}

func expiresAt(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}

	return time.Now().Add(ttl)
}

func clone(value []byte) []byte {
	v := make([]byte, len(value))
	copy(v, value)
	return v
}
