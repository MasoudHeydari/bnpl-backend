package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/invariants"
	adapterbotmessage "github.com/parstis/bnpl-backend/pkg/adapter/external/botmessage"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/messaging/consumers"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs"
	corebotmessage "github.com/parstis/bnpl-backend/pkg/core/external/botmessage"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
)

// App is the composition root for one BNPL process: configuration, shared
// infrastructure, repositories, external clients, and use cases.
//
// NewApp assembles these parts but does not start HTTP listeners or background
// workers. Call RegisterTasks, RegisterConsumers, and Run/Boot for runtime.
//
// Resource ownership:
//   - App owns shared dependencies (logger-backed adapters, messaging, cache,
//     outbound HTTP client, external transports, database pool) and releases
//     them through Close / releaseResources.
//   - Run owns ephemeral runtime surfaces (HTTP/metrics listeners and
//     background workers) and stops them during graceful shutdown before
//     releasing App-owned resources.
type App struct {
	// Config is the loaded immutable runtime configuration.
	Config *config.Config

	// Pool is the shared PostgreSQL pool for the normal application role.
	Pool repo.Pool

	// Repos holds Postgres repository adapters implementing core repo ports.
	Repos Repos

	// UseCases holds constructed application use cases.
	UseCases UseCases

	// Infrastructure holds shared runtime adapters (logger, metrics, messaging).
	Infrastructure Infrastructure

	// External holds third-party system clients (Financial Core, credit bureau, bot messaging).
	External External

	// BotMessageHandler is the application callback invoked by Telegram/Bale
	// inbound adapters after provider payloads are normalized. Concrete command
	// handlers must delegate business behavior to UseCases.
	BotMessageHandler corebotmessage.IncomingMessageHandler

	// releaseOnce ensures shared dependency cleanup runs at most once across
	// startup rollback and graceful shutdown.
	releaseOnce sync.Once

	// releaseErr is the joined error from the first releaseResources call.
	releaseErr error
}

// TaskJobResolver resolves configured task handlers to executable jobs.
type TaskJobResolver interface {
	ResolveTaskJob(handler string) (coretask.Job, error)
}

// NewApp composes a fully initialized application without starting an HTTP listener or background workers.
// Callers own the returned App and must call Close when it is no longer needed
// (Run's shutdown path already releases resources via the same ownership).
//
// Construction order follows the real dependency graph:
//  1. Infrastructure adapters that do not need the DB pool (logger, metrics,
//     cache, messaging transports, task runtime).
//  2. Database pool (reuses the process logger for query logging).
//  3. Read-only database integrity/privilege verification, then repositories.
//  4. DependencyChecker — must see the live pool so readiness never treats a
//     nil pool as healthy.
//  5. External clients and use cases.
//
// On any failure, already-constructed resources are released before returning.
func NewApp(ctx context.Context, c *config.Config) (_ *App, err error) {
	const op = "bootstrap.NewApp"

	app := &App{
		Config: c,
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, app.Close())
		}
	}()

	app.Infrastructure, err = app.NewInfrastructures(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: initialize infrastructure: %w", op, err)
	}

	app.Pool, err = app.ConnectionPool(ctx, repo.NormalRole, app.Infrastructure.Logger)
	if err != nil {
		return nil, fmt.Errorf("%s: creating DB pool: %w", op, err)
	}

	if err = invariants.VerifyRuntime(ctx, app.Pool); err != nil {
		return nil, fmt.Errorf("%s: database integrity verification: %w", op, err)
	}

	app.Repos = NewRepos()

	if err = app.wireDependencyChecker(); err != nil {
		return nil, fmt.Errorf("%s: wire dependency checker: %w", op, err)
	}

	app.External, err = app.NewExternal()
	if err != nil {
		return nil, fmt.Errorf("%s: initialize external: %w", op, err)
	}

	app.UseCases, err = app.NewUseCases()
	if err != nil {
		return nil, fmt.Errorf("%s: initialize use cases: %w", op, err)
	}

	// Command-specific bot handlers are intentionally not implemented in the
	// transport adapters. Future handlers are composed here with UseCases and can
	// replace this fallback without changing Telegram/Bale clients.
	app.BotMessageHandler, err = adapterbotmessage.NewUnhandledIncomingMessageHandler(
		app.Infrastructure.Logger,
	)
	if err != nil {
		return nil, fmt.Errorf("%s: initialize bot message handler: %w", op, err)
	}

	return app, nil
}

// ConnectionPool opens a database pool for role, using the provided process
// logger for optional query logging. Callers typically pass
// Infrastructure.Logger so the same logger instance is reused.
func (a *App) ConnectionPool(
	ctx context.Context,
	r repo.Role,
	logger log.Logger,
) (repo.Pool, error) {
	return a.Config.ConnectionPool(ctx, r, logger)
}

// RegisterTasks registers startup-known scheduled tasks.
//
// Only tasks that are known at application bootstrap and are defined in config
// should be registered here.
//
// Runtime-created or user-triggered work should not be registered here.
// Those jobs should be executed through task dispatch:
//
//	a.Infrastructure.Task.Dispatch(ctx, coretask.Command{...})
func (a *App) RegisterTasks() error {
	const op = "bootstrap.App.RegisterTasks"

	taskDefinitions, err := a.Config.GetTaskDefinitions()
	if err != nil {
		return fmt.Errorf("%s: get task definitions: %w", op, err)
	}

	registry := jobs.NewRegistry(
		a.Infrastructure.Logger,
		a.UseCases.OutboxPublisher,
		a.UseCases.InstallmentLifecycle,
		a.UseCases.Contract,
		a.UseCases.CheckoutSession,
		a.UseCases.PaymentAttemptReconciliation,
		a.UseCases.SagaExecutor,
		a.UseCases.MerchantTerminal,
		a.UseCases.RepaymentReminder,
		a.UseCases.Collections,
	)

	for _, entry := range taskDefinitions {
		newTask := coretask.Task{
			Name:          entry.Name,
			Enabled:       entry.Enabled,
			ExecutionMode: entry.Execution,
			Handler:       entry.Handler,
		}

		if entry.Enabled {
			trigger, err := entry.Trigger.ToCore()
			if err != nil {
				return fmt.Errorf("%s: build trigger for task %q: %w", op, entry.Name, err)
			}

			job, err := registry.Resolve(entry.Handler)
			if err != nil {
				return fmt.Errorf("%s: resolve job for task %q: %w", op, entry.Name, err)
			}

			newTask.Trigger = trigger
			newTask.Job = job
		}

		// Disabled definitions are intentionally newTask as metadata too.
		// Durable runtimes such as Temporal need to see them so a schedule that
		// was enabled in a previous deployment can be removed deterministically.
		if err = a.Infrastructure.Task.Register(newTask); err != nil {
			return fmt.Errorf("%s: register new task %q: %w", op, entry.Name, err)
		}
	}

	return nil
}

// StartTasks starts background workers (task scheduler and message consumers)
// using the provided root context. Prefer Run/Boot for the full process
// lifecycle; this helper exists for callers that already own orchestration.
func (a *App) StartTasks(ctx context.Context, consumerRegistry *consumers.Registry) {
	a.startBackgroundWorkers(ctx, consumerRegistry)
}
