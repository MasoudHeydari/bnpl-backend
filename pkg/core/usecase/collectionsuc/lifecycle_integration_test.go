//go:build integration && development

package collectionsuc

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/contractrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/creditaccountrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/inboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	reconciliationevent "github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	fc "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	cancellation "github.com/parstis/bnpl-backend/pkg/core/external/contractcancellation"
	"github.com/parstis/bnpl-backend/pkg/core/policy/collections"
	collectionsv1 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v1"
	collectionsv2 "github.com/parstis/bnpl-backend/pkg/core/policy/collections/v2"
	"github.com/parstis/bnpl-backend/pkg/core/saga"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	reconciliationuc "github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"

	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/collectionscaserp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerrestrictionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/decisionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/defaultepisoderp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/repaymentreminderrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/shortlinkrp"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/gin/shortlinkrs"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy/shortlinkdestination"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/repaymentreminderuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/shortlinkuc"
	"github.com/stretchr/testify/require"
)

type integrationIDs struct{}

func (integrationIDs) Generate() string { return uuid.NewString() }

type rejectingOutbox struct{ repo.Outbox }

func (r rejectingOutbox) Tx(tx repo.Tx) repo.OutboxTxQueryer {
	return rejectingOutboxQuery{r.Outbox.Tx(tx)}
}

type rejectingOutboxQuery struct{ repo.OutboxTxQueryer }

func (rejectingOutboxQuery) Create(context.Context, repo.CreateOutboxMessage) (model.OutboxMessage, error) {
	return model.OutboxMessage{}, fmt.Errorf("injected outbox failure")
}

// Every scenario owns a unique schema in an explicitly supplied disposable
// database. No scenario reads another test's history or performs broad cleanup.
type collectionsFixture struct {
	ctx    context.Context
	pool   *postgres.Pool
	uc     *UseCase
	now    time.Time
	clk    adapterclock.Fixed
	exec   func(*testing.T, string, ...any)
	scalar func(*testing.T, string, ...any) int
	seed   func(*testing.T) (model.CustomerID, uuid.UUID)
}

func newCollectionsFixture(t *testing.T, locations ...*time.Location) *collectionsFixture {
	location := time.UTC
	if len(locations) > 0 {
		location = locations[0]
	}
	t.Helper()
	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "provide a disposable PostgreSQL database")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	schema := "bnpl_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["timezone"] = location.String()
	pool, err := postgres.NewPool(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })
	// Baseline DDL is shared with the existing integration/E2E infrastructure.
	ddl, err := os.ReadFile("../../../adapter/db/postgres/migrationsrp/000_schema.sql")
	require.NoError(t, err)
	_, err = pool.Pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	// Financial triggers and Collections constraints are part of the complete
	// pre-release baseline; never replay old migrations in fresh fixtures.
	baseline := strings.SplitN(string(ddl), "-- MANAGED DATABASE PERMISSIONS: canonical initial grants (pre-release)", 2)[0]
	baseline = strings.ReplaceAll(baseline, "SET search_path TO bnpl;", "SET search_path TO "+schema+";")
	baseline = strings.ReplaceAll(baseline, "bnpl.", schema+".")
	baseline = strings.ReplaceAll(baseline, "pg_catalog, bnpl", "pg_catalog, "+schema)
	_, err = pool.Pool.Exec(ctx, baseline)
	require.NoError(t, err)

	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	clk := adapterclock.NewFixed(now, location)
	uc, err := New(pool, customerprofilerp.New(), installmentrp.New(), decisionrp.New(), defaultepisoderp.New(), customerrestrictionrp.New(), collectionscaserp.New(), outboxrp.New(), clk, integrationIDs{}, sha256fingerprint.New(), metricsnoop.New(), "bnpl.audit.events.v1", 10, currentTestPolicies(location))
	require.NoError(t, err)
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		_, e := pool.Pool.Exec(ctx, sql, args...)
		require.NoError(t, e)
	}
	scalar := func(t *testing.T, sql string, args ...any) int {
		t.Helper()
		var n int
		require.NoError(t, pool.Pool.QueryRow(ctx, sql, args...).Scan(&n))
		return n
	}
	seed := func(t *testing.T) (model.CustomerID, uuid.UUID) {
		t.Helper()
		customer, merchant, account, decision, contract, installment := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
		exec(t, `INSERT INTO customer_profiles(id,external_customer_id,jurisdiction,status) VALUES($1,$2,'IR','active')`, customer, uuid.New())
		exec(t, `INSERT INTO merchant_profiles(id,merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction) VALUES($1,$2,'test','active','low',12,'IR')`, merchant, uuid.New())
		exec(t, `INSERT INTO decisions(id,type,target_type,target_id,outcome,reason_codes,policy_id,policy_version,jurisdiction,actor_type,decided_at,idempotency_key,idempotency_fingerprint) VALUES($1,'restriction','customer_profile',$2,'restricted',ARRAY['COLLECTIONS_GRACE_PERIOD_EXCEEDED'],'collections_default','v1','IR','system',$3,$4,$5)`, decision, customer, now, uuid.NewString(), "sha256:"+fmt.Sprintf("%064x", 1))
		exec(t, `INSERT INTO credit_accounts(id,customer_id,currency,status,effective_from) VALUES($1,$2,'IRR','active',$3)`, account, customer, now)
		exec(t, `INSERT INTO contracts(id,decision_id,checkout_key,customer_id,credit_account_id,merchant_profile_id,currency,requested_amount_minor,approved_financed_amount_minor,installment_count,payable_now_amount_minor,remaining_principal_amount_minor,status,expires_at,activated_at) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,2,0,100,'active',$7,$7)`, contract, decision, uuid.NewString(), customer, account, merchant, now)
		for order := 1; order <= 2; order++ {
			id := installment
			due := now.Add(-73 * time.Hour)
			if order == 2 {
				id = uuid.New()
				due = now.Add(30 * 24 * time.Hour)
			}
			exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,$7,2,'scheduled')`, id, decision, contract, customer, merchant, order, due)
		}
		return model.CustomerID(customer), installment
	}
	return &collectionsFixture{ctx: ctx, pool: pool, uc: uc, now: now, clk: clk, exec: exec, scalar: scalar, seed: seed}
}
func currentTestPolicies(loc *time.Location) *collections.Registry {
	r, _ := collections.NewRegistry(collectionsv2.New(loc), collectionsv1.New())
	return r
}

func TestPostgreSQLCollections_DefaultEntryConcurrencyAndRecoveryLineage(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, pool, uc, now, exec, scalar, seed := f.ctx, f.pool, f.uc, f.now, f.exec, f.scalar, f.seed

	customer, installment := seed(t)
	// The read-only guard closes the task-delay gap without persisting a transition.
	require.Error(t, pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
			_, e := customerprofilerp.New().Tx(tx).LockByID(ctx, customer)
			if e != nil {
				return e
			}
			return uc.AssertNewCreditAllowed(ctx, tx, customer, now)
		})
	}))
	require.Equal(t, 0, scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1`, customer))
	var wg sync.WaitGroup
	results := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := uc.AssessCustomer(ctx, customer); results <- e }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		require.NoError(t, e)
	}
	for _, table := range []string{"default_episodes", "customer_restrictions", "collections_cases"} {
		require.Equal(t, 1, scalar(t, `SELECT count(*) FROM `+table+` WHERE customer_id=$1`, customer))
	}
	var episode model.DefaultEpisodeID
	var opening model.DecisionID
	require.NoError(t, pool.Pool.QueryRow(ctx, `SELECT id,opening_decision_id FROM default_episodes WHERE customer_id=$1`, customer).Scan(&episode, &opening))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM collections_case_events e JOIN collections_cases c ON c.id=e.case_id WHERE c.customer_id=$1`, customer))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM integration_outbox WHERE message_key=$1`, opening.String()))
	f.settleFixture(t, installment, now)
	recovered, e := uc.AssessCustomer(ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionRecovered, recovered.Transition)
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM decisions WHERE id=$1 AND prior_decision_id=$2`, recovered.ClosingDecisionID, opening))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND released_at IS NOT NULL AND default_episode_id=$2`, customer, episode))
	again, e := uc.AssessCustomer(ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionNone, again.Transition)
	// A later independent episode starts a fresh Decision chain and retains all history.
	exec(t, `UPDATE installments SET due_at=$2 WHERE customer_id=$1 AND status='scheduled'`, customer, now.Add(-72*time.Hour))
	next, e := uc.AssessCustomer(ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionDefaulted, next.Transition)
	require.Equal(t, 2, scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1`, customer))
	require.Equal(t, 2, scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1`, customer))
	require.Equal(t, 2, scalar(t, `SELECT count(*) FROM collections_cases WHERE customer_id=$1`, customer))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM decisions WHERE id=$1 AND prior_decision_id IS NULL`, next.OpeningDecisionID))
}

func TestPostgreSQLCollections_TransactionRollback(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, uc, scalar, seed := f.ctx, f.uc, f.scalar, f.seed

	customer, _ := seed(t)
	uc.auditDestination = ""
	defer func() { uc.auditDestination = "bnpl.audit.events.v1" }()
	_, e := uc.AssessCustomer(ctx, customer)
	require.Error(t, e)
	for _, table := range []string{"default_episodes", "customer_restrictions", "collections_cases"} {
		require.Zero(t, scalar(t, `SELECT count(*) FROM `+table+` WHERE customer_id=$1`, customer))
	}
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM decisions WHERE target_id=$1`, customer))
}

func TestPostgreSQLCollections_RecoveryRollbackAndRestrictionIdentity(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, uc, now, exec, scalar, seed := f.ctx, f.uc, f.now, f.exec, f.scalar, f.seed

	customer, installment := seed(t)
	opened, e := uc.AssessCustomer(ctx, customer)
	require.NoError(t, e)
	f.settleFixture(t, installment, now)
	uc.auditDestination = ""
	_, e = uc.AssessCustomer(ctx, customer)
	uc.auditDestination = "bnpl.audit.events.v1"
	require.Error(t, e)
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND status='active'`, customer))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1 AND closing_decision_id IS NULL`, customer))
	require.Equal(t, 2, scalar(t, `SELECT count(*) FROM decisions WHERE target_id=$1`, customer))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM collections_case_events e JOIN collections_cases c ON c.id=e.case_id WHERE c.customer_id=$1`, customer))
	// An unrelated/mismatched active restriction is not released by recovery.
	exec(t, `UPDATE customer_restrictions SET default_episode_id=NULL WHERE customer_id=$1`, customer)
	_, e = uc.AssessCustomer(ctx, customer)
	require.Error(t, e)
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND status='active'`, customer))
	exec(t, `UPDATE customer_restrictions SET default_episode_id=$2 WHERE customer_id=$1`, customer, opened.EpisodeID)
	_, e = uc.AssessCustomer(ctx, customer)
	require.NoError(t, e)
}

func TestPostgreSQLCollections_ReadSnapshot(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, pool, now, scalar, seed := f.ctx, f.pool, f.now, f.scalar, f.seed

	customer, installment := seed(t)
	require.NoError(t, pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		return c.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
			q := installmentrp.New().Tx(tx)
			before, e := q.CountUnpaidByCustomerID(ctx, customer)
			if e != nil {
				return e
			}
			f.settleFixture(t, installment, now)
			after, e := q.CountUnpaidByCustomerID(ctx, customer)
			if e != nil {
				return e
			}
			require.Equal(t, before, after)
			require.Equal(t, 2, after)
			return nil
		})
	}))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM installments WHERE customer_id=$1 AND status='scheduled'`, customer))
}

func TestPostgreSQLCollections_ReminderAtomicDispatch(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, pool, now, clk, exec, scalar, seed := f.ctx, f.pool, f.now, f.clk, f.exec, f.scalar, f.seed

	customer, installment := seed(t)
	exec(t, `UPDATE installments SET due_at=$2,grace_period_days=3 WHERE id=$1`, installment, now)
	policy, e := shortlinkdestination.New([]string{"pay.example.com"})
	require.NoError(t, e)
	links, e := shortlinkuc.New(pool, shortlinkrp.New(), policy, clk, integrationIDs{}, metricsnoop.New())
	require.NoError(t, e)
	build := func(outbox repo.Outbox) *repaymentreminderuc.UseCase {
		reminder, e := repaymentreminderuc.New(pool, installmentrp.New(), repaymentreminderrp.New(), paymentattemptrp.New(), outbox, links, clk, integrationIDs{}, metricsnoop.New(), "bnpl.notifications.v1", "s.example.com", "https://pay.example.com/installments", 10)
		require.NoError(t, e)
		return reminder
	}
	initialLinks := scalar(t, `SELECT count(*) FROM short_links`)
	_, e = build(rejectingOutbox{outboxrp.New()}).Dispatch(ctx)
	require.Error(t, e)
	require.Zero(t, scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
	require.Equal(t, initialLinks, scalar(t, `SELECT count(*) FROM short_links`))
	reminders := build(outboxrp.New())
	_, e = reminders.Dispatch(ctx)
	require.NoError(t, e)
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
	require.Equal(t, 1, scalar(t, `SELECT count(*) FROM integration_outbox o JOIN repayment_reminders r ON o.message_key=r.external_customer_id::text WHERE r.customer_id=$1 AND o.destination='bnpl.notifications.v1'`, customer))
	_, e = reminders.Dispatch(ctx)
	require.NoError(t, e)
	require.Equal(t, initialLinks+1, scalar(t, `SELECT count(*) FROM short_links`))
	var code string
	require.NoError(t, pool.Pool.QueryRow(ctx, `SELECT s.code FROM short_links s JOIN repayment_reminders r ON s.id=r.short_link_id WHERE r.customer_id=$1`, customer).Scan(&code))
	engine := gin.New()
	shortlinkrs.Register(engine, links, shortlinkrs.RegisterOptions{PublicHost: "s.example.com", DynamicCodes: true})
	for _, path := range []string{"/health", "/swagger", "/api/v1"} {
		engine.GET(path, func(c *gin.Context) { c.Status(http.StatusOK) })
	}
	req := httptest.NewRequest(http.MethodGet, "https://s.example.com/"+code, nil)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://pay.example.com/installments", rec.Header().Get("Location"))
	for _, path := range []string{"/health", "/swagger", "/api/v1"} {
		req = httptest.NewRequest(http.MethodGet, "https://api.example.com"+path, nil)
		rec = httptest.NewRecorder()
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusOK, rec.Code)
	}
}

func TestPostgreSQLCollections_GraceBoundaries(t *testing.T) {
	loc, e := time.LoadLocation("America/New_York")
	require.NoError(t, e)
	f := newCollectionsFixture(t, loc)
	ctx, pool, exec, seed := f.ctx, f.pool, f.exec, f.seed

	for _, due := range []time.Time{time.Date(2026, 3, 7, 15, 30, 0, 0, loc), time.Date(2026, 10, 31, 15, 30, 0, 0, loc)} {
		for _, days := range []uint{0, 1, 3, 45} {
			customer, installment := seed(t)
			exec(t, `UPDATE installments SET due_at=$2,grace_period_days=$3,status='due' WHERE id=$1`, installment, due, days)
			boundary := (model.Installment{DueAt: due, GracePeriodDays: days}).OverdueEligibleAt(loc)
			for _, offset := range []time.Duration{-time.Microsecond, 0, 48 * time.Hour} {
				at := boundary.Add(offset)
				require.NoError(t, pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
					candidates, e := installmentrp.New().Conn(c).ListCollectionsAssessmentCustomerPage(ctx, at, model.CustomerID{}, 10000)
					if e != nil {
						return e
					}
					found := false
					for _, id := range candidates {
						found = found || id == customer
					}
					// Candidate scans include all due obligations for recovery;
					// only default policy and overdue claims enforce the grace boundary.
					require.Equal(t, !at.Before(due), found)
					return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
						rows, e := installmentrp.New().Tx(tx).ClaimDueOverdueEligible(ctx, at, 10000)
						if e != nil {
							return e
						}
						found := false
						for _, row := range rows {
							found = found || row.ID.UUID() == installment
						}
						require.Equal(t, offset >= 0, found)
						return nil
					})
				}))
			}
		}
	}
}

func TestPostgreSQLCollections_LongGraceReminderCandidates(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, pool, now, exec, seed := f.ctx, f.pool, f.now, f.exec, f.seed

	customer, installment := seed(t)
	exec(t, `UPDATE installments SET due_at=$2,grace_period_days=45 WHERE id=$1`, installment, now.Add(-40*24*time.Hour))
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	require.NoError(t, pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
		rows, e := installmentrp.New().Conn(c).ListReminderCandidatePage(ctx, day.AddDate(0, 0, -31), day.AddDate(0, 0, 2), day, model.CustomerID{}, 10000)
		if e != nil {
			return e
		}
		found := false
		for _, row := range rows {
			found = found || row.Installment.CustomerID == customer
		}
		require.True(t, found)
		return nil
	}))
}

func TestPostgreSQLCollections_BatchFailures(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, uc, seed := f.ctx, f.uc, f.seed

	for range 15 {
		seed(t)
	}
	observed := 0
	uc.WithAssessmentFailureObserver(func(_ context.Context, _ model.CustomerID, _ error) { observed++ })
	defer uc.WithAssessmentFailureObserver(nil)
	uc.auditDestination = ""
	result, e := uc.AssessBatch(ctx)
	uc.auditDestination = "bnpl.audit.events.v1"
	require.Error(t, e)
	require.GreaterOrEqual(t, result.FailedCount, 15)
	require.Equal(t, result.FailedCount, observed)
	require.Contains(t, e.Error(), "first 10")
}

func TestPostgreSQLCollections_PaymentUncertainty(t *testing.T) {
	f := newCollectionsFixture(t)
	ctx, pool, uc, now, seed := f.ctx, f.pool, f.uc, f.now, f.seed

	customer, _ := seed(t)
	opened, e := uc.AssessCustomer(ctx, customer)
	require.NoError(t, e)
	episode := model.DefaultEpisode{ID: *opened.EpisodeID, CustomerID: customer}
	a, b := model.InstallmentID(uuid.New()), model.InstallmentID(uuid.New())
	for _, ids := range [][]model.InstallmentID{{a}, {a}, {a, b}, {b, a}, {}, {}, {a}, {a}} {
		require.NoError(t, pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
			return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
				_, e := customerprofilerp.New().Tx(tx).LockByID(ctx, customer)
				if e != nil {
					return e
				}
				return uc.notePaymentUncertainty(ctx, collectionscaserp.New().Tx(tx), episode, ids, now)
			})
		}))
	}
	rows, e := pool.Pool.Query(ctx, `SELECT e.payload FROM collections_case_events e JOIN collections_cases c ON c.id=e.case_id WHERE c.customer_id=$1 AND event_type='payment_uncertainty_noted' ORDER BY e.occurred_at,e.id`, customer)
	require.NoError(t, e)
	defer rows.Close()
	var states []bool
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		var state struct {
			Uncertain bool `json:"has_payment_uncertainty"`
		}
		require.NoError(t, json.Unmarshal(raw, &state))
		states = append(states, state.Uncertain)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []bool{true, true, false, true}, states)
}

func (f *collectionsFixture) reminders(t *testing.T, at time.Time, outbox repo.Outbox) *repaymentreminderuc.UseCase {
	t.Helper()
	policy, e := shortlinkdestination.New([]string{"pay.example.com"})
	require.NoError(t, e)
	clk := adapterclock.NewFixed(at, f.uc.clock.BusinessLocation())
	links, e := shortlinkuc.New(f.pool, shortlinkrp.New(), policy, clk, integrationIDs{}, metricsnoop.New())
	require.NoError(t, e)
	reminders, e := repaymentreminderuc.New(f.pool, installmentrp.New(), repaymentreminderrp.New(), paymentattemptrp.New(), outbox, links, clk, integrationIDs{}, metricsnoop.New(), "bnpl.notifications.v1", "s.example.com", "https://pay.example.com/installments", 10)
	require.NoError(t, e)
	return reminders.WithDefaultNotifications(f.uc)
}

func TestPostgreSQLRepaymentReminder_DefaultNotification(t *testing.T) {
	location, e := time.LoadLocation("Asia/Tehran")
	require.NoError(t, e)
	f := newCollectionsFixture(t, location)
	customer, installment := f.seed(t)
	due := time.Date(2026, 11, 1, 18, 0, 0, 0, location)
	f.exec(t, `UPDATE installments SET due_at=$2,grace_period_days=5 WHERE id=$1`, installment, due)
	boundary := (model.Installment{DueAt: due, GracePeriodDays: 5}).OverdueEligibleAt(location)
	f.uc.clock = adapterclock.NewFixed(boundary, location)
	opened, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionDefaulted, opened.Transition)
	f.exec(t, `UPDATE installments SET due_at=$2 WHERE customer_id=$1 AND id<>$3`, customer, boundary.Add(18*time.Hour), installment)
	// Reserve the daily slot for default, rather than sending a 10:00 due reminder.
	for _, at := range []time.Time{boundary, boundary.Add(12*time.Hour + 59*time.Minute)} {
		result, e := f.reminders(t, at, outboxrp.New()).Dispatch(f.ctx)
		require.NoError(t, e)
		require.Zero(t, result.CreatedCount)
	}
	at := boundary.Add(13 * time.Hour)
	var wg sync.WaitGroup
	errs := make(chan error, 6)
	for range 6 {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.reminders(t, at, outboxrp.New()).Dispatch(f.ctx); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1 AND reminder_type='collections_default' AND template_id='bnpl.collections.default.v1' AND notification_idempotency_key=$2`, customer, "collections-default:"+opened.EpisodeID.String()))
	for _, later := range []time.Time{at.Add(5 * time.Minute), at.AddDate(0, 0, 1)} {
		_, e = f.reminders(t, later, outboxrp.New()).Dispatch(f.ctx)
		require.NoError(t, e)
	}
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM short_links`))
}

func TestPostgreSQLRepaymentReminder_DefaultNotificationOutboxRetryAndCatchup(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, _ := f.seed(t)
	opened, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionDefaulted, opened.Transition)
	at := f.now.Add(24*time.Hour + time.Hour)
	_, e = f.reminders(t, at, rejectingOutbox{outboxrp.New()}).Dispatch(f.ctx)
	require.Error(t, e)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders`))
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM short_links`))
	result, e := f.reminders(t, at.Add(5*time.Minute), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 1, result.CreatedCount)
}

func TestPostgreSQLCollections_CancellationIsNotSettlement(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	_, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	f.exec(t, `UPDATE contracts SET status='canceling',cancellation_idempotency_key='test',cancellation_reason='customer_requested',cancellation_request_fingerprint=$2,cancellation_requested_at=$3,cancellation_financial_core_idempotency_key='test',cancellation_tenant_id=$4,cancellation_context_id=$5 WHERE customer_id=$1`, customer, "sha256:"+fmt.Sprintf("%064x", 1), f.now, uuid.New(), uuid.New())
	result, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionBlocked, result.Transition)
	require.True(t, result.HasPaymentUncertainty)
	_, e = f.reminders(t, f.now.Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders`))
	// Definitive rejection compensates to active and retains the outstanding debt.
	f.exec(t, `UPDATE contracts SET status='active' WHERE customer_id=$1`, customer)
	result, e = f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionBlocked, result.Transition)
	// This focused repository scenario represents FC-confirmed cancellation;
	// actual Saga/payment paths have separate integration coverage.
	f.exec(t, `UPDATE contracts SET status='canceled',canceled_at=$2 WHERE customer_id=$1`, customer, f.now)
	result, e = f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionRecovered, result.Transition)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1`, installment))
}

func TestPostgreSQLCollections_IndependentRestrictionSurvivesRecovery(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	opened, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	f.exec(t, `INSERT INTO customer_restrictions(id,customer_id,restriction_type,reason,status,opening_decision_id,applied_at) VALUES($1,$2,'new_credit_usage','administrative_hold','active',$3,$4)`, uuid.New(), customer, opened.OpeningDecisionID, f.now)
	f.settleFixture(t, installment, f.now)
	result, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionRecovered, result.Transition)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND reason='administrative_hold' AND status='active'`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND reason='collections_default' AND status='released'`, customer))
}

// Only the external boundary is faked. Inbox/Outbox, repositories, monetary
// application, principal release and Collections recovery use production code.
type authoritativePaymentClient struct {
	fc.Client
	status fc.GetPaymentStatusResult
}

func (c authoritativePaymentClient) GetPaymentStatus(context.Context, fc.GetPaymentStatusInput) (fc.GetPaymentStatusResult, error) {
	return c.status, nil
}

func (f *collectionsFixture) settlement(t *testing.T, customer model.CustomerID, installment uuid.UUID, paidAt time.Time, amount int64) (*financialcorepaymentresultuc.UseCase, financialcorepaymentresultuc.ProcessInput) {
	t.Helper()
	f.exec(t, `UPDATE installments SET installment_order=3 WHERE customer_id=$1 AND id<>$2`, customer, installment)
	f.exec(t, `UPDATE installments SET installment_order=2 WHERE id=$1`, installment)
	f.exec(t, `UPDATE contracts SET requested_amount_minor=200,approved_financed_amount_minor=200,remaining_principal_amount_minor=200,installment_count=3 WHERE customer_id=$1`, customer)
	f.exec(t, `UPDATE credit_accounts SET limit_amount_minor=1000,exposure_amount_minor=200 WHERE customer_id=$1`, customer)
	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))
	attempt := uuid.New()
	intent := "fc-" + uuid.NewString()

	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer tx.Rollback(f.ctx)
	charges, e := installmentchargerp.New().Tx(&postgres.Tx{Tx: tx}).ListByInstallmentIDs(f.ctx, []model.InstallmentID{model.InstallmentID(installment)})
	require.NoError(t, e)
	row := model.Installment{ID: model.InstallmentID(installment), PrincipalAmountMinor: 100, Currency: model.IRRCurrency, Status: model.ScheduledInstallmentStatus}
	items, e := model.BuildPaymentAttemptItems(row, charges)
	require.NoError(t, e)
	cash, ok := model.PayableAmountFromItems(items)
	require.True(t, ok)
	_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempts(id,contract_id,installment_id,amount_minor,currency,state,channel,idempotency_key,request_fingerprint,financial_payment_intent_id,financial_collection_obligation_id,submitted_at) VALUES($1,$2,$3,$8,'IRR','submitted','gateway',$4,$5,$6,'collection-1',$7)`, attempt, contract, installment, uuid.NewString(), "sha256:"+fmt.Sprintf("%064x", 1), intent, f.now.Add(-time.Hour), cash)
	require.NoError(t, e)
	for _, item := range items {
		_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,$2,$3,$4,$5)`, attempt, item.ItemType, item.SourceID, item.AmountMinor, item.Effect)
		require.NoError(t, e)
	}
	require.NoError(t, tx.Commit(f.ctx))

	client := authoritativePaymentClient{status: fc.GetPaymentStatusResult{PaymentIntentID: intent, Status: fc.SucceededPaymentStatus, AmountMinor: amount, Currency: "IRR", CompletedAt: paidAt}}
	result, e := financialcorepaymentresultuc.New(f.pool, &contractuc.UseCase{}, &checkoutsessionuc.UseCase{}, client, inboxrp.New(), outboxrp.New(), paymentattemptrp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), creditaccountrp.New(), integrationIDs{}, sha256fingerprint.New(), "fc.payment.applications", "bnpl.audit.events.v1", f.clk, 0, metricsnoop.New())
	require.NoError(t, e)
	result.WithCollectionsCoordination(customerprofilerp.New(), func(ctx context.Context, id model.CustomerID) error { _, e := f.uc.AssessCustomer(ctx, id); return e })
	input := financialcorepaymentresultuc.ProcessInput{EventID: uuid.NewString(), EventType: "financial_core.payment_result.v1", EventVersion: "1", SourceService: "financial-core", RawPayload: json.RawMessage(`{"wake_up":true}`), PaymentPurpose: model.InstallmentRepaymentFinancialCorePaymentPurpose, ContractID: contract, InstallmentID: model.InstallmentID(installment), PaymentIntentID: intent, PaymentStatus: model.SucceededFinancialPaymentResultStatus, EventOccurredAt: f.now}
	return result, input
}

func TestPostgreSQLCollections_PaymentResultConcurrencyAndPromptRecovery(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	// Financial settlement predates the new default boundary; delivery is delayed.
	paidAt := (model.Installment{DueAt: f.now.Add(-73 * time.Hour), GracePeriodDays: 2}).OverdueEligibleAt(time.UTC).Add(-time.Minute)
	result, input := f.settlement(t, customer, installment, paidAt, 100)
	opened, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionDefaulted, opened.Transition)
	require.True(t, opened.HasPaymentUncertainty)
	_, e = f.reminders(t, f.now.Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders`))
	var wg sync.WaitGroup
	errs := make(chan error, 9)
	wg.Add(1)
	go func() { defer wg.Done(); _, e := result.Process(f.ctx, input); errs <- e }()
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := f.uc.AssessCustomer(f.ctx, customer); errs <- e }()
	}
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := f.pool.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
				return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
					_, e := customerprofilerp.New().Tx(tx).LockByID(ctx, customer)
					if e != nil {
						return e
					}
					e = f.uc.AssertNewCreditAllowed(ctx, tx, customer, f.now)
					if e == nil {
						row, loadErr := installmentrp.New().Tx(tx).GetByID(ctx, model.InstallmentID(installment))
						if loadErr != nil {
							return loadErr
						}
						if row.Status != model.PaidInstallmentStatus {
							return fmt.Errorf("credit authorized before authoritative settlement")
						}
					}
					if e != nil && !cerr.IsCode(e, cerr.CodeConflict) {
						return e
					}
					return nil
				})
			})
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status='paid' AND paid_amount_minor=100`, installment))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1 AND status='recovered'`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND status='released'`, customer))
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	require.Equal(t, 100, f.scalar(t, `SELECT remaining_principal_amount_minor FROM contracts WHERE customer_id=$1`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM collections_case_events e JOIN collections_cases c ON c.id=e.case_id WHERE c.customer_id=$1 AND e.event_type='customer_recovered' AND e.payload->>'late_authoritative_settlement_correction'='true'`, customer))
	_, e = f.reminders(t, f.now.Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders`))
	// Inbox replay and an independent duplicate success never release twice.
	_, e = result.Process(f.ctx, input)
	require.NoError(t, e)
	input.EventID = uuid.NewString()
	_, e = result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1`, customer))
	input.EventID = uuid.NewString()
	input.PaymentStatus = model.FailedFinancialPaymentResultStatus
	_, e = result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
}

func TestPostgreSQLCollections_MismatchedAmountDoesNotSettle(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 110)
	_, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	got, e := result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultReconciliationRequired, got.Result)
	require.Equal(t, 0, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status='paid'`, installment))
	require.Equal(t, 200, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND status='active'`, customer))
}

func TestPostgreSQLCollections_AdditionalChargesRejectedDuringPayment(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 100)
	_, e := f.pool.Pool.Exec(f.ctx, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',20,'IRR','customer','platform','test','v1',$2,'{}')`, installment, f.now)
	require.Error(t, e)
	got, e := result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, got.Result)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
}

type cancellationClient struct{ accepted bool }

func (c cancellationClient) RequestCancellation(context.Context, cancellation.RequestCancellationInput) (cancellation.RequestCancellationResult, error) {
	return cancellation.RequestCancellationResult{Accepted: c.accepted}, nil
}

func TestPostgreSQLCollections_AuthoritativeCancellationSaga(t *testing.T) {
	for _, accepted := range []bool{false, true} {
		t.Run(fmt.Sprint("FC accepted=", accepted), func(t *testing.T) {
			f := newCollectionsFixture(t)
			customer, installment := f.seed(t)
			_, e := f.uc.AssessCustomer(f.ctx, customer)
			require.NoError(t, e)
			f.exec(t, `UPDATE credit_accounts SET exposure_amount_minor=100 WHERE customer_id=$1`, customer)
			f.exec(t, `UPDATE contracts SET status='canceling',cancellation_idempotency_key='test',cancellation_reason='customer_requested',cancellation_request_fingerprint=$2,cancellation_requested_at=$3,cancellation_financial_core_idempotency_key='test',cancellation_tenant_id=$4,cancellation_context_id=$5 WHERE customer_id=$1`, customer, "sha256:"+fmt.Sprintf("%064x", 1), f.now, uuid.New(), uuid.New())
			var contract model.ContractID
			require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))
			definition, e := contractuc.NewCancellationSagaDefinition(f.pool, contractrp.New(), creditaccountrp.New(), cancellationClient{accepted}, f.clk, time.Second, time.Minute)
			require.NoError(t, e)
			definition.WithCustomerSynchronization(customerprofilerp.New())
			subjectType, subjectID := "contract", contract.String()
			instance := model.SagaInstance{SubjectType: &subjectType, SubjectID: &subjectID}
			step, ok := definition.Step("contract_cancellation_external_request")
			require.True(t, ok)
			external, e := step.(saga.ExternalStep).Execute(f.ctx, instance)
			require.NoError(t, e)
			before, e := f.uc.AssessCustomer(f.ctx, customer)
			require.NoError(t, e)
			require.Equal(t, AssessTransitionBlocked, before.Transition)
			require.NoError(t, f.pool.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
				return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
					if accepted {
						require.Equal(t, saga.StepOutcomeCompleted, external.Outcome)
						step, ok := definition.Step("contract_cancellation_external_accepted")
						require.True(t, ok)
						_, e = step.(saga.TransactionalStep).ExecuteTx(ctx, tx, instance)
					} else {
						require.Equal(t, saga.StepOutcomeCompensate, external.Outcome)
						step, ok := definition.Step("contract_cancellation_local_started")
						require.True(t, ok)
						_, e = step.(saga.TransactionalCompensatableStep).CompensateTx(ctx, tx, instance)
					}
					return e
				})
			}))
			after, e := f.uc.AssessCustomer(f.ctx, customer)
			require.NoError(t, e)
			if accepted {
				require.Equal(t, AssessTransitionRecovered, after.Transition)
				require.Zero(t, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
			} else {
				require.Equal(t, AssessTransitionBlocked, after.Transition)
				require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
			}
		})
	}
}

func TestPostgreSQLCollections_ReconciledSettlementTriggersRecovery(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 100)
	var attempt model.PaymentAttemptID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT id FROM payment_attempts WHERE financial_payment_intent_id=$1`, input.PaymentIntentID).Scan(&attempt))
	f.exec(t, `UPDATE payment_attempts SET state='reconciliation_required',reconciliation_reason='financial_core_status_still_unknown',reconciliation_requested_at=$2,reconciliation_attempt_count=1 WHERE id=$1`, attempt, f.now)
	_, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	uc, e := reconciliationuc.New(f.pool, result, inboxrp.New(), outboxrp.New(), paymentattemptrp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), creditaccountrp.New(), sha256fingerprint.New(), integrationIDs{}, "fc.reconciliation.acks", "bnpl.audit.events.v1", time.Second, time.Minute, f.clk, lognoop.New())
	require.NoError(t, e)
	paidAt := f.now.Add(-time.Minute)
	message := reconciliationuc.ProcessInput{EventID: uuid.NewString(), EventType: reconciliationevent.ReconciliationResultEventType, EventVersion: "1", SourceService: "financial-core", RawPayload: json.RawMessage(`{"resolved":true}`), Data: reconciliationevent.ResultEventData{RequestEventID: reconciliationevent.RequestEventID(attempt, 1), BNPLPaymentAttemptID: attempt, ReconciliationGeneration: 1, Status: reconciliationevent.SucceededResultStatus, FinancialPaymentIntentID: &input.PaymentIntentID, PaidAt: &paidAt, AmountMinor: 100, Currency: "IRR", OccurredAt: f.now}}
	got, e := uc.Process(f.ctx, message)
	require.NoError(t, e)
	require.Equal(t, reconciliationuc.ProcessResultResolvedSucceeded, got.Result)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1 AND status='recovered'`, customer))
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	_, e = uc.Process(f.ctx, message)
	require.NoError(t, e)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
}

func TestPostgreSQLCollections_HistoricalPolicyRemainsV1(t *testing.T) {
	f := newCollectionsFixture(t)
	f.uc.policies = legacyTestPoliciesForUnit()
	customer, installment := f.seed(t)
	opening, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	f.uc.policies = currentTestPolicies(time.UTC)
	f.settleFixture(t, installment, f.now)
	recovered, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM decisions WHERE id=$1 AND prior_decision_id=$2 AND policy_id='collections_default' AND policy_version='v1'`, recovered.ClosingDecisionID, opening.OpeningDecisionID))
}

func TestPostgreSQLRepaymentReminder_IndependentEpisodesAndDailyCeiling(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	first, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	at := f.now.Add(time.Hour)
	_, e = f.reminders(t, at, outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
	// Repository fixture for notification history. Monetary settlement itself is
	// exercised separately through the actual authoritative FC application path.
	f.settleFixture(t, installment, f.now)
	recovered, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionRecovered, recovered.Transition)
	f.exec(t, `UPDATE installments SET due_at=$2 WHERE customer_id=$1 AND status='scheduled'`, customer, f.now.Add(-73*time.Hour))
	second, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.NotEqual(t, first.EpisodeID, second.EpisodeID)
	_, e = f.reminders(t, at, outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
	_, e = f.reminders(t, at.AddDate(0, 0, 1), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 2, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1 AND reminder_type='collections_default'`, customer))
	require.Equal(t, 2, f.scalar(t, `SELECT count(DISTINCT notification_idempotency_key) FROM repayment_reminders WHERE customer_id=$1`, customer))
	require.Equal(t, 2, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1`, customer))
}

func TestPostgreSQLCollections_ZeroAndNegativeCashNeedReconciliation(t *testing.T) {
	for _, discount := range []int{100, 101} {
		t.Run(fmt.Sprint(discount), func(t *testing.T) {
			f := newCollectionsFixture(t)
			customer, installment := f.seed(t)
			f.exec(t, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'discount','applied',$2,'IRR','customer','platform','test','v1',$3,'{}')`, installment, discount, f.now)
			got, e := f.uc.AssessCustomer(f.ctx, customer)
			require.NoError(t, e)
			require.Equal(t, AssessTransitionNone, got.Transition)
			require.Zero(t, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1`, customer))
			require.NoError(t, f.pool.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
				return c.Tx(ctx, func(ctx context.Context, tx repo.Tx) error {
					_, e := customerprofilerp.New().Tx(tx).LockByID(ctx, customer)
					require.NoError(t, e)
					require.Error(t, f.uc.AssertNewCreditAllowed(ctx, tx, customer, f.now))
					return nil
				})
			}))
			_, e = f.reminders(t, f.now.Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
			require.NoError(t, e)
			require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
			require.Zero(t, f.scalar(t, `SELECT count(*) FROM installments WHERE customer_id=$1 AND status='paid'`, customer))
		})
	}
}

func TestPostgreSQLRepaymentReminder_SettledTriggerWithAnotherDueWithinGrace(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 100)
	f.exec(t, `UPDATE installments SET due_at=$2,grace_period_days=5 WHERE customer_id=$1 AND id<>$3`, customer, f.now.Add(-time.Hour), installment)
	opened, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionDefaulted, opened.Transition)
	_, e = result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1 AND status='active'`, customer))
	_, e = f.reminders(t, f.now.Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND status='active'`, customer))
	// Once the remaining debt itself exceeds grace, a still-unsent episode notice
	// may truthfully reference that current debt, while the original trigger stays paid.
	later, e := f.reminders(t, f.now.AddDate(0, 0, 6).Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 1, later.CreatedCount)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1 AND $2=ANY(installment_ids)`, customer, installment))

}

func TestPostgreSQLCollections_ManualReviewStillAcceptsAuthoritativeSuccess(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 100)
	f.exec(t, `UPDATE payment_attempts SET state='manual_review_required',manual_review_required_at=$2,last_reconciliation_error='lookup timeout',next_reconciliation_at=NULL WHERE installment_id=$1`, installment, f.now)
	_, e := result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM payment_attempts WHERE installment_id=$1 AND state='succeeded' AND financial_outcome_resolved`, installment))
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
}

func TestPostgreSQLCollections_ReconciliationNotFoundKeepsFinancialFreeze(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, _ := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 100)
	var attempt model.PaymentAttemptID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT id FROM payment_attempts WHERE installment_id=$1`, installment).Scan(&attempt))
	f.exec(t, `UPDATE payment_attempts SET state='reconciliation_required',reconciliation_reason='financial_core_status_still_unknown',reconciliation_requested_at=$2,reconciliation_attempt_count=1 WHERE id=$1`, attempt, f.now)
	uc, e := reconciliationuc.New(f.pool, result, inboxrp.New(), outboxrp.New(), paymentattemptrp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), creditaccountrp.New(), sha256fingerprint.New(), integrationIDs{}, "fc.reconciliation.acks", "bnpl.audit.events.v1", time.Second, time.Minute, f.clk, lognoop.New())
	require.NoError(t, e)
	message := reconciliationuc.ProcessInput{EventID: uuid.NewString(), EventType: reconciliationevent.ReconciliationResultEventType, EventVersion: "1", SourceService: "financial-core", RawPayload: json.RawMessage(`{"absent":true}`), Data: reconciliationevent.ResultEventData{RequestEventID: reconciliationevent.RequestEventID(attempt, 1), BNPLPaymentAttemptID: attempt, ReconciliationGeneration: 1, Status: reconciliationevent.NotFoundResultStatus, AmountMinor: 100, Currency: "IRR", OccurredAt: f.now}}
	got, e := uc.Process(f.ctx, message)
	require.NoError(t, e)
	require.Equal(t, reconciliationuc.ProcessResultStillReconciliationRequired, got.Result)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM payment_attempts WHERE id=$1 AND state='reconciliation_required' AND NOT financial_outcome_resolved`, attempt))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM contracts WHERE customer_id=$1 AND status='active'`, customer))
}

// settleFixture creates explicit immutable payment evidence for policy/repository
// fixtures. Financial correctness scenarios above use the actual FC consumer.
func (f *collectionsFixture) settleFixture(t *testing.T, id uuid.UUID, paidAt time.Time) {
	t.Helper()
	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer tx.Rollback(f.ctx)
	q := &postgres.Tx{Tx: tx}
	i, e := installmentrp.New().Tx(q).LockByID(f.ctx, model.InstallmentID(id))
	require.NoError(t, e)
	charges, e := installmentchargerp.New().Tx(q).ListByInstallmentIDs(f.ctx, []model.InstallmentID{i.ID})
	require.NoError(t, e)
	items, e := model.BuildPaymentAttemptItems(i, charges)
	require.NoError(t, e)
	amount, ok := model.PayableAmountFromItems(items)
	require.True(t, ok)
	attempt := uuid.New()
	_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempts(id,contract_id,installment_id,amount_minor,currency,state,channel,idempotency_key,request_fingerprint,financial_paid_at,applied_at,financial_outcome_resolved) VALUES($1,$2,$3,$4,$5,'succeeded','gateway',$6,$7,$8,$8,true)`, attempt, i.ContractID, i.ID, amount, i.Currency, uuid.NewString(), "sha256:"+strings.Repeat("0", 64), paidAt)
	require.NoError(t, e)
	var ids []model.InstallmentChargeID
	for _, x := range items {
		_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,$2,$3,$4,$5)`, attempt, x.ItemType, x.SourceID, x.AmountMinor, x.Effect)
		require.NoError(t, e)
		if x.ItemType == model.ChargePaymentAttemptItemType {
			ids = append(ids, model.InstallmentChargeID(x.SourceID))
		}
	}
	if len(ids) > 0 {
		_, e = installmentchargerp.New().Tx(q).MarkSnapshottedChargeIDsPaid(f.ctx, ids, paidAt, paidAt)
		require.NoError(t, e)
	}
	_, e = installmentrp.New().Tx(q).MarkPaid(f.ctx, i.ID, *i.ContractID, amount, paidAt, paidAt)
	require.NoError(t, e)
	require.NoError(t, tx.Commit(f.ctx))
}
