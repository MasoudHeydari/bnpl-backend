package bootstrap

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/invariants"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/dbadminconfig"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	adapterslog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/slog"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

var cfgPath string

// FixConfigPath preserves flag > CONFIG_FILE > application default precedence.
func FixConfigPath(path string) {
	cfgPath = path
	if cfgPath == "" {
		cfgPath = os.Getenv("CONFIG_FILE")
	}

	if cfgPath == "" {
		cfgPath = "configs/dbadmin.yaml"
	}
}

func configured(fn func(context.Context, *dbadminconfig.Config, corelog.Logger) error) (err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, e := dbadminconfig.Load(cfgPath)
	if e != nil {
		return e
	}
	logger := adapterslog.New(os.Stderr, nil, corelog.Format(c.Logging.Format))
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("dbadmin: command panic; resources closed")
		}
	}()
	return fn(ctx, c, logger)
}

// Migrate uses only migrator sessions, explicitly switched to the managed owner.
func Migrate() error { return configured(migrateWithConfig) }

func migrateWithConfig(ctx context.Context, c *dbadminconfig.Config, logger corelog.Logger) error {
	p, e := c.ConnectionPool(ctx, repo.MigratorRole, logger)
	if e != nil {
		return e
	}
	defer p.Close()
	uc, e := migrationsuc.New(p, migrationsrp.New(), adapterclock.NewSystem(time.UTC, time.UTC), c.Migrations.FilesDir, logger)
	if e != nil {
		return e
	}

	if e = uc.WithLockTimeout(c.Migrations.LockTimeout).MigrateAll(ctx); e != nil {
		if detail, ok := invariants.PreflightDiagnostic(e); ok {
			return fmt.Errorf("dbadmin: %s: %w", detail, e)
		}
		return e
	}
	return nil
}

// VerifyDatabaseInvariants inspects the existing trigger contract without repair.
func VerifyDatabaseInvariants() error {
	return configured(func(ctx context.Context, c *dbadminconfig.Config, l corelog.Logger) error {
		p, e := c.ConnectionPool(ctx, repo.MigratorRole, l)
		if e != nil {
			return e
		}
		defer p.Close()
		if e = invariants.Verify(ctx, p); e != nil {
			return e
		}
		return invariants.VerifyPrivileges(ctx, p)
	})
}
