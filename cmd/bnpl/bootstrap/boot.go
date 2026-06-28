package bootstrap

import (
	"context"
	"fmt"
	"os"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

var cfgPath string

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
			fmt.Printf("failed to close pool connection: %v\n", err)
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
