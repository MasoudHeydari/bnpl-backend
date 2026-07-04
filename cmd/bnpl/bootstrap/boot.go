package bootstrap

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// cfgPath is the resolved configuration file path used by Boot, Init*, and
// Migrate. Set it through FixConfigPath before invoking those entrypoints.
var cfgPath string

const commandPanicFlushTimeout = 2 * time.Second

// Migrate applies all pending database migrations using the normal DB role.
// Used by the CLI migrate command.
func Migrate() (err error) {
	const op = "bootstrap.Migrate"

	ctx := context.Background()
	c, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("%s: config.Load(%q): %w", op, cfgPath, err)
	}

	logger, err := c.NewLoggerInfrastructure()
	if err != nil {
		return fmt.Errorf("%s: initialize logger: %w", op, err)
	}

	reporter, err := c.NewErrorReporterInfrastructure()
	if err != nil {
		return fmt.Errorf("%s: initialize error reporter: %w", op, err)
	}

	defer func() { _ = reporter.Close() }()
	defer recoverCommandPanic(
		ctx,
		"db_migrate",
		logger,
		reporter,
		&err,
	)

	if err = migrateWithConfig(ctx, c, logger); err != nil {
		return fmt.Errorf("%s: migrate.MigrateAll: %w", op, err)
	}

	logger.Info("cli command finished", "command", "db_migrate")

	return nil
}

func migrateWithConfig(ctx context.Context, c *config.Config, logger corelog.Logger) error {
	const op = "bootstrap.migrateWithConfig"

	p, err := c.ConnectionPool(ctx, repo.NormalRole, logger)
	if err != nil {
		return fmt.Errorf("%s: creating DB pool: %w", op, err)
	}

	defer func() { _ = p.Close() }()

	systemClock, err := c.NewSystemClockInfrastructure()
	if err != nil {
		return fmt.Errorf("%s: initialize system clock: %w", op, err)
	}

	migrationsRepo := migrationsrp.New()
	migrationsUC, err := c.NewMigrationsUseCase(p, migrationsRepo, systemClock, logger)
	if err != nil {
		return fmt.Errorf("%s: construct migrations use case: %w", op, err)
	}

	if err = migrationsUC.MigrateAll(ctx); err != nil {
		return fmt.Errorf("%s: migrate all: %w", op, err)
	}

	return nil
}

func recoverCommandPanic(
	ctx context.Context,
	command string,
	logger corelog.Logger,
	reporter coreerrorreporting.Reporter,
	err *error,
) {
	const op = "bootstrap.recoverCommandPanic"

	recovered := recover()
	if recovered == nil {
		return
	}

	if reporter != nil {
		reporter.CapturePanic(
			ctx,
			recovered,
			coreerrorreporting.Metadata{
				Component: "cli",
				Tags: map[string]string{
					"cli.command": command,
				},
			},
		)

		flushCtx, cancel := context.WithTimeout(context.Background(), commandPanicFlushTimeout)
		reporter.Flush(flushCtx)
		cancel()
	}

	if logger != nil {
		logger.With(
			"command", command,
			"panic", fmt.Sprint(recovered),
		).Error("cli command panic recovered")
	}

	if err != nil {
		*err = fmt.Errorf("%s: %s panic recovered: %v", op, command, recovered)
	}
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
