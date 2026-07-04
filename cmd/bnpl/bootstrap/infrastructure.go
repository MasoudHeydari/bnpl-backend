package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	adapterhealth "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
	"github.com/parstis/bnpl-backend/pkg/core/cache"
	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
	"github.com/parstis/bnpl-backend/pkg/core/clock"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/fingerprint"
	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/identifier"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	"github.com/parstis/bnpl-backend/pkg/core/metrics"
	"github.com/parstis/bnpl-backend/pkg/core/objectstorage"
	"github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Infrastructure holds the runtime adapters assembled for one App.
//
// Values are core-facing ports (logger, cache, messaging, metrics, tasks).
// Concrete providers are selected from configuration during NewInfrastructures.
type Infrastructure struct {
	// DependencyChecker aggregates readiness/liveness dependency probes.
	DependencyChecker corehealth.DependencyChecker

	// Trace issues correlation/request identifiers used across the process.
	Trace trace.Provider

	// IdentifierGenerator creates opaque public identifiers (currently the
	// same Trace provider).
	IdentifierGenerator identifier.Generator

	// FingerprintHasher builds stable request fingerprints for idempotency.
	FingerprintHasher fingerprint.Hasher

	// Clock provides wall-clock and business-timezone time for use cases.
	Clock clock.Clock

	// Logger is the process-wide structured logger.
	Logger log.Logger

	// ErrorReporter reports unexpected operational failures to the configured
	// external observability backend. It is not a readiness dependency.
	ErrorReporter coreerrorreporting.Reporter

	// Metrics is the configured runtime metrics provider. Call sites select the
	// narrow BusinessMetrics or OutboundHTTPMetrics view they need.
	Metrics metrics.RuntimeMetrics

	// HTTPClient is the shared outbound HTTP/HTTPS execution port implemented by
	// the configured infrastructure provider.
	HTTPClient corehttpclient.Client

	// MetricsHandler serves Prometheus text exposition for the dedicated
	// metrics HTTP server. It is nil when the noop provider is selected.
	MetricsHandler http.Handler

	// Cache is the shared cache port used by readiness and optional adapters.
	Cache cache.Cache

	// ObjectStorage is the provider-neutral blob store used by archival and
	// forensic infrastructure. It is not a readiness dependency for the main API.
	ObjectStorage objectstorage.Store

	// Publisher publishes durable outbox and other outbound messages.
	Publisher messaging.Publisher

	// Subscriber consumes inbound Kafka (or noop) messages.
	Subscriber messaging.Subscriber

	// MessagingHealth reports broker/consumer connectivity for readiness.
	MessagingHealth messaging.Health

	// LifecycleState tracks process drain/shutdown for health endpoints.
	LifecycleState corehealth.LifecycleState

	// ConsumerState tracks Kafka consumer liveness for readiness probes.
	ConsumerState corehealth.ConsumerStateWriter

	// CircuitBreaker protects outbound external calls.
	CircuitBreaker circuitbreaker.CircuitBreaker

	// Task is the scheduled/background task runtime.
	Task task.Runtime
}

// Close releases resources owned by initialized infrastructure adapters.
//
// Prefer App.Close / releaseResources for process lifecycle. This method exists
// so NewInfrastructures can roll back a partially constructed adapter set
// before the App owns the pool and dependency checker.
func (i *Infrastructure) Close() error {
	var err error
	err = errors.Join(err, i.closeHTTPClient())
	err = errors.Join(err, i.closePublisher())
	err = errors.Join(err, i.closeSubscriber())
	err = errors.Join(err, i.closeMessagingHealth())
	err = errors.Join(err, i.closeCache())
	err = errors.Join(err, i.closeObjectStorage())
	err = errors.Join(err, i.closeTask())
	err = errors.Join(err, i.closeErrorReporter())

	return err
}

func (i *Infrastructure) closeHTTPClient() error {
	if i == nil || i.HTTPClient == nil {
		return nil
	}

	err := i.HTTPClient.Close()
	i.HTTPClient = nil

	return err
}

func (i *Infrastructure) closePublisher() error {
	if i == nil || i.Publisher == nil {
		return nil
	}

	err := i.Publisher.Close()
	i.Publisher = nil

	return err
}

func (i *Infrastructure) closeSubscriber() error {
	if i == nil || i.Subscriber == nil {
		return nil
	}

	err := i.Subscriber.Close()
	i.Subscriber = nil

	return err
}

func (i *Infrastructure) closeMessagingHealth() error {
	if i == nil || i.MessagingHealth == nil {
		return nil
	}

	err := i.MessagingHealth.Close()
	i.MessagingHealth = nil

	return err
}

func (i *Infrastructure) closeErrorReporter() error {
	if i == nil || i.ErrorReporter == nil {
		return nil
	}

	err := i.ErrorReporter.Close()
	i.ErrorReporter = nil

	return err
}

func (i *Infrastructure) closeCache() error {
	if i == nil || i.Cache == nil {
		return nil
	}

	err := i.Cache.Close()
	i.Cache = nil

	return err
}

func (i *Infrastructure) closeObjectStorage() error {
	if i == nil || i.ObjectStorage == nil {
		return nil
	}

	err := i.ObjectStorage.Close()
	i.ObjectStorage = nil

	return err
}

func (i *Infrastructure) closeTask() error {
	if i == nil || i.Task == nil {
		return nil
	}

	err := i.Task.Close()
	i.Task = nil

	return err
}

// NewInfrastructures constructs the infrastructure adapters required by App.
//
// Construction order is intentional: Trace/Clock/Logger first, then error
// reporting and Metrics so the shared outbound HTTP client can observe queue
// and saturation signals.
// Messaging and task adapters come last. The process logger created here is
// reused by later bootstrap steps such as ConnectionPool query logging.
//
// DependencyChecker is intentionally not constructed here: it requires the
// live database pool. Call wireDependencyChecker after ConnectionPool succeeds.
func (a *App) NewInfrastructures(ctx context.Context) (_ Infrastructure, err error) {
	const op = "bootstrap.App.NewInfrastructures"

	infra := Infrastructure{
		LifecycleState: adapterhealth.NewLifecycleState(),
		ConsumerState:  adapterhealth.NewConsumerState(),
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, infra.Close())
		}
	}()

	infra.Trace, err = a.Config.NewTraceInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize trace: %w", op, err)
	}

	infra.IdentifierGenerator = infra.Trace
	infra.FingerprintHasher = sha256fingerprint.New()

	infra.Clock, err = a.Config.NewSystemClockInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize system clock: %w", op, err)
	}

	infra.Logger, err = a.Config.NewLoggerInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize logger: %w", op, err)
	}

	infra.ErrorReporter, err = a.Config.NewErrorReporterInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize error reporter: %w", op, err)
	}

	runtimeMetrics, metricsHandler, err := a.Config.NewMetricsInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize metrics: %w", op, err)
	}

	infra.Metrics = runtimeMetrics
	infra.MetricsHandler = metricsHandler

	infra.HTTPClient, err = a.Config.NewHTTPClientInfrastructure(
		infra.Clock,
		infra.Logger,
		infra.Metrics,
	)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize outbound http client: %w", op, err)
	}

	infra.Cache, err = a.Config.NewCacheInfrastructure(ctx)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize cache: %w", op, err)
	}

	infra.ObjectStorage, err = a.Config.NewObjectStorageInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize object storage: %w", op, err)
	}

	infra.Publisher, err = a.Config.NewMessagingPublisherInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize messaging publisher: %w", op, err)
	}

	infra.Subscriber, err = a.Config.NewMessagingSubscriberInfrastructure(
		infra.Logger,
		infra.Publisher,
		infra.ConsumerState,
	)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize messaging subscriber: %w", op, err)
	}

	infra.MessagingHealth, err = a.Config.NewMessagingHealthInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize messaging health: %w", op, err)
	}

	infra.CircuitBreaker, err = a.Config.NewCircuitBreakerInfrastructure()
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize circuit breaker: %w", op, err)
	}

	infra.Task, err = a.Config.NewTaskInfrastructure(infra.Logger, infra.ErrorReporter)
	if err != nil {
		return Infrastructure{}, fmt.Errorf("%s: initialize task: %w", op, err)
	}

	return infra, nil
}

// wireDependencyChecker constructs readiness dependency probes against the
// live pool and messaging/cache adapters already held on App.
//
// Must run after Pool is assigned. Building this earlier left DatabaseDependencyChecker
// with a nil pool, which treated the database as healthy and defeated readiness.
func (a *App) wireDependencyChecker() error {
	const op = "bootstrap.App.wireDependencyChecker"

	checker, err := a.Config.NewDependencyCheckerInfrastructureWithConsumer(
		a.Pool,
		a.Repos.Health,
		a.Infrastructure.Cache,
		a.Infrastructure.MessagingHealth,
		a.Infrastructure.ConsumerState,
	)
	if err != nil {
		return fmt.Errorf("%s: initialize dependency checker: %w", op, err)
	}

	a.Infrastructure.DependencyChecker = checker

	return nil
}
