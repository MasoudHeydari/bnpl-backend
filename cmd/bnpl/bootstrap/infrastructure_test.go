package bootstrap

import (
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/stretchr/testify/require"
)

func noopTestConfig() *config.Config {
	return &config.Config{
		Usecases: config.Usecases{
			OutboxPublisher: config.OutboxPublisher{
				WorkerID:       "worker-1",
				BatchSize:      5,
				RetryBaseDelay: time.Second,
				RetryMaxDelay:  time.Minute,
			},
			Health: config.Health{
				Enabled: true,
			},
		},
		Infrastructure: config.Infrastructure{
			Clock: config.Clock{
				Location:         "UTC",
				BusinessLocation: "UTC",
			},
			Logger: config.Logger{
				Provider: log.NoopProvider,
			},
			Cache: config.Cache{
				Provider: adaptercache.NoopProvider,
			},
			Messaging: config.Messaging{
				Provider: messaging.NoopProvider,
			},
			CircuitBreaker: config.CircuitBreaker{
				Provider: circuitbreaker.NoopProvider,
			},
			Task: config.Task{
				Provider: taskadapter.NoopProvider,
				Scheduled: config.ScheduledTasks{
					RuntimeGoroutineMetrics: config.TaskDefinition{
						Enabled: false,
						Handler: "runtime_goroutine_metrics",
						Trigger: config.TaskTrigger{
							Type: task.TriggerManual,
						},
					},
					OutboxPublisher: config.TaskDefinition{
						Enabled: false,
						Handler: "outbox_publisher",
						Trigger: config.TaskTrigger{
							Type:     task.TriggerInterval,
							Interval: "5s",
						},
					},
				},
			},
		},
	}
}

func testAppWithInfrastructure(t *testing.T, cfg *config.Config) *App {
	t.Helper()

	app := &App{
		Config: cfg,
		Pool:   mock.NewPool(t),
		Repos:  NewRepos(),
	}

	infrastructure, err := app.NewInfrastructures()
	require.NoError(t, err)

	app.Infrastructure = infrastructure

	return app
}

func TestNewInfrastructures_NoopConfig(t *testing.T) {
	t.Parallel()

	app := testAppWithInfrastructure(t, noopTestConfig())
	infra := app.Infrastructure

	require.NotNil(t, infra.DependencyChecker)
	require.NotNil(t, infra.Trace)
	require.NotNil(t, infra.Clock)
	require.NotNil(t, infra.Logger)
	require.NotNil(t, infra.Cache)
	require.NotNil(t, infra.Publisher)
	require.NotNil(t, infra.Subscriber)
	require.NotNil(t, infra.LifecycleState)
	require.NotNil(t, infra.CircuitBreaker)
	require.NotNil(t, infra.Task)
}
