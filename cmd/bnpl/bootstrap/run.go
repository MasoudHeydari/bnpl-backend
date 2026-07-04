package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	adaptermetrics "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// notifyContextFunc creates a context canceled by OS signals.
type notifyContextFunc func(context.Context, ...os.Signal) (context.Context, context.CancelFunc)

var notifyContext notifyContextFunc = defaultNotifyContext

func defaultNotifyContext(
	parent context.Context,
	signals ...os.Signal,
) (context.Context, context.CancelFunc) {
	return signalNotifyContext(parent, signals...)
}

// Boot loads configuration, starts HTTP/metrics listeners and background
// workers, then blocks until SIGINT or SIGTERM.
//
// The first signal cancels the process context and starts graceful shutdown.
// Process-level exit remains in the CLI layer (command.Execute); bootstrap does
// not call os.Exit. A second signal during shutdown is left to the supervisor
// (for example Kubernetes SIGKILL after terminationGracePeriodSeconds).
func Boot() error {
	ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return Run(ctx)
}

// Run is the long-running server entrypoint.
//
// Lifecycle (deterministic):
//
//	load config
//	→ construct App (dependency order, rollback on failure)
//	→ prepare (tasks, consumers, routes)
//	→ bind listeners (fail before starting workers if ports are taken)
//	→ start workers + serve
//	→ supervise until stop cause
//	→ drain (readiness fails, drain_delay, HTTP/metrics shutdown)
//	→ stop workers
//	→ close App-owned resources (reverse dependency order)
//
// ctx cancellation (SIGINT/SIGTERM via Boot) or a fatal listener/worker exit
// triggers the same graceful shutdown path.
func Run(ctx context.Context) (err error) {
	const op = "bootstrap.Run"

	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("%s: config.Load(%q): %w", op, cfgPath, err)
	}

	app, err := NewApp(ctx, cfg)
	if err != nil {
		return fmt.Errorf("%s: initialize app: %w", op, err)
	}

	// Startup failures after NewApp must release App-owned resources. After a
	// successful supervise→shutdown cycle, shutdown already released them via
	// the same Close ownership (idempotent); skip a second join of the same
	// release error into the return value.
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			err = errors.Join(err, app.Close())
		}
	}()

	if err = app.RegisterTasks(); err != nil {
		return fmt.Errorf("%s: registering tasks: %w", op, err)
	}

	consumerRegistry, err := app.RegisterConsumers()
	if err != nil {
		return fmt.Errorf("%s: registering consumers: %w", op, err)
	}

	//nolint:contextcheck // Engine middleware uses per-request context, not process Boot context.
	engine, err := cfg.NewGinEngine(
		app.Infrastructure.Trace,
		app.Infrastructure.Logger,
		app.Infrastructure.ErrorReporter,
	)
	if err != nil {
		return fmt.Errorf("%s: initialize http engine: %w", op, err)
	}

	if err = app.RegisterRoutes(engine); err != nil {
		return fmt.Errorf("%s: registering http: %w", op, err)
	}

	metricsServer, err := newMetricsHTTPServer(
		cfg,
		app.Infrastructure.MetricsHandler,
		app.Infrastructure.Logger,
		app.Infrastructure.ErrorReporter,
	)
	if err != nil {
		return fmt.Errorf("%s: initialize metrics server: %w", op, err)
	}

	// Bind before starting workers so a busy HTTP/metrics port fails fast
	// without leaving background goroutines running for a few milliseconds.
	httpLn, err := net.Listen("tcp", cfg.GetGinListeningAddress())
	if err != nil {
		return fmt.Errorf("%s: bind http listener: %w", op, err)
	}

	var metricsLn net.Listener
	if metricsServer != nil {
		metricsLn, err = net.Listen("tcp", cfg.GetMetricsListeningAddress())
		if err != nil {
			_ = httpLn.Close()
			return fmt.Errorf("%s: bind metrics listener: %w", op, err)
		}
	}

	server := &stdHTTPServer{
		srv: &http.Server{
			Addr:              cfg.GetGinListeningAddress(),
			Handler:           engine,
			ReadHeaderTimeout: cfg.GetGinReadHeaderTimeout(),
			ReadTimeout:       cfg.GetGinReadTimeout(),
			WriteTimeout:      cfg.GetGinWriteTimeout(),
			IdleTimeout:       cfg.GetGinIdleTimeout(),
			MaxHeaderBytes:    cfg.GetGinMaxHeaderBytes(),
		},
	}

	runtimeCtx, stopRuntime := context.WithCancel(ctx)
	defer stopRuntime()

	workers := app.startBackgroundWorkers(runtimeCtx, consumerRegistry)

	httpListenErr := make(chan error, 1)
	go func() {
		httpListenErr <- serveWithPanicRecovery(
			runtimeCtx,
			"http_server",
			func() error { return server.serve(httpLn) },
			app.Infrastructure.Logger,
			app.Infrastructure.ErrorReporter,
		)
	}()

	metricsListenErr := make(chan error, 1)
	if metricsServer != nil {
		go func() {
			metricsListenErr <- serveWithPanicRecovery(
				runtimeCtx,
				"metrics_server",
				func() error { return metricsServer.serve(metricsLn) },
				app.Infrastructure.Logger,
				app.Infrastructure.ErrorReporter,
			)
		}()
	}

	stopErr, listenerResult, listenerStopped, metricsResult, metricsStopped := waitForStop(
		ctx,
		httpListenErr,
		metricsListenErr,
		workers.FatalError(),
	)

	if stopErr != nil && app.Infrastructure.ErrorReporter != nil {
		app.Infrastructure.ErrorReporter.CaptureError(
			runtimeCtx,
			stopErr,
			coreerrorreporting.Metadata{
				Component: "runtime_supervisor",
				Tags: map[string]string{
					"phase": "serve",
				},
			},
		)
	}

	// Mark not-ready before stopping work so load balancers stop routing new
	// traffic. Workers stay up until after HTTP drain so in-flight requests can
	// still use DB/cache/messaging/external clients.
	app.Infrastructure.LifecycleState.StartDraining()

	shutdownCtx, cancel := shutdownContext(ctx, cfg.GetShutdownTimeout())
	defer cancel()

	shutdownErr := app.shutdown(shutdownCtx, server, metricsServer, workers, stopRuntime)
	if errors.Is(shutdownErr, ErrShutdownTimedOut) {
		app.Infrastructure.LifecycleState.MarkShutdownTimedOut()
	}

	releaseOnReturn = false
	// Close is idempotent; invoke once more so any future caller sees a finished App.
	_ = app.Close()

	// waitForStop may already have consumed a listener result. Read remaining
	// results only when shutdown was initiated by context cancellation or a
	// fatal worker.
	if !listenerStopped {
		select {
		case listenerResult = <-httpListenErr:

		case <-shutdownCtx.Done():
			listenerResult = shutdownCtx.Err()
		}
	}

	logListenError(app.Infrastructure.Logger, "http server", listenerResult)

	if metricsListenErr != nil && !metricsStopped {
		select {
		case metricsResult = <-metricsListenErr:

		case <-shutdownCtx.Done():
			metricsResult = shutdownCtx.Err()
		}
	}

	logListenError(app.Infrastructure.Logger, "metrics http server", metricsResult)

	return errors.Join(stopErr, shutdownErr)
}

func serveWithPanicRecovery(
	ctx context.Context,
	component string,
	serve func() error,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (err error) {
	const op = "bootstrap.serveWithPanicRecovery"

	defer func() {
		recovered := recover()
		if recovered == nil {
			return
		}

		if reporter != nil {
			reporter.CapturePanic(
				ctx,
				recovered,
				coreerrorreporting.Metadata{
					Component: component,
					Tags: map[string]string{
						"runtime.boundary": "listener",
					},
				},
			)
		}

		if logger != nil {
			logger.With(
				"component", component,
				"panic", fmt.Sprint(recovered),
			).Error("listener panic recovered")
		}

		// The panic itself is contained, but a listener whose Serve loop has
		// unwound is no longer serving traffic. Return an ordinary error to the
		// existing lifecycle supervisor instead of leaving a zombie process or
		// re-panicking the whole application.
		err = fmt.Errorf("%s: %s panic recovered: %v", op, component, recovered)
	}()

	if serve == nil {
		return errors.New("listener serve function is required")
	}

	return serve()
}

func panicSafeHTTPHandler(
	component string,
	next http.Handler,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			recovered := recover()
			if recovered == nil {
				return
			}

			ctx := context.Background()
			if r != nil && r.Context() != nil {
				ctx = r.Context()
			}

			if reporter != nil {
				reporter.CapturePanic(
					ctx,
					recovered,
					coreerrorreporting.Metadata{
						Component: component,
						Tags: map[string]string{
							"http.surface": "internal",
						},
					},
				)
			}

			if logger != nil {
				logger.With(
					"component", component,
					"panic", fmt.Sprint(recovered),
				).Error("http handler panic recovered")
			}

			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}()

		next.ServeHTTP(w, r)
	})
}

// newMetricsHTTPServer constructs the dedicated scrape listener when enabled.
//
// Returns (nil, nil) when metrics exposition is disabled so Run skips starting
// and shutting down a metrics listener.
func newMetricsHTTPServer(
	cfg *config.Config,
	handler http.Handler,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
) (httpServer, error) {
	if !cfg.MetricsServerEnabled() {
		return nil, nil
	}

	metricsPath, err := cfg.GetMetricsPath()
	if err != nil {
		return nil, err
	}

	if handler == nil {
		return nil, errors.New("metrics handler is required when metrics server is enabled")
	}

	ms, err := adaptermetrics.NewServer(
		adaptermetrics.ServerOptions{
			Addr:              cfg.GetMetricsListeningAddress(),
			Path:              metricsPath,
			Handler:           panicSafeHTTPHandler("metrics_http", handler, logger, reporter),
			ReadHeaderTimeout: cfg.GetGinReadHeaderTimeout(),
		},
	)
	if err != nil {
		return nil, err
	}

	return &metricsHTTPServer{server: ms}, nil
}
