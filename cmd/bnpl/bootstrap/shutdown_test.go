package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	cachenoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	errorreportingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting/noop"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	messagingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/noop"
	tasknoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/noop"
	adaptertrace "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

type recordingHTTPServer struct {
	shutdownCalled atomic.Bool
	releaseListen  chan struct{}
}

func newRecordingHTTPServer() *recordingHTTPServer {
	return &recordingHTTPServer{
		releaseListen: make(chan struct{}),
	}
}

func (s *recordingHTTPServer) serve(net.Listener) error {
	<-s.releaseListen
	return http.ErrServerClosed
}

func (s *recordingHTTPServer) shutdown(context.Context) error {
	s.shutdownCalled.Store(true)
	close(s.releaseListen)
	return nil
}

func TestShutdownCallsHTTPServerShutdown(t *testing.T) {
	t.Parallel()

	app := testShutdownApp(t)
	server := newRecordingHTTPServer()
	workers := closedBackgroundWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := app.shutdown(ctx, server, nil, workers, func() {})
	if err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	if !server.shutdownCalled.Load() {
		t.Fatal("expected http server Shutdown to be called")
	}
}

func TestShutdownCallsMetricsHTTPServerShutdown(t *testing.T) {
	t.Parallel()

	app := testShutdownApp(t)
	server := newRecordingHTTPServer()
	metricsServer := newRecordingHTTPServer()
	workers := closedBackgroundWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := app.shutdown(ctx, server, metricsServer, workers, func() {})
	if err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	if !metricsServer.shutdownCalled.Load() {
		t.Fatal("expected metrics http server Shutdown to be called")
	}
}

func TestContextCancelStopsBackgroundTask(t *testing.T) {
	t.Parallel()

	logger := lognoop.New()
	runtime := tasknoop.New(logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	errCh := make(chan error, 1)

	go func() {
		errCh <- runtime.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:

	case <-time.After(time.Second):
		t.Fatal("background task did not exit after context cancellation")
	}

	if err := <-errCh; err != nil {
		t.Fatalf("runtime.Start: %v", err)
	}
}

func TestShutdownTimeoutReturnsError(t *testing.T) {
	t.Parallel()

	app := testShutdownApp(t)
	server := &blockingHTTPServer{}
	workers := closedBackgroundWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := app.shutdown(ctx, server, nil, workers, func() {})
	if err == nil {
		t.Fatal("expected shutdown timeout error")
	}

	if !errors.Is(err, ErrShutdownTimedOut) {
		t.Fatalf("expected ErrShutdownTimedOut, got %v", err)
	}
}

func TestSIGTERMTriggersShutdown(t *testing.T) {
	signalCtx, signalStop := context.WithCancel(context.Background())
	notifyContext = func(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
		if len(signals) != 2 {
			t.Fatalf("signals = %v, want [SIGINT SIGTERM]", signals)
		}

		if signals[0] != os.Interrupt || signals[1] != syscall.SIGTERM {
			t.Fatalf("unexpected signals: %v", signals)
		}

		return signalCtx, signalStop
	}
	t.Cleanup(func() {
		notifyContext = defaultNotifyContext
		signalStop()
	})

	app := testShutdownApp(t)
	server := newRecordingHTTPServer()
	registry, err := consumers.NewRegistry(app.Infrastructure.Logger, app.Infrastructure.ErrorReporter)
	require.NoError(t, err)
	workers := app.startBackgroundWorkers(signalCtx, registry)

	go func() {
		time.Sleep(20 * time.Millisecond)
		signalStop()
	}()

	shutdownCtx, cancel := shutdownContext(signalCtx, time.Second)
	defer cancel()

	err = app.shutdown(shutdownCtx, server, nil, workers, func() {})
	if err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}

	if !server.shutdownCalled.Load() {
		t.Fatal("expected shutdown after signal context cancellation")
	}
}

type blockingHTTPServer struct{}

func (s *blockingHTTPServer) serve(net.Listener) error {
	select {}
}

func (s *blockingHTTPServer) shutdown(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestShutdownPublisherCloseErrorJoinsAndContinues(t *testing.T) {
	t.Parallel()

	publisherErr := errors.New("publisher close failed")

	pool := mock.NewPool(t)
	pool.EXPECT().Close().Return(nil).Once()

	subscriber := mock.NewSubscriber(t)
	subscriber.EXPECT().Close().Return(nil).Once()

	cache := mock.NewCache(t)
	cache.EXPECT().Close().Return(nil).Once()

	publisher := mock.NewPublisher(t)
	publisher.EXPECT().Close().Return(publisherErr).Once()

	app := &App{
		Pool: pool,
		Infrastructure: Infrastructure{
			Logger:     lognoop.New(),
			Cache:      cache,
			Publisher:  publisher,
			Subscriber: subscriber,
			Task:       tasknoop.New(lognoop.New()),
		},
	}

	server := newRecordingHTTPServer()
	workers := closedBackgroundWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := app.shutdown(ctx, server, nil, workers, func() {})
	if err == nil {
		t.Fatal("expected shutdown error")
	}

	if !errors.Is(err, publisherErr) {
		t.Fatalf("expected publisher close error, got %v", err)
	}

	if !strings.Contains(err.Error(), "publisher:") {
		t.Fatalf("expected joined shutdown error to mention publisher, got %v", err)
	}
}

func testShutdownApp(t *testing.T) *App {
	t.Helper()

	pool := mock.NewPool(t)
	pool.EXPECT().Close().Return(nil).Once()

	return &App{
		Pool: pool,
		Infrastructure: Infrastructure{
			Logger:         lognoop.New(),
			ErrorReporter:  errorreportingnoop.New(),
			Cache:          cachenoop.New(),
			Publisher:      messagingnoop.NewPublisher(),
			Subscriber:     messagingnoop.NewSubscriber(),
			Task:           tasknoop.New(lognoop.New()),
			CircuitBreaker: cbnoop.New(),
			Trace:          adaptertrace.NewULIDProvider(),
		},
	}
}

func closedBackgroundWorkers() *backgroundWorkers {
	workers := &backgroundWorkers{
		taskDone:         make(chan struct{}),
		consumerDone:     make(chan struct{}),
		botMessagingDone: make(chan struct{}),
	}
	close(workers.taskDone)
	close(workers.consumerDone)
	close(workers.botMessagingDone)
	return workers
}

func TestWaitForStopReturnsConsumedListenerResult(t *testing.T) {
	t.Parallel()

	listenErr := make(chan error, 1)
	fatalErr := make(chan error)
	listenerErr := errors.New("listen failed")
	listenErr <- listenerErr

	stopErr, result, listenerStopped, _, _ := waitForStop(
		context.Background(),
		listenErr,
		nil,
		fatalErr,
	)

	if !listenerStopped {
		t.Fatal("expected listener result to be marked as consumed")
	}

	if !errors.Is(stopErr, listenerErr) {
		t.Fatalf("stop error = %v, want listener error", stopErr)
	}

	if !errors.Is(result, listenerErr) {
		t.Fatalf("listener result = %v, want %v", result, listenerErr)
	}

	select {
	case <-listenErr:
		t.Fatal("listener result must be consumed exactly once")

	default:
	}
}

func TestWaitForStopFatalWorkerLeavesListenerResultUnread(t *testing.T) {
	t.Parallel()

	listenErr := make(chan error, 1)
	fatalErr := make(chan error, 1)
	fatal := errors.New("consumer failed")
	fatalErr <- fatal

	stopErr, result, listenerStopped, _, _ := waitForStop(
		context.Background(),
		listenErr,
		nil,
		fatalErr,
	)

	if listenerStopped {
		t.Fatal("listener result should remain unread when a worker fails first")
	}

	if result != nil {
		t.Fatalf("listener result = %v, want nil", result)
	}

	if !errors.Is(stopErr, fatal) {
		t.Fatalf("stop error = %v, want %v", stopErr, fatal)
	}
}

func TestWaitForStopReturnsMetricsListenerError(t *testing.T) {
	t.Parallel()

	listenErr := make(chan error)
	metricsListenErr := make(chan error, 1)
	fatalErr := make(chan error)
	metricsErr := errors.New("metrics bind failed")
	metricsListenErr <- metricsErr

	stopErr, appResult, appStopped, metricsResult, metricsStopped := waitForStop(
		context.Background(),
		listenErr,
		metricsListenErr,
		fatalErr,
	)

	if appStopped {
		t.Fatal("application listener should remain unread when metrics fails first")
	}

	if appResult != nil {
		t.Fatalf("application listener result = %v, want nil", appResult)
	}

	if !metricsStopped {
		t.Fatal("expected metrics listener result to be marked as consumed")
	}

	if !errors.Is(stopErr, metricsErr) {
		t.Fatalf("stop error = %v, want metrics error", stopErr)
	}

	if !errors.Is(metricsResult, metricsErr) {
		t.Fatalf("metrics result = %v, want %v", metricsResult, metricsErr)
	}
}

func TestWaitDrainDelayUsesConfiguredValue(t *testing.T) {
	t.Parallel()

	cfg := noopTestConfig()
	cfg.Infrastructure.Shutdown.DrainDelay = time.Millisecond
	app := &App{Config: cfg}

	startedAt := time.Now()
	err := app.waitDrainDelay(context.Background())
	if err != nil {
		t.Fatalf("waitDrainDelay() error = %v", err)
	}

	if elapsed := time.Since(startedAt); elapsed < time.Millisecond {
		t.Fatalf("drain delay elapsed = %s, want at least 1ms", elapsed)
	}
}
