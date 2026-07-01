package config

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrations_newUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	migrations := mock.NewMigrations(t)

	m := Migrations{filesDir: "/var/migrations"}

	uc := m.newUseCase(pool, migrations)
	require.NotNil(t, uc)
}

func TestOutboxPublisher_newUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	outbox := mock.NewOutbox(t)
	publisher := mock.NewPublisher(t)
	clock := mock.NewClock(t)
	logger := mock.NewLogger(t)

	obp := OutboxPublisher{
		WorkerID:       "worker-1",
		BatchSize:      25,
		RetryBaseDelay: 2 * time.Second,
		RetryMaxDelay:  30 * time.Second,
	}

	uc := obp.newUseCase(pool, outbox, publisher, clock, logger)
	require.NotNil(t, uc)
	assert.Equal(t, "worker-1", uc.WorkerID)
	assert.Equal(t, 25, uc.BatchSize)
	assert.Equal(t, 2*time.Second, uc.RetryBaseDelay)
	assert.Equal(t, 30*time.Second, uc.RetryMaxDelay)
}

func TestHealth_newUseCase(t *testing.T) {
	t.Parallel()

	lifecycle := mock.NewLifecycleState(t)
	checker := mock.NewDependencyChecker(t)

	h := Health{Enabled: true}

	uc := h.newUseCase(lifecycle, checker)
	require.NotNil(t, uc)
}

func TestConfig_NewMigrationsUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	migrations := mock.NewMigrations(t)

	cfg := &Config{
		Usecases: Usecases{
			Migrations: Migrations{filesDir: "/data/migrations"},
		},
	}

	uc := cfg.NewMigrationsUseCase(pool, migrations)
	require.NotNil(t, uc)
}

func TestConfig_NewOutboxPublisherUseCase(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	outbox := mock.NewOutbox(t)
	publisher := mock.NewPublisher(t)
	clock := mock.NewClock(t)
	logger := mock.NewLogger(t)

	cfg := &Config{
		Usecases: Usecases{
			OutboxPublisher: OutboxPublisher{
				WorkerID:       "bootstrap-worker",
				BatchSize:      10,
				RetryBaseDelay: time.Second,
				RetryMaxDelay:  time.Minute,
			},
		},
	}

	uc := cfg.NewOutboxPublisherUseCase(pool, outbox, publisher, clock, logger)
	require.NotNil(t, uc)
	assert.Equal(t, "bootstrap-worker", uc.WorkerID)
}

func TestConfig_NewHealthUseCase(t *testing.T) {
	t.Parallel()

	lifecycle := mock.NewLifecycleState(t)
	checker := mock.NewDependencyChecker(t)

	cfg := &Config{
		Usecases: Usecases{
			Health: Health{Enabled: true},
		},
	}

	uc := cfg.NewHealthUseCase(lifecycle, checker)
	require.NotNil(t, uc)
}
