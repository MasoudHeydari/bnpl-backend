package redis_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	redisadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/redis"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
)

func newTestCache(t *testing.T) (*redisadapter.Cache, *miniredis.Miniredis) {
	t.Helper()

	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return redisadapter.New(client), server
}

func TestGetSetAndDelete(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "user:1", []byte("alice"), time.Minute))

	value, err := c.Get(ctx, "user:1")
	require.NoError(t, err)
	require.Equal(t, []byte("alice"), value)

	require.NoError(t, c.Delete(ctx, "user:1"))

	_, err = c.Get(ctx, "user:1")
	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestSetNX(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	ok, err := c.SetNX(ctx, "lock", []byte("1"), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = c.SetNX(ctx, "lock", []byte("2"), time.Minute)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestExistsAndTTL(t *testing.T) {
	c, _ := newTestCache(t)
	ctx := context.Background()

	exists, err := c.Exists(ctx, "missing")
	require.NoError(t, err)
	require.False(t, exists)

	require.NoError(t, c.Set(ctx, "session", []byte("token"), 2*time.Minute))

	exists, err = c.Exists(ctx, "session")
	require.NoError(t, err)
	require.True(t, exists)

	ttl, err := c.TTL(ctx, "session")
	require.NoError(t, err)
	require.Greater(t, ttl, time.Minute)
}

func TestIncrementAppliesTTLOnFirstWrite(t *testing.T) {
	c, server := newTestCache(t)
	ctx := context.Background()

	value, err := c.Increment(ctx, "counter", 4, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(4), value)

	value, err = c.Increment(ctx, "counter", 2, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(6), value)

	require.True(t, server.Exists("counter"))
}

func TestCloseClosesUnderlyingClient(t *testing.T) {
	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	client := goredis.NewClient(&goredis.Options{Addr: server.Addr()})
	c := redisadapter.New(client)

	require.NoError(t, c.Close())
}
