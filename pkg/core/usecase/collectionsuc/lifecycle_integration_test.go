//go:build integration && development

package collectionsuc

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"os"
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

// This suite requires an explicitly supplied disposable database. No unavailable
// database path is reported as a passing or skipped verification.
func TestPostgreSQLCollectionsLifecycle(t *testing.T) {
	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "provide a disposable PostgreSQL database")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.ConnConfig.RuntimeParams["search_path"] = "bnpl"
	cfg.ConnConfig.RuntimeParams["timezone"] = "America/New_York"
	pool, err := postgres.NewPool(ctx, cfg)
	require.NoError(t, err)
	defer pool.Close()
	// Baseline DDL is shared with the existing integration/E2E infrastructure.
	ddl, err := os.ReadFile("../../../adapter/db/postgres/migrationsrp/000_schema.sql")
	require.NoError(t, err)
	_, err = pool.Pool.Exec(ctx, "CREATE SCHEMA IF NOT EXISTS bnpl")
	require.NoError(t, err)
	_, err = pool.Pool.Exec(ctx, string(ddl))
	require.NoError(t, err)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	clk := adapterclock.NewFixed(now, time.UTC)
	uc, err := New(pool, customerprofilerp.New(), installmentrp.New(), decisionrp.New(), defaultepisoderp.New(), customerrestrictionrp.New(), collectionscaserp.New(), outboxrp.New(), clk, integrationIDs{}, sha256fingerprint.New(), metricsnoop.New(), "bnpl.audit.events.v1", 10)
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
			due := now.Add(-49 * time.Hour)
			if order == 2 {
				id = uuid.New()
				due = now.Add(30 * 24 * time.Hour)
			}
			exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,$7,2,'scheduled')`, id, decision, contract, customer, merchant, order, due)
		}
		return model.CustomerID(customer), installment
	}
	t.Run("concurrent entry recovery lineage and history", func(t *testing.T) {
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
		exec(t, `UPDATE installments SET status='paid',paid_amount_minor=100,paid_at=$2 WHERE id=$1`, installment, now)
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
	})
	t.Run("outbox failure rolls back whole entry", func(t *testing.T) {
		customer, _ := seed(t)
		uc.auditDestination = ""
		defer func() { uc.auditDestination = "bnpl.audit.events.v1" }()
		_, e := uc.AssessCustomer(ctx, customer)
		require.Error(t, e)
		for _, table := range []string{"default_episodes", "customer_restrictions", "collections_cases"} {
			require.Zero(t, scalar(t, `SELECT count(*) FROM `+table+` WHERE customer_id=$1`, customer))
		}
		require.Equal(t, 1, scalar(t, `SELECT count(*) FROM decisions WHERE target_id=$1`, customer))
	})
	t.Run("recovery failure preserves restriction and history", func(t *testing.T) {
		customer, installment := seed(t)
		opened, e := uc.AssessCustomer(ctx, customer)
		require.NoError(t, e)
		exec(t, `UPDATE installments SET status='paid',paid_amount_minor=100,paid_at=$2 WHERE id=$1`, installment, now)
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
	})

	t.Run("read snapshot remains coherent during settlement", func(t *testing.T) {
		customer, installment := seed(t)
		require.NoError(t, pool.Conn(ctx, func(ctx context.Context, c repo.Conn) error {
			return c.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
				q := installmentrp.New().Tx(tx)
				before, e := q.CountUnpaidByCustomerID(ctx, customer)
				if e != nil {
					return e
				}
				exec(t, `UPDATE installments SET status='paid',paid_amount_minor=100,paid_at=$2 WHERE id=$1`, installment, now)
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
	})
	t.Run("reminder short link and outbox atomicity and public redirect", func(t *testing.T) {
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
	})

	t.Run("SQL grace boundaries match policy across timezone transitions", func(t *testing.T) {
		loc, e := time.LoadLocation("America/New_York")
		require.NoError(t, e)
		for _, due := range []time.Time{time.Date(2026, 3, 7, 15, 30, 0, 0, loc), time.Date(2026, 10, 31, 15, 30, 0, 0, loc)} {
			for _, days := range []uint{0, 1, 3, 45} {
				customer, installment := seed(t)
				exec(t, `UPDATE installments SET due_at=$2,grace_period_days=$3,status='due' WHERE id=$1`, installment, due, days)
				boundary := due.Add(time.Duration(days) * 24 * time.Hour)
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
	})

	t.Run("longer persisted grace is included in reminder candidates", func(t *testing.T) {
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
	})

	t.Run("batch failure samples are bounded and every failure is observed", func(t *testing.T) {
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
	})

	t.Run("uncertainty transitions include resolution and recurrence", func(t *testing.T) {
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
	})
}
