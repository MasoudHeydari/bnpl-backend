package bootstrap

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
)

// App composes configuration, infrastructure, repositories, and use cases.
type App struct {
	Config         *config.Config
	Pool           repo.Pool
	Repos          Repos
	UseCases       UseCases
	Infrastructure Infrastructure
}

// TaskJobResolver resolves configured task handlers to executable jobs.
type TaskJobResolver interface {
	ResolveTaskJob(handler string) (coretask.Job, error)
}

func newApp(c *config.Config, p repo.Pool) (*App, error) {
	app := &App{
		Config: c,
		Pool:   p,
		Repos:  NewRepos(),
	}

	infrastructure, err := app.NewInfrastructures()
	if err != nil {
		return nil, fmt.Errorf("bootstrap.newApp: initialize infrastructure: %w", err)
	}
	app.Infrastructure = infrastructure

	usecases, err := app.NewUseCases()
	if err != nil {
		return nil, fmt.Errorf("bootstrap.newApp: initialize uses cases: %w", err)
	}
	app.UseCases = usecases

	return app, nil
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
	taskDefinitions, err := a.Config.GetTaskDefinitions()
	if err != nil {
		return fmt.Errorf("bootstrap.RegisterTasks: get task definitions: %w", err)
	}

	registry := jobs.NewRegistry(
		a.Infrastructure.Logger,
		a.UseCases.OutboxPublisher,
	)

	for _, entry := range taskDefinitions {
		if !entry.Enabled {
			continue
		}

		trigger, err := entry.Trigger.ToCore()
		if err != nil {
			return fmt.Errorf("bootstrap.RegisterTasks: build trigger for task %q: %w", entry.Name, err)
		}

		job, err := registry.Resolve(entry.Handler)
		if err != nil {
			return fmt.Errorf("bootstrap.RegisterTasks: resolve job for task %q: %w", entry.Name, err)
		}

		if err = a.Infrastructure.Task.Register(
			coretask.Task{
				Name:    entry.Name,
				Enabled: entry.Enabled,
				Handler: entry.Handler,
				Trigger: trigger,
				Job:     job,
			},
		); err != nil {
			return fmt.Errorf("bootstrap.RegisterTasks: register task %q: %w", entry.Name, err)
		}
	}

	return nil
}

// StartTasks starts background workers using the provided root context.
func (a *App) StartTasks(ctx context.Context) {
	a.startBackgroundWorkers(ctx)
}
