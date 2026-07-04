package migrationsuc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// migrateDeps bundles the mocks required to exercise the migration use case.
type migrateDeps struct {
	pool      *mock.Pool
	migs      *mock.Migrations
	tx        *mock.Tx
	conn      *mock.Conn
	connQuery *mock.MigrationsConnQueryer
	txQuery   *mock.MigrationsTxQueryer
	clock     *mock.Clock
	dir       string
}

func newMigrateDeps(t *testing.T) migrateDeps {
	t.Helper()

	return migrateDeps{
		pool:      mock.NewPool(t),
		migs:      mock.NewMigrations(t),
		tx:        mock.NewTx(t),
		conn:      mock.NewConn(t),
		connQuery: mock.NewMigrationsConnQueryer(t),
		txQuery:   mock.NewMigrationsTxQueryer(t),
		clock:     mock.NewClock(t),
		dir:       t.TempDir(),
	}
}

func (d migrateDeps) newUseCase() *UseCase {
	uc, err := New(
		d.pool,
		d.migs,
		d.clock,
		d.dir,
		lognoop.New(),
	)
	if err != nil {
		panic(err)
	}

	return uc
}

// expectMigrationLock wires pool -> Conn -> advisory lock -> ListApplied and
// schedules ReleaseLock for the deferred unlock path.
func (d migrateDeps) expectMigrationLock(ctx context.Context, applied []model.Migration) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.migs.EXPECT().Conn(d.conn).Return(d.connQuery).Once()
	// AcquireLock uses a timeout-derived context, not the caller's ctx.
	d.connQuery.On("AcquireLock", testifymock.Anything).Return(nil).Once()
	d.connQuery.On("ListApplied", ctx).Return(applied, nil).Once()
	d.connQuery.On("ReleaseLock", testifymock.Anything).Return(nil).Once()
}

func (d migrateDeps) expectApplyTx(ctx context.Context) {
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
	d.migs.EXPECT().Tx(d.tx).Return(d.txQuery).Once()
}

func writeMigration(t *testing.T, dir, name, contents string) {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(contents), 0o600))
}

func migrationChecksum(contents string) string {
	digest := sha256.Sum256([]byte(contents))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func TestNew_SetsFields(t *testing.T) {
	t.Parallel()

	pool := mock.NewPool(t)
	migs := mock.NewMigrations(t)
	clk := mock.NewClock(t)
	logger := lognoop.New()

	uc, err := New(
		pool,
		migs,
		clk,
		"/tmp/migrations",
		logger,
	)

	require.NoError(t, err)
	require.NotNil(t, uc)
	assert.Equal(t, "/tmp/migrations", uc.migDirPath)
	assert.Same(t, pool, uc.pool)
	assert.Same(t, migs, uc.migrations)
	assert.Same(t, clk, uc.clock)
	assert.Same(t, logger, uc.logger)
}

func TestNew_RequiresLogger(t *testing.T) {
	t.Parallel()

	_, err := New(
		mock.NewPool(t),
		mock.NewMigrations(t),
		mock.NewClock(t),
		"/tmp/migrations",
		nil,
	)
	require.ErrorContains(t, err, "logger is required")
}

func TestMigrateOne_Success_RegistersWithClockTime(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	now := time.Date(
		2026,
		7,
		3,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	const (
		migName = "001_init.sql"
		migSQL  = "CREATE TABLE example (id int);"
	)
	writeMigration(t, deps.dir, migName, migSQL)
	checksum := migrationChecksum(migSQL)

	deps.expectMigrationLock(ctx, nil)
	deps.expectApplyTx(ctx)
	deps.txQuery.EXPECT().Migrate(ctx, migName, migSQL).Return(nil).Once()
	deps.clock.EXPECT().Now().Return(now).Once()
	deps.txQuery.EXPECT().Register(ctx, migName, checksum, now).
		Return(model.Migration{ID: 1, Name: migName, Checksum: checksum}, nil).Once()

	err := deps.newUseCase().MigrateOne(ctx, migName)

	require.NoError(t, err)
}

func TestMigrateOne_FileMissing_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)

	err := deps.newUseCase().MigrateOne(ctx, "does_not_exist.sql")

	require.Error(t, err)
	require.ErrorContains(t, err, "stat migration")
}

func TestMigrateOne_MigrateFails_DoesNotRegister(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	migrateErr := errors.New("bad statement")

	const migName = "001_init.sql"
	writeMigration(t, deps.dir, migName, "SELECT 1;")

	deps.expectMigrationLock(ctx, nil)
	deps.expectApplyTx(ctx)
	deps.txQuery.EXPECT().Migrate(ctx, migName, "SELECT 1;").Return(migrateErr).Once()

	err := deps.newUseCase().MigrateOne(ctx, migName)

	require.Error(t, err)
	require.ErrorIs(t, err, migrateErr)
}

func TestMigrateOne_RegisterFails_ReturnsError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	now := time.Date(
		2026,
		7,
		3,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	registerErr := errors.New("duplicate migration")

	const migName = "001_init.sql"
	const migSQL = "SELECT 1;"
	writeMigration(t, deps.dir, migName, migSQL)
	checksum := migrationChecksum(migSQL)

	deps.expectMigrationLock(ctx, nil)
	deps.expectApplyTx(ctx)
	deps.txQuery.EXPECT().Migrate(ctx, migName, migSQL).Return(nil).Once()
	deps.clock.EXPECT().Now().Return(now).Once()
	deps.txQuery.EXPECT().Register(ctx, migName, checksum, now).
		Return(model.Migration{}, registerErr).Once()

	err := deps.newUseCase().MigrateOne(ctx, migName)

	require.Error(t, err)
	require.ErrorIs(t, err, registerErr)
}

func TestMigrateAll_AppliesFilesInOrder(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	now := time.Date(
		2026,
		7,
		3,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	writeMigration(t, deps.dir, "001_a.sql", "SELECT 1;")
	writeMigration(t, deps.dir, "002_b.sql", "SELECT 2;")

	var applied []string

	deps.expectMigrationLock(ctx, nil)
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
	deps.txQuery.EXPECT().Register(ctx, testifymock.Anything, testifymock.Anything, now).
		Return(model.Migration{}, nil).Times(2)

	err := deps.newUseCase().MigrateAll(ctx)

	require.NoError(t, err)
	assert.Equal(t, []string{"001_a.sql", "002_b.sql"}, applied)
}

func TestMigrateAll_DirMissing_ReturnsError(t *testing.T) {
	t.Parallel()

	deps := newMigrateDeps(t)
	uc, err := New(
		deps.pool,
		deps.migs,
		deps.clock,
		filepath.Join(deps.dir, "missing"),
		lognoop.New(),
	)
	require.NoError(t, err)

	err = uc.MigrateAll(context.Background())

	require.Error(t, err)
	require.ErrorContains(t, err, "read migration directory")
}

func TestMigrateAll_StopsOnFirstFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newMigrateDeps(t)
	migrateErr := errors.New("first failed")

	writeMigration(t, deps.dir, "001_a.sql", "SELECT 1;")
	writeMigration(t, deps.dir, "002_b.sql", "SELECT 2;")

	deps.expectMigrationLock(ctx, nil)
	deps.expectApplyTx(ctx)
	deps.txQuery.EXPECT().Migrate(ctx, "001_a.sql", "SELECT 1;").Return(migrateErr).Once()

	err := deps.newUseCase().MigrateAll(ctx)

	require.Error(t, err)
	require.ErrorIs(t, err, migrateErr)
}
