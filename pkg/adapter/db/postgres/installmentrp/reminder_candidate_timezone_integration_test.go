//go:build integration && development

package installmentrp_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// Every scenario owns a unique schema in an explicitly supplied disposable
// database. Timezone assertions must exercise real PostgreSQL AT TIME ZONE
// semantics against the production discovery SQL.
type reminderCandidateFixture struct {
	ctx    context.Context
	pool   *postgres.Pool
	now    time.Time
	schema string
	exec   func(*testing.T, string, ...any)
}

func newReminderCandidateFixture(t *testing.T, sessionTimezone string) *reminderCandidateFixture {
	t.Helper()

	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "provide a disposable PostgreSQL database")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	t.Cleanup(cancel)

	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	schema := "bnpl_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	cfg.ConnConfig.RuntimeParams["timezone"] = sessionTimezone

	pool, err := postgres.NewPool(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = pool.Close() })

	ddl, err := os.ReadFile("../migrationsrp/000_schema.sql")
	require.NoError(t, err)

	_, err = pool.Pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)

	baseline := strings.SplitN(string(ddl), "-- MANAGED DATABASE PERMISSIONS: canonical initial grants (pre-release)", 2)[0]
	baseline = strings.ReplaceAll(baseline, "SET search_path TO bnpl;", "SET search_path TO "+schema+";")
	baseline = strings.ReplaceAll(baseline, "bnpl.", schema+".")
	baseline = strings.ReplaceAll(baseline, "pg_catalog, bnpl", "pg_catalog, "+schema)
	_, err = pool.Pool.Exec(ctx, baseline)
	require.NoError(t, err)

	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		_, e := pool.Pool.Exec(ctx, sql, args...)
		require.NoError(t, e)
	}

	return &reminderCandidateFixture{ctx: ctx, pool: pool, now: now, schema: schema, exec: exec}
}

func (f *reminderCandidateFixture) seedInstallment(
	t *testing.T,
	dueAt time.Time,
	graceDays int,
	status model.InstallmentStatus,
) (model.CustomerID, model.InstallmentID) {
	t.Helper()

	customer, merchant, account, decision, contract, installment := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO customer_profiles(id,external_subject_id,external_customer_id,jurisdiction,status) VALUES($1,$2,$3,'IR','active')`, customer, uuid.New(), uuid.New())
	f.exec(t, `INSERT INTO merchant_profiles(id,external_merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction) VALUES($1,$2,'test','active','low',12,'IR')`, merchant, uuid.New())
	f.exec(t, `INSERT INTO decisions(id,type,target_type,target_id,outcome,reason_codes,policy_id,policy_version,jurisdiction,actor_type,decided_at,idempotency_key,idempotency_fingerprint) VALUES($1,'credit_limit','customer_profile',$2,'approved',ARRAY['CHECKOUT_APPROVED'],'checkout_contract_approval','v1','IR','system',$3,$4,$5)`, decision, customer, f.now, uuid.NewString(), "sha256:"+fmt.Sprintf("%064x", 1))
	f.exec(t, `INSERT INTO credit_accounts(id,customer_id,currency,status,effective_from) VALUES($1,$2,'IRR','active',$3)`, account, customer, f.now)
	f.exec(t, `INSERT INTO contracts(id,decision_id,checkout_key,customer_id,credit_account_id,merchant_profile_id,currency,requested_amount_minor,approved_financed_amount_minor,installment_count,payable_now_amount_minor,remaining_principal_amount_minor,status,expires_at,activated_at) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,1,0,100,'active',$7,$7)`, contract, decision, uuid.NewString(), customer, account, merchant, f.now)
	f.exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,1,'IRR',100,100,$6,$7,$8)`, installment, decision, contract, customer, merchant, dueAt.UTC(), graceDays, status)

	return model.CustomerID(customer), model.InstallmentID(installment)
}

func listCandidates(
	t *testing.T,
	f *reminderCandidateFixture,
	dueAtFrom, dueAtTo, businessDate time.Time,
	businessTimezone string,
) []repo.ReminderCandidate {
	t.Helper()
	return listCandidatesAfter(t, f, dueAtFrom, dueAtTo, businessDate, model.CustomerID{}, 10000, businessTimezone)
}

func listCandidatesAfter(
	t *testing.T,
	f *reminderCandidateFixture,
	dueAtFrom, dueAtTo, businessDate time.Time,
	afterCustomerID model.CustomerID,
	customerLimit int,
	businessTimezone string,
) []repo.ReminderCandidate {
	t.Helper()

	var rows []repo.ReminderCandidate
	err := f.pool.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
		listed, listErr := installmentrp.New().Conn(c).ListReminderCandidatePage(
			ctx,
			dueAtFrom,
			dueAtTo,
			businessDate,
			afterCustomerID,
			customerLimit,
			businessTimezone,
		)
		if listErr != nil {
			return listErr
		}

		rows = listed
		return nil
	})
	require.NoError(t, err)
	return rows
}

func candidateIDs(rows []repo.ReminderCandidate) []model.InstallmentID {
	out := make([]model.InstallmentID, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Installment.ID)
	}

	return out
}

func containsInstallment(rows []repo.ReminderCandidate, id model.InstallmentID) bool {
	for _, row := range rows {
		if row.Installment.ID == id {
			return true
		}
	}

	return false
}

// graceOnlyDiscoveryWindow returns a discovery interval where the installment is
// selected solely by the Grace Period extension predicate:
//
//	due_at < dueAtFrom
//	AND OverdueEligibleAt(businessTZ) >= dueAtFrom
//	AND due_at < dueAtTo
//
// For the Tehran midnight-crossing due used below, an old
// current_setting('TimeZone') expression under UTC/America/New_York sessions
// yields a boundary before dueAtFrom and would miss the installment.
func graceOnlyDiscoveryWindow(t *testing.T) (
	tehran *time.Location,
	dueLocal time.Time,
	graceDays int,
	businessDay time.Time,
	dueAtFrom time.Time,
	dueAtTo time.Time,
	overdueAtBusiness time.Time,
) {
	t.Helper()

	var err error
	tehran, err = time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	// 2026-03-20 00:30 Asia/Tehran == 2026-03-19 21:00 UTC.
	// Business local due date is 2026-03-20; UTC session calendar date is 2026-03-19.
	dueLocal = time.Date(2026, time.March, 20, 0, 30, 0, 0, tehran)
	graceDays = 5
	overdueAtBusiness = (model.Installment{
		DueAt:           dueLocal,
		GracePeriodDays: uint(graceDays),
	}).OverdueEligibleAt(tehran)
	// March 26 00:00 Tehran == March 25 20:30 UTC.
	require.Equal(t, time.Date(2026, time.March, 26, 0, 0, 0, 0, tehran).UTC(), overdueAtBusiness)

	businessDay = time.Date(2026, time.March, 25, 0, 0, 0, 0, tehran)
	// Place dueAtFrom after due_at and after the wrong (UTC-session) grace
	// boundary, but still at/before the correct Tehran OverdueEligibleAt.
	dueAtFrom = time.Date(2026, time.March, 25, 12, 0, 0, 0, time.UTC)
	dueAtTo = time.Date(2026, time.March, 27, 0, 0, 0, 0, time.UTC)

	require.True(t, dueLocal.UTC().Before(dueAtFrom), "due_at must be excluded from i.due_at >= dueAtFrom")
	require.False(t, overdueAtBusiness.Before(dueAtFrom), "Tehran grace boundary must still include the installment")
	require.True(t, dueLocal.UTC().Before(dueAtTo))

	return tehran, dueLocal, graceDays, businessDay, dueAtFrom, dueAtTo, overdueAtBusiness
}

func TestPostgreSQLReminderCandidates_SessionTimezoneIndependent(t *testing.T) {
	tehran, dueLocal, graceDays, businessDay, dueAtFrom, dueAtTo, overdueAtBusiness := graceOnlyDiscoveryWindow(t)

	for _, sessionTZ := range []string{"UTC", "Asia/Tehran", "America/New_York"} {
		t.Run("session_"+sessionTZ, func(t *testing.T) {
			f := newReminderCandidateFixture(t, sessionTZ)
			_, installmentID := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

			require.True(t, dueLocal.UTC().Before(dueAtFrom))

			rows := listCandidates(t, f, dueAtFrom, dueAtTo, businessDay, tehran.String())
			require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows),
				"session=%s businessTZ=%s overdueAt=%s dueAtFrom=%s must discover exclusively via grace",
				sessionTZ, tehran, overdueAtBusiness, dueAtFrom)
		})
	}
}

func TestPostgreSQLReminderCandidates_GraceExpressionRegression(t *testing.T) {
	tehran, dueLocal, graceDays, _, dueAtFrom, _, overdueAtBusiness := graceOnlyDiscoveryWindow(t)

	// Session timezone deliberately mismatches the business timezone so the
	// historical current_setting('TimeZone') expression diverges.
	f := newReminderCandidateFixture(t, "UTC")
	_, installmentID := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

	var (
		sessionTZ          string
		correctedBoundary  time.Time
		legacySessionBound time.Time
		dueAt              time.Time
		gracePeriodDays    int
	)
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT
	current_setting('TimeZone'),
	i.due_at,
	i.grace_period_days,
	(((i.due_at AT TIME ZONE $2::text)::date + i.grace_period_days + 1)::timestamp AT TIME ZONE $2::text) AS corrected,
	(((i.due_at AT TIME ZONE current_setting('TimeZone'))::date + i.grace_period_days + 1)::timestamp AT TIME ZONE current_setting('TimeZone')) AS legacy_session
FROM installments i
WHERE i.id = $1
`, installmentID, tehran.String()).Scan(
		&sessionTZ,
		&dueAt,
		&gracePeriodDays,
		&correctedBoundary,
		&legacySessionBound,
	))

	require.Equal(t, "UTC", sessionTZ)
	require.Equal(t, graceDays, gracePeriodDays)
	require.True(t, dueAt.Equal(dueLocal.UTC()))
	require.True(t, correctedBoundary.Equal(overdueAtBusiness),
		"corrected SQL must match domain OverdueEligibleAt: got %s want %s",
		correctedBoundary, overdueAtBusiness)
	require.False(t, legacySessionBound.Equal(correctedBoundary),
		"legacy session expression must diverge under UTC session: legacy=%s corrected=%s",
		legacySessionBound, correctedBoundary)
	require.True(t, legacySessionBound.Before(dueAtFrom),
		"legacy UTC-session boundary %s must fall before dueAtFrom %s so the old bug would miss the installment",
		legacySessionBound, dueAtFrom)
	require.False(t, correctedBoundary.Before(dueAtFrom),
		"corrected boundary must remain discoverable")

	rows := listCandidates(t, f, dueAtFrom, dueAtToForRegression(dueAtFrom), time.Date(2026, time.March, 25, 0, 0, 0, 0, tehran), tehran.String())
	require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows))
}

func dueAtToForRegression(dueAtFrom time.Time) time.Time {
	return dueAtFrom.Add(48 * time.Hour)
}

func TestPostgreSQLReminderCandidates_GraceFinalDayExclusiveBoundary(t *testing.T) {
	tehran, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	f := newReminderCandidateFixture(t, "UTC")
	dueLocal := time.Date(2026, time.March, 10, 18, 0, 0, 0, tehran)
	graceDays := 5
	inst := model.Installment{DueAt: dueLocal, GracePeriodDays: uint(graceDays)}
	overdueAt := inst.OverdueEligibleAt(tehran)

	_, installmentID := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

	// Instant immediately before exclusive end: still discoverable via grace
	// even though due_at itself is before dueAtFrom.
	dueAtFromBefore := overdueAt.Add(-time.Second)
	dueAtTo := overdueAt.Add(48 * time.Hour)
	businessDay := time.Date(2026, time.March, 15, 0, 0, 0, 0, tehran)
	require.True(t, dueLocal.UTC().Before(dueAtFromBefore))

	rows := listCandidates(t, f, dueAtFromBefore, dueAtTo, businessDay, tehran.String())
	require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows))

	// Discovery uses grace_boundary >= dueAtFrom, so the exclusive overdue
	// instant itself remains a broad discovery candidate. Policy evaluation
	// separately refuses ordinary Grace reminders once now is at/after that
	// instant; discovery must not silently drop the row at equality.
	rows = listCandidates(t, f, overdueAt, dueAtTo, businessDay, tehran.String())
	require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows))

	// One second past the exclusive overdue instant drops grace lookback; due_at
	// remains before dueAtFrom, so no other OR branch can select the row.
	rows = listCandidates(t, f, overdueAt.Add(time.Second), dueAtTo, businessDay, tehran.String())
	require.Empty(t, candidateIDs(rows))
}

func TestPostgreSQLReminderCandidates_HistoricalGraceBeyondOperationalMax(t *testing.T) {
	tehran, err := time.LoadLocation("Asia/Tehran")
	require.NoError(t, err)

	f := newReminderCandidateFixture(t, "UTC")
	graceDays := int(model.MaxInstallmentGracePeriodDays) + 15
	dueLocal := time.Date(2026, time.February, 1, 12, 0, 0, 0, tehran)
	inst := model.Installment{DueAt: dueLocal, GracePeriodDays: uint(graceDays)}
	overdueAt := inst.OverdueEligibleAt(tehran)

	_, installmentID := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

	businessDay := overdueAt.In(tehran).AddDate(0, 0, -2)
	businessDay = time.Date(businessDay.Year(), businessDay.Month(), businessDay.Day(), 0, 0, 0, 0, tehran)
	dueAtFrom := businessDay.AddDate(0, 0, -int(model.MaxInstallmentGracePeriodDays)).UTC()
	dueAtTo := businessDay.AddDate(0, 0, 2).UTC()

	require.True(t, dueLocal.Before(dueAtFrom), "due_at alone must be outside the operational floor")
	require.False(t, overdueAt.Before(dueAtFrom), "historical grace boundary must still intersect the window")

	rows := listCandidates(t, f, dueAtFrom, dueAtTo, businessDay, tehran.String())
	require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows))
}

func TestPostgreSQLReminderCandidates_DSTCalendarDays(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err)

	t.Run("spring_forward", func(t *testing.T) {
		f := newReminderCandidateFixture(t, "UTC")

		// 2026-03-29 02:00 → 03:00 in Europe/Berlin.
		dueLocal := time.Date(2026, time.March, 27, 15, 0, 0, 0, berlin)
		graceDays := 3
		overdueAt := (model.Installment{DueAt: dueLocal, GracePeriodDays: uint(graceDays)}).OverdueEligibleAt(berlin)
		require.True(t, overdueAt.Equal(time.Date(2026, time.March, 31, 0, 0, 0, 0, berlin).UTC()))

		_, installmentID := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

		dueAtFrom := overdueAt.Add(-time.Second)
		dueAtTo := overdueAt.Add(48 * time.Hour)
		businessDay := time.Date(2026, time.March, 30, 0, 0, 0, 0, berlin)
		require.True(t, dueLocal.UTC().Before(dueAtFrom))

		rows := listCandidates(t, f, dueAtFrom, dueAtTo, businessDay, berlin.String())
		require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows))

		rows = listCandidates(t, f, overdueAt.Add(time.Second), dueAtTo, businessDay, berlin.String())
		require.Empty(t, candidateIDs(rows))
	})

	t.Run("fall_back", func(t *testing.T) {
		f := newReminderCandidateFixture(t, "UTC")

		// 2026-10-25 03:00 → 02:00 in Europe/Berlin.
		dueLocal := time.Date(2026, time.October, 23, 15, 0, 0, 0, berlin)
		graceDays := 3
		overdueAt := (model.Installment{DueAt: dueLocal, GracePeriodDays: uint(graceDays)}).OverdueEligibleAt(berlin)
		require.True(t, overdueAt.Equal(time.Date(2026, time.October, 27, 0, 0, 0, 0, berlin).UTC()))

		_, installmentID := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

		dueAtFrom := overdueAt.Add(-time.Second)
		dueAtTo := overdueAt.Add(48 * time.Hour)
		businessDay := time.Date(2026, time.October, 26, 0, 0, 0, 0, berlin)
		require.True(t, dueLocal.UTC().Before(dueAtFrom))

		rows := listCandidates(t, f, dueAtFrom, dueAtTo, businessDay, berlin.String())
		require.Equal(t, []model.InstallmentID{installmentID}, candidateIDs(rows))

		rows = listCandidates(t, f, overdueAt.Add(time.Second), dueAtTo, businessDay, berlin.String())
		require.Empty(t, candidateIDs(rows))
	})
}

func TestPostgreSQLReminderCandidates_CustomerPaginationWithGraceOnly(t *testing.T) {
	tehran, dueLocal, graceDays, businessDay, dueAtFrom, dueAtTo, _ := graceOnlyDiscoveryWindow(t)
	f := newReminderCandidateFixture(t, "UTC")

	// Customer A has two grace-only installments; customer B has one.
	customerA, installmentA1 := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)
	installmentA2 := model.InstallmentID(uuid.New())
	var (
		decisionID uuid.UUID
		contractID uuid.UUID
		merchantID uuid.UUID
	)
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT decision_id, contract_id, merchant_profile_id
FROM installments WHERE id=$1`, installmentA1).Scan(&decisionID, &contractID, &merchantID))
	f.exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,2,'IRR',100,100,$6,$7,'overdue')`, installmentA2, decisionID, contractID, customerA, merchantID, dueLocal.Add(time.Minute).UTC(), graceDays)

	customerB, installmentB := f.seedInstallment(t, dueLocal, graceDays, model.OverdueInstallmentStatus)

	page1 := listCandidatesAfter(t, f, dueAtFrom, dueAtTo, businessDay, model.CustomerID{}, 1, tehran.String())
	require.NotEmpty(t, page1)

	firstCustomer := page1[0].Installment.CustomerID
	for _, row := range page1 {
		require.Equal(t, firstCustomer, row.Installment.CustomerID, "page must not split a customer")
	}

	var seen = map[model.CustomerID]struct{}{firstCustomer: {}}
	if firstCustomer == customerA {
		require.ElementsMatch(t, []model.InstallmentID{installmentA1, installmentA2}, candidateIDs(page1))
	} else {
		require.Equal(t, customerB, firstCustomer)
		require.Equal(t, []model.InstallmentID{installmentB}, candidateIDs(page1))
	}

	page2 := listCandidatesAfter(t, f, dueAtFrom, dueAtTo, businessDay, firstCustomer, 1, tehran.String())
	require.NotEmpty(t, page2)
	secondCustomer := page2[0].Installment.CustomerID
	_, repeated := seen[secondCustomer]
	require.False(t, repeated, "keyset must not repeat a customer")
	seen[secondCustomer] = struct{}{}

	for _, row := range page2 {
		require.Equal(t, secondCustomer, row.Installment.CustomerID)
	}

	if secondCustomer == customerA {
		require.ElementsMatch(t, []model.InstallmentID{installmentA1, installmentA2}, candidateIDs(page2))
	} else {
		require.Equal(t, customerB, secondCustomer)
		require.Equal(t, []model.InstallmentID{installmentB}, candidateIDs(page2))
	}

	page3 := listCandidatesAfter(t, f, dueAtFrom, dueAtTo, businessDay, secondCustomer, 1, tehran.String())
	require.Empty(t, page3, "no further customers after the second page")
	require.Len(t, seen, 2)
}
