package outboxpublisheruc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
)

func TestNextAttemptAt_AttemptOne_UsesBaseDelay(t *testing.T) {
	now := time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	uc := newRetryUseCase(now)

	assert.Equal(t, now.Add(time.Second), uc.nextAttemptAt(1))
}

func TestNextAttemptAt_AttemptTwo_DoubleDelay(t *testing.T) {
	now := time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	uc := newRetryUseCase(now)

	assert.Equal(t, now.Add(2*time.Second), uc.nextAttemptAt(2))
}

func TestNextAttemptAt_DelayCapAtMax(t *testing.T) {
	now := time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	uc := newRetryUseCase(now)
	uc.RetryMaxDelay = 4 * time.Second

	assert.Equal(t, now.Add(4*time.Second), uc.nextAttemptAt(10))
}

func TestNextAttemptAt_AttemptZero_UsesBaseDelay(t *testing.T) {
	now := time.Date(
		2026,
		6,
		25,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	uc := newRetryUseCase(now)

	assert.Equal(t, now.Add(time.Second), uc.nextAttemptAt(0))
}

func newRetryUseCase(now time.Time) *UseCase {
	return &UseCase{
		clock:          adapterclock.NewFixed(now, time.UTC),
		RetryBaseDelay: time.Second,
		RetryMaxDelay:  8 * time.Second,
	}
}
