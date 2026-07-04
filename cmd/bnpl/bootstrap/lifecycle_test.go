package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	cachenoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	errorreportingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting/noop"
	adapterhealth "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/health"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	messagingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/noop"
	tasknoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/noop"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
	coremock "github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
)

// lifecycle_test.go covers process lifecycle contracts that shutdown_test.go
// does not: DependencyChecker wiring after the DB pool exists, shared-resource
// Close ownership/idempotency, graceful-shutdown ordering, and classifying
// expected stop causes as non-fatal.
//
// These are unit tests with fakes/mocks — they do not bind real HTTP ports or
// run Boot/Run end-to-end.

// stubHTTPClient is a closable outbound HTTP client double used to assert that
// App.Close / shutdown release the shared HTTP client.
type stubHTTPClient struct {
	closed  atomic.Bool
	onClose func()
}

func (c *stubHTTPClient) Do(*http.Request) (*http.Response, error) {
	return nil, corehttpclient.ErrDisabled
}

func (c *stubHTTPClient) Close() error {
	if c.onClose != nil {
		c.onClose()
	}

	c.closed.Store(true)

	return nil
}

type bootstrapPanicReporter struct {
	panics chan any
}

func (*bootstrapPanicReporter) CaptureError(context.Context, error, coreerrorreporting.Metadata) {}

func (r *bootstrapPanicReporter) CapturePanic(
	_ context.Context,
	recovered any,
	_ coreerrorreporting.Metadata,
) {
	select {
	case r.panics <- recovered:

	default:
	}
}

func (*bootstrapPanicReporter) Flush(context.Context) bool { return true }
func (*bootstrapPanicReporter) Close() error               { return nil }

// TestWireDependencyChecker_UsesLivePool ensures readiness database checks are
// wired only after Pool exists, and that Check() actually pings through that
// live pool (the old startup bug left DependencyChecker with a nil pool).
func TestWireDependencyChecker_UsesLivePool(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Infrastructure.HealthCheck.Checks = []adapterhealth.Check{adapterhealth.DatabaseCheck}

	app := &App{
		Config: cfg,
		Repos:  NewRepos(),
	}

	infra, err := app.NewInfrastructures(context.Background())
	require.NoError(t, err)

	app.Infrastructure = infra
	require.Nil(t, app.Infrastructure.DependencyChecker)

	pool := coremock.NewPool(t)
	healthRepo := coremock.NewRepoHealth(t)
	conn := coremock.NewConn(t)
	healthConn := coremock.NewHealthConnQueryer(t)

	pool.EXPECT().Conn(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	healthRepo.EXPECT().Conn(conn).Return(healthConn).Once()
	healthConn.EXPECT().Ping(mock.Anything).Return(nil).Once()

	app.Pool = pool
	app.Repos.Health = healthRepo

	require.NoError(t, app.wireDependencyChecker())
	require.NotNil(t, app.Infrastructure.DependencyChecker)
	require.NoError(t, app.Infrastructure.DependencyChecker.Check(context.Background()))
}

// TestClose_IsIdempotent verifies App.Close releases shared dependencies once
// (pool, messaging, cache, outbound HTTP client) and a second Close is a no-op
// so startup rollback and graceful shutdown can share the same ownership path.
func TestClose_IsIdempotent(t *testing.T) {
	t.Parallel()

	pool := coremock.NewPool(t)
	pool.EXPECT().Close().Return(nil).Once()

	publisher := coremock.NewPublisher(t)
	publisher.EXPECT().Close().Return(nil).Once()

	subscriber := coremock.NewSubscriber(t)
	subscriber.EXPECT().Close().Return(nil).Once()

	cache := coremock.NewCache(t)
	cache.EXPECT().Close().Return(nil).Once()

	httpClient := &stubHTTPClient{}

	app := &App{
		Pool: pool,
		Infrastructure: Infrastructure{
			Logger:     lognoop.New(),
			HTTPClient: httpClient,
			Cache:      cache,
			Publisher:  publisher,
			Subscriber: subscriber,
			Task:       tasknoop.New(lognoop.New()),
		},
	}

	require.NoError(t, app.Close())
	require.NoError(t, app.Close())
	require.Nil(t, app.Pool)
	require.Nil(t, app.Infrastructure.HTTPClient)
	require.Nil(t, app.Infrastructure.Task)
	require.True(t, httpClient.closed.Load())
}

// TestShutdown_OrderDrainingHTTPWorkersThenDependencies locks the graceful
// shutdown sequence: after draining is marked, HTTP stops before workers are
// canceled, and shared dependencies close afterward with the database last so
// in-flight HTTP work never loses its dependencies mid-flight.
func TestShutdown_OrderDrainingHTTPWorkersThenDependencies(t *testing.T) {
	t.Parallel()

	var (
		mu    sync.Mutex
		steps []string
	)
	track := func(name string) {
		mu.Lock()
		steps = append(steps, name)
		mu.Unlock()
	}

	lifecycle := adapterhealth.NewLifecycleState()
	httpClient := &stubHTTPClient{onClose: func() { track("http_client") }}

	publisher := coremock.NewPublisher(t)
	publisher.EXPECT().Close().Run(func() { track("publisher") }).Return(nil).Once()

	subscriber := coremock.NewSubscriber(t)
	subscriber.EXPECT().Close().Run(func() { track("subscriber") }).Return(nil).Once()

	cache := coremock.NewCache(t)
	cache.EXPECT().Close().Run(func() { track("cache") }).Return(nil).Once()

	pool := coremock.NewPool(t)
	pool.EXPECT().Close().Run(func() { track("database") }).Return(nil).Once()

	taskRuntime := &trackingTaskRuntime{onClose: func() { track("task") }}

	taskDone := make(chan struct{})
	consumerDone := make(chan struct{})
	botMessagingDone := make(chan struct{})
	workers := &backgroundWorkers{
		taskDone:         taskDone,
		consumerDone:     consumerDone,
		botMessagingDone: botMessagingDone,
		fatalErr:         make(chan error, 1),
	}

	server := &orderedHTTPServer{
		onShutdown: func() {
			track("http")
			require.True(t, lifecycle.IsDraining(), "readiness must fail before HTTP shutdown")
		},
	}

	cfg := noopTestConfig()
	cfg.Infrastructure.Shutdown.DrainDelay = time.Millisecond

	app := &App{
		Config: cfg,
		Pool:   pool,
		Infrastructure: Infrastructure{
			Logger:         lognoop.New(),
			LifecycleState: lifecycle,
			HTTPClient:     httpClient,
			Cache:          cache,
			Publisher:      publisher,
			Subscriber:     subscriber,
			Task:           taskRuntime,
		},
	}

	lifecycle.StartDraining()
	track("draining")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := app.shutdown(ctx, server, nil, workers, func() {
		track("stop_workers")
		close(taskDone)
		close(consumerDone)
		close(botMessagingDone)
	})
	require.NoError(t, err)

	mu.Lock()
	got := append([]string(nil), steps...)
	mu.Unlock()

	require.Equal(t, []string{
		"draining",
		"http",
		"stop_workers",
		"http_client",
		"publisher",
		"subscriber",
		"cache",
		"task",
		"database",
	}, got)
}

// TestShutdown_ClosesHTTPClientAndContinuesAfterComponentError checks that
// shutdown still closes the outbound HTTP client and remaining dependents
// (including the DB pool) when one component Close fails, and joins that
// failure into the returned error.
func TestShutdown_ClosesHTTPClientAndContinuesAfterComponentError(t *testing.T) {
	t.Parallel()

	publisherErr := errors.New("publisher close failed")
	httpClient := &stubHTTPClient{}

	publisher := coremock.NewPublisher(t)
	publisher.EXPECT().Close().Return(publisherErr).Once()

	subscriber := coremock.NewSubscriber(t)
	subscriber.EXPECT().Close().Return(nil).Once()

	cache := coremock.NewCache(t)
	cache.EXPECT().Close().Return(nil).Once()

	pool := coremock.NewPool(t)
	pool.EXPECT().Close().Return(nil).Once()

	app := &App{
		Pool: pool,
		Infrastructure: Infrastructure{
			Logger:     lognoop.New(),
			HTTPClient: httpClient,
			Cache:      cache,
			Publisher:  publisher,
			Subscriber: subscriber,
			Task:       tasknoop.New(lognoop.New()),
		},
	}

	err := app.shutdown(
		context.Background(),
		newRecordingHTTPServer(),
		nil,
		closedBackgroundWorkers(),
		func() {},
	)
	require.ErrorIs(t, err, publisherErr)
	require.True(t, httpClient.closed.Load())
	require.Nil(t, app.Infrastructure.HTTPClient)
	require.Nil(t, app.Pool)
}

// TestWaitForStop_ErrServerClosedIsNotFatal ensures a normal HTTP listener
// shutdown result (http.ErrServerClosed) does not become a fatal process
// stopErr — only unexpected listen failures should.
func TestWaitForStop_ErrServerClosedIsNotFatal(t *testing.T) {
	t.Parallel()

	listenErr := make(chan error, 1)
	listenErr <- http.ErrServerClosed

	stopErr, _, stopped, _, _ := waitForStop(context.Background(), listenErr, nil, make(chan error))
	require.True(t, stopped)
	require.NoError(t, stopErr)
}

func TestStartBackgroundWorkers_TaskPanicIsRecoveredAndDoesNotStopProcess(t *testing.T) {
	t.Parallel()

	runtime := coremock.NewRuntime(t)
	runtime.EXPECT().Start(mock.Anything).Run(func(context.Context) {
		panic("task-runtime-boom")
	}).Once()

	subscriber := coremock.NewSubscriber(t)
	subscriber.EXPECT().Subscribe(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ messaging.Handler) error {
			<-ctx.Done()
			return ctx.Err()
		},
	).Once()

	reporter := &bootstrapPanicReporter{panics: make(chan any, 1)}
	app := &App{
		Infrastructure: Infrastructure{
			Logger:         lognoop.New(),
			ErrorReporter:  reporter,
			Task:           runtime,
			Subscriber:     subscriber,
			LifecycleState: adapterhealth.NewLifecycleState(),
			ConsumerState:  adapterhealth.NewConsumerState(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	registry, err := consumers.NewRegistry(app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)
	workers := app.startBackgroundWorkers(ctx, registry)

	select {
	case <-workers.taskDone:

	case <-time.After(time.Second):
		t.Fatal("task worker panic was not recovered")
	}

	select {

	case recovered := <-reporter.panics:
		require.Equal(t, "task-runtime-boom", recovered)

	case <-time.After(time.Second):
		t.Fatal("recovered task runtime panic was not reported")
	}

	select {

	case err := <-workers.FatalError():
		t.Fatalf("panic must not be promoted to a process-fatal error: %v", err)

	default:
	}

	cancel()
	select {

	case <-workers.consumerDone:

	case <-time.After(time.Second):
		t.Fatal("consumer worker did not stop")
	}
}

// TestStartBackgroundWorkers_ContextCancelIsNotFatal ensures canceling the
// runtime context stops the task scheduler and message consumer without
// reporting a fatal worker error (expected graceful stop, not crash).
func TestStartBackgroundWorkers_ContextCancelIsNotFatal(t *testing.T) {
	t.Parallel()

	subscriber := coremock.NewSubscriber(t)
	subscriber.EXPECT().Subscribe(mock.Anything, mock.Anything).RunAndReturn(
		func(ctx context.Context, _ messaging.Handler) error {
			<-ctx.Done()
			return ctx.Err()
		},
	).Once()

	app := &App{
		Infrastructure: Infrastructure{
			Logger:         lognoop.New(),
			ErrorReporter:  errorreportingnoop.New(),
			Task:           tasknoop.New(lognoop.New()),
			Subscriber:     subscriber,
			LifecycleState: adapterhealth.NewLifecycleState(),
			ConsumerState:  adapterhealth.NewConsumerState(),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	registry, err := consumers.NewRegistry(app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)
	workers := app.startBackgroundWorkers(ctx, registry)

	cancel()

	select {
	case <-workers.taskDone:

	case <-time.After(time.Second):
		t.Fatal("task worker did not stop")
	}

	select {

	case <-workers.consumerDone:

	case <-time.After(time.Second):
		t.Fatal("consumer worker did not stop")
	}

	select {

	case err := <-workers.FatalError():
		t.Fatalf("unexpected fatal error: %v", err)

	default:
	}
}

// TestShutdown_ReadinessFailsBeforeDrainDelay verifies drain semantics used by
// load balancers: StartDraining has already failed readiness before HTTP
// Shutdown runs, and the configured drain_delay is actually waited.
func TestShutdown_ReadinessFailsBeforeDrainDelay(t *testing.T) {
	t.Parallel()

	lifecycle := adapterhealth.NewLifecycleState()
	pool := coremock.NewPool(t)
	pool.EXPECT().Close().Return(nil).Once()

	cfg := noopTestConfig()
	cfg.Infrastructure.Shutdown.DrainDelay = 20 * time.Millisecond

	app := &App{
		Config: cfg,
		Pool:   pool,
		Infrastructure: Infrastructure{
			Logger:         lognoop.New(),
			LifecycleState: lifecycle,
			Cache:          cachenoop.New(),
			Publisher:      messagingnoop.NewPublisher(),
			Subscriber:     messagingnoop.NewSubscriber(),
			Task:           tasknoop.New(lognoop.New()),
		},
	}

	lifecycle.StartDraining()
	require.True(t, lifecycle.IsDraining())

	sawDrainingDuringHTTP := atomic.Bool{}
	server := &orderedHTTPServer{
		onShutdown: func() {
			sawDrainingDuringHTTP.Store(lifecycle.IsDraining())
		},
	}

	started := time.Now()
	err := app.shutdown(
		context.Background(),
		server,
		nil,
		closedBackgroundWorkers(),
		func() {},
	)
	require.NoError(t, err)
	require.GreaterOrEqual(t, time.Since(started), 20*time.Millisecond)
	require.True(t, sawDrainingDuringHTTP.Load())
}

// orderedHTTPServer records when Shutdown runs so tests can assert relative
// ordering against worker stop and dependency Close calls.
type orderedHTTPServer struct {
	onShutdown func()
}

func (s *orderedHTTPServer) serve(net.Listener) error {
	return nil
}

func (s *orderedHTTPServer) shutdown(context.Context) error {
	if s.onShutdown != nil {
		s.onShutdown()
	}

	return nil
}

// trackingTaskRuntime is a minimal Runtime used to observe Close ordering.
type trackingTaskRuntime struct {
	onClose func()
}

func (r *trackingTaskRuntime) Register(coretask.Task) error { return nil }

func (r *trackingTaskRuntime) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (r *trackingTaskRuntime) Dispatch(context.Context, coretask.Command) error {
	return nil
}

func (r *trackingTaskRuntime) Close() error {
	if r.onClose != nil {
		r.onClose()
	}

	return nil
}
