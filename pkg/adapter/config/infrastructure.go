package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/IBM/sarama"
	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	redis2 "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/redis"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	circuitbreakergobreaker "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/gobreaker"
	noopcircuitbreacker "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adapterslog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/slog"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	noopmessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/noop"
	saramamessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/sarama"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/inprocess"
	tasknoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
	corecb "github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	coremessaging "github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/redis/go-redis/v9"
)

// Infrastructure groups runtime infrastructure configuration.
//
// Each field configures a concrete infrastructure component that is
// constructed during application bootstrap.
type Infrastructure struct {
	// Trace configures distributed tracing infrastructure.
	Trace Trace `yaml:"trace"`

	// Clock configures the system clock implementation.
	Clock Clock `yaml:"clock"`

	// Logger configures the application logger.
	Logger Logger `yaml:"logger"`

	// Cache configures the cache infrastructure.
	Cache Cache `yaml:"cache"`

	// Messaging configures the messaging infrastructure.
	Messaging Messaging `yaml:"messaging"`

	// Task configures the task runtime infrastructure.
	Task Task `yaml:"task"`

	// CircuitBreaker configures the circuit breaker infrastructure.
	CircuitBreaker CircuitBreaker `yaml:"circuit_breaker"`
}

// Trace contains tracing infrastructure configuration.
type Trace struct{}

func (t Trace) newTraceInfrastructure() (*trace.Provider, error) {
	return trace.NewULIDProvider(), nil
}

// Clock defines runtime clock configuration.
type Clock struct {
	// Location is the application's default timezone.
	Location string `yaml:"location"`

	// BusinessLocation is the timezone used for business rules and
	// business-date calculations.
	BusinessLocation string `yaml:"business_location"`
}

func (c Clock) newSystemClockInfrastructure() (coreclock.Clock, error) {
	location, err := time.LoadLocation(c.Location)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid infrastructure.clock.location %q: %w",
			c.Location,
			err,
		)
	}

	businessLocation, err := time.LoadLocation(c.BusinessLocation)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid infrastructure.clock.business_location %q: %w",
			c.BusinessLocation,
			err,
		)
	}

	return clock.NewSystem(location, businessLocation), nil
}

// Logger configures the logging infrastructure.
type Logger struct {
	// Provider selects the logger implementation.
	//
	// Supported values:
	//   - noop
	//   - slog
	Provider log.Provider `yaml:"provider"`

	// Noop contains configuration for the noop logger provider.
	Noop Noop `yaml:"noop"`

	// Slog contains configuration for the slog logger provider.
	Slog Slog `yaml:"slog"`
}

// Slog configures the slog logger.
type Slog struct {
	// Level controls log verbosity.
	Level corelog.Level `yaml:"level"`

	// Format controls log output encoding.
	Format corelog.Format `yaml:"format"`
}

// newLoggerInfrastructure creates the configured logger.
func (l Logger) newLoggerInfrastructure() (corelog.Logger, error) {
	switch l.Provider {
	case log.SlogProvider:
		return l.newSlogLoggerInfrastructure()

	case log.NoopProvider:
		return l.newNoopLoggerInfrastructure()

	default:
		return nil, fmt.Errorf("unsupported log provider: %q", l.Provider)
	}
}

// newNoopLoggerInfrastructure creates a logger that discards all records.
func (l Logger) newNoopLoggerInfrastructure() (corelog.Logger, error) {
	return lognoop.New(), nil
}

// newSlogLoggerInfrastructure creates a slog-backed logger.
func (l Logger) newSlogLoggerInfrastructure() (corelog.Logger, error) {
	level, err := l.slogLevel()
	if err != nil {
		return nil, err
	}

	format, err := l.logFormat()
	if err != nil {
		return nil, err
	}

	return adapterslog.New(os.Stdout, level, format), nil
}

// slogLevel converts config log level to slog level.
func (l Logger) slogLevel() (slog.Level, error) {
	switch l.Slog.Level {
	case "":
		return slog.LevelInfo, nil
	case corelog.DebugLevel:
		return slog.LevelDebug, nil
	case corelog.InfoLevel:
		return slog.LevelInfo, nil
	case corelog.WarnLevel:
		return slog.LevelWarn, nil
	case corelog.ErrorLevel:
		return slog.LevelError, nil
	default:
		return slog.LevelInfo, fmt.Errorf("invalid log level: %q", l.Slog.Level)
	}
}

// logFormat validates log output format.
func (l Logger) logFormat() (corelog.Format, error) {
	switch l.Slog.Format {
	case "":
		return corelog.JSONFormat, nil
	case corelog.JSONFormat, corelog.TextFormat:
		return l.Slog.Format, nil
	default:
		return corelog.JSONFormat, fmt.Errorf("invalid log format: %q", l.Slog.Format)
	}
}

// Cache defines cache provider configuration and adapter-specific settings.
//
// Provider selects the cache implementation constructed during bootstrap.
// Only the selected provider is initialized at runtime.
type Cache struct {
	// Provider identifies the cache implementation to use.
	//
	// Supported values:
	//   - noop
	//   - redis
	Provider adaptercache.Provider `yaml:"provider"`

	// Noop contains configuration for the noop cache provider.
	Noop Noop `yaml:"noop"`

	// Redis contains configuration for the Redis cache provider.
	Redis Redis `yaml:"redis"`
}

// Redis contains Redis cache provider settings.
type Redis struct {
	// Addr is the Redis server address in host:port format.
	Addr string `yaml:"addr"`

	// Username is the Redis ACL username.
	//
	// Leave empty when ACL authentication is not configured.
	Username string `yaml:"username"`

	// Password is the Redis authentication secret.
	Password string `yaml:"password"`

	// DB is the Redis logical database number.
	DB int `yaml:"db"`

	// DialTimeout limits the time spent establishing new connections.
	DialTimeout time.Duration `yaml:"dial_timeout"`

	// ReadTimeout limits Redis read operations.
	ReadTimeout time.Duration `yaml:"read_timeout"`

	// WriteTimeout limits Redis write operations.
	WriteTimeout time.Duration `yaml:"write_timeout"`

	// PoolSize is the maximum number of connections managed by the client.
	PoolSize int `yaml:"pool_size"`

	// MinIdleConns is the minimum number of idle connections maintained in the
	// connection pool.
	MinIdleConns int `yaml:"min_idle_conns"`
}

// newCacheInfrastructure constructs the configured cache provider.
//
// The selected implementation is determined by Cache.Provider. Startup fails
// when an unsupported provider is configured
func (c Cache) newCacheInfrastructure() (cache.Cache, error) {
	switch c.Provider {
	case adaptercache.RedisProvider:
		return c.newRedisInfrastructure()

	case adaptercache.NoopProvider:
		return c.newNoopCacheInfrastructure()

	default:
		return nil, fmt.Errorf(
			"unsupported cache provider: %q",
			c.Provider,
		)
	}
}

// newNoopCacheInfrastructure constructs a noop cache provider.
//
// The noop implementation never stores data and is primarily intended for
// environments where caching is disabled, local development, or tests that do
// not require cache behavior.
func (c Cache) newNoopCacheInfrastructure() (cache.Cache, error) {
	return noop.New(), nil
}

// newRedisInfrastructure constructs and validates a Redis-backed cache
// provider.
//
// The Redis client is created from configuration and a connectivity check is
// performed before the cache adapter is returned. Startup fails when Redis is
// unreachable or the configuration is invalid.
func (c Cache) newRedisInfrastructure() (cache.Cache, error) {
	if c.Redis.Addr == "" {
		return nil, fmt.Errorf("invalid cache.redis.addr: empty")
	}

	redisOpt := &redis.Options{
		Addr:         c.Redis.Addr,
		Username:     c.Redis.Username,
		Password:     c.Redis.Password,
		DB:           c.Redis.DB,
		DialTimeout:  c.Redis.DialTimeout,
		ReadTimeout:  c.Redis.ReadTimeout,
		WriteTimeout: c.Redis.WriteTimeout,
		PoolSize:     c.Redis.PoolSize,
		MinIdleConns: c.Redis.MinIdleConns,
	}

	client := redis.NewClient(redisOpt)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		redisOpt.DialTimeout,
	)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf(
			"connect redis cache %q: %w",
			c.Redis.Addr,
			err,
		)
	}

	return redis2.New(client), nil
}

// Messaging defines messaging provider configuration and adapter-specific
// settings.
//
// Provider selects the messaging implementation constructed during bootstrap.
// Only the selected provider is initialized at runtime.
type Messaging struct {
	// Provider identifies the messaging implementation to use.
	//
	// Supported values:
	//   - noop
	//   - kafka
	Provider messaging.Provider `yaml:"provider"`

	// Noop contains configuration for the noop messaging provider.
	Noop Noop `yaml:"noop"`

	// Kafka contains configuration for the Kafka messaging provider.
	Kafka Kafka `yaml:"kafka"`
}

// Kafka contains Kafka messaging provider settings.
type Kafka struct {
	// Brokers contains Kafka bootstrap broker addresses.
	Brokers []string `yaml:"brokers"`

	// ClientID identifies this application to Kafka brokers.
	ClientID string `yaml:"client_id"`

	// GroupID identifies the consumer group used by Kafka subscribers.
	GroupID string `yaml:"group_id"`

	// Topics contains logical topic mappings used by the application.
	Topics Topics `yaml:"topics"`

	// Producer contains Kafka producer configuration.
	Producer Producer `yaml:"producer"`

	// Consumer contains Kafka consumer configuration.
	Consumer Consumer `yaml:"consumer"`
}

// Topics contains application topic mappings.
//
// Topic names are centralized here so use cases and adapters do not depend on
// hard-coded Kafka topic strings.
type Topics struct {
	// PaymentResults receives payment gateway or financial-core payment result
	// events.
	PaymentResults string `yaml:"payment_results"`
}

// Producer contains Kafka producer settings.
type Producer struct {
	// RequiredAcks controls broker acknowledgement behavior.
	//
	// Common values:
	//   0  = no acknowledgement
	//   1  = leader acknowledgement
	//  -1  = all in-sync replicas acknowledgement
	RequiredAcks int16 `yaml:"required_acks"`

	// MaxRetries is the maximum number of retries before publish fails.
	MaxRetries int `yaml:"max_retries"`

	// ReturnSuccesses enables success notifications from the producer.
	ReturnSuccesses bool `yaml:"return_successes"`
}

// Consumer contains Kafka consumer settings.
type Consumer struct {
	// Topics contains physical Kafka topics consumed by this service.
	//
	// Topic names must match the actual topics configured in the Kafka cluster.
	// Multiple topics may be consumed by the same consumer group.
	Topics []string `yaml:"topics"`
}

// newPublisherInfrastructure creates the configured messaging publisher
// implementation.
func (m Messaging) newPublisherInfrastructure() (coremessaging.Publisher, error) {
	switch m.Provider {
	case messaging.NoopProvider:
		return noopmessaging.NewPublisher(), nil

	case messaging.KafkaProvider:
		return saramamessaging.NewProducer(
			m.Kafka.Brokers,
			m.Kafka.Producer.ReturnSuccesses,
			sarama.RequiredAcks(m.Kafka.Producer.RequiredAcks),
			m.Kafka.Producer.MaxRetries,
		)

	default:
		return nil, fmt.Errorf("unsupported messaging provider: %q", m.Provider)
	}
}

// newSubscriberInfrastructure creates the configured messaging subscriber
// implementation.
func (m Messaging) newSubscriberInfrastructure(
	logger corelog.Logger,
) (coremessaging.Subscriber, error) {
	switch m.Provider {
	case messaging.NoopProvider:
		return noopmessaging.NewSubscriber(), nil

	case messaging.KafkaProvider:
		return saramamessaging.NewConsumer(
			m.Kafka.Brokers,
			m.Kafka.GroupID,
			m.Kafka.Consumer.Topics,
			logger,
		)

	default:
		return nil, fmt.Errorf("unsupported messaging provider: %q", m.Provider)
	}
}

// Task configures the task runtime and startup-known scheduled tasks.
type Task struct {
	// Provider selects the task runtime implementation.
	//
	// Supported values:
	//   - noop
	//   - inprocess
	Provider taskadapter.Provider `yaml:"provider"`

	// Noop contains configuration for the noop task runtime.
	Noop Noop `yaml:"noop"`

	// InProcess contains configuration for the in-process task runtime.
	InProcess InProcess `yaml:"inprocess"`

	// Scheduled contains tasks known at application startup.
	Scheduled ScheduledTasks `yaml:"scheduled"`
}

// InProcess configures the in-process task runtime.
type InProcess struct {
	// DefaultTimezone is used for cron triggers without task-specific timezone.
	DefaultTimezone string `yaml:"default_timezone"`
}

// ScheduledTasks contains tasks known at application startup.
//
// These tasks are loaded from configuration and registered during bootstrap.
// Runtime-created or user-triggered work should be executed through Dispatch,
// not registered here.
type ScheduledTasks struct {
	// RuntimeGoroutineMetrics periodically records runtime goroutine metrics.
	RuntimeGoroutineMetrics TaskDefinition `yaml:"runtime_goroutine_metrics"`

	// OutboxPublisher publishes pending outbox messages.
	OutboxPublisher TaskDefinition `yaml:"outbox_publisher"`
}

// TaskDefinitionEntry associates a configured task definition with its
// configuration key.
type TaskDefinitionEntry struct {
	// Name is the configuration key of the task.
	Name string

	// TaskDefinition contains the task configuration.
	TaskDefinition
}

// TaskDefinition describes a configured scheduled background task.
type TaskDefinition struct {
	// Enabled determines whether this task should be registered.
	Enabled bool `yaml:"enabled"`

	// Handler is a stable provider-agnostic task identifier.
	Handler string `yaml:"handler"`

	// Trigger describes when this task should be executed.
	Trigger TaskTrigger `yaml:"trigger"`
}

type TaskTrigger struct {
	// Type selects the scheduling strategy.
	Type task.TriggerType `yaml:"type"`

	// Cron contains the cron expression used when Type is cron.
	Cron string `yaml:"cron"`

	// Timezone overrides the runtime default timezone for cron schedules.
	Timezone string `yaml:"timezone"`

	// Interval contains the execution interval when Type is interval.
	//
	// The value must be a valid Go duration such as "30s", "5m", or "1h".
	Interval string `yaml:"interval"`

	// RunOnStart determines whether the task executes immediately after
	// registration.
	RunOnStart bool `yaml:"run_on_start"`
}

// ToCore converts config trigger values into the core task trigger model.
func (t TaskTrigger) ToCore() (task.Trigger, error) {
	trigger := task.Trigger{
		Type:       t.Type,
		Cron:       t.Cron,
		Timezone:   t.Timezone,
		RunOnStart: t.RunOnStart,
	}

	if t.Interval != "" {
		interval, err := time.ParseDuration(t.Interval)
		if err != nil {
			return task.Trigger{}, fmt.Errorf("parse task interval %q: %w", t.Interval, err)
		}

		trigger.Interval = interval
	}

	return trigger, nil
}

// newTaskInfrastructure creates the configured task runtime.
func (t Task) newTaskInfrastructure(logger corelog.Logger) (task.Runtime, error) {
	switch t.Provider {
	case taskadapter.NoopProvider:
		return t.newNoopTaskInfrastructure(logger)

	case taskadapter.InProcessProvider:
		return t.newInProcessTaskInfrastructure(logger)

	default:
		return nil, fmt.Errorf("unsupported task provider: %q", t.Provider)
	}
}

// newNoopTaskInfrastructure creates a no-op task runtime.
func (t Task) newNoopTaskInfrastructure(logger corelog.Logger) (task.Runtime, error) {
	return tasknoop.New(logger), nil
}

// newInProcessTaskInfrastructure creates an in-process task runtime.
func (t Task) newInProcessTaskInfrastructure(logger corelog.Logger) (task.Runtime, error) {
	return inprocess.New(
		inprocess.Config{
			DefaultTimezone: t.InProcess.DefaultTimezone,
		},
		logger,
	), nil
}

// getTaskDefinitions returns all configured startup-known tasks.
func (t Task) getTaskDefinitions() ([]TaskDefinitionEntry, error) {
	return []TaskDefinitionEntry{
		{
			TaskDefinition: t.Scheduled.RuntimeGoroutineMetrics,
			Name:           "runtime_goroutine_metrics",
		},
		{
			TaskDefinition: t.Scheduled.OutboxPublisher,
			Name:           "outbox_publisher",
		},
	}, nil
}

// CircuitBreaker contains configuration for the runtime circuit breaker
// infrastructure.
type CircuitBreaker struct {
	// Provider selects the circuit breaker implementation.
	//
	// Supported values are "noop" and "gobreaker".
	Provider circuitbreaker.Provider `yaml:"provider"`

	// Noop contains configuration for the no-op provider.
	Noop Noop `yaml:"noop"`

	// GoBreaker contains configuration for the sony/gobreaker provider.
	GoBreaker GoBreakerCircuitBreaker `yaml:"gobreaker"`
}

// GoBreakerCircuitBreaker configures the sony/gobreaker adapter.
type GoBreakerCircuitBreaker struct {
	// MaxRequests is the maximum number of calls allowed while the circuit is
	// half-open.
	MaxRequests uint32 `yaml:"max_requests"`

	// Interval is the period after which gobreaker clears internal counters
	// while the circuit is closed.
	Interval time.Duration `yaml:"interval"`

	// Timeout is the duration the circuit remains open before probing the
	// dependency again.
	Timeout time.Duration `yaml:"timeout"`

	// FailureThreshold is the minimum number of consecutive failed executions
	// required before the circuit transitions from closed to open.
	FailureThreshold uint32 `yaml:"failure_threshold"`
}

// newCircuitBreakerInfrastructure creates the configured circuit breaker.
func (cb CircuitBreaker) newCircuitBreakerInfrastructure() (corecb.CircuitBreaker, error) {
	switch cb.Provider {
	case circuitbreaker.NoopProvider:
		return cb.newNoopCircuitBreakerInfrastructure()

	case circuitbreaker.GoBreakerProvider:
		return cb.newGoBreakerCircuitBreakerInfrastructure()

	default:
		return nil, fmt.Errorf("unsupported circuit breaker provider: %q", cb.Provider)
	}
}

// newNoopCircuitBreakerInfrastructure creates a no-op circuit breaker.
func (cb CircuitBreaker) newNoopCircuitBreakerInfrastructure() (corecb.CircuitBreaker, error) {
	return noopcircuitbreacker.New(), nil
}

// newGoBreakerCircuitBreakerInfrastructure creates a gobreaker-backed circuit breaker.
func (cb CircuitBreaker) newGoBreakerCircuitBreakerInfrastructure() (corecb.CircuitBreaker, error) {
	return circuitbreakergobreaker.New(
		circuitbreakergobreaker.Config{
			MaxRequests:      cb.GoBreaker.MaxRequests,
			Interval:         cb.GoBreaker.Interval,
			Timeout:          cb.GoBreaker.Timeout,
			FailureThreshold: cb.GoBreaker.FailureThreshold,
		},
	), nil
}
