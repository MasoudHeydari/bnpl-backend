//go:build development && integration

package invariants

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

const insertChargeSQL = `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',10,'IRR','customer','platform','test','v1',now(),'{}')`
const markPaidSQL = `UPDATE installments SET status='paid',paid_at=now(),paid_amount_minor=100 WHERE id=$1`

func TestConcurrentParentSerialization(t *testing.T) {
	t.Run("charge wins before MarkPaid", func(t *testing.T) {
		f := newFixture(t, true)
		tx, e := f.pool.Pool.Begin(f.ctx)
		require.NoError(t, e)
		defer tx.Rollback(f.ctx)
		_, e = tx.Exec(f.ctx, insertChargeSQL, f.installment)
		require.NoError(t, e)
		other, e := f.pool.Pool.Acquire(f.ctx)
		require.NoError(t, e)
		defer other.Release()
		pid := other.Conn().PgConn().PID()
		done := make(chan error, 1)
		go func() { _, e := other.Exec(f.ctx, markPaidSQL, f.installment); done <- e }()
		require.Eventually(t, func() bool {
			var waiting bool
			e := f.pool.Pool.QueryRow(f.ctx, `SELECT coalesce(wait_event_type='Lock',false) FROM pg_stat_activity WHERE pid=$1`, pid).Scan(&waiting)
			return e == nil && waiting
		}, time.Second, 5*time.Millisecond)

		require.NoError(t, tx.Commit(f.ctx))
		assertViolation(t, <-done, "BN002")
	})
	t.Run("MarkPaid wins before charge", func(t *testing.T) {
		f := newFixture(t, true)
		attempt := f.attempt(t)
		tx, e := f.pool.Pool.Begin(f.ctx)
		require.NoError(t, e)
		defer tx.Rollback(f.ctx)
		_, e = tx.Exec(f.ctx, `UPDATE payment_attempts SET state='succeeded',financial_paid_at=now(),applied_at=now(),financial_outcome_resolved=true WHERE id=$1`, attempt)
		require.NoError(t, e)
		_, e = tx.Exec(f.ctx, markPaidSQL, f.installment)
		require.NoError(t, e)
		_, e = f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
		assertViolation(t, e, "BN003")
		require.NoError(t, tx.Commit(f.ctx))
		_, e = f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
		assertViolation(t, e, "BN001")
	})
	t.Run("attempt wins before charge", func(t *testing.T) {
		f := newFixture(t, true)
		tx, e := f.pool.Pool.Begin(f.ctx)
		require.NoError(t, e)
		defer tx.Rollback(f.ctx)
		require.NoError(t, f.insertAttempt(tx, uuid.New()))
		_, e = f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
		assertViolation(t, e, "BN003")
		require.NoError(t, tx.Commit(f.ctx))
		_, e = f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
		assertViolation(t, e, "BN003")
	})
	t.Run("charge wins before attempt and stale snapshot cannot commit", func(t *testing.T) {
		f := newFixture(t, true)
		tx, e := f.pool.Pool.Begin(f.ctx)
		require.NoError(t, e)
		defer tx.Rollback(f.ctx)
		_, e = tx.Exec(f.ctx, insertChargeSQL, f.installment)
		require.NoError(t, e)
		other, e := f.pool.Pool.Begin(f.ctx)
		require.NoError(t, e)
		assertViolation(t, f.insertAttempt(other, uuid.New()), "BN003")
		require.NoError(t, other.Rollback(f.ctx))
		require.NoError(t, tx.Commit(f.ctx))
		retry, e := f.pool.Pool.Begin(f.ctx)
		require.NoError(t, e)
		require.NoError(t, f.insertAttempt(retry, uuid.New()))
		assertViolation(t, retry.Commit(f.ctx), "BN004")
		var count int
		require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT count(*) FROM payment_attempts WHERE installment_id=$1`, f.installment).Scan(&count))
		require.Zero(t, count)
	})
	t.Run("repeatable read cannot use old charge snapshot", func(t *testing.T) {
		f := newFixture(t, true)
		tx, e := f.pool.Pool.BeginTx(f.ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
		require.NoError(t, e)
		defer tx.Rollback(f.ctx)
		_, e = tx.Exec(f.ctx, `SELECT * FROM installments WHERE id=$1`, f.installment)
		require.NoError(t, e)
		f.charge(t, "late_fee", "customer")
		assertViolation(t, f.insertAttempt(tx, uuid.New()), "40001")
	})
}
func TestFinalizedComponentsAndFreeze(t *testing.T) {
	for _, tc := range []struct{ name, sql, code string }{
		{"edit paid charge", `UPDATE installment_charges SET amount_minor=11 WHERE id=$1`, "BN001"},
		{"delete paid charge", `DELETE FROM installment_charges WHERE id=$1`, "BN004"},
		{"move paid charge", `UPDATE installment_charges SET installment_id=$2 WHERE id=$1`, "BN004"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, true)
			ch := f.charge(t, "late_fee", "customer")
			f.settleFixture(t)
			var e error
			if tc.name == "move paid charge" {
				_, e = f.pool.Pool.Exec(f.ctx, tc.sql, ch, uuid.New())
			} else {
				_, e = f.pool.Pool.Exec(f.ctx, tc.sql, ch)
			}
			assertViolation(t, e, tc.code)
		})
	}
	for _, state := range []string{"submitted", "reconciliation_required", "manual_review_required"} {
		t.Run(state, func(t *testing.T) {
			f := newFixture(t, true)
			id := f.attempt(t)
			f.exec(t, `UPDATE payment_attempts SET state=$2,submitted_at=now(),reconciliation_reason='financial_core_status_still_unknown',reconciliation_requested_at=now(),manual_review_required_at=now(),last_reconciliation_error='uncertain' WHERE id=$1`, id, state)
			_, e := f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
			assertViolation(t, e, "BN003")
		})
	}
}

func TestLocalTerminalDoesNotUnfreezeAndSnapshotsCannotGrow(t *testing.T) {
	f := newFixture(t, true)
	attempt := f.attempt(t)
	f.exec(t, `UPDATE payment_attempts SET state='canceled',canceled_at=now(),failure_code='contract_expired_before_payment_submission' WHERE id=$1`, attempt)
	_, e := f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
	assertViolation(t, e, "BN003")
	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	assertViolation(t, f.insertAttempt(tx, uuid.New()), "BN003")
	require.NoError(t, tx.Rollback(f.ctx))
	_, e = f.pool.Pool.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'upfront',$2,1,'debit')`, attempt, f.installment)
	assertViolation(t, e, "BN004")
}

func TestCustomerComponentCannotSettleWithoutItsInstallment(t *testing.T) {
	f := newFixture(t, true)
	charge := f.charge(t, "late_fee", "customer")
	_, e := f.pool.Pool.Exec(f.ctx, `UPDATE installment_charges SET charge_status='paid',paid_at=now() WHERE id=$1`, charge)
	assertViolation(t, e, "BN002")
	var status string
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT charge_status FROM installment_charges WHERE id=$1`, charge).Scan(&status))
	require.Equal(t, "applied", status)
}

func TestPaidRequiresCompleteSuccessfulSnapshot(t *testing.T) {
	f := newFixture(t, true)
	_, e := f.pool.Pool.Exec(f.ctx, markPaidSQL, f.installment)
	assertViolation(t, e, "BN002")
	f.attempt(t)
	_, e = f.pool.Pool.Exec(f.ctx, `UPDATE installments SET status='paid',paid_at=now(),paid_amount_minor=99 WHERE id=$1`, f.installment)
	assertViolation(t, e, "BN002")
	// Even a matching cash amount cannot commit while its financial outcome is unresolved.
	_, e = f.pool.Pool.Exec(f.ctx, markPaidSQL, f.installment)
	assertViolation(t, e, "BN002")
	var status string
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT status FROM installments WHERE id=$1`, f.installment).Scan(&status))
	require.Equal(t, "scheduled", status)
}

func TestUnresolvedPaymentPreventsLocalObligationExtinguishment(t *testing.T) {
	f := newFixture(t, true)
	f.attempt(t)
	_, e := f.pool.Pool.Exec(f.ctx, `UPDATE installments SET status='canceled' WHERE id=$1`, f.installment)
	assertViolation(t, e, "BN003")
}

func TestUnresolvedExpiredPaymentFreezesFinancialSnapshot(t *testing.T) {
	f := newFixture(t, true)
	attempt := f.attempt(t)
	// Partial FC session identity (no PaymentURL) keeps this expired row
	// financially unresolved and blocking — not abandoned-creation eligibility.
	f.exec(t, `
UPDATE payment_attempts
SET state='expired',
    expired_at=now(),
    financial_outcome_resolved=false,
    financial_session_create_attempt_count=3,
    financial_payment_session_id='ps-ambiguous',
    next_reconciliation_at=NULL
WHERE id=$1
`, attempt)

	_, e := f.pool.Pool.Exec(f.ctx, insertChargeSQL, f.installment)
	assertViolation(t, e, "BN003")

	_, e = f.pool.Pool.Exec(f.ctx, `UPDATE installments SET status='canceled' WHERE id=$1`, f.installment)
	assertViolation(t, e, "BN003")

	_, e = f.pool.Pool.Exec(f.ctx, `UPDATE installments SET principal_amount_minor=99 WHERE id=$1`, f.installment)
	assertViolation(t, e, "BN003")

	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	assertViolation(t, f.insertAttempt(tx, uuid.New()), "BN003")
	require.NoError(t, tx.Rollback(f.ctx))

	// Unrelated metadata-only no-op remains allowed.
	_, e = f.pool.Pool.Exec(f.ctx, `UPDATE installments SET updated_at=updated_at WHERE id=$1`, f.installment)
	require.NoError(t, e)
}

func TestConcurrentExpirationWorkersClaimOnce(t *testing.T) {
	f := newFixture(t, true)
	now := time.Now().UTC()
	createdAt := now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	leaseUntil := now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)
	attempt := uuid.New()
	seed, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	_, e = seed.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count, financial_session_create_lease_until
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'created', 'gateway',
	$4, $5, $6, $6, 3, $7
)
`, attempt, f.contract, f.installment, uuid.NewString(), "sha256:"+strings.Repeat("b", 64), createdAt, leaseUntil)
	require.NoError(t, e)
	_, e = seed.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'principal',$2,100,'debit')`, attempt, f.installment)
	require.NoError(t, e)
	require.NoError(t, seed.Commit(f.ctx))

	txA, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer txA.Rollback(f.ctx)
	txB, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer txB.Rollback(f.ctx)

	qa := &postgres.Tx{Tx: txA}
	qb := &postgres.Tx{Tx: txB}

	claimedA, e := paymentattemptrp.ClaimCandidatesForSessionCreationExpiration(
		f.ctx, qa, now, model.FinancialSessionCreateExpirationSafetyInterval, 10,
	)
	require.NoError(t, e)

	done := make(chan []model.PaymentAttempt, 1)
	errCh := make(chan error, 1)
	go func() {
		claimed, claimErr := paymentattemptrp.ClaimCandidatesForSessionCreationExpiration(
			f.ctx, qb, now, model.FinancialSessionCreateExpirationSafetyInterval, 10,
		)
		errCh <- claimErr
		done <- claimed
	}()

	foundA := false
	for _, c := range claimedA {
		if c.ID == model.PaymentAttemptID(attempt) {
			foundA = true
			_, markErr := paymentattemptrp.MarkExpired(f.ctx, qa, c.ID, c.State, now, now)
			require.NoError(t, markErr)
		}
	}
	require.True(t, foundA)
	require.NoError(t, txA.Commit(f.ctx))

	require.NoError(t, <-errCh)
	claimedB := <-done
	for _, c := range claimedB {
		require.NotEqual(t, model.PaymentAttemptID(attempt), c.ID)
	}
	require.NoError(t, txB.Rollback(f.ctx))

	var state string
	var resolved bool
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT state, financial_outcome_resolved FROM payment_attempts WHERE id=$1`, attempt).Scan(&state, &resolved))
	require.Equal(t, "expired", state)
	require.False(t, resolved)
}

func TestReconciliationClaimVsExpirationClaim_SkipLocked(t *testing.T) {
	f := newFixture(t, true)
	now := time.Now().UTC()
	createdAt := now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	leaseUntil := now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)
	attempt := uuid.New()
	reason := model.FinancialCoreStatusStillUnknownPaymentReconciliationReason
	nextRetry := now.Add(-time.Second)

	seed, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	_, e = seed.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count, financial_session_create_lease_until,
	reconciliation_reason, reconciliation_requested_at, next_reconciliation_at
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'reconciliation_required', 'gateway',
	$4, $5, $6, $6, 3, $7, $8, $6, $9
)
`, attempt, f.contract, f.installment, uuid.NewString(), "sha256:"+strings.Repeat("c", 64), createdAt, leaseUntil, reason, nextRetry)
	require.NoError(t, e)
	_, e = seed.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'principal',$2,100,'debit')`, attempt, f.installment)
	require.NoError(t, e)
	require.NoError(t, seed.Commit(f.ctx))

	txExpire, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer txExpire.Rollback(f.ctx)
	txRecon, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer txRecon.Rollback(f.ctx)

	qExpire := &postgres.Tx{Tx: txExpire}
	qRecon := &postgres.Tx{Tx: txRecon}

	claimedExpire, e := paymentattemptrp.ClaimCandidatesForSessionCreationExpiration(
		f.ctx, qExpire, now, model.FinancialSessionCreateExpirationSafetyInterval, 10,
	)
	require.NoError(t, e)

	done := make(chan []model.PaymentAttempt, 1)
	errCh := make(chan error, 1)
	go func() {
		claimed, claimErr := paymentattemptrp.ClaimDueReconciliationRequired(
			f.ctx, qRecon, now, now.Add(time.Minute), now, 10,
		)
		errCh <- claimErr
		done <- claimed
	}()

	foundExpire := false
	for _, c := range claimedExpire {
		if c.ID == model.PaymentAttemptID(attempt) {
			foundExpire = true
			_, markErr := paymentattemptrp.MarkExpired(f.ctx, qExpire, c.ID, c.State, now, now)
			require.NoError(t, markErr)
		}
	}
	require.True(t, foundExpire)
	require.NoError(t, txExpire.Commit(f.ctx))

	require.NoError(t, <-errCh)
	claimedRecon := <-done
	for _, c := range claimedRecon {
		require.NotEqual(t, model.PaymentAttemptID(attempt), c.ID)
	}
	require.NoError(t, txRecon.Rollback(f.ctx))

	var state string
	var next *time.Time
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT state, next_reconciliation_at FROM payment_attempts WHERE id=$1`, attempt).Scan(&state, &next))
	require.Equal(t, "expired", state)
	require.Nil(t, next)
}

func TestUnresolvedExpiredBlocksSecondPaymentAttemptUniqueIndex(t *testing.T) {
	f := newFixture(t, true)
	attempt := f.attempt(t)
	f.exec(t, `
UPDATE payment_attempts
SET state='expired',
    expired_at=now(),
    financial_outcome_resolved=false,
    financial_payment_session_id='ps-ambiguous'
WHERE id=$1
`, attempt)

	var indexDef string
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT indexdef FROM pg_indexes
WHERE schemaname = current_schema() AND indexname = 'uq_payment_attempts_active_installment'
`).Scan(&indexDef))
	require.Contains(t, indexDef, "expired")
	require.Contains(t, indexDef, "financial_outcome_resolved")

	// Trigger fires first (BN003); the partial unique index is the second line of defense.
	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer tx.Rollback(f.ctx)
	assertViolation(t, f.insertAttempt(tx, uuid.New()), "BN003")
}
