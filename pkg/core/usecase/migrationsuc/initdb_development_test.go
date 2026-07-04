//go:build development

package migrationsuc_test

import (
	"context"
	"testing"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

func (d initDBDeps) expectNormalInitDev(ctx context.Context, t *testing.T) {
	t.Helper()

	settler := mock.NewSchemaInitializer(t)
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
	d.settings.EXPECT().SchemaInitializer(d.tx).Return(settler).Once()
	settler.EXPECT().InitDevSchema(ctx).Return(nil).Once()
	d.pool.EXPECT().Close().Return(nil).Once()
}

func TestInitDev_RequiresDevelopmentEnvironment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newInitDBDeps(t)
	uc := migrationsuc.NewInitDB(deps.settings, model.ProductionRuntimeEnvironment, nil)

	err := uc.InitDev(ctx)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestInitDev_AlwaysDropsAndRecreates(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newInitDBDeps(t)
	uc := migrationsuc.NewInitDB(deps.settings, model.DevelopmentRuntimeEnvironment, nil)

	deps.expectAdminTx(ctx)
	deps.schemaTx.EXPECT().DropIfExists(ctx, "bnpl").Return(nil).Once()
	deps.expectCreateRolePrivs(ctx)
	deps.expectNormalInitDev(ctx, t)

	require.NoError(t, uc.InitDev(ctx))
}
