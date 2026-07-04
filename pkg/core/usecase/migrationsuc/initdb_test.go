package migrationsuc_test

import (
	"context"
	"testing"

	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
)

type initDBDeps struct {
	settings *mock.SchemaSettings
	pool     *mock.Pool
	conn     *mock.Conn
	tx       *mock.Tx
	schema   *mock.Schema
	schemaTx *mock.SchemaTxQueryer
}

func newInitDBDeps(t *testing.T) initDBDeps {
	t.Helper()

	return initDBDeps{
		settings: mock.NewSchemaSettings(t),
		pool:     mock.NewPool(t),
		conn:     mock.NewConn(t),
		tx:       mock.NewTx(t),
		schema:   mock.NewSchema(t),
		schemaTx: mock.NewSchemaTxQueryer(t),
	}
}

func (d initDBDeps) expectAdminTx(ctx context.Context) {
	d.settings.EXPECT().ConnectionPool(ctx, repo.AdminRole, nil).Return(d.pool, nil).Once()
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
	d.settings.EXPECT().NewSchemaRepo().Return(d.schema).Once()
	d.schema.EXPECT().Tx(d.tx).Return(d.schemaTx).Once()
	d.pool.EXPECT().Close().Return(nil).Once()
}

func (d initDBDeps) expectCreateRolePrivs(ctx context.Context) {
	d.schemaTx.EXPECT().CreateSchema(ctx, "bnpl").Return(nil).Once()
	d.settings.EXPECT().GetRolePassword(repo.NormalRole).Return("secret", nil).Once()
	d.schemaTx.EXPECT().CreateRoleIfNotExists(ctx, repo.NormalRole, "secret").Return(nil).Once()
	d.schemaTx.EXPECT().GrantPrivileges(ctx, "bnpl", repo.NormalRole).Return(nil).Once()
	d.schemaTx.EXPECT().SetSearchPath(ctx, "bnpl", repo.NormalRole).Return(nil).Once()
}

func TestSchemaName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "bnpl", migrationsuc.SchemaName())
}
