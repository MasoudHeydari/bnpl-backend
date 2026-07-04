//go:build development && integration

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/parstis/bnpl-backend/pkg/adapter/config"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/dbadminconfig"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"github.com/stretchr/testify/require"
)

// This test imports administrative adapters only for disposable fixture setup.
// They are absent from the compiled Runtime dependency graph.
func TestRuntimeStartupFailsClosedAndClosesPoolOnDatabaseDrift(t *testing.T) {
	dsn := os.Getenv("DATABASE_SECURITY_TEST_URL")
	require.NotEmpty(t, dsn, "dedicated security PostgreSQL required; startup tests never skip")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	root, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	defer root.Close(context.Background())
	// Role-drift tests in another package use this same dedicated-cluster lock.
	_, e = root.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(0x424e504c53454355))
	require.NoError(t, e)
	defer root.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", int64(0x424e504c53454355))
	name := "bnpl_bootstrap_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	_, e = root.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, e)
	defer root.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	parsed, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	c := &dbadminconfig.Config{}
	c.Database.Host = parsed.Host
	c.Database.Port = int(parsed.Port)
	c.Database.Name = name
	c.Database.SSLMode = "disable"
	c.Database.MaxConnections = 2
	c.Database.ConnectionTimeout = 5 * time.Second
	c.Database.PGPassEntries.Admin = model.NewSecret(fmt.Sprintf("%s:%d:%s:admin:%s", parsed.Host, parsed.Port, name, parsed.Password))
	c.Database.PGPassEntries.Migrator = model.NewSecret(fmt.Sprintf("%s:%d:%s:bnpl_migrator:security-test-migrator", parsed.Host, parsed.Port, name))
	c.Provisioning.NormalPassword = model.NewSecret("security-test-runtime")
	require.NoError(t, migrationsuc.NewInitDB(c, model.DevelopmentRuntimeEnvironment, lognoop.New()).InitDev(ctx))
	p, e := c.ConnectionPool(ctx, repo.MigratorRole, nil)
	require.NoError(t, e)
	uc, e := migrationsuc.New(p, migrationsrp.New(), adapterclock.NewSystem(time.UTC, time.UTC), filepath.Join("..", "..", "..", "assets", "db", "migrations"), lognoop.New())
	require.NoError(t, e)
	require.NoError(t, uc.MigrateAll(ctx))
	require.NoError(t, p.Close())
	cfg := noopTestConfig()
	cfg.Database = config.Database{Host: parsed.Host, Port: int(parsed.Port), Name: name, SSLMode: "disable", MaxConnections: 2, HealthCheckPeriod: time.Minute, ConnectionTimeout: 5 * time.Second}
	cfg.Database.PGPassEntries.Normal = model.NewSecret(fmt.Sprintf("%s:%d:%s:bnpl:security-test-runtime", parsed.Host, parsed.Port, name))
	app, e := NewApp(ctx, cfg)
	require.NoError(t, e)
	require.NoError(t, app.Close())
	parsed.Database = name
	admin, e := pgx.ConnectConfig(ctx, parsed)
	require.NoError(t, e)
	defer admin.Close(context.Background())
	for _, sql := range []string{`GRANT DELETE ON bnpl.decisions TO bnpl`, `ALTER TABLE bnpl.installments DISABLE TRIGGER installment_complete_settlement`} {
		_, e = admin.Exec(ctx, sql)
		require.NoError(t, e)
		app, e = NewApp(ctx, cfg)
		require.ErrorContains(t, e, "database integrity verification")
		require.Nil(t, app)
		var sessions int
		require.NoError(t, admin.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=$1 AND usename='bnpl'`, name).Scan(&sessions))
		require.Zero(t, sessions, "failed startup leaked normal-role connections")
		_, e = admin.Exec(ctx, `REVOKE DELETE ON bnpl.decisions FROM bnpl; ALTER TABLE bnpl.installments ENABLE TRIGGER installment_complete_settlement`)
		require.NoError(t, e)
	}
}
