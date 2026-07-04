package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	saramamessaging "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/sarama"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// ErrShutdownTimedOut indicates graceful shutdown exceeded its deadline.
var ErrShutdownTimedOut = errors.New("graceful shutdown timed out")

// ErrFatalConsumerExit indicates the message consumer exited unexpectedly.
var ErrFatalConsumerExit = errors.New("message consumer exited with fatal error")

// ErrFatalTaskRuntimeExit indicates the task scheduler exited unexpectedly.
var ErrFatalTaskRuntimeExit = errors.New("task runtime exited with fatal error")

// ErrFatalBotMessagingExit indicates a configured bot listener exited unexpectedly.
var ErrFatalBotMessagingExit = errors.New("bot messaging runtime exited with fatal error")

// httpServer abstracts the HTTP listener so production code and tests can share
// the same shutdown orchestration without binding tests to net/http.Server.
type httpServer interface {
	serve(ln net.Listener) error
	shutdown(ctx context.Context) error
}

// stdHTTPServer adapts net/http.Server to the httpServer interface used by Run.
type stdHTTPServer struct {
	// srv is the application REST listener.
	srv *http.Server
}

func (s *stdHTTPServer) serve(ln net.Listener) error {
	return s.srv.Serve(ln)
}

func (s *stdHTTPServer) shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// metricsHTTPServer adapts the dedicated metrics listener to httpServer.
type metricsHTTPServer struct {
	// server is the Prometheus scrape HTTP listener.
	server interface {
		Serve(net.Listener) error
		Shutdown(context.Context) error
	}
}

func (s *metricsHTTPServer) serve(ln net.Listener) error {
	return s.server.Serve(ln)
}

func (s *metricsHTTPServer) shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// backgroundWorkers tracks long-lived goroutines started during Run.
type backgroundWorkers struct {
	// taskDone closes when the task runtime goroutine exits.
	taskDone chan struct{}

	// consumerDone closes when the message consumer goroutine exits.
	consumerDone chan struct{}

	// botMessagingDone closes when all configured bot listeners exit.
	botMessagingDone chan struct{}

	// fatalErr receives at most one unexpected worker failure.
	fatalErr chan error
}

// startBackgroundWorkers launches infrastructure workers that share the root runtime context.
func (a *App) startBackgroundWorkers(ctx context.Context, registry *consumers.Registry) *backgroundWorkers {
	const op = "bootstrap.App.startBackgroundWorkers"

	workers := &backgroundWorkers{
		taskDone:         make(chan struct{}),
		consumerDone:     make(chan struct{}),
		botMessagingDone: make(chan struct{}),
		fatalErr:         make(chan error, 1),
	}

	reportFatal := func(err error) {
		if err == nil {
			return
		}

		if a.Infrastructure.LifecycleState != nil {
			a.Infrastructure.LifecycleState.StartDraining()
		}

		select {
		case workers.fatalErr <- err:

		default:
		}
	}

	go func() {
		defer close(workers.taskDone)
		defer a.recoverBackgroundPanic(ctx, "task_runtime", nil)
		err := a.Infrastructure.Task.Start(ctx)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}

		if err == nil {
			err = errors.New("task runtime stopped without context cancellation")
		}

		a.Infrastructure.Logger.With("error", err.Error()).Error("task runtime stopped unexpectedly")
		reportFatal(fmt.Errorf("%s: %w: %w", op, ErrFatalTaskRuntimeExit, err))
	}()

	go func() {
		defer close(workers.consumerDone)
		defer a.recoverBackgroundPanic(
			ctx,
			"message_consumer",
			func() {
				if a.Infrastructure.ConsumerState != nil {
					a.Infrastructure.ConsumerState.SetFatal()
				}
			},
		)
		err := a.Infrastructure.Subscriber.Subscribe(ctx, registry.Handle)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			if a.Infrastructure.ConsumerState != nil {
				a.Infrastructure.ConsumerState.SetStopped()
			}

			return
		}

		if err == nil {
			err = errors.New("message consumer stopped without context cancellation")
		}

		if a.Infrastructure.ConsumerState != nil {
			a.Infrastructure.ConsumerState.SetFatal()
		}

		a.Infrastructure.Logger.With("error", err.Error()).Error("message consumer stopped unexpectedly")
		if isFatalConsumerSubscribeError(err) {
			reportFatal(fmt.Errorf("%s: %w: %w", op, ErrFatalConsumerExit, err))
			return
		}

		// Any unexpected subscriber exit is fatal even when the adapter did not
		// wrap it in a provider-specific sentinel.
		reportFatal(fmt.Errorf("%s: %w: %w", op, ErrFatalConsumerExit, err))
	}()

	go func() {
		defer close(workers.botMessagingDone)
		defer a.recoverBackgroundPanic(ctx, "bot_messaging", nil)

		if a.External.BotMessaging == nil {
			return
		}

		err := a.External.BotMessaging.Run(ctx, a.BotMessageHandler)
		if ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}

		if err == nil {
			err = errors.New("bot messaging runtime stopped without context cancellation")
		}

		a.Infrastructure.Logger.With("error", err.Error()).Error("bot messaging runtime stopped unexpectedly")
		reportFatal(fmt.Errorf("%s: %w: %w", op, ErrFatalBotMessagingExit, err))
	}()

	return workers
}

func (a *App) recoverBackgroundPanic(ctx context.Context, component string, onPanic func()) {
	recovered := recover()
	if recovered == nil {
		return
	}

	if onPanic != nil {
		onPanic()
	}

	if a.Infrastructure.ErrorReporter != nil {
		a.Infrastructure.ErrorReporter.CapturePanic(
			ctx,
			recovered,
			coreerrorreporting.Metadata{
				Component: component,
				Tags: map[string]string{
					"runtime.boundary": "background_worker",
				},
			},
		)
	}

	if a.Infrastructure.Logger != nil {
		a.Infrastructure.Logger.With(
			"component", component,
			"panic", fmt.Sprint(recovered),
		).Error("background worker panic recovered")
	}
}

func isFatalConsumerSubscribeError(err error) bool {
	if err == nil {
		return false
	}

	return errors.Is(err, ErrFatalConsumerExit) || errors.Is(err, saramamessaging.ErrFatalConsumerExit)
}

// FatalError returns when a background worker exits fatally (task runtime or
// message consumer, or bot messaging runtime). The channel is buffered and delivers at most one error.
func (w *backgroundWorkers) FatalError() <-chan error {
	return w.fatalErr
}

// shutdown stops runtime surfaces then App-owned dependencies under one global
// budget (infrastructure.shutdown.timeout).
//
// Order is deliberate:
//  1. drain_delay — readiness already failed; wait for LB propagation while
//     HTTP still accepts (and can finish) requests against live dependencies.
//  2. HTTP / metrics Shutdown — stop accepting; wait for in-flight work.
//  3. stopWorkers — cancel task/consumer runtime only after HTTP drained so
//     in-flight requests never lose their dependencies mid-flight.
//  4. wait for worker goroutines.
//  5. releaseResources — outbound HTTP, messaging, cache, external, database.
//
// Component close failures are joined; cleanup continues. Close() without
// context may block; the shutdown context bounds waits that honor it, and the
// overall process budget is still the configured timeout for context-aware
// steps. Non-context Close calls run after workers stop and should be fast.
func (a *App) shutdown(
	shutdownCtx context.Context,
	server httpServer,
	metricsServer httpServer,
	workers *backgroundWorkers,
	stopWorkers context.CancelFunc,
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

	record("drain_delay", a.waitDrainDelay(shutdownCtx))
	record("http", server.shutdown(shutdownCtx))
	if metricsServer != nil {
		record("metrics_http", metricsServer.shutdown(shutdownCtx))
	}

	if stopWorkers != nil {
		stopWorkers()
	}

	a.waitBackground(shutdownCtx, workers, record)

	// Flush error reporting while the shutdown budget is still available and
	// before the reporter transport is closed with the shared resources. A flush
	// timeout is observability loss, not an application shutdown failure.
	if a.Infrastructure.ErrorReporter != nil && !a.Infrastructure.ErrorReporter.Flush(shutdownCtx) {
		logger.Warn("error reporter flush did not complete before shutdown deadline")
	}

	// Log each shared dependency close through record, but join errors from the
	// single ownership path (releaseErr) so Close/shutdown do not duplicate.
	_ = a.releaseResources(func(step string, err error) {
		if err == nil {
			logger.With("component", step).Info("component stopped")
			return
		}

		logger.With("component", step, "error", err.Error()).Error("component stop failed")
	})
	shutdownErr = errors.Join(shutdownErr, a.releaseErr)

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
			defer a.recoverBackgroundPanic(ctx, "shutdown_waiter", nil)
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
	wait("bot_messaging", workers.botMessagingDone)

	wg.Wait()
}

func (a *App) waitDrainDelay(ctx context.Context) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Tests and manually assembled Apps may not carry configuration. In that
	// case there is no load-balancer drain contract to honor.
	if a.Config == nil {
		return nil
	}

	delay := a.Config.GetShutdownDrainDelay()
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}

// waitForStop waits for the first process stop condition. listenerStopped /
// metricsListenerStopped are true when this function consumed the corresponding
// listener result, preventing a second read from the one-shot channel during
// shutdown. metricsListenErr may be nil when the metrics server is disabled.
//
// Classification: http.ErrServerClosed is a normal shutdown result, not a fatal
// process error. Unexpected listen/serve failures and fatal worker exits become
// stopErr.
func waitForStop(
	ctx context.Context,
	listenErr <-chan error,
	metricsListenErr <-chan error,
	fatalErr <-chan error,
) (stopErr error, listenerResult error, listenerStopped bool, metricsResult error, metricsStopped bool) {
	const op = "bootstrap.waitForStop"

	select {
	case err := <-listenErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			stopErr = fmt.Errorf("%s: http server: %w", op, err)
		}

		return stopErr, err, true, nil, false

	case err := <-metricsListenErr:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			stopErr = fmt.Errorf("%s: metrics http server: %w", op, err)
		}

		return stopErr, nil, false, err, true

	case err := <-fatalErr:
		return err, nil, false, nil, false

	case <-ctx.Done():
		return nil, nil, false, nil, false
	}
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

func logListenError(logger log.Logger, component string, err error) {
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.With("component", component, "error", err.Error()).Error("http listener stopped with error")
	}
}
