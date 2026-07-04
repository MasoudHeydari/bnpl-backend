//go:build production

package bootstrap

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/dbadminconfig"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// InitProd reuses the environment-gated existing initialization and migration engine.
func InitProd() error {
	return configured(func(ctx context.Context, c *dbadminconfig.Config, l corelog.Logger) error {
		env, e := model.ParseRuntimeEnvironment(c.Runtime.Environment)
		if e != nil {
			return e
		}

		if e = migrationsuc.NewInitDB(c, env, l).InitProd(ctx); e != nil {
			return e
		}
		return migrateWithConfig(ctx, c, l)
	})
}
