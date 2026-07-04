//go:build production

package migrationsuc

import (
	"context"
	"fmt"
	"log"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// InitProd creates the application schema and loads production-style seed data
// when schema bnpl does not exist yet.
//
// Requires runtime.environment=production (APP_ENVIRONMENT=production).
// If schema bnpl already exists, returns a conflict error and does not drop or
// mutate existing objects; operators must use `db migrate` for schema evolution.
// It is not safe to run concurrently against the same database.
func (iduc *InitDBUseCase) InitProd(ctx context.Context) error {
	if !iduc.env.IsProduction() {
		return initProdRequiresProductionError(iduc.env)
	}

	return iduc.initDB(
		ctx,
		iduc.ensureFreshProdSchema,
		func(ctx context.Context, si repo.SchemaInitializer) error {
			return si.InitProdSchema(ctx)
		},
	)
}

// ensureFreshProdSchema creates managed schema bnpl only when it is absent.
// An existing schema is treated as a conflict so production data cannot be wiped.
func (iduc *InitDBUseCase) ensureFreshProdSchema(ctx context.Context) error {
	const op = "migrationsuc.InitDBUseCase.ensureFreshProdSchema"

	connPool, err := iduc.ss.ConnectionPool(ctx, repo.AdminRole, iduc.logger)
	if err != nil {
		return fmt.Errorf("%s: creating DB pool for admin: %w", op, err)
	}

	defer func() {
		if err = connPool.Close(); err != nil {
			log.Printf("failed to close pool connection: %v", err)
		}
	}()

	err = connPool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := iduc.ss.NewSchemaRepo().Tx(tx)
			schemaName := SchemaName()

			exists, err := q.Exists(ctx, schemaName)
			if err != nil {
				return fmt.Errorf("%s: checking whether schema %q exists: %w", op, schemaName, err)
			}

			if exists {
				return schemaAlreadyExistsConflictError(schemaName)
			}

			return iduc.createSchemaRoleAndPrivileges(ctx, q, schemaName)
		})
	})
	if err != nil {
		return fmt.Errorf("%s: admin connection: %w", op, err)
	}

	return nil
}
