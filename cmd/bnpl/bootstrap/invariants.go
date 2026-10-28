package bootstrap

import (
	"context"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/invariants"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// VerifyDatabaseInvariants uses normal configuration/runtime-role privileges and
// intentionally does not construct App or run schema migrations.
func VerifyDatabaseInvariants() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	logger, err := c.NewLoggerInfrastructure()
	if err != nil {
		return err
	}
	p, err := c.ConnectionPool(ctx, repo.NormalRole, logger)
	if err != nil {
		return err
	}
	defer p.Close()
	return invariants.Verify(ctx, p)
}
