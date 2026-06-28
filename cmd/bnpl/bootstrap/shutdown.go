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

// httpServer abstracts the HTTP listener for tests and production wiring.
type httpServer interface {
	listenAndServe() error
	shutdown(ctx context.Context) error
}

type stdHTTPServer struct {
	srv *http.Server
}

func (s *stdHTTPServer) listenAndServe() error {
	return s.srv.ListenAndServe()
}

func (s *stdHTTPServer) shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

type backgroundWorkers struct {
	taskDone     chan struct{}
	consumerDone chan struct{}
}

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

func shutdownContext(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if deadline, ok := parent.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining < timeout {
			timeout = remaining
		}
	}

	return context.WithTimeout(context.Background(), timeout)
}

func logListenError(logger log.Logger, err error) {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.With("error", err.Error()).Error("http server stopped with error")
	}
}
