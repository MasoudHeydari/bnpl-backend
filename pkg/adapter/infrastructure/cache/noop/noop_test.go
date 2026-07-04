package noop_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cachenoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
)

func TestGetAlwaysMisses(t *testing.T) {
	c := cachenoop.New()

	_, err := c.Get(context.Background(), "missing")
	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestSetAndDeleteSucceedWithoutStoring(t *testing.T) {
	c := cachenoop.New()
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "key", []byte("value"), time.Minute))
	require.NoError(t, c.Delete(ctx, "key"))

	exists, err := c.Exists(ctx, "key")
	require.NoError(t, err)
	require.False(t, exists)
}

func TestSetNXReturnsTrueWithoutRetainingState(t *testing.T) {
	c := cachenoop.New()
	ctx := context.Background()

	ok, err := c.SetNX(ctx, "key", []byte("value"), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = c.SetNX(ctx, "key", []byte("other"), time.Minute)
	require.NoError(t, err)
	require.True(t, ok)
}

func TestTTLAlwaysMisses(t *testing.T) {
	c := cachenoop.New()

	_, err := c.TTL(context.Background(), "key")
	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestIncrementReturnsDeltaWithoutAccumulating(t *testing.T) {
	c := cachenoop.New()
	ctx := context.Background()

	first, err := c.Increment(ctx, "counter", 3, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(3), first)

	second, err := c.Increment(ctx, "counter", 5, time.Minute)
	require.NoError(t, err)
	require.Equal(t, int64(5), second)
}

func TestCloseSucceeds(t *testing.T) {
	require.NoError(t, cachenoop.New().Close())
}
