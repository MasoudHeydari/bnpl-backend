//go:build development && integration

package invariants

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
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
