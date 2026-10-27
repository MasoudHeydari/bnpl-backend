package config

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/IBM/sarama"
	"github.com/redis/go-redis/v9"

	adaptercache "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	redis2 "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/redis"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker"
	circuitbreakergobreaker "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/gobreaker"
	noopcircuitbreacker "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	adaptererrorreporting "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting"
	errorreportingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting/noop"
	errorreportingsentry "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting/sentry"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
	adapterhttpclient "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient"
	httpclientnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient/noop"
	httpclientstandard "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/httpclient/standard"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adapterslog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/slog"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging"
	noopmessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/noop"
	saramamessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/sarama"
	adaptermetrics "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	prometheusmetrics "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/prometheus"
	adapterobjectstorage "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/objectstorage"
	objectstoragefilesystem "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/objectstorage/filesystem"
	objectstoragenoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/objectstorage/noop"
	taskadapter "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/composite"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/inprocess"
	tasknoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/noop"
	tasktemporal "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/temporal"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
	corecb "github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	coremessaging "github.com/parstis/bnpl-backend/pkg/core/messaging"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	coreobjectstorage "github.com/parstis/bnpl-backend/pkg/core/objectstorage"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

// Infrastructure groups runtime infrastructure configuration.
//
// Each field configures a concrete infrastructure component that is
// constructed during application bootstrap.
type Infrastructure struct {
	// Shutdown configures the shutdown infrastructure.
	Shutdown Shutdown `yaml:"shutdown"`

	// HealthCheck configures the health check infrastructure.
	HealthCheck HealthCheck `yaml:"health"`

	// Trace configures distributed tracing infrastructure.
	Trace Trace `yaml:"trace"`

	// Clock configures the system clock implementation.
	Clock Clock `yaml:"clock"`

	// Logger configures the application logger.
	Logger Logger `yaml:"logger"`

	// ErrorReporting configures unexpected operational error reporting.
	ErrorReporting ErrorReporting `yaml:"error_reporting"`

	// Metrics configures business and operational metrics collection and exposition.
	Metrics Metrics `yaml:"metrics"`

	// HTTPClient configures shared outbound HTTP/HTTPS infrastructure used by
	// adapters that call third-party APIs.
	HTTPClient HTTPClient `yaml:"http_client"`

	// Cache configures the cache infrastructure.
	Cache Cache `yaml:"cache"`

	// ObjectStorage configures immutable-style blob storage used by archival and
	// forensic workflows. Business use cases depend only on the core Store port.
	ObjectStorage ObjectStorage `yaml:"object_storage"`

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
	const op = "config.Clock.newSystemClockInfrastructure"

	location, err := time.LoadLocation(c.Location)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: invalid infrastructure.clock.location %q: %w", op, c.Location,
			err,
		)
	}

	businessLocation, err := time.LoadLocation(c.BusinessLocation)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: invalid infrastructure.clock.business_location %q: %w", op, c.BusinessLocation,
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
	const op = "config.Logger.newLoggerInfrastructure"

	switch l.Provider {
	case log.SlogProvider:
		return l.newSlogLoggerInfrastructure()

	case log.NoopProvider:
		return l.newNoopLoggerInfrastructure()

	default:
		return nil, fmt.Errorf("%s: unsupported log provider: %q", op, l.Provider)
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
	const op = "config.Logger.slogLevel"

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
		return slog.LevelInfo, fmt.Errorf("%s: invalid log level: %q", op, l.Slog.Level)
	}
}

// logFormat validates log output format.
func (l Logger) logFormat() (corelog.Format, error) {
	const op = "config.Logger.logFormat"

	switch l.Slog.Format {
	case "":
		return corelog.JSONFormat, nil

	case corelog.JSONFormat, corelog.TextFormat:
		return l.Slog.Format, nil

	default:
		return corelog.JSONFormat, fmt.Errorf("%s: invalid log format: %q", op, l.Slog.Format)
	}
}

// ErrorReporting configures unexpected operational error reporting.
type ErrorReporting struct {
	// Provider selects the error-reporting implementation.
	//
	// Supported values:
	//   - noop
	//   - sentry
	Provider adaptererrorreporting.Provider `yaml:"provider"`

	// Noop contains configuration for the noop provider.
	Noop Noop `yaml:"noop"`

	// Sentry contains configuration for the Sentry provider.
	Sentry SentryErrorReporting `yaml:"sentry"`
}

// SentryErrorReporting configures the Sentry SDK adapter.
type SentryErrorReporting struct {
	// DSN is the Sentry project data source name.
	DSN string `yaml:"dsn"`

	// Environment overrides runtime.environment when non-empty.
	Environment string `yaml:"environment"`

	// Release identifies the deployed application artifact, preferably a Git
	// commit SHA or immutable release version.
	Release string `yaml:"release"`

	// SampleRate controls error-event sampling in the range (0, 1].
	SampleRate float64 `yaml:"sample_rate"`

	// Debug enables Sentry SDK diagnostic output. Keep disabled in production
	// unless actively troubleshooting the integration.
	Debug bool `yaml:"debug"`
}

// newErrorReporterInfrastructure creates the configured error reporter.
func (e ErrorReporting) newErrorReporterInfrastructure(
	defaultEnvironment string,
) (coreerrorreporting.Reporter, error) {
	const op = "config.ErrorReporting.newErrorReporterInfrastructure"

	switch e.Provider {
	case "", adaptererrorreporting.NoopProvider:
		return errorreportingnoop.New(), nil

	case adaptererrorreporting.SentryProvider:
		environment := strings.TrimSpace(e.Sentry.Environment)
		if environment == "" {
			environment = strings.TrimSpace(defaultEnvironment)
		}

		return errorreportingsentry.New(errorreportingsentry.Config{
			DSN:         e.Sentry.DSN,
			Environment: environment,
			Release:     e.Sentry.Release,
			SampleRate:  e.Sentry.SampleRate,
			Debug:       e.Sentry.Debug,
		})

	default:
		return nil, fmt.Errorf("%s: unsupported error reporting provider: %q", op, e.Provider)
	}
}

// Metrics configures the business metrics provider.
//
// Provider selects the collector implementation. Prometheus scrape exposition
// is controlled separately by Prometheus.Enabled and runs on a dedicated HTTP
// server — never on the application REST listener.
type Metrics struct {
	// Provider selects the metrics collector implementation.
	Provider adaptermetrics.Provider `yaml:"provider"`

	// Noop contains configuration for the noop metrics provider.
	Noop Noop `yaml:"noop"`

	// Prometheus configures Prometheus metric collection and scrape exposition.
	Prometheus PrometheusMetrics `yaml:"prometheus"`
}

// PrometheusMetrics configures Prometheus metric collection and the dedicated
// scrape HTTP server.
//
// When Enabled is false the collector may still record in-process counters if
// the provider is prometheus, but no scrape listener is started. The scrape
// endpoint is internal-only; do not publish it on public ingress.
type PrometheusMetrics struct {
	// Enabled controls whether the dedicated Prometheus scrape HTTP server is started.
	Enabled bool `yaml:"enabled"`

	// Host is the Prometheus scrape HTTP listen hostname.
	Host string `yaml:"host"`

	// Port is the Prometheus scrape HTTP listen port.
	Port int `yaml:"port"`

	// Path is the HTTP path that exposes Prometheus metrics.
	Path string `yaml:"path"`
}

const (
	defaultMetricsHost = "0.0.0.0"
	defaultMetricsPort = 9090
)

// ServerEnabled reports whether the dedicated Prometheus scrape server should run.
func (m Metrics) ServerEnabled() bool {
	return m.Provider == adaptermetrics.PrometheusProvider && m.Prometheus.Enabled
}

// Validate checks metrics configuration for the selected provider.
func (m Metrics) Validate() error {
	const op = "config.Metrics.Validate"

	switch m.Provider {
	case "", adaptermetrics.NoopProvider:
		return nil

	case adaptermetrics.PrometheusProvider:
		return m.Prometheus.validate(m.ServerEnabled())

	default:
		return fmt.Errorf("%s: unsupported metrics provider: %q", op, m.Provider)
	}
}

func (p PrometheusMetrics) validate(serverEnabled bool) error {
	const op = "config.PrometheusMetrics.validate"

	if _, err := adaptermetrics.NormalizePath(p.Path); err != nil {
		return fmt.Errorf("%s: infrastructure.metrics.prometheus.path: %w", op, err)
	}

	if !serverEnabled {
		return nil
	}

	if err := validateMetricsHost(p.Host); err != nil {
		return fmt.Errorf("%s: infrastructure.metrics.prometheus.host: %w", op, err)
	}

	port := p.Port
	if port == 0 {
		port = defaultMetricsPort
	}

	if port < 1 || port > 65535 {
		return fmt.Errorf("%s: infrastructure.metrics.prometheus.port: invalid port %d", op, p.Port)
	}

	return nil
}

func validateMetricsHost(host string) error {
	const op = "config.validateMetricsHost"

	host = strings.TrimSpace(host)
	if host == "" {
		return nil
	}

	if strings.ContainsAny(host, ":/") {
		return fmt.Errorf("%s: must be a hostname or IP without port or scheme, got %q", op, host)
	}

	return nil
}

// getListeningAddress formats host and port for net/http.Server.Addr.
func (p PrometheusMetrics) getListeningAddress() string {
	host := strings.TrimSpace(p.Host)
	if host == "" {
		host = defaultMetricsHost
	}

	port := p.Port
	if port == 0 {
		port = defaultMetricsPort
	}

	return fmt.Sprintf("%s:%d", host, port)
}

// normalizedPath returns the configured scrape path or the default.
func (p PrometheusMetrics) normalizedPath() (string, error) {
	return adaptermetrics.NormalizePath(p.Path)
}

func (m Metrics) newMetricsInfrastructure() (coremetrics.RuntimeMetrics, http.Handler, error) {
	const op = "config.Metrics.newMetricsInfrastructure"

	if err := m.Validate(); err != nil {
		return nil, nil, err
	}

	switch m.Provider {
	case "", adaptermetrics.NoopProvider:
		return metricsnoop.New(), nil, nil

	case adaptermetrics.PrometheusProvider:
		business := prometheusmetrics.New()
		return business, business, nil

	default:
		return nil, nil, fmt.Errorf("%s: unsupported metrics provider: %q", op, m.Provider)
	}
}

// HTTPClient configures the shared outbound HTTP/HTTPS client.
//
// The first implementation phase intentionally exposes one default profile for
// all third-party HTTP adapters. Named per-client overrides can be added later
// without changing the standard client's runtime behavior.
type HTTPClient struct {
	// Provider selects the outbound HTTP implementation.
	//
	// Supported values:
	//   - noop
	//   - standard
	Provider adapterhttpclient.Provider `yaml:"provider"`

	// Noop contains configuration for the disabled provider.
	Noop Noop `yaml:"noop"`

	// Standard contains the single shared default profile used for all outbound
	// third-party HTTP/HTTPS calls in this phase.
	Standard HTTPClientStandard `yaml:"standard"`
}

// HTTPClientStandard contains bounded-concurrency and net/http transport
// settings for the standard provider.
type HTTPClientStandard struct {
	// MaxInFlight is the maximum number of requests that may execute
	// concurrently. This is the worker-pool-equivalent capacity, implemented as
	// a semaphore so net/http.Client and its connection pool can be reused.
	MaxInFlight int `yaml:"max_in_flight"`

	// MaxQueueSize is the maximum number of callers allowed to wait for an
	// execution slot. Zero means fail fast when MaxInFlight is exhausted.
	MaxQueueSize int `yaml:"max_queue_size"`

	// QueueWaitTimeout limits how long a caller may remain in the bounded queue.
	QueueWaitTimeout time.Duration `yaml:"queue_wait_timeout"`

	// Timeout is the end-to-end net/http.Client timeout.
	Timeout time.Duration `yaml:"timeout"`

	// Transport configures reusable connection-pool and protocol timeouts.
	Transport HTTPClientTransport `yaml:"transport"`
}

// HTTPClientTransport contains standard library http.Transport limits.
type HTTPClientTransport struct {
	// DialTimeout limits TCP connection establishment for a new connection.
	DialTimeout time.Duration `yaml:"dial_timeout"`

	// TLSHandshakeTimeout limits TLS negotiation for HTTPS connections.
	TLSHandshakeTimeout time.Duration `yaml:"tls_handshake_timeout"`

	// ResponseHeaderTimeout limits how long the transport waits for response
	// headers after the request has been written successfully.
	ResponseHeaderTimeout time.Duration `yaml:"response_header_timeout"`

	// IdleConnTimeout controls how long an unused keep-alive connection may
	// remain in the reusable transport pool.
	IdleConnTimeout time.Duration `yaml:"idle_conn_timeout"`

	// MaxIdleConns limits pooled idle keep-alive connections across all hosts.
	MaxIdleConns int `yaml:"max_idle_conns"`

	// MaxIdleConnsPerHost limits pooled idle keep-alive connections retained for
	// one external host.
	MaxIdleConnsPerHost int `yaml:"max_idle_conns_per_host"`

	// MaxConnsPerHost limits dialing, active, and idle transport connections for
	// one external host.
	MaxConnsPerHost int `yaml:"max_conns_per_host"`
}

// newHTTPClientInfrastructure creates the configured shared outbound HTTP
// client. Retry and circuit-breaker policies deliberately remain outside this
// infrastructure because they depend on each external API's semantics.
func (h HTTPClient) newHTTPClientInfrastructure(
	clk coreclock.Clock,
	logger corelog.Logger,
	metrics coremetrics.OutboundHTTPMetrics,
) (corehttpclient.Client, error) {
	const op = "config.HTTPClient.newHTTPClientInfrastructure"

	switch h.Provider {
	case adapterhttpclient.NoopProvider:
		return httpclientnoop.New(logger)

	case adapterhttpclient.StandardProvider:
		return httpclientstandard.New(
			httpclientstandard.Config{
				MaxInFlight:           h.Standard.MaxInFlight,
				MaxQueueSize:          h.Standard.MaxQueueSize,
				QueueWaitTimeout:      h.Standard.QueueWaitTimeout,
				Timeout:               h.Standard.Timeout,
				DialTimeout:           h.Standard.Transport.DialTimeout,
				TLSHandshakeTimeout:   h.Standard.Transport.TLSHandshakeTimeout,
				ResponseHeaderTimeout: h.Standard.Transport.ResponseHeaderTimeout,
				IdleConnTimeout:       h.Standard.Transport.IdleConnTimeout,
				MaxIdleConns:          h.Standard.Transport.MaxIdleConns,
				MaxIdleConnsPerHost:   h.Standard.Transport.MaxIdleConnsPerHost,
				MaxConnsPerHost:       h.Standard.Transport.MaxConnsPerHost,
			},
			clk,
			logger,
			metrics,
		)

	default:
		return nil, fmt.Errorf("%s: unsupported http client provider: %q", op, h.Provider)
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
func (c Cache) newCacheInfrastructure(ctx context.Context) (cache.Cache, error) {
	const op = "config.Cache.newCacheInfrastructure"

	switch c.Provider {
	case adaptercache.RedisProvider:
		return c.newRedisInfrastructure(ctx)

	case adaptercache.NoopProvider:
		return c.newNoopCacheInfrastructure()

	default:
		return nil, fmt.Errorf(
			"%s: unsupported cache provider: %q", op, c.Provider,
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
func (c Cache) newRedisInfrastructure(ctx context.Context) (cache.Cache, error) {
	const op = "config.Cache.newRedisInfrastructure"

	if c.Redis.Addr == "" {
		return nil, fmt.Errorf("%s: invalid cache.redis.addr: empty", op)
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

	pingCtx := ctx
	if c.Redis.DialTimeout > 0 {
		var cancel context.CancelFunc
		pingCtx, cancel = context.WithTimeout(ctx, c.Redis.DialTimeout)
		defer cancel()
	}

	if err := client.Ping(pingCtx).Err(); err != nil {
		closeErr := client.Close()
		return nil, errors.Join(
			fmt.Errorf("%s: connect redis cache %q: %w", op, c.Redis.Addr, err),
			closeErr,
		)
	}

	return redis2.New(client), nil
}

// ObjectStorage configures provider-neutral blob persistence.
type ObjectStorage struct {
	// Provider selects the storage implementation.
	Provider adapterobjectstorage.Provider `yaml:"provider"`

	// Noop keeps storage disabled while satisfying dependency wiring.
	Noop Noop `yaml:"noop"`

	// Filesystem stores immutable-style objects below one local directory and is
	// intended for local development/integration testing.
	Filesystem ObjectStorageFilesystem `yaml:"filesystem"`
}

// ObjectStorageFilesystem configures the local filesystem object store.
type ObjectStorageFilesystem struct {
	RootDir string `yaml:"root_dir"`
}

func (o ObjectStorage) newObjectStorageInfrastructure() (coreobjectstorage.Store, error) {
	const op = "config.ObjectStorage.newObjectStorageInfrastructure"

	switch o.Provider {
	case "", adapterobjectstorage.NoopProvider:
		return objectstoragenoop.New(), nil

	case adapterobjectstorage.FilesystemProvider:
		store, err := objectstoragefilesystem.New(o.Filesystem.RootDir)
		if err != nil {
			return nil, fmt.Errorf("%s: initialize filesystem provider: %w", op, err)
		}

		return store, nil

	default:
		return nil, fmt.Errorf("%s: unsupported object storage provider: %q", op, o.Provider)
	}
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

	// AuditEvents is the Kafka destination for BNPL audit-event outbox messages.
	AuditEvents string `yaml:"audit_events"`

	// PaymentApplicationResults is the Kafka destination for BNPL payment
	// application acknowledgement outbox messages.
	PaymentApplicationResults string `yaml:"payment_application_results"`

	// InstallmentPaymentRequests is the Kafka destination for BNPL installment
	// payment request outbox messages.
	InstallmentPaymentRequests string `yaml:"installment_payment_requests"`

	// InstallmentPaymentSessions is the Kafka topic for Financial Core installment
	// payment-session-created events consumed by BNPL.
	InstallmentPaymentSessions string `yaml:"installment_payment_sessions"`

	// InstallmentPaymentReconciliationRequests is the Kafka destination for BNPL
	// installment payment reconciliation request outbox messages.
	InstallmentPaymentReconciliationRequests string `yaml:"installment_payment_reconciliation_requests"`

	// InstallmentPaymentReconciliationResults is the Kafka topic for Financial Core
	// installment payment reconciliation result events consumed by BNPL.
	InstallmentPaymentReconciliationResults string `yaml:"installment_payment_reconciliation_results"`

	// InstallmentPaymentReconciliationAcknowledgements is the Kafka destination for BNPL
	// installment payment reconciliation acknowledgement outbox messages.
	InstallmentPaymentReconciliationAcknowledgements string `yaml:"installment_payment_reconciliation_acknowledgements"`

	// NotificationSMSRequests is the Kafka destination for BNPL repayment-reminder
	// (and other non-OTP) SMS notification request outbox messages.
	NotificationSMSRequests string `yaml:"notification_sms_requests"`
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
	Topics []string `yaml:"topics"`

	// InitialOffset is the transport-neutral policy ("oldest" | "newest") used
	// when a consumer group has no committed offset. Parsed via
	// messaging.ParseInitialOffset and mapped to broker constants by the Kafka
	// adapter.
	InitialOffset string `yaml:"initial_offset"`

	// AllowNewestInitialOffset must be true to use InitialOffset=newest outside
	// production. Production always rejects newest.
	AllowNewestInitialOffset bool `yaml:"allow_newest_initial_offset"`

	// RetryInitialBackoff configures the initial backoff before retrying a failed consume.
	RetryInitialBackoff time.Duration `yaml:"retry_initial_backoff"`

	// RetryMaxBackoff configures the maximum backoff between consume retries.
	RetryMaxBackoff time.Duration `yaml:"retry_max_backoff"`

	// QuarantineTopic stores transport/envelope failures that handlers explicitly
	// classify as non-retryable. It must not be one of the consumed source topics.
	QuarantineTopic string `yaml:"quarantine_topic"`
}

func (c Consumer) quarantineTopic(groupID string) string {
	if topic := strings.TrimSpace(c.QuarantineTopic); topic != "" {
		return topic
	}

	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return ""
	}

	return groupID + ".quarantine.v1"
}

// newPublisherInfrastructure creates the configured messaging publisher
// implementation.
func (m Messaging) newPublisherInfrastructure() (coremessaging.Publisher, error) {
	const op = "config.Messaging.newPublisherInfrastructure"

	switch m.Provider {
	case messaging.NoopProvider:
		return noopmessaging.NewPublisher(), nil

	case messaging.KafkaProvider:
		if err := m.validateKafkaTopics(); err != nil {
			return nil, err
		}

		return saramamessaging.NewProducer(
			m.Kafka.Brokers,
			m.Kafka.Producer.ReturnSuccesses,
			sarama.RequiredAcks(m.Kafka.Producer.RequiredAcks),
			m.Kafka.Producer.MaxRetries,
		)

	default:
		return nil, fmt.Errorf("%s: unsupported messaging provider: %q", op, m.Provider)
	}
}

func (m Messaging) validateKafkaTopics() error {
	const op = "config.Messaging.validateKafkaTopics"

	if strings.TrimSpace(m.Kafka.Topics.PaymentResults) == "" {
		return fmt.Errorf("%s: kafka topics payment_results is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.PaymentApplicationResults) == "" {
		return fmt.Errorf("%s: kafka topics payment_application_results is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.InstallmentPaymentRequests) == "" {
		return fmt.Errorf("%s: kafka topics installment_payment_requests is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.InstallmentPaymentSessions) == "" {
		return fmt.Errorf("%s: kafka topics installment_payment_sessions is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.InstallmentPaymentReconciliationRequests) == "" {
		return fmt.Errorf("%s: kafka topics installment_payment_reconciliation_requests is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.InstallmentPaymentReconciliationResults) == "" {
		return fmt.Errorf("%s: kafka topics installment_payment_reconciliation_results is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.NotificationSMSRequests) == "" {
		return fmt.Errorf("%s: kafka topics notification_sms_requests is required when messaging provider is kafka", op)
	}

	if strings.TrimSpace(m.Kafka.Topics.InstallmentPaymentReconciliationAcknowledgements) == "" {
		return fmt.Errorf("%s: kafka topics installment_payment_reconciliation_acknowledgements is required when messaging provider is kafka", op)
	}

	return nil
}

// newSubscriberInfrastructure creates the configured messaging subscriber
// implementation.
func (m Messaging) newSubscriberInfrastructure(
	logger corelog.Logger,
	publisher coremessaging.Publisher,
) (coremessaging.Subscriber, error) {
	return m.newSubscriberInfrastructureWithConsumer(logger, publisher, nil)
}

func (m Messaging) newSubscriberInfrastructureWithConsumer(
	logger corelog.Logger,
	publisher coremessaging.Publisher,
	consumerHealth corehealth.ConsumerStateWriter,
) (coremessaging.Subscriber, error) {
	const op = "config.Messaging.newSubscriberInfrastructureWithConsumer"

	switch m.Provider {
	case messaging.NoopProvider:
		return noopmessaging.NewSubscriberWithHealth(consumerHealth), nil

	case messaging.KafkaProvider:
		if err := m.validateKafkaTopics(); err != nil {
			return nil, err
		}

		initialOffset, err := messaging.ParseInitialOffset(m.Kafka.Consumer.InitialOffset)
		if err != nil {
			return nil, err
		}

		if m.Kafka.Consumer.RetryInitialBackoff <= 0 {
			return nil, fmt.Errorf("%s: kafka consumer retry_initial_backoff must be positive", op)
		}

		if m.Kafka.Consumer.RetryMaxBackoff < m.Kafka.Consumer.RetryInitialBackoff {
			return nil, fmt.Errorf("%s: kafka consumer retry_max_backoff must be greater than or equal to retry_initial_backoff", op)
		}

		return saramamessaging.NewConsumer(
			m.Kafka.Brokers,
			m.Kafka.GroupID,
			m.Kafka.Consumer.Topics,
			logger,
			saramamessaging.ConsumerConfig{
				InitialOffset:       initialOffset,
				RetryInitialBackoff: m.Kafka.Consumer.RetryInitialBackoff,
				RetryMaxBackoff:     m.Kafka.Consumer.RetryMaxBackoff,
				ConsumerHealth:      consumerHealth,
				QuarantineTopic:     m.Kafka.Consumer.quarantineTopic(m.Kafka.GroupID),
				QuarantinePublisher: publisher,
			},
		)

	default:
		return nil, fmt.Errorf("%s: unsupported messaging provider: %q", op, m.Provider)
	}
}

// Task configures the task runtime and startup-known scheduled tasks.
type Task struct {
	// Provider selects the task runtime implementation.
	//
	// Supported values:
	//   - noop
	//   - inprocess
	//   - temporal
	//   - composite
	Provider taskadapter.Provider `yaml:"provider"`

	// Noop contains configuration for the noop task runtime.
	Noop Noop `yaml:"noop"`

	// InProcess contains configuration for the in-process task runtime.
	InProcess InProcess `yaml:"inprocess"`

	// Temporal contains configuration for the Temporal task runtime.
	Temporal Temporal `yaml:"temporal"`

	// Composite exists as an explicit configuration node for schema clarity.
	// The composite runtime is intentionally fixed to in-process + Temporal and
	// therefore has no provider list of its own.
	Composite Composite `yaml:"composite"`

	// Scheduled contains tasks known at application startup.
	Scheduled ScheduledTasks `yaml:"scheduled"`
}

// InProcess configures the in-process task runtime.
type InProcess struct {
	// DefaultTimezone is used for cron triggers without task-specific timezone.
	DefaultTimezone string `yaml:"default_timezone"`

	// MaxConcurrentRuns bounds scheduled and manually dispatched work in one process.
	// Zero selects the runtime default.
	MaxConcurrentRuns int `yaml:"max_concurrent_runs"`
}

// Temporal configures the Temporal client, worker, and generic task Activity.
type Temporal struct {
	// Address is the Temporal frontend gRPC endpoint.
	Address string `yaml:"address"`

	// Namespace is the Temporal namespace used by BNPL task workflows.
	Namespace string `yaml:"namespace"`

	// TaskQueue is shared by the generic task Workflow and Activities.
	TaskQueue string `yaml:"task_queue"`

	// ActivityStartToCloseTimeout is the infrastructure safety bound applied to
	// generic task Activities. Existing jobs retain ownership of retry policy.
	ActivityStartToCloseTimeout time.Duration `yaml:"activity_start_to_close_timeout"`

	// Worker configures Temporal worker concurrency.
	Worker TemporalWorker `yaml:"worker"`

	// TLS configures optional TLS for the Temporal service connection.
	TLS TemporalTLS `yaml:"tls"`
}

// TemporalWorker configures worker concurrency limits. Zero delegates to SDK defaults.
type TemporalWorker struct {
	MaxConcurrentActivities    int `yaml:"max_concurrent_activities"`
	MaxConcurrentWorkflowTasks int `yaml:"max_concurrent_workflow_tasks"`
}

// TemporalTLS configures TLS for the Temporal client connection.
type TemporalTLS struct {
	Enabled    bool   `yaml:"enabled"`
	ServerName string `yaml:"server_name"`
}

// Composite marks the intentionally fixed in-process + Temporal runtime.
type Composite struct{}

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

	// InstallmentOfferExpiry expires offered checkout installment offers.
	InstallmentOfferExpiry TaskDefinition `yaml:"installment_offer_expiry"`

	// ContractPaymentPendingExpiry expires payment_pending contracts.
	ContractPaymentPendingExpiry TaskDefinition `yaml:"contract_payment_pending_expiry"`

	// CheckoutSessionExpiry expires due open checkout sessions.
	CheckoutSessionExpiry TaskDefinition `yaml:"checkout_session_expiry"`

	// CheckoutSessionConfirmingRecovery releases stale unlinked confirming claims.
	CheckoutSessionConfirmingRecovery TaskDefinition `yaml:"checkout_session_confirming_recovery"`

	// MerchantTerminalIssuanceScrub clears ciphertext from expired issuance
	// deliveries, leaving idempotency tombstones.
	MerchantTerminalIssuanceScrub TaskDefinition `yaml:"merchant_terminal_issuance_scrub"`

	// SagaExecutor resumes due durable Saga instances.
	SagaExecutor TaskDefinition `yaml:"saga_executor"`

	// InstallmentMarkDue transitions scheduled installments to due.
	InstallmentMarkDue TaskDefinition `yaml:"installment_mark_due"`

	// InstallmentMarkOverdue transitions due installments to overdue.
	InstallmentMarkOverdue TaskDefinition `yaml:"installment_mark_overdue"`

	// PaymentAttemptReconciliation schedules payment attempt reconciliation work.
	PaymentAttemptReconciliation TaskDefinition `yaml:"payment_attempt_reconciliation"`

	// RepaymentReminderDispatch evaluates due schedules and publishes reminder SMS requests.
	RepaymentReminderDispatch TaskDefinition `yaml:"repayment_reminder_dispatch"`
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
	// Enabled determines whether this task is active for execution. Disabled
	// definitions may still be passed to runtimes for durable-state cleanup.
	Enabled bool `yaml:"enabled"`

	// Execution selects the concrete backend when provider=composite. It must
	// be empty for standalone providers.
	Execution task.ExecutionMode `yaml:"execution"`

	// Handler is a stable provider-agnostic task identifier.
	Handler string `yaml:"handler"`

	// Trigger describes when this task should be executed.
	Trigger TaskTrigger `yaml:"trigger"`
}

// TaskTrigger describes when a configured task should run.
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
	const op = "config.TaskTrigger.ToCore"

	trigger := task.Trigger{
		Type:       t.Type,
		Cron:       t.Cron,
		Timezone:   t.Timezone,
		RunOnStart: t.RunOnStart,
	}

	if t.Interval != "" {
		interval, err := time.ParseDuration(t.Interval)
		if err != nil {
			return task.Trigger{}, fmt.Errorf("%s: parse task interval %q: %w", op, t.Interval, err)
		}

		trigger.Interval = interval
	}

	return trigger, nil
}

// newTaskInfrastructure creates the configured task runtime.
func (t Task) newTaskInfrastructure(
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (task.Runtime, error) {
	const op = "config.Task.newTaskInfrastructure"

	switch t.Provider {
	case taskadapter.NoopProvider:
		return t.newNoopTaskInfrastructure(logger)

	case taskadapter.InProcessProvider:
		return t.newInProcessTaskInfrastructure(logger, reporter)

	case taskadapter.TemporalProvider:
		return t.newTemporalTaskInfrastructure(logger)

	case taskadapter.CompositeProvider:
		return t.newCompositeTaskInfrastructure(logger, reporter)

	default:
		return nil, fmt.Errorf("%s: unsupported task provider: %q", op, t.Provider)
	}
}

// newNoopTaskInfrastructure creates a no-op task runtime.
func (t Task) newNoopTaskInfrastructure(logger corelog.Logger) (task.Runtime, error) {
	return tasknoop.New(logger), nil
}

// newInProcessTaskInfrastructure creates an in-process task runtime.
func (t Task) newInProcessTaskInfrastructure(
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (task.Runtime, error) {
	const op = "config.Task.newInProcessTaskInfrastructure"

	if t.InProcess.MaxConcurrentRuns < 0 {
		return nil, fmt.Errorf("%s: task.inprocess.max_concurrent_runs must not be negative", op)
	}

	return inprocess.New(
		inprocess.Config{
			DefaultTimezone:   t.InProcess.DefaultTimezone,
			MaxConcurrentRuns: t.InProcess.MaxConcurrentRuns,
		},
		logger,
		reporter,
	)
}

// newTemporalTaskInfrastructure creates a Temporal-backed task runtime.
func (t Task) newTemporalTaskInfrastructure(logger corelog.Logger) (task.Runtime, error) {
	return tasktemporal.New(
		tasktemporal.Config{
			Address:                     t.Temporal.Address,
			Namespace:                   t.Temporal.Namespace,
			TaskQueue:                   t.Temporal.TaskQueue,
			ActivityStartToCloseTimeout: t.Temporal.ActivityStartToCloseTimeout,
			MaxConcurrentActivities:     t.Temporal.Worker.MaxConcurrentActivities,
			MaxConcurrentWorkflowTasks:  t.Temporal.Worker.MaxConcurrentWorkflowTasks,
			TLS: tasktemporal.TLSConfig{
				Enabled:    t.Temporal.TLS.Enabled,
				ServerName: t.Temporal.TLS.ServerName,
			},
		},
		logger,
	)
}

// newCompositeTaskInfrastructure creates the intentionally fixed
// in-process + Temporal composite runtime.
func (t Task) newCompositeTaskInfrastructure(
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (task.Runtime, error) {
	inProcessRuntime, err := t.newInProcessTaskInfrastructure(logger, reporter)
	if err != nil {
		return nil, err
	}

	temporalRuntime, err := t.newTemporalTaskInfrastructure(logger)
	if err != nil {
		return nil, err
	}

	runtime, err := composite.New(inProcessRuntime, temporalRuntime)
	if err != nil {
		if closer, ok := temporalRuntime.(interface{ Close() error }); ok {
			_ = closer.Close()
		}

		return nil, err
	}

	return runtime, nil
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
		{
			TaskDefinition: t.Scheduled.InstallmentOfferExpiry,
			Name:           "installment_offer_expiry",
		},
		{
			TaskDefinition: t.Scheduled.ContractPaymentPendingExpiry,
			Name:           "contract_payment_pending_expiry",
		},
		{
			TaskDefinition: t.Scheduled.CheckoutSessionExpiry,
			Name:           "checkout_session_expiry",
		},
		{
			TaskDefinition: t.Scheduled.CheckoutSessionConfirmingRecovery,
			Name:           "checkout_session_confirming_recovery",
		},
		{
			TaskDefinition: t.Scheduled.MerchantTerminalIssuanceScrub,
			Name:           "merchant_terminal_issuance_scrub",
		},
		{
			TaskDefinition: t.Scheduled.SagaExecutor,
			Name:           "saga_executor",
		},
		{
			TaskDefinition: t.Scheduled.InstallmentMarkDue,
			Name:           "installment_mark_due",
		},
		{
			TaskDefinition: t.Scheduled.InstallmentMarkOverdue,
			Name:           "installment_mark_overdue",
		},
		{
			TaskDefinition: t.Scheduled.PaymentAttemptReconciliation,
			Name:           "payment_attempt_reconciliation",
		},
		{
			TaskDefinition: t.Scheduled.RepaymentReminderDispatch,
			Name:           "repayment_reminder_dispatch",
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
	const op = "config.CircuitBreaker.newCircuitBreakerInfrastructure"

	switch cb.Provider {
	case circuitbreaker.NoopProvider:
		return cb.newNoopCircuitBreakerInfrastructure()

	case circuitbreaker.GoBreakerProvider:
		return cb.newGoBreakerCircuitBreakerInfrastructure()

	default:
		return nil, fmt.Errorf("%s: unsupported circuit breaker provider: %q", op, cb.Provider)
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

const (
	defaultShutdownTimeout    = 30 * time.Second
	defaultShutdownDrainDelay = 5 * time.Second
)

// Shutdown configures graceful process shutdown behavior.
type Shutdown struct {
	// Timeout limits how long the entire graceful shutdown sequence may take
	// before the process is considered unhealthy.
	Timeout time.Duration `yaml:"timeout"`

	// DrainDelay waits after the application starts rejecting new work and
	// before shutting down the HTTP server, giving external load balancers time
	// to stop routing new requests.
	DrainDelay time.Duration `yaml:"drain_delay"`
}

// getShutdownTimeout returns the configured shutdown timeout or the default.
func (s Shutdown) getShutdownTimeout() time.Duration {
	if s.Timeout <= 0 {
		return defaultShutdownTimeout
	}

	return s.Timeout
}

// getShutdownDrainDelay returns the configured drain delay.
//
// A zero value means no drain wait. This must not fall back to the conventional
// 5s default: env-expanded config sets drain_delay to 0s intentionally in
// tests and local runs that do not front a load balancer. The 5s default lives
// in .env.example / deployment config instead.
func (s Shutdown) getShutdownDrainDelay() time.Duration {
	if s.DrainDelay < 0 {
		return defaultShutdownDrainDelay
	}

	return s.DrainDelay
}

// HealthCheck configures application health checks.
type HealthCheck struct {
	// Checks defines which runtime dependencies are included in readiness.
	//
	// Supported values:
	//   - database
	//   - cache
	//   - messaging
	Checks []health.Check `yaml:"checks"`
}

// newDependencyCheckerInfrastructure constructs a composite dependency checker from the
// configured health checks.
//
// Only the dependency checks listed in the configuration are included in the
// resulting checker. The returned checker reports the application as ready only
// when all configured dependencies are healthy.
func (hc HealthCheck) newDependencyCheckerInfrastructure(
	pool repo.Pool,
	healthRepo repo.Health,
	cache cache.Cache,
	messagingHealth coremessaging.Health,
) (corehealth.DependencyChecker, error) {
	return hc.newDependencyCheckerInfrastructureWithConsumer(
		pool,
		healthRepo,
		cache,
		messagingHealth,
		nil,
	)
}

func (hc HealthCheck) newDependencyCheckerInfrastructureWithConsumer(
	pool repo.Pool,
	healthRepo repo.Health,
	cache cache.Cache,
	messagingHealth coremessaging.Health,
	consumerHealth corehealth.ConsumerHealth,
) (corehealth.DependencyChecker, error) {
	const op = "config.HealthCheck.newDependencyCheckerInfrastructureWithConsumer"

	checkers := make([]corehealth.DependencyChecker, 0, len(hc.Checks))

	for _, check := range hc.Checks {
		switch check {
		case health.DatabaseCheck:
			checker, err := health.NewDatabaseDependencyChecker(pool, healthRepo)
			if err != nil {
				return nil, fmt.Errorf("%s: database dependency checker: %w", op, err)
			}

			checkers = append(checkers, checker)

		case health.CacheCheck:
			checkers = append(checkers, health.NewRedisDependencyChecker(cache))

		case health.MessagingCheck:
			checkers = append(checkers, health.NewMessagingDependencyChecker(messagingHealth))

		case health.ConsumerCheck:
			checkers = append(checkers, health.NewConsumerDependencyChecker(consumerHealth))

		default:
			return nil, fmt.Errorf("%s: unknown health check %q", op, check)
		}
	}

	return health.NewCompositeDependencyChecker(checkers...), nil
}

// newHealthInfrastructure constructs the messaging health checker for the
// configured messaging provider.
func (m Messaging) newHealthInfrastructure() (coremessaging.Health, error) {
	const op = "config.Messaging.newHealthInfrastructure"

	switch m.Provider {
	case messaging.NoopProvider:
		return noopmessaging.NewHealthChecker(), nil

	case messaging.KafkaProvider:
		client, err := sarama.NewClient(m.Kafka.Brokers, sarama.NewConfig())
		if err != nil {
			return nil, fmt.Errorf("%s: create kafka health client: %w", op, err)
		}

		return saramamessaging.NewHealthChecker(client), nil

	default:
		return nil, fmt.Errorf("%s: unknown messaging provider %q", op, m.Provider)
	}
}
