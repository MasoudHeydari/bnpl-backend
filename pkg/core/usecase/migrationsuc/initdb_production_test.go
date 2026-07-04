//go:build production

package migrationsuc_test

import (
	"context"
	"testing"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

// prodSchemaInitializer is a minimal test double for production-only
// SchemaInitializer. The generated mock is built under the development profile.
type prodSchemaInitializer struct{}

func (prodSchemaInitializer) InitProdSchema(context.Context) error { return nil }

func (d initDBDeps) expectNormalInitProd(ctx context.Context) {
	d.settings.EXPECT().ConnectionPool(ctx, repo.NormalRole, nil).Return(d.pool, nil).Once()
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
	d.settings.EXPECT().SchemaInitializer(d.tx).Return(prodSchemaInitializer{}).Once()
	d.pool.EXPECT().Close().Return(nil).Once()
}

func TestInitProd_RequiresProductionEnvironment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newInitDBDeps(t)
	uc := migrationsuc.NewInitDB(deps.settings, model.DevelopmentRuntimeEnvironment, nil)

	err := uc.InitProd(ctx)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestInitProd_ExistingSchema_ReturnsConflict(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newInitDBDeps(t)
	uc := migrationsuc.NewInitDB(deps.settings, model.ProductionRuntimeEnvironment, nil)

	deps.expectAdminTx(ctx)
	deps.schemaTx.EXPECT().Exists(ctx, "bnpl").Return(true, nil).Once()

	err := uc.InitProd(ctx)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

func TestInitProd_MissingSchema_Bootstraps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newInitDBDeps(t)
	uc := migrationsuc.NewInitDB(deps.settings, model.ProductionRuntimeEnvironment, nil)

	deps.expectAdminTx(ctx)
	deps.schemaTx.EXPECT().Exists(ctx, "bnpl").Return(false, nil).Once()
	deps.expectCreateRolePrivs(ctx)
	deps.expectNormalInitProd(ctx)

	require.NoError(t, uc.InitProd(ctx))
}
