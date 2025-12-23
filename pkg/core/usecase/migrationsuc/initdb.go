package migrationsuc

import (
	"context"
	"fmt"

	"gitlab.com/mehdi.fakhraei-group/bnpl/pkg/core/repo"
)

type InitDBUseCase struct {
	ss repo.SchemaSettings
}

func NewInitDB(ss repo.SchemaSettings) *InitDBUseCase {
	return &InitDBUseCase{ss: ss}
}

func (iduc *InitDBUseCase) InitProd(ctx context.Context) error {
	return iduc.initDB(
		ctx,
		func(ctx context.Context, si repo.SchemaInitializer) error {
			return si.InitProdSchema(ctx)
		},
	)
}

func (iduc *InitDBUseCase) InitDev(ctx context.Context) error {
	return iduc.initDB(
		ctx,
		func(ctx context.Context, si repo.SchemaInitializer) error {
			return si.InitDevSchema(ctx)
		},
	)
}

func (iduc *InitDBUseCase) initDB(
	ctx context.Context,
	dbi func(ctx context.Context, si repo.SchemaInitializer) error,
) error {
	if err := iduc.dropAndCreateAgain(ctx); err != nil {
		return fmt.Errorf("dropping/recreating schema: %w", err)
	}

	connPool, err := iduc.ss.ConnectionPool(ctx, repo.NormalRole)
	if err != nil {
		return fmt.Errorf("creating DB pool for normal role: %w", err)
	}
	defer connPool.Close()

	err = connPool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			si := iduc.ss.SchemaInitializer(tx)
			if err := dbi(ctx, si); err != nil {
				return fmt.Errorf("initializing schema: %w", err)
			}

			return nil
		})
	})

	if err != nil {
		return fmt.Errorf("normal connection: %w", err)
	}

	return nil
}

func (iduc *InitDBUseCase) dropAndCreateAgain(
	ctx context.Context,
) error {
	connPool, err := iduc.ss.ConnectionPool(ctx, repo.AdminRole)
	if err != nil {
		return fmt.Errorf("creating DB pool for admin: %w", err)
	}
	defer connPool.Close()

	err = connPool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := iduc.ss.NewSchemaRepo().Tx(tx)
			schemaName := SchemaName()
			if err := q.DropIfExists(ctx, schemaName); err != nil {
				return fmt.Errorf("dropping %q: %w", schemaName, err)
			}

			if err := q.CreateSchema(ctx, schemaName); err != nil {
				return fmt.Errorf("creating %q: %w", schemaName, err)
			}

			var normalRolePassword string
			normalRolePassword, err = iduc.ss.GetRolePassword(repo.NormalRole)
			if err != nil {
				return fmt.Errorf("getting normal role password %q: %w", schemaName, err)
			}

			if err := q.CreateRoleIfNotExists(
				ctx, repo.NormalRole, normalRolePassword,
			); err != nil {
				return fmt.Errorf("creating normal role: %w", err)
			}

			if err := q.GrantPrivileges(
				ctx, schemaName, repo.NormalRole,
			); err != nil {
				return fmt.Errorf("granting normal role privs: %w", err)
			}

			if err := q.SetSearchPath(
				ctx, schemaName, repo.NormalRole,
			); err != nil {
				return fmt.Errorf(
					"setting search_path of normal role to %q: %w",
					schemaName, err,
				)
			}

			return nil
		})
	})

	if err != nil {
		return fmt.Errorf("admin connection: %w", err)
	}

	return nil
}

func SchemaName() string {
	return "bnpl"
}
