// Package bootstrap wires configuration, infrastructure, repositories, and
// use cases into the application entrypoint.
package bootstrap

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

var cfgPath string

// App holds the assembled application dependencies used by the entrypoint.
type App struct {
	Config   *config.Config
	Pool     repo.Pool
	Repos    Repos
	UseCases UseCases
}

func newApp(c *config.Config, p repo.Pool) (*App, error) {
	app := &App{
		Config: c,
		Pool:   p,
		Repos:  NewRepos(),
	}

	usecases, err := app.NewUseCases()
	if err != nil {
		return nil, fmt.Errorf("bootstrap.newApp: initialize uses cases: %w", err)
	}
	app.UseCases = usecases

	return app, nil
}

// Boot loads configuration, assembles the application, and starts serving.
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

	e := c.NewGinEngine()

	if err = app.RegisterRoutes(ctx, e); err != nil {
		return fmt.Errorf("bootstrap.Boot: registering http: %w", err)
	}

	if err = e.Run(c.GetGinListeningAddress()); err != nil {
		return fmt.Errorf("bootstrap.Boot: running Gin engine: %w", err)
	}

	return nil
}

// InitProd recreates the managed schema and loads the production seed data.
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
		return fmt.Errorf("initializing DB with prod data: %w", err)
	}

	return nil
}

// InitDev recreates the managed schema and loads the development seed data.
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

// Migrate applies all configured migrations.
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

// FixConfigPath initializes cfgPath from the environment or the default path.
func FixConfigPath() {
	if cfgPath != "" {
		return
	}

	var found bool
	if cfgPath, found = os.LookupEnv("CONFIG_FILE"); !found {
		// The default path should usually be in the /etc directory.
		cfgPath = "configs/config.local.yaml"
	}
}
