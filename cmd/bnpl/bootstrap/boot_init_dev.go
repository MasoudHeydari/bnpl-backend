//go:build development

package bootstrap

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// InitDev resets and initializes the development schema (destructive recreate).
//
// Requires runtime.environment=development. Used by the CLI db init-dev
// command; never invoke from the long-running server path.
func InitDev() (err error) {
	const op = "bootstrap.InitDev"

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
		"db_init_dev",
		logger,
		reporter,
		&err,
	)

	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return fmt.Errorf("%s: parse runtime.environment: %w", op, err)
	}

	muc := migrationsuc.NewInitDB(c, env, logger)
	err = muc.InitDev(ctx)
	if err != nil {
		return fmt.Errorf("%s: initializing DB with dev data: %w", op, err)
	}

	if err = migrateWithConfig(ctx, c, logger); err != nil {
		return fmt.Errorf("%s: apply incremental migrations after fresh initialization: %w", op, err)
	}

	return nil
}
