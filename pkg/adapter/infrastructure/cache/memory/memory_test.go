package memory

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGet_MissingKey_ReturnsMiss(t *testing.T) {
	c := New()

	_, err := c.Get(context.Background(), "missing")

	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestGet_ExpiredKey_ReturnsMiss(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("v"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	_, err := c.Get(context.Background(), "k")

	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestGet_ExistingKey_ReturnsValue(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Minute))

	got, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	got[0] = 'V'

	again, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), again)
}

func TestSet_ZeroTTL_KeyDoesNotExpire(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), 0))
	time.Sleep(time.Millisecond)

	got, err := c.Get(context.Background(), "k")

	require.NoError(t, err)
	assert.Equal(t, []byte("value"), got)
}

func TestSetNX_KeyAbsent_StoresAndReturnsTrue(t *testing.T) {
	c := New()

	stored, err := c.SetNX(context.Background(), "k", []byte("value"), time.Minute)

	require.NoError(t, err)
	assert.True(t, stored)
}

func TestSetNX_KeyPresent_DoesNotOverwriteReturnsFalse(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Minute))

	stored, err := c.SetNX(context.Background(), "k", []byte("new"), time.Minute)
	require.NoError(t, err)
	assert.False(t, stored)

	got, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.Equal(t, []byte("value"), got)
}

func TestSetNX_KeyExpired_StoresAndReturnsTrue(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	stored, err := c.SetNX(context.Background(), "k", []byte("new"), time.Minute)
	require.NoError(t, err)
	assert.True(t, stored)

	got, err := c.Get(context.Background(), "k")
	require.NoError(t, err)
	assert.Equal(t, []byte("new"), got)
}

func TestDelete_ExistingKey_Removed(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Minute))

	require.NoError(t, c.Delete(context.Background(), "k"))
	_, err := c.Get(context.Background(), "k")
	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestDelete_MissingKey_NoError(t *testing.T) {
	c := New()

	require.NoError(t, c.Delete(context.Background(), "missing"))
}

func TestDelete_MultipleKeys(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "a", []byte("1"), time.Minute))
	require.NoError(t, c.Set(context.Background(), "b", []byte("2"), time.Minute))

	require.NoError(t, c.Delete(context.Background(), "a", "b"))
	_, errA := c.Get(context.Background(), "a")
	_, errB := c.Get(context.Background(), "b")
	require.ErrorIs(t, errA, cache.ErrMiss)
	require.ErrorIs(t, errB, cache.ErrMiss)
}

func TestExists_ExistingKey_ReturnsTrue(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Minute))

	exists, err := c.Exists(context.Background(), "k")

	require.NoError(t, err)
	assert.True(t, exists)
}

func TestExists_MissingKey_ReturnsFalse(t *testing.T) {
	c := New()

	exists, err := c.Exists(context.Background(), "missing")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestExists_ExpiredKey_ReturnsFalse(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Nanosecond))
	time.Sleep(time.Millisecond)

	exists, err := c.Exists(context.Background(), "k")

	require.NoError(t, err)
	assert.False(t, exists)
}

func TestTTL_ExistingKeyWithTTL_ReturnsPositive(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Second))

	ttl, err := c.TTL(context.Background(), "k")

	require.NoError(t, err)
	assert.Positive(t, ttl)
}

func TestTTL_ExistingKeyWithoutTTL_ReturnsZero(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), 0))

	ttl, err := c.TTL(context.Background(), "k")

	require.NoError(t, err)
	assert.Zero(t, ttl)
}

func TestTTL_MissingKey_ReturnsMiss(t *testing.T) {
	c := New()

	_, err := c.TTL(context.Background(), "missing")

	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestIncrement_NewKey_ReturnsInitialDelta(t *testing.T) {
	c := New()

	got, err := c.Increment(context.Background(), "count", 3, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, int64(3), got)
}

func TestIncrement_ExistingKey_Accumulates(t *testing.T) {
	c := New()
	_, err := c.Increment(context.Background(), "count", 3, time.Minute)
	require.NoError(t, err)

	got, err := c.Increment(context.Background(), "count", 2, time.Minute)

	require.NoError(t, err)
	assert.Equal(t, int64(5), got)
}

func TestIncrement_ExistingKey_KeepsOriginalTTL(t *testing.T) {
	c := New()
	_, err := c.Increment(context.Background(), "count", 1, 100*time.Millisecond)
	require.NoError(t, err)
	time.Sleep(10 * time.Millisecond)
	before, err := c.TTL(context.Background(), "count")
	require.NoError(t, err)

	_, err = c.Increment(context.Background(), "count", 1, time.Hour)
	require.NoError(t, err)
	after, err := c.TTL(context.Background(), "count")
	require.NoError(t, err)

	assert.LessOrEqual(t, after, before)
	assert.Less(t, after, time.Hour)
}

func TestIncrement_NonNumericValue_ReturnsError(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "count", []byte("nope"), time.Minute))

	_, err := c.Increment(context.Background(), "count", 1, time.Minute)

	require.Error(t, err)
}

func TestClose_SubsequentGetReturnsMiss(t *testing.T) {
	c := New()
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Minute))
	require.NoError(t, c.Close())

	_, err := c.Get(context.Background(), "k")

	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestClose_SubsequentSetIsNoOp(t *testing.T) {
	c := New()
	require.NoError(t, c.Close())
	require.NoError(t, c.Set(context.Background(), "k", []byte("value"), time.Minute))

	_, err := c.Get(context.Background(), "k")

	require.ErrorIs(t, err, cache.ErrMiss)
}

func TestClose_IsIdempotent(t *testing.T) {
	c := New()

	require.NoError(t, c.Close())
	require.NoError(t, c.Close())
}

func TestConcurrency_ParallelSetGet(t *testing.T) {
	c := New()
	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", i)
			value := []byte(fmt.Sprintf("value-%d", i))
			require.NoError(t, c.Set(ctx, key, value, time.Minute))
			got, err := c.Get(ctx, key)
			require.NoError(t, err)
			assert.Equal(t, value, got)
		}(i)
	}

	wg.Wait()
}

func TestContextCancellationIsRespected(t *testing.T) {
	c := New()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := c.Get(ctx, "k")

	require.True(t, errors.Is(err, context.Canceled))
}
