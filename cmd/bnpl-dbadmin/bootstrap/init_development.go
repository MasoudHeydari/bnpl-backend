//go:build development

package bootstrap

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/adapter/dbadminconfig"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// InitDev reuses the environment-gated existing initialization and migration engine.
func InitDev() error {
	return configured(func(ctx context.Context, c *dbadminconfig.Config, l corelog.Logger) error {
		env, e := model.ParseRuntimeEnvironment(c.Runtime.Environment)
		if e != nil {
			return e
		}
		if e = migrationsuc.NewInitDB(c, env, l).InitDev(ctx); e != nil {
			return e
		}
		return migrateWithConfig(ctx, c, l)
	})
}
