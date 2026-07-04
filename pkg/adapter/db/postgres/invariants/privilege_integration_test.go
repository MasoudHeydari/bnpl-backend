//go:build development && integration

package invariants

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantterminalrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/migrationsrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/schemarp"
	"github.com/parstis/bnpl-backend/pkg/adapter/dbadminconfig"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/migrationsuc"
	"github.com/stretchr/testify/require"
)

type securityFixture struct {
	ctx      context.Context
	admin    *pgx.Conn
	c        *dbadminconfig.Config
	runtime  *postgres.Pool
	migrator repo.Pool
}

func newSecurityFixture(t *testing.T) *securityFixture {
	t.Helper()
	dsn := os.Getenv("DATABASE_SECURITY_TEST_URL")
	require.NotEmpty(t, dsn, "DATABASE_SECURITY_TEST_URL must point to a dedicated disposable PostgreSQL cluster; security tests never skip")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	root, e := pgx.Connect(ctx, dsn)
	require.NoError(t, e)
	t.Cleanup(func() { root.Close(context.Background()) })
	_, e = root.Exec(ctx, "SELECT pg_advisory_lock($1)", int64(0x424e504c53454355))
	require.NoError(t, e)
	t.Cleanup(func() {
		_, _ = root.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", int64(0x424e504c53454355))
	})

	name := "bnpl_security_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:10]
	_, e = root.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	require.NoError(t, e)
	t.Cleanup(func() {
		_, _ = root.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
	})
	cfg, e := pgx.ParseConfig(dsn)
	require.NoError(t, e)
	cfg.Database = name
	admin, e := pgx.ConnectConfig(ctx, cfg)
	require.NoError(t, e)
	t.Cleanup(func() { admin.Close(context.Background()) })
	c := &dbadminconfig.Config{}
	c.Database.Host = cfg.Host
	c.Database.Port = int(cfg.Port)
	c.Database.Name = name
	c.Database.SSLMode = "disable"
	c.Database.MaxConnections = 3
	c.Database.ConnectionTimeout = 5 * time.Second
	c.Database.PGPassEntries.Admin = model.NewSecret(fmt.Sprintf("%s:%d:%s:admin:%s", cfg.Host, cfg.Port, name, cfg.Password))
	c.Database.PGPassEntries.Migrator = model.NewSecret(fmt.Sprintf("%s:%d:%s:bnpl_migrator:security-test-migrator", cfg.Host, cfg.Port, name))
	c.Provisioning.NormalPassword = model.NewSecret("security-test-runtime")
	c.Migrations.FilesDir = filepath.Join("..", "..", "..", "..", "..", "assets", "db", "migrations")
	f := &securityFixture{ctx: ctx, admin: admin, c: c}
	return f
}
func (f *securityFixture) init(t *testing.T) {
	t.Helper()
	require.NoError(t, migrationsuc.NewInitDB(f.c, model.DevelopmentRuntimeEnvironment, lognoop.New()).InitDev(f.ctx))
	f.migrate(t)
	cfg, e := pgx.ParseConfig(fmt.Sprintf("postgres://bnpl:security-test-runtime@%s:%d/%s?sslmode=disable", f.c.Database.Host, f.c.Database.Port, f.c.Database.Name))
	require.NoError(t, e)
	pc, e := pgxpool.ParseConfig(cfg.ConnString())
	require.NoError(t, e)
	f.runtime, e = postgres.NewPool(f.ctx, pc)
	require.NoError(t, e)
	t.Cleanup(func() { f.runtime.Close() })
}
func (f *securityFixture) migrate(t *testing.T) {
	t.Helper()
	p, e := f.c.ConnectionPool(f.ctx, repo.MigratorRole, nil)
	require.NoError(t, e)
	defer p.Close()
	uc, e := migrationsuc.New(p, migrationsrp.New(), adapterclock.NewSystem(time.UTC, time.UTC), f.c.Migrations.FilesDir, lognoop.New())
	require.NoError(t, e)
	require.NoError(t, uc.MigrateAll(f.ctx))
}
func TestRuntimePrivilegesRealRolesAndDrift(t *testing.T) {
	f := newSecurityFixture(t)
	f.init(t)
	require.NoError(t, VerifyRuntime(f.ctx, f.runtime))
	f.migrate(t)

	// Session-local temporary tables are allowed; managed-schema DDL is not.
	_, tempErr := f.runtime.Exec(f.ctx, "CREATE TEMP TABLE bnpl_runtime_temp_test(id bigint)")
	require.NoError(t, tempErr)

	// Every reviewed table/column is exercised by an authenticated Runtime session.
	for _, p := range PrivilegeRegistry() {
		obj := pgx.Identifier{p.Schema, p.Table}.Sanitize()
		_, e := f.runtime.Exec(f.ctx, "SELECT * FROM "+obj+" LIMIT 0")
		require.NoError(t, e, p.Table)
		_, e = f.runtime.Exec(f.ctx, "INSERT INTO "+obj+" SELECT * FROM "+obj+" WHERE false")
		if slices.Contains(p.Operations, InsertOperation) {
			require.NoError(t, e, p.Table)
		} else {
			require.Error(t, e, p.Table)
		}
		for _, col := range p.UpdateColumns {
			_, e = f.runtime.Exec(f.ctx, "UPDATE "+obj+" SET "+pgx.Identifier{col}.Sanitize()+"="+pgx.Identifier{col}.Sanitize()+" WHERE false")
			require.NoError(t, e, p.Table+"."+col)
		}

		if len(p.UpdateColumns) > 0 || slices.Contains(p.Operations, UpdateOperation) {
			_, e = f.runtime.Exec(f.ctx, "SELECT * FROM "+obj+" WHERE false FOR UPDATE")
			require.NoError(t, e, p.Table)
		}
	}
	// Every registered table: PostgreSQL itself refuses both destructive commands,
	// even when no rows match. No SET ROLE simulation is used for Runtime.
	for _, p := range PrivilegeRegistry() {
		for _, sql := range []string{"DELETE FROM " + pgx.Identifier{p.Schema, p.Table}.Sanitize() + " WHERE false", "TRUNCATE " + pgx.Identifier{p.Schema, p.Table}.Sanitize()} {
			_, e := f.runtime.Exec(f.ctx, sql)
			var pg *pgconn.PgError
			require.ErrorAs(t, e, &pg, p.Table)
			require.Equal(t, "42501", pg.Code, p.Table)
		}
	}
	for _, sql := range []string{"CREATE TABLE bnpl.bad(id int)", "ALTER TABLE bnpl.decisions ADD COLUMN bad int", "ALTER TABLE bnpl.decisions OWNER TO bnpl", "SET ROLE admin", "SET ROLE bnpl_owner", "SET ROLE bnpl_migrator", "CREATE OR REPLACE FUNCTION bnpl.guard_payment_items() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END $$"} {
		_, e := f.runtime.Exec(f.ctx, sql)
		require.Error(t, e, sql)
	}
	// Required grants and every column are compared by effective-right checks.
	cases := []struct{ name, sql string }{
		{"delete", "GRANT DELETE ON bnpl.decisions TO bnpl"},
		{"missing column", "REVOKE UPDATE(exposure_amount_minor) ON bnpl.credit_accounts FROM bnpl"},
		{"delegation", "GRANT SELECT ON bnpl.decisions TO bnpl WITH GRANT OPTION"},
		{"future default", "ALTER DEFAULT PRIVILEGES FOR ROLE bnpl_owner IN SCHEMA bnpl GRANT INSERT ON TABLES TO PUBLIC"},
		{"function overload", "CREATE FUNCTION bnpl.guard_payment_items(integer) RETURNS integer LANGUAGE SQL AS 'SELECT $1'; GRANT EXECUTE ON FUNCTION bnpl.guard_payment_items(integer) TO PUBLIC"},
		{"inherited path", "CREATE ROLE bnpl_security_donor NOLOGIN; GRANT DELETE ON bnpl.decisions TO bnpl_security_donor; GRANT bnpl_security_donor TO bnpl"},

		{"truncate public", "GRANT TRUNCATE ON bnpl.contracts TO PUBLIC"},
		{"missing select", "REVOKE SELECT ON bnpl.credit_accounts FROM bnpl"},
		{"extra table update", "GRANT UPDATE ON bnpl.credit_accounts TO bnpl"},
		{"extra column update", "GRANT UPDATE(amount_minor) ON bnpl.installment_charges TO bnpl"},
		{"owner", "ALTER TABLE bnpl.decisions OWNER TO bnpl"},
		{"membership", "GRANT bnpl_owner TO bnpl"},
		{"unknown table", "SET ROLE bnpl_owner; CREATE TABLE bnpl.unregistered(id int); RESET ROLE"},
		{"disabled trigger", "ALTER TABLE bnpl.installments DISABLE TRIGGER installment_complete_settlement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx, e := f.admin.Begin(f.ctx)
			require.NoError(t, e)
			defer tx.Rollback(f.ctx)
			_, e = tx.Exec(f.ctx, tc.sql)
			require.NoError(t, e)
			// ACL changes must be committed to be visible from the normal connection;
			// restore explicitly after the assertion using a known inverse.
			require.NoError(t, tx.Commit(f.ctx))
			require.Error(t, VerifyRuntime(f.ctx, f.runtime))
			switch tc.name {
			case "missing column":
				_, e = f.admin.Exec(f.ctx, "GRANT UPDATE(exposure_amount_minor) ON bnpl.credit_accounts TO bnpl")

			case "delegation":
				_, e = f.admin.Exec(f.ctx, "REVOKE GRANT OPTION FOR SELECT ON bnpl.decisions FROM bnpl")

			case "future default":
				_, e = f.admin.Exec(f.ctx, "ALTER DEFAULT PRIVILEGES FOR ROLE bnpl_owner IN SCHEMA bnpl REVOKE INSERT ON TABLES FROM PUBLIC")

			case "function overload":
				_, e = f.admin.Exec(f.ctx, "DROP FUNCTION bnpl.guard_payment_items(integer)")

			case "inherited path":
				_, e = f.admin.Exec(f.ctx, "REVOKE bnpl_security_donor FROM bnpl; REVOKE DELETE ON bnpl.decisions FROM bnpl_security_donor; DROP ROLE bnpl_security_donor")

			case "delete":
				_, e = f.admin.Exec(f.ctx, "REVOKE DELETE ON bnpl.decisions FROM bnpl")

			case "truncate public":
				_, e = f.admin.Exec(f.ctx, "REVOKE TRUNCATE ON bnpl.contracts FROM PUBLIC")

			case "missing select":
				_, e = f.admin.Exec(f.ctx, "GRANT SELECT ON bnpl.credit_accounts TO bnpl")

			case "extra table update":
				_, e = f.admin.Exec(f.ctx, "REVOKE UPDATE ON bnpl.credit_accounts FROM bnpl; GRANT UPDATE(effective_from, expires_at, exposure_amount_minor, last_limit_decision_id, limit_amount_minor, revoked_at, status, updated_at, version) ON bnpl.credit_accounts TO bnpl")

			case "extra column update":
				_, e = f.admin.Exec(f.ctx, "REVOKE UPDATE(amount_minor) ON bnpl.installment_charges FROM bnpl")

			case "owner":
				_, e = f.admin.Exec(f.ctx, "ALTER TABLE bnpl.decisions OWNER TO bnpl_owner; GRANT SELECT,INSERT,UPDATE ON bnpl.decisions TO bnpl")

			case "membership":
				_, e = f.admin.Exec(f.ctx, "REVOKE bnpl_owner FROM bnpl")

			case "unknown table":
				_, e = f.admin.Exec(f.ctx, "DROP TABLE bnpl.unregistered")

			case "disabled trigger":
				_, e = f.admin.Exec(f.ctx, "ALTER TABLE bnpl.installments ENABLE TRIGGER installment_complete_settlement")
			}
			require.NoError(t, e)
			require.NoError(t, VerifyRuntime(f.ctx, f.runtime))
		})
	}
}
func TestLegacyOwnershipUpgradePreservesBusinessData(t *testing.T) {
	f := newSecurityFixture(t)
	// Legacy objects were created by Runtime. Do not DROP/REASSIGN unrelated data.
	roleTx, e := f.admin.Begin(f.ctx)
	require.NoError(t, e)
	for _, r := range []repo.Role{repo.OwnerRole, repo.MigratorRole, repo.NormalRole} {
		pass := ""
		if r != repo.OwnerRole {
			var e error
			pass, e = f.c.GetRolePassword(r)
			require.NoError(t, e)
		}
		require.NoError(t, schemarp.CreateRoleIfNotExists(f.ctx, &postgres.Tx{Tx: roleTx}, r, pass))
	}
	require.NoError(t, roleTx.Commit(f.ctx))
	_, e = f.admin.Exec(f.ctx, "CREATE SCHEMA bnpl AUTHORIZATION bnpl; CREATE SCHEMA unrelated; CREATE TABLE unrelated.keep(id int); INSERT INTO unrelated.keep VALUES(7); SET ROLE bnpl; SET search_path=bnpl; CREATE TYPE bnpl.legacy_enum AS ENUM('retained'); CREATE DOMAIN bnpl.legacy_domain AS int; CREATE TYPE bnpl.legacy_composite AS (value int)")
	require.NoError(t, e)
	// The new baseline can only be installed after ownership handoff.
	_, e = f.admin.Exec(f.ctx, "RESET ROLE")
	require.NoError(t, e)
	// Preflight/plan doesn't change ownership.
	planTx, e := f.admin.Begin(f.ctx)
	require.NoError(t, e)
	plan, e := schemarp.OwnershipPlan(f.ctx, &postgres.Tx{Tx: planTx})
	require.NoError(t, planTx.Rollback(f.ctx))
	require.NoError(t, e)
	require.NotEmpty(t, plan)
	var countBefore int
	require.NoError(t, f.admin.QueryRow(f.ctx, "SELECT count(*) FROM unrelated.keep").Scan(&countBefore))
	tx, e := f.admin.Begin(f.ctx)
	require.NoError(t, e)
	pt := &postgres.Tx{Tx: tx}
	require.NoError(t, schemarp.GrantPrivileges(f.ctx, pt, "bnpl", repo.MigratorRole))
	require.NoError(t, schemarp.ApplyOwnership(f.ctx, pt, plan))
	require.NoError(t, tx.Commit(f.ctx))
	p, e := f.c.ConnectionPool(f.ctx, repo.MigratorRole, nil)
	require.NoError(t, e)
	defer p.Close()
	// Install the complete baseline as the effective NOLOGIN owner, after
	// transferring legacy schema/type ownership. No old migration files run.
	require.NoError(t, p.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			return migrationsrp.NewSettler(tx).InitDevSchema(ctx)
		})
	}))
	f.migrate(t)
	f.migrate(t)
	var countAfter, unrelated int
	require.NoError(t, f.admin.QueryRow(f.ctx, "SELECT count(*) FROM unrelated.keep").Scan(&countAfter))
	require.Equal(t, countBefore, countAfter)
	require.NoError(t, f.admin.QueryRow(f.ctx, "SELECT id FROM unrelated.keep").Scan(&unrelated))
	require.Equal(t, 7, unrelated)
	require.NoError(t, Verify(f.ctx, p))
	require.NoError(t, VerifyPrivileges(f.ctx, p))
	// Explicit upgrade is repeatable and remains scoped.
	planTx, e = f.admin.Begin(f.ctx)
	require.NoError(t, e)
	plan, e = schemarp.OwnershipPlan(f.ctx, &postgres.Tx{Tx: planTx})
	require.NoError(t, planTx.Rollback(f.ctx))
	require.NoError(t, e)
	for _, v := range plan {
		require.Equal(t, string(repo.OwnerRole), v.Owner)
	}
}

func TestRuntimeTerminalPermissionHistoryAndConcurrency(t *testing.T) {
	f := newSecurityFixture(t)
	f.init(t)
	merchant, terminal := uuid.New(), uuid.New()
	_, e := f.runtime.Exec(f.ctx, `INSERT INTO merchant_profiles(merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction) VALUES($1,'security','active','low',12,'IR')`, merchant)
	require.NoError(t, e)
	_, e = f.runtime.Exec(f.ctx, `INSERT INTO merchant_terminals(id,merchant_id,terminal_id,status) VALUES($1,$2,999991,'active')`, terminal, merchant)
	require.NoError(t, e)
	replace := func(permissions []string) error {
		return f.runtime.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
			return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				return merchantterminalrp.New().Tx(tx).ReplacePermissions(ctx, model.MerchantTerminalID(terminal), permissions, time.Now())
			})
		})
	}
	list := func() []string {
		var out []string
		require.NoError(t, f.runtime.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
			var err error
			out, err = merchantterminalrp.New().Conn(c).ListPermissions(ctx, model.MerchantTerminalID(terminal))
			return err
		}))
		return out
	}
	require.NoError(t, replace([]string{"a", "a", "b"}))
	require.Equal(t, []string{"a", "b"}, list())
	require.NoError(t, replace([]string{"b"}))
	require.Equal(t, []string{"b"}, list())
	require.NoError(t, replace([]string{"a", "b"}))
	var count int
	require.NoError(t, f.runtime.QueryRow(f.ctx, `SELECT count(*) FROM bnpl.merchant_terminal_permissions WHERE merchant_terminal_id=$1`, terminal).Scan(&count))
	require.Equal(t, 3, count)
	require.Error(t, replace([]string{""}))
	require.Equal(t, []string{"a", "b"}, list())
	require.NoError(t, replace(nil))
	require.Empty(t, list())
	// Both concurrent replacements use the parent lock and a fresh post-lock
	// statement snapshot. The final set is one whole replacement, never a union.
	done := make(chan error, 2)
	start := make(chan struct{})
	for _, set := range [][]string{{"x"}, {"y"}} {
		go func(set []string) { <-start; done <- replace(set) }(set)
	}
	close(start)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	got := list()
	require.True(t, slices.Equal(got, []string{"x"}) || slices.Equal(got, []string{"y"}), got)
	a, e := f.runtime.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, e)
	defer a.Rollback(f.ctx)
	b, e := f.runtime.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	require.NoError(t, e)
	defer b.Rollback(f.ctx)
	_, e = b.Exec(f.ctx, `SELECT * FROM bnpl.merchant_terminal_permissions WHERE merchant_terminal_id=$1`, terminal)
	require.NoError(t, e)
	require.NoError(t, merchantterminalrp.New().Tx(&postgres.Tx{Tx: a}).ReplacePermissions(f.ctx, model.MerchantTerminalID(terminal), []string{"rr-winner"}, time.Now()))
	require.NoError(t, a.Commit(f.ctx))
	require.Error(t, merchantterminalrp.New().Tx(&postgres.Tx{Tx: b}).ReplacePermissions(f.ctx, model.MerchantTerminalID(terminal), []string{"rr-stale"}, time.Now()))
	require.NoError(t, b.Rollback(f.ctx))
	require.Equal(t, []string{"rr-winner"}, list())

}

func TestDefaultDenyAndMigratorEffectiveRole(t *testing.T) {
	f := newSecurityFixture(t)
	f.init(t)
	p, e := f.c.ConnectionPool(f.ctx, repo.MigratorRole, nil)
	require.NoError(t, e)
	defer p.Close()
	require.NoError(t, p.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
		var current, session string
		q := postgres.MustConn(c)
		require.NoError(t, q.QueryRow(ctx, `SELECT current_user,session_user`).Scan(&current, &session))
		require.Equal(t, "bnpl_owner", current)
		require.Equal(t, "bnpl_migrator", session)
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := postgres.MustTx(tx)
			require.NoError(t, q.QueryRow(ctx, `SELECT current_user`).Scan(&current))
			require.Equal(t, "bnpl_owner", current)
			_, e := q.Exec(ctx, `CREATE TABLE bnpl.future_table(id int); CREATE SEQUENCE bnpl.future_sequence`)
			return e
		})
	}))
	for _, sql := range []string{`SELECT * FROM bnpl.future_table`, `INSERT INTO bnpl.future_table VALUES(1)`, `SELECT nextval('bnpl.future_sequence')`} {
		_, e = f.runtime.Exec(f.ctx, sql)
		var pg *pgconn.PgError
		require.ErrorAs(t, e, &pg)
		require.Equal(t, "42501", pg.Code)
	}
	require.ErrorContains(t, VerifyPrivileges(f.ctx, f.runtime), "unregistered managed relation")
	_, e = f.admin.Exec(f.ctx, `DROP TABLE bnpl.future_table; DROP SEQUENCE bnpl.future_sequence`)
	require.NoError(t, e)
	require.NoError(t, VerifyRuntime(f.ctx, f.runtime))
}

// A temporary post-release migration verifies that checksum and dirty-state
// enforcement remain intact after the pre-release SQL migration squash.
func TestRealMigratorRejectsChecksumAndDirtyHistory(t *testing.T) {
	f := newSecurityFixture(t)
	f.init(t)
	p, e := f.c.ConnectionPool(f.ctx, repo.MigratorRole, nil)
	require.NoError(t, e)
	defer p.Close()

	migrationDir := t.TempDir()
	name := "000001_future_test.sql"
	contents := []byte("-- A future incremental migration.\nSELECT 1;\n")
	require.NoError(t, os.WriteFile(filepath.Join(migrationDir, name), contents, 0600))
	uc, e := migrationsuc.New(p, migrationsrp.New(), adapterclock.NewSystem(time.UTC, time.UTC), migrationDir, lognoop.New())
	require.NoError(t, e)
	require.NoError(t, uc.MigrateAll(f.ctx))
	require.NoError(t, uc.MigrateAll(f.ctx))

	_, e = f.admin.Exec(f.ctx, `UPDATE bnpl.migrations SET checksum='sha256:0000000000000000000000000000000000000000000000000000000000000000' WHERE name=$1`, name)
	require.NoError(t, e)
	require.Error(t, uc.MigrateAll(f.ctx))
	_, e = f.admin.Exec(f.ctx, `UPDATE bnpl.migrations SET checksum=$2,dirty=true WHERE name=$1`, name, MigrationFingerprint(contents))
	require.NoError(t, e)
	require.Error(t, uc.MigrateAll(f.ctx))
	_, e = f.admin.Exec(f.ctx, `UPDATE bnpl.migrations SET dirty=false WHERE name=$1`, name)
	require.NoError(t, e)
	require.NoError(t, uc.MigrateAll(f.ctx))
	require.NoError(t, VerifyRuntime(f.ctx, f.runtime))
}
