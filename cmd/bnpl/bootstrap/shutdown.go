package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

// ErrShutdownTimedOut indicates graceful shutdown exceeded its deadline.
var ErrShutdownTimedOut = errors.New("graceful shutdown timed out")

// httpServer abstracts the HTTP listener so production code and tests can share
// the same shutdown orchestration without binding tests to net/http.Server.
type httpServer interface {
	listenAndServe() error
	shutdown(ctx context.Context) error
}

// stdHTTPServer adapts net/http.Server to the httpServer interface used by Run.
type stdHTTPServer struct {
	srv *http.Server
}

func (s *stdHTTPServer) listenAndServe() error {
	return s.srv.ListenAndServe()
}

func (s *stdHTTPServer) shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// backgroundWorkers tracks long-lived goroutines started during Run.
//
// Each done channel is closed after its worker exits. Shutdown waits on these
// channels before closing publishers, subscribers, and the database pool.
type backgroundWorkers struct {
	taskDone     chan struct{}
	consumerDone chan struct{}
}

// startBackgroundWorkers launches infrastructure workers that share the root
// runtime context from Run.
//
// Workers observe the same ctx passed to Boot/Run, so SIGINT and SIGTERM
// propagate to the task scheduler and message consumer without extra wiring.
func (a *App) startBackgroundWorkers(ctx context.Context) *backgroundWorkers {
	workers := &backgroundWorkers{
		taskDone:     make(chan struct{}),
		consumerDone: make(chan struct{}),
	}

	go func() {
		defer close(workers.taskDone)
		if err := a.Infrastructure.Task.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			a.Infrastructure.Logger.
				With("error", err.Error()).
				Error("task runtime stopped with error")
		}
	}()

	go func() {
		defer close(workers.consumerDone)
		if err := a.Infrastructure.Subscriber.Subscribe(ctx, a.messageHandler()); err != nil &&
			!errors.Is(err, context.Canceled) {
			a.Infrastructure.Logger.
				With("error", err.Error()).
				Error("message consumer stopped with error")
		}
	}()

	return workers
}

// messageHandler is the bootstrap-level consumer callback until feature routes
// register domain-specific handlers.
//
// It acknowledges messages without business handling so Kafka consumers can
// start safely during shutdown testing and early integrations.
func (a *App) messageHandler() messaging.Handler {
	return func(ctx context.Context, msg messaging.Message) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		a.Infrastructure.Logger.With(
			"topic", msg.Topic,
			"partition", msg.Partition,
			"offset", msg.Offset,
		).Warn("received message without registered handler")

		return nil
	}
}

// shutdown stops application components in a fixed order and records progress
// through the application logger.
//
// The sequence is intentional:
//  1. stop accepting HTTP traffic
//  2. stop task schedulers and message consumers
//  3. allow a short in-flight drain window
//  4. close publishers, subscribers, cache, and the database pool
//
// shutdownCtx bounds the entire sequence. When it expires, remaining steps may
// fail with context.DeadlineExceeded and ErrShutdownTimedOut is returned.
func (a *App) shutdown(
	shutdownCtx context.Context,
	server httpServer,
	workers *backgroundWorkers,
) error {
	logger := a.Infrastructure.Logger
	logger.Info("shutdown started")

	var shutdownErr error
	record := func(step string, err error) {
		if err == nil {
			logger.With("component", step).Info("component stopped")
			return
		}

		shutdownErr = errors.Join(shutdownErr, fmt.Errorf("%s: %w", step, err))
		logger.With("component", step, "error", err.Error()).Error("component stop failed")
	}

	record("http", server.shutdown(shutdownCtx))
	a.waitBackground(shutdownCtx, workers, record)
	record("publisher", a.Infrastructure.Publisher.Close())
	record("subscriber", a.Infrastructure.Subscriber.Close())
	record("cache", a.Infrastructure.Cache.Close())
	record("database", a.Pool.Close())

	if errors.Is(shutdownErr, context.DeadlineExceeded) {
		shutdownErr = errors.Join(shutdownErr, ErrShutdownTimedOut)
		logger.Error("shutdown timed out")
	} else if shutdownErr == nil {
		logger.Info("shutdown completed")
	}

	return shutdownErr
}

// waitBackground blocks until background workers exit or shutdownCtx expires.
//
// Workers are stopped indirectly: Run cancels the root context before calling
// shutdown, which causes Task.Start and Subscriber.Subscribe to return.
func (a *App) waitBackground(
	ctx context.Context,
	workers *backgroundWorkers,
	record func(step string, err error),
) {
	var wg sync.WaitGroup

	wait := func(name string, done <-chan struct{}) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case <-done:
				record(name, nil)
			case <-ctx.Done():
				record(name, ctx.Err())
			}
		}()
	}

	wait("task_scheduler", workers.taskDone)
	wait("message_consumer", workers.consumerDone)

	wg.Wait()

	if err := a.drainInFlight(ctx); err != nil {
		record("in_flight_work", err)
	}
}

// drainInFlight provides a short grace period for work already started by HTTP
// handlers, dispatched tasks, or outbox jobs to finish before publishers and
// database connections are closed.
func (a *App) drainInFlight(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// waitForListen blocks until the process receives a shutdown signal or the HTTP
// server fails before it begins serving traffic.
func waitForListen(ctx context.Context, listenErr <-chan error) error {
	select {
	case err := <-listenErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	case <-ctx.Done():
	}

	return nil
}

// shutdownContext derives a bounded shutdown deadline from the configured
// timeout.
//
// A fresh context is used so shutdown work is not tied to the already-canceled
// signal context while still respecting any earlier parent deadline.
func shutdownContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	return context.WithTimeout(context.Background(), timeout)
}

// logListenError records unexpected HTTP server termination after shutdown.
func logListenError(logger log.Logger, err error) {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.With("error", err.Error()).Error("http server stopped with error")
	}
}
