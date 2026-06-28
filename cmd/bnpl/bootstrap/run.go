package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"syscall"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
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

// Boot starts the HTTP server and blocks until SIGINT or SIGTERM is received.
func Boot() error {
	ctx, stop := notifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	return Run(ctx)
}

// Run starts the application and performs graceful shutdown when ctx is canceled.
func Run(ctx context.Context) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}

	pool, err := cfg.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("bootstrap.Run: creating DB pool: %w", err)
	}

	app, err := newApp(cfg, pool)
	if err != nil {
		_ = pool.Close()
		return fmt.Errorf("bootstrap.Run: initialize app: %w", err)
	}

	if err = app.RegisterTasks(); err != nil {
		_ = pool.Close()
		return fmt.Errorf("bootstrap.Run: registering tasks: %w", err)
	}

	workers := app.startBackgroundWorkers(ctx)

	engine := cfg.NewGinEngine(app.Infrastructure.Trace)
	if err = app.RegisterRoutes(ctx, engine); err != nil {
		_ = pool.Close()
		return fmt.Errorf("bootstrap.Run: registering http: %w", err)
	}

	server := &stdHTTPServer{
		srv: &http.Server{
			Addr:    cfg.GetGinListeningAddress(),
			Handler: engine,
		},
	}

	listenErr := make(chan error, 1)
	go func() {
		listenErr <- server.listenAndServe()
	}()

	if err = waitForListen(ctx, listenErr); err != nil {
		_ = pool.Close()
		return err
	}

	app.ShutdownGate.StartDraining()

	shutdownCtx, cancel := shutdownContext(ctx, cfg.GetShutdownTimeout())
	defer cancel()

	shutdownErr := app.shutdown(shutdownCtx, server, workers)
	if errors.Is(shutdownErr, ErrShutdownTimedOut) {
		app.ShutdownGate.MarkShutdownTimedOut()
	}
	logListenError(app.Infrastructure.Logger, <-listenErr)

	return shutdownErr
}
