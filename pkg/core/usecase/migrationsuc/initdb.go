// Package migrationsuc implements database initialization and migration use
// cases. It belongs to the application layer and coordinates repository
// abstractions from the core layer. The package owns orchestration and
// transaction boundaries, but not migration statement definitions.
package migrationsuc

import (
	"context"
	"fmt"
	"log"

	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// InitDBUseCase recreates the managed schema and seeds it with the
// base dataset required for either development or production environments.
// The use case is stateful only in its dependencies and should be reused only
// with immutable configuration.
type InitDBUseCase struct {
	ss repo.SchemaSettings
}

// NewInitDB constructs a database initialization use case.
// The provided schema settings abstraction must be able to open both admin and
// application-role connections for the target database.
func NewInitDB(ss repo.SchemaSettings) *InitDBUseCase {
	return &InitDBUseCase{ss: ss}
}

// InitProd recreates the application schema and loads production-style seed
// data from embedded migration assets. Like InitDev, it drops the managed schema first and
// is destructive. Do not run against live production databases. It is not safe
// to run concurrently against the same database.
func (iduc *InitDBUseCase) InitProd(ctx context.Context) error {
	return iduc.initDB(
		ctx,
		func(ctx context.Context, si repo.SchemaInitializer) error {
			return si.InitProdSchema(ctx)
		},
	)
}

// InitDev recreates the application schema and loads development seed data from
// embedded migration assets. It drops the managed schema first and is destructive. It is
// not safe to run concurrently against the same database.
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
	defer func() {
		if err = connPool.Close(); err != nil {
			log.Printf("failed to close pool connection: %v", err)
		}
	}()

	err = connPool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		// Schema data initialization is executed inside a single transaction so
		// DDL and seed data changes are committed or rolled back together.
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

// SchemaName returns the schema name managed by the migration use cases. The
// value is currently fixed for the entire application.
func SchemaName() string {
	return "bnpl"
}
