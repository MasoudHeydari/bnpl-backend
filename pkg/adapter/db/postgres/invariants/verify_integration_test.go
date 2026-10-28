//go:build development && integration

package invariants

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/pagination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/require"
)

type fixture struct {
	ctx                             context.Context
	pool                            *postgres.Pool
	schema                          string
	customer, contract, installment uuid.UUID
}

func newFixture(t *testing.T, install bool) *fixture {
	t.Helper()
	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "dedicated disposable PostgreSQL is required; tests never skip")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	cfg, e := pgxpool.ParseConfig(dsn)
	require.NoError(t, e)
	schema := "invariant_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["lock_timeout"] = "2000"
	cfg.ConnConfig.RuntimeParams["statement_timeout"] = "10000"
	p, e := postgres.NewPool(ctx, cfg)
	require.NoError(t, e)
	t.Cleanup(func() { p.Close() })
	f := &fixture{ctx: ctx, pool: p, schema: schema, customer: uuid.New(), contract: uuid.New(), installment: uuid.New()}
	f.exec(t, "CREATE SCHEMA "+schema)
	ddl, e := os.ReadFile("../migrationsrp/000_schema.sql")
	require.NoError(t, e)
	f.exec(t, strings.ReplaceAll(string(ddl), "SET search_path TO bnpl;", "SET search_path TO "+schema+";"))
	f.exec(t, `CREATE TABLE IF NOT EXISTS migrations(id bigserial PRIMARY KEY,name text UNIQUE NOT NULL,checksum text NOT NULL,dirty boolean NOT NULL DEFAULT false,created_at timestamptz NOT NULL DEFAULT now())`)
	for _, file := range []string{"000001_collections_default_notification.sql", "000002_independent_credit_restrictions.sql"} {
		f.apply(t, file)
	}
	if install {
		f.apply(t, Migration)
		f.exec(t, `INSERT INTO migrations(name,checksum) VALUES($1,$2)`, Migration, MigrationSHA256)
	}
	merchant, account, decision := uuid.New(), uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO customer_profiles(id,external_customer_id,jurisdiction,status) VALUES($1,$2,'IR','active')`, f.customer, uuid.New())
	f.exec(t, `INSERT INTO merchant_profiles(id,merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction) VALUES($1,$2,'test','active','low',12,'IR')`, merchant, uuid.New())
	f.exec(t, `INSERT INTO decisions(id,type,target_type,target_id,outcome,reason_codes,policy_id,policy_version,jurisdiction,actor_type,decided_at,idempotency_key,idempotency_fingerprint) VALUES($1,'restriction','customer_profile',$2,'restricted',ARRAY['COLLECTIONS_GRACE_PERIOD_EXCEEDED'],'collections_default','v2','IR','system',now(),$3,$4)`, decision, f.customer, uuid.NewString(), "sha256:"+strings.Repeat("0", 64))
	f.exec(t, `INSERT INTO credit_accounts(id,customer_id,currency,status,effective_from) VALUES($1,$2,'IRR','active',now())`, account, f.customer)
	f.exec(t, `INSERT INTO contracts(id,decision_id,checkout_key,customer_id,credit_account_id,merchant_profile_id,currency,requested_amount_minor,approved_financed_amount_minor,installment_count,payable_now_amount_minor,remaining_principal_amount_minor,status,expires_at,activated_at) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,2,0,100,'active',now(),now())`, f.contract, decision, uuid.NewString(), f.customer, account, merchant)
	f.exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,2,'IRR',100,100,now(),2,'scheduled')`, f.installment, decision, f.contract, f.customer, merchant)
	return f
}
func (f *fixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	_, e := f.pool.Pool.Exec(f.ctx, sql, args...)
	if pg, ok := e.(*pgconn.PgError); ok {
		if pg.Position > 0 {
			n := int(pg.Position) - 1
			a := n - 100
			if a < 0 {
				a = 0
			}
			b := n + 100
			if b > len(sql) {
				b = len(sql)
			}
			t.Logf("near: %s", sql[a:b])
		}
		t.Logf("position=%d internal=%d where=%s query=%s detail=%s", pg.Position, pg.InternalPosition, pg.Where, pg.InternalQuery, pg.Detail)
	}
	require.NoError(t, e)
}
func (f *fixture) apply(t *testing.T, name string) {
	t.Helper()
	sql, e := os.ReadFile("../../../../../assets/db/migrations/" + name)
	require.NoError(t, e)
	s := strings.ReplaceAll(string(sql), "bnpl.", f.schema+".")
	s = strings.ReplaceAll(s, "pg_catalog, bnpl", "pg_catalog, "+f.schema)
	f.exec(t, s)
}
func (f *fixture) charge(t *testing.T, kind, payer string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO installment_charges(id,installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,$2,$3,'applied',10,'IRR',$4,'platform','test','v1',now(),'{}')`, id, f.installment, kind, payer)
	return id
}
func (f *fixture) attempt(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer tx.Rollback(f.ctx)
	require.NoError(t, f.insertAttempt(tx, id))
	require.NoError(t, tx.Commit(f.ctx))
	return id
}
func (f *fixture) insertAttempt(tx pgx.Tx, id uuid.UUID) error {
	_, e := tx.Exec(f.ctx, `INSERT INTO payment_attempts(id,contract_id,installment_id,amount_minor,currency,state,channel,idempotency_key,request_fingerprint) VALUES($1,$2,$3,100,'IRR','created','gateway',$4,$5)`, id, f.contract, f.installment, uuid.NewString(), "sha256:"+strings.Repeat("0", 64))
	if e != nil {
		return e
	}
	_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'principal',$2,100,'debit')`, id, f.installment)
	return e
}
func assertViolation(t *testing.T, e error, code string) {
	t.Helper()
	require.Error(t, e)
	var pg *pgconn.PgError
	require.ErrorAs(t, e, &pg)
	require.Equal(t, code, pg.Code)
}

func TestInstalledAndDriftedDefinitions(t *testing.T) {
	for _, tc := range []struct{ name, sql string }{
		{"correct", ""}, {"missing", `DROP TRIGGER installment_no_charges_after_paid ON %s.installment_charges`},
		{"disabled", `ALTER TABLE %s.installment_charges DISABLE TRIGGER installment_no_charges_after_paid`},
		{"wrong function", `DROP TRIGGER installment_no_charges_after_paid ON %s.installment_charges; CREATE TRIGGER installment_no_charges_after_paid BEFORE INSERT OR UPDATE OR DELETE ON %[1]s.installment_charges FOR EACH ROW EXECUTE FUNCTION %[1]s.guard_payment_items()`},
		{"wrong event", `DROP TRIGGER installment_no_charges_after_paid ON %s.installment_charges; CREATE TRIGGER installment_no_charges_after_paid BEFORE INSERT ON %[1]s.installment_charges FOR EACH ROW EXECUTE FUNCTION %[1]s.guard_installment_charge()`},
		{"body drift", `CREATE OR REPLACE FUNCTION %s.guard_installment_charge() RETURNS trigger LANGUAGE plpgsql AS 'BEGIN RETURN NEW; END'`},
		{"missing migration", `DELETE FROM %s.migrations WHERE name='000003_installment_payment_invariants.sql'`},
		{"incompatible migration", `UPDATE %s.migrations SET checksum='sha256:0000000000000000000000000000000000000000000000000000000000000000'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, true)
			if tc.sql != "" {
				f.exec(t, fmt.Sprintf(tc.sql, f.schema))
			}
			e := VerifySchema(f.ctx, f.pool, f.schema)
			if tc.sql == "" {
				require.NoError(t, e)
			} else {
				require.Error(t, e)
			}
		})
	}
}

func TestDirectDatabaseInvariants(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*testing.T, *fixture)
	}{
		{"unpaid insertion", func(t *testing.T, f *fixture) { f.charge(t, "late_fee", "customer") }},
		{"paid insertion", func(t *testing.T, f *fixture) {
			f.settleFixture(t)
			_, e := f.pool.Pool.Exec(f.ctx, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',10,'IRR','customer','platform','test','v1',now(),'{}')`, f.installment)
			assertViolation(t, e, "BN001")
		}},
		{"unsettled debit", func(t *testing.T, f *fixture) {
			f.charge(t, "late_fee", "customer")
			_, e := f.pool.Pool.Exec(f.ctx, `UPDATE installments SET status='paid',paid_at=now(),paid_amount_minor=110 WHERE id=$1`, f.installment)
			assertViolation(t, e, "BN002")
		}},
		{"unresolved discount", func(t *testing.T, f *fixture) {
			f.charge(t, "discount", "customer")
			_, e := f.pool.Pool.Exec(f.ctx, `UPDATE installments SET status='paid',paid_at=now(),paid_amount_minor=90 WHERE id=$1`, f.installment)
			assertViolation(t, e, "BN002")
		}},
		{"merchant independent", func(t *testing.T, f *fixture) {
			f.charge(t, "commission", "merchant")
			f.settleFixture(t)
		}},
		{"paid terminal", func(t *testing.T, f *fixture) {
			f.settleFixture(t)
			_, e := f.pool.Pool.Exec(f.ctx, `UPDATE installments SET status='scheduled' WHERE id=$1`, f.installment)
			assertViolation(t, e, "BN001")
		}},
		{"frozen amount", func(t *testing.T, f *fixture) {
			f.attempt(t)
			_, e := f.pool.Pool.Exec(f.ctx, `UPDATE installments SET principal_amount_minor=101 WHERE id=$1`, f.installment)
			assertViolation(t, e, "BN003")
		}},
		{"frozen insert", func(t *testing.T, f *fixture) {
			f.attempt(t)
			_, e := f.pool.Pool.Exec(f.ctx, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',10,'IRR','customer','platform','test','v1',now(),'{}')`, f.installment)
			assertViolation(t, e, "BN003")
		}},
		{"authoritative failure unfreezes", func(t *testing.T, f *fixture) {
			id := f.attempt(t)
			f.exec(t, `UPDATE payment_attempts SET state='failed',financial_outcome_resolved=true,failure_code='declined',failed_at=now() WHERE id=$1`, id)
			f.charge(t, "late_fee", "customer")
		}},
		{"immutable items", func(t *testing.T, f *fixture) {
			id := f.attempt(t)
			_, e := f.pool.Pool.Exec(f.ctx, `UPDATE payment_attempt_items SET amount_minor=1 WHERE payment_attempt_id=$1`, id)
			assertViolation(t, e, "BN004")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) { tc.run(t, newFixture(t, true)) })
	}
}

func TestHistoricalIntegrityPreflightDoesNotRepairData(t *testing.T) {
	f := newFixture(t, false)
	f.exec(t, markPaidSQL, f.installment)
	charge := f.charge(t, "late_fee", "customer")
	sql, e := os.ReadFile("../../../../../assets/db/audits/installment_integrity.sql")
	require.NoError(t, e)
	query := strings.Split(strings.Split(string(sql), "BEGIN TRANSACTION READ ONLY;")[1], ";")[0]
	query = strings.ReplaceAll(query, "bnpl.", f.schema+".")
	var issue, kind, status, currency string
	var customer, contract, installment, id uuid.UUID
	var amount int
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, query).Scan(&issue, &customer, &contract, &installment, &id, &kind, &status, &amount, &currency))
	require.Equal(t, f.customer, customer)
	require.Equal(t, f.contract, contract)
	require.Equal(t, f.installment, installment)
	require.Equal(t, charge, id)
	ddl, e := os.ReadFile("../../../../../assets/db/migrations/" + Migration)
	require.NoError(t, e)
	_, e = f.pool.Pool.Exec(f.ctx, strings.ReplaceAll(string(ddl), "bnpl.", f.schema+"."))
	assertViolation(t, e, "BN002")
	require.Contains(t, e.Error(), charge.String())
	var actual string
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT charge_status FROM installment_charges WHERE id=$1`, charge).Scan(&actual))
	require.Equal(t, "applied", actual)
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT status FROM installments WHERE id=$1`, f.installment).Scan(&actual))
	require.Equal(t, "paid", actual)
	var hasRevision bool
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=$1 AND table_name='installments' AND column_name='financial_revision')`, f.schema).Scan(&hasRevision))
	require.False(t, hasRevision)
	// Invalid historical paid rows are never presented as another payable installment.
	q := installmentrp.New()
	require.NoError(t, f.pool.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
		rows, e := q.Conn(c).ListUnpaidByCustomerID(ctx, model.CustomerID(f.customer), pagination.KeysetRequest{Limit: 10})
		require.NoError(t, e)
		require.Empty(t, rows)
		count, e := q.Conn(c).CountUnpaidByCustomerID(ctx, model.CustomerID(f.customer))
		require.NoError(t, e)
		require.Zero(t, count)
		return nil
	}))
}

func TestVerificationNeedsOnlyReadPrivileges(t *testing.T) {
	f := newFixture(t, true)
	role := "invariant_reader_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	f.exec(t, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` NOLOGIN`)
	f.exec(t, `GRANT USAGE ON SCHEMA `+pgx.Identifier{f.schema}.Sanitize()+` TO `+pgx.Identifier{role}.Sanitize())
	f.exec(t, `GRANT SELECT ON ALL TABLES IN SCHEMA `+pgx.Identifier{f.schema}.Sanitize()+` TO `+pgx.Identifier{role}.Sanitize())
	cfg, e := pgxpool.ParseConfig(os.Getenv("COLLECTIONS_TEST_DATABASE_URL"))
	require.NoError(t, e)
	cfg.ConnConfig.RuntimeParams["search_path"] = f.schema
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, e := c.Exec(ctx, "SET ROLE "+pgx.Identifier{role}.Sanitize())
		return e
	}
	reader, e := postgres.NewPool(f.ctx, cfg)
	require.NoError(t, e)
	defer reader.Close()
	_, e = reader.Pool.Exec(f.ctx, `CREATE TABLE `+pgx.Identifier{f.schema, "forbidden_runtime_ddl"}.Sanitize()+`(id int)`)
	assertViolation(t, e, "42501")
	require.NoError(t, VerifySchema(f.ctx, reader, f.schema))
}

// Financial-state fixtures carry a complete successful snapshot. Real monetary
// application itself is exercised by the Core integration tests.
func (f *fixture) settleFixture(t *testing.T) {
	t.Helper()
	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer tx.Rollback(f.ctx)
	q := &postgres.Tx{Tx: tx}
	row, e := installmentrp.New().Tx(q).LockByID(f.ctx, model.InstallmentID(f.installment))
	require.NoError(t, e)
	charges, e := installmentchargerp.New().Tx(q).ListByInstallmentIDs(f.ctx, []model.InstallmentID{row.ID})
	require.NoError(t, e)
	items, e := model.BuildPaymentAttemptItems(row, charges)
	require.NoError(t, e)
	amount, ok := model.PayableAmountFromItems(items)
	require.True(t, ok)
	id := uuid.New()
	_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempts(id,contract_id,installment_id,amount_minor,currency,state,channel,idempotency_key,request_fingerprint,financial_paid_at,applied_at,financial_outcome_resolved) VALUES($1,$2,$3,$4,'IRR','succeeded','gateway',$5,$6,now(),now(),true)`, id, f.contract, f.installment, amount, uuid.NewString(), "sha256:"+strings.Repeat("0", 64))
	require.NoError(t, e)
	var ids []model.InstallmentChargeID
	for _, x := range items {
		_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,$2,$3,$4,$5)`, id, x.ItemType, x.SourceID, x.AmountMinor, x.Effect)
		require.NoError(t, e)
		if x.ItemType == model.ChargePaymentAttemptItemType {
			ids = append(ids, model.InstallmentChargeID(x.SourceID))
		}
	}
	if len(ids) > 0 {
		_, e = installmentchargerp.New().Tx(q).MarkSnapshottedChargeIDsPaid(f.ctx, ids, time.Now().UTC(), time.Now().UTC())
		require.NoError(t, e)
	}
	_, e = tx.Exec(f.ctx, `UPDATE installments SET status='paid',paid_at=now(),paid_amount_minor=$2 WHERE id=$1`, f.installment, amount)
	require.NoError(t, e)
	require.NoError(t, tx.Commit(f.ctx))
}
