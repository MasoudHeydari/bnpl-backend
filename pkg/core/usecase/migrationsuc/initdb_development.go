//go:build development

package migrationsuc

import (
	"context"
	"fmt"
	"log"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// InitDev recreates the application schema and loads development seed data.
//
// Requires runtime.environment=development (APP_ENVIRONMENT=development).
// Always drops and recreates managed schema bnpl, whether empty or populated.
// It is not safe to run concurrently against the same database.
func (iduc *InitDBUseCase) InitDev(ctx context.Context) error {
	if !iduc.env.IsDevelopment() {
		return initDevRequiresDevelopmentError(iduc.env)
	}

	return iduc.initDB(
		ctx,
		iduc.dropAndCreateAgain,
		func(ctx context.Context, si repo.SchemaInitializer) error {
			return si.InitDevSchema(ctx)
		},
	)
}

func (iduc *InitDBUseCase) dropAndCreateAgain(
	ctx context.Context,
) error {
	const op = "migrationsuc.InitDBUseCase.dropAndCreateAgain"

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
		// Administrative bootstrap is also transactional to keep role, schema,
		// and privilege changes consistent when a later statement fails.
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := iduc.ss.NewSchemaRepo().Tx(tx)
			schemaName := SchemaName()
			if err := q.DropIfExists(ctx, schemaName); err != nil {
				return fmt.Errorf("%s: dropping %q: %w", op, schemaName, err)
			}

			return iduc.createSchemaRoleAndPrivileges(ctx, q, schemaName)
		})
	})
	if err != nil {
		return fmt.Errorf("%s: admin connection: %w", op, err)
	}

	return nil
}
