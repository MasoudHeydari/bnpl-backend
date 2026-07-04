package migrationsuc

import (
	"context"
	"fmt"
	"log"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// InitDBUseCase bootstraps the managed schema for the active build profile.
//
// Environment gating uses config runtime.environment (APP_ENVIRONMENT).
// Profile-specific entry points (InitDev / InitProd) are compiled only into
// the matching build-profile binary.
type InitDBUseCase struct {
	ss     repo.SchemaSettings
	env    model.RuntimeEnvironment
	logger corelog.Logger
}

// NewInitDB constructs a database initialization use case.
// The provided schema settings abstraction must be able to open both admin and
// application-role connections for the target database.
// logger is the shared process logger reused for optional query logging on
// pools opened during bootstrap; it must not be recreated per pool.
func NewInitDB(
	ss repo.SchemaSettings,
	env model.RuntimeEnvironment,
	logger corelog.Logger,
) *InitDBUseCase {
	return &InitDBUseCase{ss: ss, env: env, logger: logger}
}

func (iduc *InitDBUseCase) initDB(
	ctx context.Context,
	bootstrapAdmin func(ctx context.Context) error,
	dbi func(ctx context.Context, si repo.SchemaInitializer) error,
) error {
	const op = "migrationsuc.InitDBUseCase.initDB"

	if err := bootstrapAdmin(ctx); err != nil {
		return err
	}

	connPool, err := iduc.ss.ConnectionPool(ctx, repo.NormalRole, iduc.logger)
	if err != nil {
		return fmt.Errorf("%s: creating DB pool for normal role: %w", op, err)
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
				return fmt.Errorf("%s: initializing schema: %w", op, err)
			}

			return nil
		})
	})
	if err != nil {
		return fmt.Errorf("%s: normal connection: %w", op, err)
	}

	return nil
}

func (iduc *InitDBUseCase) createSchemaRoleAndPrivileges(
	ctx context.Context,
	q repo.SchemaTxQueryer,
	schemaName string,
) error {
	const op = "migrationsuc.InitDBUseCase.createSchemaRoleAndPrivileges"

	if err := q.CreateSchema(ctx, schemaName); err != nil {
		return fmt.Errorf("%s: creating %q: %w", op, schemaName, err)
	}

	normalRolePassword, err := iduc.ss.GetRolePassword(repo.NormalRole)
	if err != nil {
		return fmt.Errorf("%s: getting normal role password %q: %w", op, schemaName, err)
	}

	if err := q.CreateRoleIfNotExists(
		ctx, repo.NormalRole, normalRolePassword,
	); err != nil {
		return fmt.Errorf("%s: creating normal role: %w", op, err)
	}

	if err := q.GrantPrivileges(
		ctx, schemaName, repo.NormalRole,
	); err != nil {
		return fmt.Errorf("%s: granting normal role privs: %w", op, err)
	}

	if err := q.SetSearchPath(
		ctx, schemaName, repo.NormalRole,
	); err != nil {
		return fmt.Errorf(
			"%s: setting search_path of normal role to %q: %w", op, schemaName, err,
		)
	}

	return nil
}

// SchemaName returns the schema name managed by the migration use cases. The
// value is currently fixed for the entire application.
func SchemaName() string {
	return "bnpl"
}
