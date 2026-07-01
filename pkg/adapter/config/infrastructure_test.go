package config

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
	adapterlog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCache_newCacheInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := (Cache{Provider: "memcached"}).newCacheInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported cache provider: "memcached"`)
}

func TestCache_newNoopCacheInfrastructure(t *testing.T) {
	t.Parallel()

	cacheInfra, err := (Cache{Provider: adaptercache.NoopProvider}).newCacheInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, cacheInfra)
}

func TestCache_newRedisInfrastructure_EmptyAddr(t *testing.T) {
	t.Parallel()

	_, err := (Cache{
		Provider: adaptercache.RedisProvider,
	}).newRedisInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cache.redis.addr: empty")
}

func TestCache_newRedisInfrastructure_Miniredis(t *testing.T) {
	t.Parallel()

	server, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(server.Close)

	cacheInfra, err := (Cache{
		Provider: adaptercache.RedisProvider,
		Redis: Redis{
			Addr:        server.Addr(),
			DialTimeout: time.Second,
			ReadTimeout: time.Second,
		},
	}).newCacheInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, cacheInfra)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, cacheInfra.Set(ctx, "config:test", []byte("ok"), time.Minute))
}

func TestCache_newRedisInfrastructure_UnreachableAddr(t *testing.T) {
	t.Parallel()

	_, err := (Cache{
		Provider: adaptercache.RedisProvider,
		Redis: Redis{
			Addr:        "127.0.0.1:1",
			DialTimeout: 100 * time.Millisecond,
		},
	}).newCacheInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "connect redis cache")
}

func TestMessaging_newPublisherInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := (Messaging{Provider: "nats"}).newPublisherInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported messaging provider: "nats"`)
}

func TestMessaging_newPublisherInfrastructure_Noop(t *testing.T) {
	t.Parallel()

	publisher, err := (Messaging{Provider: messaging.NoopProvider}).newPublisherInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, publisher)
	require.NoError(t, publisher.Close())
}

func TestMessaging_newPublisherInfrastructure_KafkaEmptyBrokers(t *testing.T) {
	t.Parallel()

	_, err := (Messaging{
		Provider: messaging.KafkaProvider,
		Kafka:    Kafka{Brokers: []string{}},
	}).newPublisherInfrastructure()
	require.Error(t, err)
}

func TestMessaging_newSubscriberInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)

	_, err := (Messaging{Provider: "nats"}).newSubscriberInfrastructure(logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported messaging provider: "nats"`)
}

func TestMessaging_newSubscriberInfrastructure_Noop(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)

	subscriber, err := (Messaging{Provider: messaging.NoopProvider}).newSubscriberInfrastructure(logger)
	require.NoError(t, err)
	require.NotNil(t, subscriber)
	require.NoError(t, subscriber.Close())
}

func TestMessaging_newSubscriberInfrastructure_KafkaEmptyBrokers(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)

	_, err := (Messaging{
		Provider: messaging.KafkaProvider,
		Kafka:    Kafka{Brokers: nil},
	}).newSubscriberInfrastructure(logger)
	require.Error(t, err)
}

func TestMessaging_newHealthInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := (Messaging{Provider: "nats"}).newHealthInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown messaging provider "nats"`)
}

func TestMessaging_newHealthInfrastructure_Noop(t *testing.T) {
	t.Parallel()

	checker, err := (Messaging{Provider: messaging.NoopProvider}).newHealthInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, checker)
}

func TestMessaging_newHealthInfrastructure_KafkaEmptyBrokers(t *testing.T) {
	t.Parallel()

	_, err := (Messaging{
		Provider: messaging.KafkaProvider,
		Kafka:    Kafka{Brokers: []string{}},
	}).newHealthInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create kafka health client")
}

func TestLogger_newLoggerInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := (Logger{Provider: "zap"}).newLoggerInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported log provider: "zap"`)
}

func TestLogger_newNoopLoggerInfrastructure(t *testing.T) {
	t.Parallel()

	logger, err := (Logger{Provider: adapterlog.NoopProvider}).newLoggerInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, logger)
}

func TestLogger_newSlogLoggerInfrastructure(t *testing.T) {
	t.Parallel()

	logger, err := (Logger{
		Provider: adapterlog.SlogProvider,
		Slog: Slog{
			Level:  log.InfoLevel,
			Format: log.JSONFormat,
		},
	}).newLoggerInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, logger)
}

func TestLogger_slogLevel_Invalid(t *testing.T) {
	t.Parallel()

	_, err := (Logger{
		Slog: Slog{Level: "verbose"},
	}).slogLevel()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid log level: "verbose"`)
}

func TestLogger_logFormat_Invalid(t *testing.T) {
	t.Parallel()

	_, err := (Logger{
		Slog: Slog{Format: "xml"},
	}).logFormat()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `invalid log format: "xml"`)
}

func TestTask_newTaskInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)

	_, err := (Task{Provider: "cron"}).newTaskInfrastructure(logger)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported task provider: "cron"`)
}

func TestTask_newNoopTaskInfrastructure(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)

	runtime, err := (Task{Provider: taskadapter.NoopProvider}).newTaskInfrastructure(logger)
	require.NoError(t, err)
	require.NotNil(t, runtime)
}

func TestTask_newInProcessTaskInfrastructure(t *testing.T) {
	t.Parallel()

	logger := mock.NewLogger(t)

	runtime, err := (Task{
		Provider: taskadapter.InProcessProvider,
		InProcess: InProcess{
			DefaultTimezone: "UTC",
		},
	}).newTaskInfrastructure(logger)
	require.NoError(t, err)
	require.NotNil(t, runtime)
}

func TestTask_getTaskDefinitions(t *testing.T) {
	t.Parallel()

	taskCfg := Task{
		Scheduled: ScheduledTasks{
			RuntimeGoroutineMetrics: TaskDefinition{
				Enabled: true,
				Handler: "metrics",
			},
			OutboxPublisher: TaskDefinition{
				Enabled: false,
				Handler: "outbox",
			},
		},
	}

	definitions, err := taskCfg.getTaskDefinitions()
	require.NoError(t, err)
	require.Len(t, definitions, 2)
	assert.Equal(t, "runtime_goroutine_metrics", definitions[0].Name)
	assert.True(t, definitions[0].Enabled)
	assert.Equal(t, "outbox_publisher", definitions[1].Name)
	assert.False(t, definitions[1].Enabled)
}

func TestTaskTrigger_ToCore_InvalidInterval(t *testing.T) {
	t.Parallel()

	_, err := (TaskTrigger{
		Type:     task.TriggerInterval,
		Interval: "not-a-duration",
	}).ToCore()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse task interval")
}

func TestTaskTrigger_ToCore_ValidInterval(t *testing.T) {
	t.Parallel()

	trigger, err := (TaskTrigger{
		Type:     task.TriggerInterval,
		Interval: "30s",
	}).ToCore()
	require.NoError(t, err)
	assert.Equal(t, 30*time.Second, trigger.Interval)
}

func TestCircuitBreaker_newCircuitBreakerInfrastructure_UnknownProvider(t *testing.T) {
	t.Parallel()

	_, err := (CircuitBreaker{Provider: "hystrix"}).newCircuitBreakerInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unsupported circuit breaker provider: "hystrix"`)
}

func TestCircuitBreaker_newNoopCircuitBreakerInfrastructure(t *testing.T) {
	t.Parallel()

	cb, err := (CircuitBreaker{Provider: circuitbreaker.NoopProvider}).newCircuitBreakerInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, cb)
}

func TestCircuitBreaker_newGoBreakerCircuitBreakerInfrastructure(t *testing.T) {
	t.Parallel()

	cb, err := (CircuitBreaker{
		Provider: circuitbreaker.GoBreakerProvider,
		GoBreaker: GoBreakerCircuitBreaker{
			MaxRequests:      3,
			Interval:         time.Minute,
			Timeout:          30 * time.Second,
			FailureThreshold: 5,
		},
	}).newCircuitBreakerInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, cb)
}

func TestClock_newSystemClockInfrastructure_InvalidLocation(t *testing.T) {
	t.Parallel()

	_, err := (Clock{Location: "Invalid/Timezone"}).newSystemClockInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid infrastructure.clock.location")
}

func TestClock_newSystemClockInfrastructure_InvalidBusinessLocation(t *testing.T) {
	t.Parallel()

	_, err := (Clock{
		Location:         "UTC",
		BusinessLocation: "Invalid/Timezone",
	}).newSystemClockInfrastructure()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid infrastructure.clock.business_location")
}

func TestClock_newSystemClockInfrastructure_Valid(t *testing.T) {
	t.Parallel()

	clockInfra, err := (Clock{
		Location:         "UTC",
		BusinessLocation: "Asia/Tehran",
	}).newSystemClockInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, clockInfra)
}

func TestTrace_newTraceInfrastructure(t *testing.T) {
	t.Parallel()

	provider, err := (Trace{}).newTraceInfrastructure()
	require.NoError(t, err)
	require.NotNil(t, provider)
}

func TestHealthCheck_newDependencyCheckerInfrastructure_UnknownCheck(t *testing.T) {
	t.Parallel()

	_, err := (HealthCheck{
		Checks: []health.Check{"unknown"},
	}).newDependencyCheckerInfrastructure(nil, nil, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown health check "unknown"`)
}

func TestHealthCheck_newDependencyCheckerInfrastructure_KnownChecks(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	healthRepo := mock.NewRepoHealth(t)
	cacheInfra := mock.NewCache(t)
	messagingHealth := mock.NewMessagingHealth(t)

	checker, err := (HealthCheck{
		Checks: []health.Check{
			health.DatabaseCheck,
			health.CacheCheck,
			health.MessagingCheck,
		},
	}).newDependencyCheckerInfrastructure(pool, healthRepo, cacheInfra, messagingHealth)
	require.NoError(t, err)
	require.NotNil(t, checker)
}
