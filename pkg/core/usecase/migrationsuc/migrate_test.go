package migrationsuc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// migrateDeps bundles the mocks required to exercise the migration use case.
type migrateDeps struct {
	pool    *mock.Pool
	migs    *mock.Migrations
	tx      *mock.Tx
	conn    *mock.Conn
	txQuery *mock.MigrationsTxQueryer
	clock   *mock.Clock
	dir     string
}

func newMigrateDeps(t *testing.T) migrateDeps {
	t.Helper()

	return migrateDeps{
		pool:    mock.NewPool(t),
		migs:    mock.NewMigrations(t),
		tx:      mock.NewTx(t),
		conn:    mock.NewConn(t),
		txQuery: mock.NewMigrationsTxQueryer(t),
		clock:   mock.NewClock(t),
		dir:     t.TempDir(),
	}
}

func (d migrateDeps) newUseCase() *UseCase {
	return New(d.pool, d.migs, d.clock, d.dir)
}

// expectConnTx wires the pool -> conn -> tx callbacks so the handler passed to
// MigrateOne is executed against the mocked transaction.
func (d migrateDeps) expectConnTx(ctx context.Context) {
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
}

func writeMigration(t *testing.T, dir, name, contents string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600))
}

func TestNew_SetsFields(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	migs := mock.NewMigrations(t)
	clk := mock.NewClock(t)

	uc := New(pool, migs, clk, "/tmp/migrations")

	require.NotNil(t, uc)
	assert.Equal(t, "/tmp/migrations", uc.migDirPath)
	assert.Same(t, pool, uc.pool)
	assert.Same(t, migs, uc.migrations)
	assert.Same(t, clk, uc.clock)
}

func TestMigrateOne_Success_RegistersWithClockTime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	const (
		migName = "001_init.sql"
		migSQL  = "CREATE TABLE example (id int);"
	)
	writeMigration(t, deps.dir, migName, migSQL)

	deps.expectConnTx(ctx)
	deps.migs.EXPECT().Tx(deps.tx).Return(deps.txQuery).Once()
	deps.txQuery.EXPECT().Migrate(ctx, migName, migSQL).Return(nil).Once()
	deps.clock.EXPECT().Now().Return(now).Once()
	deps.txQuery.EXPECT().Register(ctx, migName, now).
		Return(model.Migration{ID: 1, Name: migName}, nil).Once()

	err := deps.newUseCase().MigrateOne(ctx, migName)

	require.NoError(t, err)
}

func TestMigrateOne_FileMissing_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)

	err := deps.newUseCase().MigrateOne(ctx, "does_not_exist.sql")

	require.Error(t, err)
	require.ErrorContains(t, err, "os.readFile")
}

func TestMigrateOne_MigrateFails_DoesNotRegister(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	migrateErr := errors.New("bad statement")

	const migName = "001_init.sql"
	writeMigration(t, deps.dir, migName, "SELECT 1;")

	deps.expectConnTx(ctx)
	deps.migs.EXPECT().Tx(deps.tx).Return(deps.txQuery).Once()
	deps.txQuery.EXPECT().Migrate(ctx, migName, "SELECT 1;").Return(migrateErr).Once()

	err := deps.newUseCase().MigrateOne(ctx, migName)

	require.Error(t, err)
	require.ErrorIs(t, err, migrateErr)
}

func TestMigrateOne_RegisterFails_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)
	registerErr := errors.New("duplicate migration")

	const migName = "001_init.sql"
	writeMigration(t, deps.dir, migName, "SELECT 1;")

	deps.expectConnTx(ctx)
	deps.migs.EXPECT().Tx(deps.tx).Return(deps.txQuery).Once()
	deps.txQuery.EXPECT().Migrate(ctx, migName, "SELECT 1;").Return(nil).Once()
	deps.clock.EXPECT().Now().Return(now).Once()
	deps.txQuery.EXPECT().Register(ctx, migName, now).
		Return(model.Migration{}, registerErr).Once()

	err := deps.newUseCase().MigrateOne(ctx, migName)

	require.Error(t, err)
	require.ErrorIs(t, err, registerErr)
}

func TestMigrateAll_AppliesFilesInOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	now := time.Date(2026, 7, 3, 12, 0, 0, 0, time.UTC)

	writeMigration(t, deps.dir, "001_a.sql", "SELECT 1;")
	writeMigration(t, deps.dir, "002_b.sql", "SELECT 2;")

	var applied []string

	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, deps.conn)
		},
	).Times(2)
	deps.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, deps.tx)
		},
	).Times(2)
	deps.migs.EXPECT().Tx(deps.tx).Return(deps.txQuery).Times(2)
	deps.clock.EXPECT().Now().Return(now).Times(2)
	deps.txQuery.EXPECT().Migrate(ctx, testifymock.Anything, testifymock.Anything).
		RunAndReturn(func(_ context.Context, migName, _ string) error {
			applied = append(applied, migName)
			return nil
		}).Times(2)
	deps.txQuery.EXPECT().Register(ctx, testifymock.Anything, now).
		Return(model.Migration{}, nil).Times(2)

	err := deps.newUseCase().MigrateAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"001_a.sql", "002_b.sql"}, applied)
}

func TestMigrateAll_DirMissing_ReturnsError(t *testing.T) {
	t.Parallel()

	deps := newMigrateDeps(t)
	uc := New(deps.pool, deps.migs, deps.clock, filepath.Join(deps.dir, "missing"))

	err := uc.MigrateAll(context.Background())

	require.Error(t, err)
	require.ErrorContains(t, err, "os.readDir")
}

func TestMigrateAll_StopsOnFirstFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	migrateErr := errors.New("first failed")

	writeMigration(t, deps.dir, "001_a.sql", "SELECT 1;")
	writeMigration(t, deps.dir, "002_b.sql", "SELECT 2;")

	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, deps.conn)
		},
	).Once()
	deps.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, deps.tx)
		},
	).Once()
	deps.migs.EXPECT().Tx(deps.tx).Return(deps.txQuery).Once()
	deps.txQuery.EXPECT().Migrate(ctx, "001_a.sql", "SELECT 1;").Return(migrateErr).Once()

	err := deps.newUseCase().MigrateAll(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, migrateErr)
}
