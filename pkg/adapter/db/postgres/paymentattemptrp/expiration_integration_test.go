//go:build integration && development

package paymentattemptrp_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

type expirationFixture struct {
	ctx         context.Context
	pool        *pgxpool.Pool
	schema      string
	customer    uuid.UUID
	contract    uuid.UUID
	installment uuid.UUID
	now         time.Time
}

func newExpirationFixture(t *testing.T) *expirationFixture {
	t.Helper()

	dsn := os.Getenv("COLLECTIONS_TEST_DATABASE_URL")
	require.NotEmpty(t, dsn, "dedicated disposable PostgreSQL is required; tests never skip")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)

	schema := "pa_expire_it_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	cfg.ConnConfig.RuntimeParams["search_path"] = schema

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	f := &expirationFixture{
		ctx:         ctx,
		pool:        pool,
		schema:      schema,
		customer:    uuid.New(),
		contract:    uuid.New(),
		installment: uuid.New(),
		now:         time.Date(2026, 3, 15, 12, 30, 0, 0, time.UTC),
	}

	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "DROP SCHEMA IF EXISTS "+schema+" CASCADE")
	})

	ddl, err := os.ReadFile("../migrationsrp/000_schema.sql")
	require.NoError(t, err)

	baseline := strings.SplitN(string(ddl), "-- MANAGED DATABASE PERMISSIONS: canonical initial grants (pre-release)", 2)[0]
	baseline = strings.ReplaceAll(baseline, "SET search_path TO bnpl;", "SET search_path TO "+schema+";")
	baseline = strings.ReplaceAll(baseline, "bnpl.", schema+".")
	baseline = strings.ReplaceAll(baseline, "pg_catalog, bnpl", "pg_catalog, "+schema)
	_, err = pool.Exec(ctx, baseline)
	require.NoError(t, err)

	merchant, account, decision := uuid.New(), uuid.New(), uuid.New()
	fingerprint := "sha256:" + strings.Repeat("0", 64)

	exec := func(sql string, args ...any) {
		t.Helper()
		_, execErr := pool.Exec(ctx, sql, args...)
		require.NoError(t, execErr)
	}

	exec(`INSERT INTO customer_profiles(id,external_subject_id,external_customer_id,jurisdiction,status) VALUES($1,$2,$3,'IR','active')`, f.customer, uuid.New(), uuid.New())
	exec(`INSERT INTO merchant_profiles(id,external_merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction) VALUES($1,$2,'expire-it','active','low',12,'IR')`, merchant, uuid.New())
	exec(`INSERT INTO decisions(id,type,target_type,target_id,outcome,reason_codes,policy_id,policy_version,jurisdiction,actor_type,decided_at,idempotency_key,idempotency_fingerprint) VALUES($1,'restriction','customer_profile',$2,'restricted',ARRAY['COLLECTIONS_GRACE_PERIOD_EXCEEDED'],'collections_default','v2','IR','system',now(),$3,$4)`, decision, f.customer, uuid.NewString(), fingerprint)
	exec(`INSERT INTO credit_accounts(id,customer_id,currency,status,effective_from) VALUES($1,$2,'IRR','active',now())`, account, f.customer)
	exec(`INSERT INTO contracts(id,decision_id,checkout_key,customer_id,credit_account_id,merchant_profile_id,currency,requested_amount_minor,approved_financed_amount_minor,installment_count,payable_now_amount_minor,remaining_principal_amount_minor,status,expires_at,activated_at) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,2,0,100,'active',now(),now())`, f.contract, decision, uuid.NewString(), f.customer, account, merchant)
	exec(`INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,2,'IRR',100,100,now(),2,'due')`, f.installment, decision, f.contract, f.customer, merchant)

	return f
}

func (f *expirationFixture) cloneInstallment(t *testing.T, id uuid.UUID, order int) {
	t.Helper()
	_, err := f.pool.Exec(f.ctx, `
INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status)
SELECT $1, decision_id, contract_id, customer_id, merchant_profile_id, $3, currency, 100, 100, due_at, grace_period_days, status
FROM installments WHERE id=$2
`, id, f.installment, order)
	require.NoError(t, err)
}

func (f *expirationFixture) insertAttempt(
	t *testing.T,
	id uuid.UUID,
	installmentID uuid.UUID,
	purpose model.FinancialCorePaymentPurpose,
	state model.PaymentAttemptState,
	createdAt time.Time,
	leaseUntil *time.Time,
	sessionID *string,
	reason *model.PaymentReconciliationReason,
	attemptCount int,
) {
	t.Helper()

	fingerprint := "sha256:" + strings.Repeat("a", 64)
	tx, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)

	var successURL, failureURL, cancelURL *string
	if purpose == model.CheckoutFirstInstallmentFinancialCorePaymentPurpose {
		s, fl, c := "https://example.test/ok", "https://example.test/fail", "https://example.test/cancel"
		successURL, failureURL, cancelURL = &s, &fl, &c
	}

	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts (
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count, financial_session_create_lease_until,
	financial_payment_session_id, reconciliation_reason, reconciliation_requested_at,
	submitted_at, success_return_url, failure_return_url, cancel_return_url
) VALUES (
	$1, $2, $3, 100, 'IRR', $4, $5, 'gateway',
	$6, $7, $8, $8,
	$9, $10,
	$11, $12, CASE WHEN $5::text = 'reconciliation_required' THEN $8::timestamptz ELSE NULL END,
	CASE WHEN $5::text IN ('submitted','succeeded') THEN $8::timestamptz ELSE NULL END,
	$13, $14, $15
)
`, id, f.contract, installmentID, purpose, state, uuid.NewString(), fingerprint, createdAt, attemptCount, leaseUntil, sessionID, reason, successURL, failureURL, cancelURL)
	require.NoError(t, err)

	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect)
VALUES ($1,'principal',$2,100,'debit')
`, id, installmentID)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(f.ctx))
}

func TestClaimAndMarkExpired_SessionCreationWindow(t *testing.T) {
	f := newExpirationFixture(t)
	safety := model.FinancialSessionCreateExpirationSafetyInterval

	staleID := uuid.New()
	freshID := uuid.New()
	activeLeaseID := uuid.New()
	safetyHoldID := uuid.New()
	submittedID := uuid.New()
	resultTimeoutID := uuid.New()
	checkoutID := uuid.New()
	otherInstallment := uuid.New()
	freshInstallment := uuid.New()
	activeInstallment := uuid.New()
	safetyInstallment := uuid.New()
	submittedInstallment := uuid.New()
	checkoutInstallment := uuid.New()

	f.cloneInstallment(t, otherInstallment, 3)
	f.cloneInstallment(t, freshInstallment, 4)
	f.cloneInstallment(t, activeInstallment, 5)
	f.cloneInstallment(t, safetyInstallment, 6)
	f.cloneInstallment(t, submittedInstallment, 7)
	f.cloneInstallment(t, checkoutInstallment, 1)

	staleCreatedAt := f.now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	staleLease := f.now.Add(-safety - time.Second)
	f.insertAttempt(t, staleID, f.installment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.CreatedPaymentAttemptState, staleCreatedAt, &staleLease, nil, nil, 3)
	f.insertAttempt(t, freshID, freshInstallment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.CreatedPaymentAttemptState, f.now.Add(-time.Minute), nil, nil, nil, 1)

	activeLeaseUntil := f.now.Add(10 * time.Second)
	f.insertAttempt(t, activeLeaseID, activeInstallment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.CreatedPaymentAttemptState, staleCreatedAt, &activeLeaseUntil, nil, nil, 3)

	safetyLeaseUntil := f.now.Add(-safety + time.Second)
	f.insertAttempt(t, safetyHoldID, safetyInstallment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.CreatedPaymentAttemptState, staleCreatedAt, &safetyLeaseUntil, nil, nil, 3)

	sessionID := "fc-session"
	f.insertAttempt(t, submittedID, submittedInstallment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.SubmittedPaymentAttemptState, staleCreatedAt, nil, &sessionID, nil, 1)

	resultTimeout := model.PaymentResultTimeoutPaymentReconciliationReason
	f.insertAttempt(t, resultTimeoutID, otherInstallment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.ReconciliationRequiredPaymentAttemptState, staleCreatedAt, &staleLease, &sessionID, &resultTimeout, 1)
	f.insertAttempt(t, checkoutID, checkoutInstallment, model.CheckoutFirstInstallmentFinancialCorePaymentPurpose, model.CreatedPaymentAttemptState, staleCreatedAt, &staleLease, nil, nil, 3)

	tx, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	q := &postgres.Tx{Tx: tx}

	candidates, err := paymentattemptrp.ClaimCandidatesForSessionCreationExpiration(f.ctx, q, f.now, safety, 50)
	require.NoError(t, err)

	claimedIDs := map[uuid.UUID]struct{}{}
	for _, c := range candidates {
		claimedIDs[c.ID.UUID()] = struct{}{}
	}

	require.Contains(t, claimedIDs, staleID)
	require.NotContains(t, claimedIDs, freshID)
	require.NotContains(t, claimedIDs, activeLeaseID)
	require.NotContains(t, claimedIDs, safetyHoldID)
	require.NotContains(t, claimedIDs, submittedID)
	require.NotContains(t, claimedIDs, resultTimeoutID)
	require.NotContains(t, claimedIDs, checkoutID)

	var staleCandidate model.PaymentAttempt
	for _, c := range candidates {
		if c.ID.UUID() == staleID {
			staleCandidate = c
			break
		}
	}
	require.True(t, staleCandidate.EligibleForSessionCreationExpiration(f.now, safety))
	require.NoError(t, staleCandidate.State.ValidateTransitionTo(model.ExpiredPaymentAttemptState))

	expired, err := paymentattemptrp.MarkExpired(f.ctx, q, staleCandidate.ID, staleCandidate.State, f.now, f.now)
	require.NoError(t, err)
	require.Equal(t, model.ExpiredPaymentAttemptState, expired.State)
	require.False(t, expired.FinancialOutcomeResolved)
	require.NotNil(t, expired.ExpiredAt)
	require.Nil(t, expired.NextReconciliationAt)
	require.Nil(t, expired.FinancialSessionCreateLeaseUntil)

	require.NoError(t, tx.Commit(f.ctx))

	tx2, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx2.Rollback(f.ctx)
	q2 := &postgres.Tx{Tx: tx2}

	again, err := paymentattemptrp.ClaimCandidatesForSessionCreationExpiration(f.ctx, q2, f.now, safety, 50)
	require.NoError(t, err)
	for _, c := range again {
		require.NotEqual(t, model.PaymentAttemptID(staleID), c.ID)
	}

	_, err = paymentattemptrp.MarkExpired(f.ctx, q2, staleCandidate.ID, model.CreatedPaymentAttemptState, f.now, f.now)
	require.Error(t, err)
	require.NoError(t, tx2.Rollback(f.ctx))
}

func TestMarkSubmittedFromExpired_PreservesFinancialIdentity(t *testing.T) {
	f := newExpirationFixture(t)
	safety := model.FinancialSessionCreateExpirationSafetyInterval
	staleID := uuid.New()
	staleCreatedAt := f.now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	staleLease := f.now.Add(-safety - time.Second)
	f.insertAttempt(t, staleID, f.installment, model.InstallmentRepaymentFinancialCorePaymentPurpose, model.CreatedPaymentAttemptState, staleCreatedAt, &staleLease, nil, nil, 3)

	tx, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	q := &postgres.Tx{Tx: tx}

	expired, err := paymentattemptrp.MarkExpired(f.ctx, q, model.PaymentAttemptID(staleID), model.CreatedPaymentAttemptState, f.now, f.now)
	require.NoError(t, err)
	require.Equal(t, model.ExpiredPaymentAttemptState, expired.State)
	require.False(t, expired.FinancialOutcomeResolved)

	intentID := "fc-intent-" + staleID.String()
	sessionID := "fc-session-" + staleID.String()
	paymentURL := "https://pay.example.test/session"
	expiresAt := f.now.Add(10 * time.Minute)
	submitted, err := paymentattemptrp.MarkSubmittedFromExpired(
		f.ctx,
		q,
		model.PaymentAttemptID(staleID),
		intentID,
		sessionID,
		&paymentURL,
		&expiresAt,
		f.now,
		f.now,
	)
	require.NoError(t, err)
	require.Equal(t, model.SubmittedPaymentAttemptState, submitted.State)
	require.NotNil(t, submitted.FinancialPaymentIntentID)
	require.Equal(t, intentID, *submitted.FinancialPaymentIntentID)
	require.NotNil(t, submitted.FinancialPaymentSessionID)
	require.Equal(t, sessionID, *submitted.FinancialPaymentSessionID)
	require.False(t, submitted.FinancialOutcomeResolved)
	require.NoError(t, tx.Commit(f.ctx))
}
