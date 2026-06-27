package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/jobs"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	coretask "github.com/parstis/bnpl-backend/pkg/core/task"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

var cfgPath string

type App struct {
	Config         *config.Config
	Pool           repo.Pool
	Repos          Repos
	UseCases       UseCases
	Infrastructure Infrastructure
}

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

func Boot() error {
	ctx := context.Background()

	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}

	p, err := c.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("bootstrap.Boot: creating DB pool: %w", err)
	}
	defer func() {
		if err = p.Close(); err != nil {
			log.Printf("bootstrap.Boot: failed to close pool connection: %v", err)
		}
	}()

	app, err := newApp(c, p)
	if err != nil {
		return fmt.Errorf("bootstrap.Boot: initialize App: %w", err)
	}

	if err = app.RegisterTasks(); err != nil {
		return fmt.Errorf("bootstrap.Boot: registering tasks: %w", err)
	}

	app.StartTasks(ctx)

	e := c.NewGinEngine(app.Infrastructure.Trace)

	if err = app.RegisterRoutes(ctx, e); err != nil {
		return fmt.Errorf("bootstrap.Boot: registering http: %w", err)
	}

	if err = e.Run(c.GetGinListeningAddress()); err != nil {
		return fmt.Errorf("bootstrap.Boot: running Gin engine: %w", err)
	}

	return nil
}

func InitProd() error {
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}
	fmt.Printf("configs: %v\n", c)

	muc := migrationsuc.NewInitDB(c)
	err = muc.InitProd(ctx)
	if err != nil {
		return fmt.Errorf("initializing DB with dev data: %w", err)
	}

	return nil
}

func InitDev() error {
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}
	fmt.Printf("configs: %v\n", c)

	muc := migrationsuc.NewInitDB(c)
	err = muc.InitDev(ctx)
	if err != nil {
		return fmt.Errorf("initializing DB with dev data: %w", err)
	}

	return nil
}

func Migrate() error {
	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("config.Load(%q): %w", cfgPath, err)
	}

	p, err := c.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("creating DB pool: %w", err)
	}
	defer func() {
		if err = p.Close(); err != nil {
			log.Printf("failed to close pool connection: %v", err)
		}
	}()

	migrationsRepo := migrationsrp.New()
	migrationsUC := c.NewMigrationsUseCase(p, migrationsRepo)
	err = migrationsUC.MigrateAll(ctx)
	if err != nil {
		return fmt.Errorf("migrate.MigrateAll: %w", err)
	}

	return nil
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

// StartTasks starts the configured task runtime in the background.
//
// StartTasks only starts execution for tasks registered by RegisterTasks.
// On-demand work is executed separately through Dispatch.
func (a *App) StartTasks(ctx context.Context) {
	go func() {
		if err := a.Infrastructure.Task.Start(ctx); err != nil {
			a.Infrastructure.Logger.
				With("error", err.Error()).
				Error("task runtime stopped with error")
		}
	}()
}

// FixConfigPath resolves the configuration file path from the CLI flag,
// CONFIG_FILE environment variable, or the repository default.
func FixConfigPath(cliPath string) {
	if cliPath != "" {
		cfgPath = cliPath
		return
	}

	if path, found := os.LookupEnv("CONFIG_FILE"); found {
		cfgPath = path
		return
	}

	cfgPath = "configs/config.yaml"
}
