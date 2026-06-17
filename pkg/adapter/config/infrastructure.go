package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	redis2 "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/redis"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adapterslog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/slog"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/redis/go-redis/v9"
)

type Infrastructure struct {
	Trace  Trace
	Clock  Clock
	Logger Logger
	Cache  Cache
}

type Trace struct{}

func (t Trace) newTraceInfrastructure() (*trace.Provider, error) {
	return trace.NewULIDProvider(), nil
}

type Clock struct {
	Location         string `yaml:"location"`
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
	Provider log.Provider `yaml:"provider"`
	Noop     Noop         `yaml:"noop"`
	Slog     Slog         `yaml:"slog"`
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
