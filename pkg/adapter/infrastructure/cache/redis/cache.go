package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/redis/go-redis/v9"
)

// Verify Cache implements the core cache contract at compile time.
var _ cache.Cache = (*Cache)(nil)

// Cache adapts a go-redis client to the core cache contract.
//
// Cache does not own cache semantics such as key naming or value serialization;
// callers are responsible for namespacing keys and encoding values.
type Cache struct {
	client *redis.Client
}

// New constructs a Redis-backed cache adapter.
//
// The caller provides a configured Redis client. Close closes that client.
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// Get returns the value associated with key.
//
// It returns cache.ErrMiss when Redis reports that the key does not exist.
func (c *Cache) Get(ctx context.Context, key string) ([]byte, error) {
	value, err := c.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, cache.ErrMiss
	}
	if err != nil {
		return nil, fmt.Errorf("redis cache get %q: %w", key, err)
	}

	return value, nil
}

// Set stores value under key with ttl.
//
// A zero ttl means the key does not expire.
func (c *Cache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("redis cache set %q: %w", key, err)
	}

	return nil
}

// SetNX stores value only when key does not already exist.
//
// It returns true when the value was stored. A zero ttl means the key does not
// expire.
//
// This uses Redis SET with NX instead of the deprecated SETNX command so TTL
// and conditional creation are applied atomically in one command.
func (c *Cache) SetNX(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	result, err := c.client.SetArgs(ctx, key, value, redis.SetArgs{
		Mode: "NX",
		TTL:  ttl,
	}).Result()

	if errors.Is(err, redis.Nil) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("redis cache setnx %q: %w", key, err)
	}

	return result == "OK", nil
}

// Delete removes one or more keys.
//
// Deleting a missing key is not an error.
func (c *Cache) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("redis cache delete keys: %w", err)
	}

	return nil
}

// Exists reports whether key exists.
func (c *Cache) Exists(ctx context.Context, key string) (bool, error) {
	n, err := c.client.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("redis cache exists %q: %w", key, err)
	}

	return n > 0, nil
}

// TTL returns the remaining lifetime of key.
//
// It returns cache.ErrMiss when key does not exist. It returns 0 when key
// exists without expiration.
func (c *Cache) TTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := c.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("redis cache ttl %q: %w", key, err)
	}

	switch ttl {
	case -2 * time.Second:
		return 0, cache.ErrMiss
	case -1 * time.Second:
		return 0, nil
	default:
		return ttl, nil
	}
}

// incrementScript atomically increments a numeric value and applies ttl only
// when the key is newly created.
//
// Redis INCRBY does not support setting expiration in the same command, so Lua
// is used to keep the increment and first-write expiration atomic.
var incrementScript = redis.NewScript(`
local exists = redis.call("EXISTS", KEYS[1])
local value = redis.call("INCRBY", KEYS[1], ARGV[1])

if exists == 0 and tonumber(ARGV[2]) > 0 then
	redis.call("PEXPIRE", KEYS[1], ARGV[2])
end

return value
`)

// Increment atomically increases a numeric value by delta.
//
// If key is created and ttl is greater than zero, ttl is applied to the new
// key. Existing keys keep their current expiration.
func (c *Cache) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	ttlMillis := int64(0)
	if ttl > 0 {
		ttlMillis = ttl.Milliseconds()
		if ttlMillis <= 0 {
			ttlMillis = 1
		}
	}

	value, err := incrementScript.Run(
		ctx,
		c.client,
		[]string{key},
		delta,
		ttlMillis,
	).Int64()
	if err != nil {
		return 0, fmt.Errorf("redis cache increment %q: %w", key, err)
	}

	return value, nil
}

// Ping verifies that the Redis server is reachable.
func (c *Cache) Ping(ctx context.Context) error {
	if err := c.client.Ping(ctx).Err(); err != nil {
		return cerr.ServiceUnavailable(fmt.Errorf("ping: %w", err))
	}

	return nil
}

// Close closes the underlying Redis client.
func (c *Cache) Close() error {
	if err := c.client.Close(); err != nil {
		return fmt.Errorf("redis cache close: %w", err)
	}

	return nil
}
