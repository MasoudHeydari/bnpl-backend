package bootstrap

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	cachenoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/cache/noop"
	cbnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/circuitbreaker/noop"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
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

func (s *recordingHTTPServer) listenAndServe() error {
	<-s.releaseListen
	return errors.New("server closed")
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

	err := app.shutdown(ctx, server, workers)
	if err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	if !server.shutdownCalled.Load() {
		t.Fatal("expected http server Shutdown to be called")
	}
}

func TestContextCancelStopsBackgroundTask(t *testing.T) {
	t.Parallel()

	logger := lognoop.New()
	runtime := tasknoop.New(logger)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		_ = runtime.Start(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("background task did not exit after context cancellation")
	}
}

func TestShutdownTimeoutReturnsError(t *testing.T) {
	t.Parallel()

	app := testShutdownApp(t)
	server := &blockingHTTPServer{}
	workers := closedBackgroundWorkers()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	err := app.shutdown(ctx, server, workers)
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
	workers := app.startBackgroundWorkers(signalCtx)

	go func() {
		time.Sleep(20 * time.Millisecond)
		signalStop()
	}()

	shutdownCtx, cancel := shutdownContext(signalCtx, time.Second)
	defer cancel()

	err := app.shutdown(shutdownCtx, server, workers)
	if err != nil {
		t.Fatalf("shutdown() error = %v", err)
	}
	if !server.shutdownCalled.Load() {
		t.Fatal("expected shutdown after signal context cancellation")
	}
}

type blockingHTTPServer struct{}

func (s *blockingHTTPServer) listenAndServe() error {
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

	err := app.shutdown(ctx, server, workers)
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
		taskDone:     make(chan struct{}),
		consumerDone: make(chan struct{}),
	}
	close(workers.taskDone)
	close(workers.consumerDone)
	return workers
}
