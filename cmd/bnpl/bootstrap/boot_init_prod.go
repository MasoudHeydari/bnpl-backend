//go:build production

package bootstrap

import (
	"context"
	"fmt"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// InitProd initializes a missing production schema (non-destructive).
//
// Requires runtime.environment=production and refuses when schema bnpl already
// exists. Used by the CLI db init-prod command, not by the long-running server.
func InitProd() (err error) {
	const op = "bootstrap.InitProd"

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
	defer recoverCommandPanic(ctx, "db_init_prod", logger, reporter, &err)

	env, err := c.Runtime.ParsedEnvironment()
	if err != nil {
		return fmt.Errorf("%s: parse runtime.environment: %w", op, err)
	}

	muc := migrationsuc.NewInitDB(c, env, logger)
	err = muc.InitProd(ctx)
	if err != nil {
		return fmt.Errorf("%s: initializing DB with production data: %w", op, err)
	}

	if err = migrateWithConfig(ctx, c, logger); err != nil {
		return fmt.Errorf("%s: apply incremental migrations after fresh initialization: %w", op, err)
	}

	return nil
}
