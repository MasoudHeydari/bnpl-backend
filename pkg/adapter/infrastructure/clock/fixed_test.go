package clock

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixed_Now_ReturnsSameInstant(t *testing.T) {
	loc := time.UTC
	fixed := NewFixed(time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		loc,
	), loc)

	assert.Equal(t, fixed.Now(), fixed.Now())
}

func TestFixed_Now_UsesBusinessLocation(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)
	fixed := NewFixed(time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		time.UTC,
	), loc)

	now := fixed.Now()

	assert.Equal(t, loc, now.Location())
}

func TestFixed_Add_ReturnsCorrectOffset(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)
	base := time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	fixed := NewFixed(base, loc)
	d := 3 * time.Hour

	got := fixed.Add(base, d)

	assert.Equal(t, base.In(loc).Add(d), got)
	assert.Equal(t, loc, got.Location())
}
