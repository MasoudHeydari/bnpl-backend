//go:build integration && development

package collectionsuc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/contractrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/creditaccountrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/inboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/identifier/uuidgen"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	reconciliationevent "github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	fc "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"
	reconciliationuc "github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentreconciliationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptexpirationuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/paymentattemptreconciliationuc"
)

func (f *collectionsFixture) insertExpiredSessionCreationAttempt(
	t *testing.T,
	installment uuid.UUID,
	reconCount int,
	intentID *string,
) (model.PaymentAttemptID, model.ContractID) {
	t.Helper()

	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))

	attemptID := model.PaymentAttemptID(uuid.New())
	createdAt := f.now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	leaseUntil := f.now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)
	reason := model.PaymentSessionCreationTimeoutPaymentReconciliationReason

	tx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)

	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count, financial_session_create_lease_until,
	financial_payment_intent_id, reconciliation_reason, reconciliation_requested_at,
	reconciliation_attempt_count, next_reconciliation_at
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'reconciliation_required', 'gateway',
	$4, $5, $6, $6, 3, $7, $8, $9, $6, $10, $6
)`, attemptID, contract, installment, uuid.NewString(), "sha256:"+strings.Repeat("e", 64), createdAt, leaseUntil, intentID, reason, reconCount)
	require.NoError(t, err)

	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect)
VALUES ($1,'principal',$2,100,'debit')
`, attemptID, installment)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(f.ctx))

	expireTx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer expireTx.Rollback(f.ctx)

	_, err = paymentattemptrp.MarkExpired(
		f.ctx,
		&postgres.Tx{Tx: expireTx},
		attemptID,
		model.ReconciliationRequiredPaymentAttemptState,
		f.now,
		f.now,
	)
	require.NoError(t, err)
	require.NoError(t, expireTx.Commit(f.ctx))

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1 AND state='expired' AND financial_outcome_resolved=false AND expired_at IS NOT NULL
`, attemptID))

	return attemptID, contract
}

func (f *collectionsFixture) newPaymentResultUC(
	t *testing.T,
	client fc.Client,
	pool repo.Pool,
	paymentAttempt repo.PaymentAttempt,
) *financialcorepaymentresultuc.UseCase {
	t.Helper()

	if pool == nil {
		pool = f.pool
	}

	if paymentAttempt == nil {
		paymentAttempt = paymentattemptrp.New()
	}

	uc, err := financialcorepaymentresultuc.New(
		pool,
		&contractuc.UseCase{},
		&checkoutsessionuc.UseCase{},
		client,
		inboxrp.New(),
		outboxrp.New(),
		paymentAttempt,
		contractrp.New(),
		installmentrp.New(),
		installmentchargerp.New(),
		creditaccountrp.New(),
		uuidgen.New(),
		sha256fingerprint.New(),
		"fc.payment.applications",
		"bnpl.audit.events.v1",
		f.clk,
		0,
		metricsnoop.New(),
	)
	require.NoError(t, err)

	uc.WithCollectionsCoordination(customerprofilerp.New(), func(ctx context.Context, id model.CustomerID) error {
		_, e := f.uc.AssessCustomer(ctx, id)
		return e
	})

	return uc
}

// pidCapturingPool is a test-only Pool wrapper that publishes the PostgreSQL
// backend PID of each leased connection used by a Use Case.
type pidCapturingPool struct {
	inner  *postgres.Pool
	onPID  func(int32)
	closed atomic.Bool
}

func (p *pidCapturingPool) Conn(ctx context.Context, handler repo.ConnHandler) error {
	return p.inner.Conn(ctx, func(ctx context.Context, conn repo.Conn) error {
		pid, err := queryBackendPIDFromConn(ctx, conn)
		if err == nil && p.onPID != nil {
			p.onPID(pid)
		}

		return handler(ctx, conn)
	})
}

func (p *pidCapturingPool) Close() error {
	if p.closed.Swap(true) {
		return nil
	}

	return p.inner.Close()
}

func queryBackendPIDFromConn(ctx context.Context, conn repo.Conn) (int32, error) {
	rows, err := conn.Query(ctx, `SELECT pg_backend_pid()`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}

		return 0, fmt.Errorf("pg_backend_pid returned no rows")
	}

	var pid int32
	if err := rows.Scan(&pid); err != nil {
		return 0, err
	}

	return pid, rows.Err()
}

func waitUntilProcessorBlockedByHolder(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	holderPID int32,
	capturedPIDs *[]int32,
	mu *sync.Mutex,
	timeout time.Duration,
) int32 {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		mu.Lock()
		snapshot := append([]int32(nil), (*capturedPIDs)...)
		mu.Unlock()

		for _, processorPID := range snapshot {
			var blockers []int32
			err := pool.QueryRow(ctx, `SELECT coalesce(pg_blocking_pids($1), '{}'::int[])`, processorPID).Scan(&blockers)
			require.NoError(t, err)

			for _, blocker := range blockers {
				if blocker == holderPID {
					return processorPID
				}
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	mu.Lock()
	snapshot := append([]int32(nil), (*capturedPIDs)...)
	mu.Unlock()

	observed := make(map[int32][]int32, len(snapshot))
	for _, processorPID := range snapshot {
		var blockers []int32
		_ = pool.QueryRow(ctx, `SELECT coalesce(pg_blocking_pids($1), '{}'::int[])`, processorPID).Scan(&blockers)
		observed[processorPID] = blockers
	}

	t.Fatalf(
		"timed out after %s waiting for processor blocked by holderPID=%d; capturedPIDs=%v observedBlocking=%v",
		timeout,
		holderPID,
		snapshot,
		observed,
	)
	return 0
}

// lockGatePaymentAttempt is a test-only PaymentAttempt wrapper that pauses after
// LockByIDWithItems so reverse lock-order scenarios can run expiration while the
// FC result transaction still holds the PaymentAttempt row lock.
type lockGatePaymentAttempt struct {
	repo.PaymentAttempt
	locked  chan struct{}
	proceed chan struct{}
	once    sync.Once
}

func (p *lockGatePaymentAttempt) Tx(tx repo.Tx) repo.PaymentAttemptTxQueryer {
	return &lockGatePaymentAttemptTx{
		PaymentAttemptTxQueryer: p.PaymentAttempt.Tx(tx),
		parent:                  p,
	}
}

type lockGatePaymentAttemptTx struct {
	repo.PaymentAttemptTxQueryer
	parent *lockGatePaymentAttempt
}

func (q *lockGatePaymentAttemptTx) LockByIDWithItems(
	ctx context.Context,
	id model.PaymentAttemptID,
) (model.PaymentAttempt, error) {
	attempt, err := q.PaymentAttemptTxQueryer.LockByIDWithItems(ctx, id)
	if err != nil {
		return attempt, err
	}

	q.parent.once.Do(func() {
		close(q.parent.locked)
	})

	select {
	case <-q.parent.proceed:
		return attempt, nil

	case <-ctx.Done():
		return model.PaymentAttempt{}, ctx.Err()
	}
}

type repaymentFinancialSnapshot struct {
	AttemptState             string
	FinancialOutcomeResolved bool
	IntentID                 *string
	SessionID                *string
	AmountMinor              int64
	InstallmentStatus        string
	PaidAmountMinor          int64
	ChargeStatuses           string
	ExposureMinor            int64
	RemainingPrincipal       int64
	InboxProcessed           int
	PaymentAppOutbox         int
	AuditOutbox              int
	AttemptItems             int
}

func (f *collectionsFixture) captureRepaymentFinancialSnapshot(
	t *testing.T,
	customer model.CustomerID,
	installment uuid.UUID,
	attemptID model.PaymentAttemptID,
	eventID string,
) repaymentFinancialSnapshot {
	t.Helper()

	var snap repaymentFinancialSnapshot
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT state::text,
       financial_outcome_resolved,
       financial_payment_intent_id,
       financial_payment_session_id,
       amount_minor
FROM payment_attempts WHERE id=$1
`, attemptID).Scan(
		&snap.AttemptState,
		&snap.FinancialOutcomeResolved,
		&snap.IntentID,
		&snap.SessionID,
		&snap.AmountMinor,
	))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT status::text, paid_amount_minor FROM installments WHERE id=$1
`, installment).Scan(&snap.InstallmentStatus, &snap.PaidAmountMinor))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT coalesce(string_agg(charge_status::text || ':' || charge_type::text, ',' ORDER BY id), '')
FROM installment_charges WHERE installment_id=$1
`, installment).Scan(&snap.ChargeStatuses))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1
`, customer).Scan(&snap.ExposureMinor))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `
SELECT remaining_principal_amount_minor FROM contracts WHERE customer_id=$1
`, customer).Scan(&snap.RemainingPrincipal))
	snap.InboxProcessed = f.scalar(t, `
SELECT count(*) FROM integration_inbox WHERE event_id=$1 AND status='processed'
`, eventID)
	intentKey := ""
	if snap.IntentID != nil {
		intentKey = *snap.IntentID
	}
	snap.PaymentAppOutbox = f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='fc.payment.applications' AND message_key=$1
`, intentKey)
	snap.AuditOutbox = f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1'
  AND payload::text LIKE '%' || $1 || '%'
`, attemptID.String())
	snap.AttemptItems = f.scalar(t, `
SELECT count(*) FROM payment_attempt_items WHERE payment_attempt_id=$1
`, attemptID)

	return snap
}

func (f *collectionsFixture) newReconciliationResultUC(
	t *testing.T,
	paymentResult *financialcorepaymentresultuc.UseCase,
) *reconciliationuc.UseCase {
	t.Helper()

	uc, err := reconciliationuc.New(
		f.pool,
		paymentResult,
		inboxrp.New(),
		outboxrp.New(),
		paymentattemptrp.New(),
		contractrp.New(),
		installmentrp.New(),
		installmentchargerp.New(),
		creditaccountrp.New(),
		sha256fingerprint.New(),
		uuidgen.New(),
		"fc.reconciliation.acks",
		"bnpl.audit.events.v1",
		time.Second,
		time.Minute,
		f.clk,
		lognoop.New(),
	)
	require.NoError(t, err)

	return uc
}

func TestPostgreSQLExpiredPendingIncomplete_DurablyAcknowledgedWithoutRetry(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	attemptID, _ := f.insertExpiredSessionCreationAttempt(t, installment, 1, nil)

	resultUC := f.newPaymentResultUC(t, authoritativePaymentClient{}, nil, nil)
	reconUC := f.newReconciliationResultUC(t, resultUC)

	message := reconciliationuc.ProcessInput{
		EventID:       uuid.NewString(),
		EventType:     reconciliationevent.ReconciliationResultEventType,
		EventVersion:  "1",
		SourceService: reconciliationevent.FinancialCoreSourceService,
		RawPayload:    json.RawMessage(`{"pending":"incomplete"}`),
		Data: reconciliationevent.ResultEventData{
			RequestEventID:           reconciliationevent.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:     attemptID,
			ReconciliationGeneration: 1,
			Status:                   reconciliationevent.PendingResultStatus,
			FinancialPaymentIntentID: strPtr("pi-incomplete"),
			AmountMinor:              100,
			Currency:                 "IRR",
			OccurredAt:               f.now,
		},
	}

	got, err := reconUC.Process(f.ctx, message)
	require.NoError(t, err)
	require.Equal(t, reconciliationuc.ProcessResultStillReconciliationRequired, got.Result)

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='expired'
  AND financial_outcome_resolved=false
  AND next_reconciliation_at IS NULL
  AND financial_payment_intent_id='pi-incomplete'
`, attemptID))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM integration_inbox WHERE event_id=$1 AND status='processed'`, message.EventID))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='fc.reconciliation.acks'
  AND payload::text LIKE '%still_reconciliation_required%'
  AND payload::text LIKE '%pi-incomplete%'
`))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
`))
	auditBeforeReplay := f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1' AND message_key=$1
`, attemptID.String())

	replay, err := reconUC.Process(f.ctx, message)
	require.NoError(t, err)
	require.Equal(t, reconciliationuc.ProcessResultAlreadyProcessed, replay.Result)
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='fc.reconciliation.acks' AND payload::text LIKE '%still_reconciliation_required%'
`))
	require.Equal(t, auditBeforeReplay, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1' AND message_key=$1
`, attemptID.String()))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1 AND financial_payment_intent_id='pi-incomplete' AND state='expired'
`, attemptID))
}

func TestPostgreSQLExpiredPending_FCSessionAlreadyExpired_NoRetry(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	attemptID, _ := f.insertExpiredSessionCreationAttempt(t, installment, 1, nil)

	resultUC := f.newPaymentResultUC(t, authoritativePaymentClient{}, nil, nil)
	reconUC := f.newReconciliationResultUC(t, resultUC)

	url := "https://pay.example/expired-session"
	expires := f.now.Add(-time.Minute)
	message := reconciliationuc.ProcessInput{
		EventID:       uuid.NewString(),
		EventType:     reconciliationevent.ReconciliationResultEventType,
		EventVersion:  "1",
		SourceService: reconciliationevent.FinancialCoreSourceService,
		RawPayload:    json.RawMessage(`{"pending":"expired_session"}`),
		Data: reconciliationevent.ResultEventData{
			RequestEventID:            reconciliationevent.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    reconciliationevent.PendingResultStatus,
			FinancialPaymentIntentID:  strPtr("pi-late"),
			FinancialPaymentSessionID: strPtr("ps-late"),
			PaymentURL:                &url,
			ExpiresAt:                 &expires,
			AmountMinor:               100,
			Currency:                  "IRR",
			OccurredAt:                f.now,
		},
	}

	got, err := reconUC.Process(f.ctx, message)
	require.NoError(t, err)
	require.Equal(t, reconciliationuc.ProcessResultStillReconciliationRequired, got.Result)
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='expired'
  AND financial_outcome_resolved=false
  AND financial_payment_intent_id='pi-late'
  AND financial_payment_session_id='ps-late'
  AND payment_url IS NULL
  AND next_reconciliation_at IS NULL
`, attemptID))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
`))
}

func TestPostgreSQLExpiredLateAuthoritativeSuccess_AndReplay(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	paidAt := f.now.Add(-time.Minute)
	resultUC, input := f.settlement(t, customer, installment, paidAt, 100)

	var attemptID model.PaymentAttemptID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT id FROM payment_attempts WHERE financial_payment_intent_id=$1`, input.PaymentIntentID).Scan(&attemptID))

	// Locally expire after three CreateCheckoutPayment attempts without inventing FC failure.
	f.exec(t, `
UPDATE payment_attempts
SET state='expired',
    expired_at=$2,
    financial_outcome_resolved=false,
    financial_session_create_attempt_count=3,
    financial_payment_session_id=NULL,
    payment_url=NULL,
    financial_payment_expires_at=NULL,
    next_reconciliation_at=NULL,
    submitted_at=NULL
WHERE id=$1
`, attemptID, f.now)

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1 AND state='expired' AND NOT financial_outcome_resolved
`, attemptID))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status<>'paid'`, installment))

	baseline := f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID)
	require.Equal(t, "expired", baseline.AttemptState)
	require.False(t, baseline.FinancialOutcomeResolved)
	require.Equal(t, int64(200), baseline.ExposureMinor)

	input.SourceType = "bnpl_payment_attempt"
	input.SourceID = attemptID.String()

	got, err := resultUC.Process(f.ctx, input)
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, got.Result)

	afterApply := f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID)
	require.Equal(t, "succeeded", afterApply.AttemptState)
	require.True(t, afterApply.FinancialOutcomeResolved)
	require.NotNil(t, afterApply.IntentID)
	require.Equal(t, input.PaymentIntentID, *afterApply.IntentID)
	require.Equal(t, "paid", afterApply.InstallmentStatus)
	require.Equal(t, int64(100), afterApply.PaidAmountMinor)
	require.Equal(t, afterApply.AmountMinor, afterApply.PaidAmountMinor)
	require.Equal(t, 1, afterApply.InboxProcessed)
	require.Equal(t, 1, afterApply.PaymentAppOutbox)
	require.GreaterOrEqual(t, afterApply.AuditOutbox, 1)
	require.Equal(t, int64(100), afterApply.ExposureMinor)
	require.Equal(t, int64(100), afterApply.RemainingPrincipal)
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM installments WHERE customer_id=$1 AND id<>$2 AND status<>'paid'
`, customer, installment))

	// Identical EventID is absorbed by Inbox idempotency.
	replaySameEvent, err := resultUC.Process(f.ctx, input)
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultEventAlreadyProcessed, replaySameEvent.Result)
	afterSameEvent := f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID)
	require.Equal(t, afterApply, afterSameEvent)

	// A distinct wake-up for the same succeeded payment must be already_applied.
	replayNewEvent := input
	replayNewEvent.EventID = uuid.NewString()
	replayNewEvent.RawPayload = json.RawMessage(`{"late":true,"replay":2}`)
	replayApplied, err := resultUC.Process(f.ctx, replayNewEvent)
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultAlreadyApplied, replayApplied.Result)

	afterNewEvent := f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID)
	assertRepaymentFinancialEffectsUnchanged(t, afterApply, afterNewEvent)
	// A new EventID may emit a legitimate already_applied acknowledgement Outbox row;
	// that is delivery/ack traffic, not a second financial application.
	require.GreaterOrEqual(t, afterNewEvent.PaymentAppOutbox, afterApply.PaymentAppOutbox)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM integration_inbox WHERE event_id=$1 AND status='processed'`, replayNewEvent.EventID))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts WHERE installment_id=$1 AND state='succeeded'
`, installment))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='fc.payment.applications'
  AND message_key=$1
  AND payload #>> '{data,result}' = 'applied'
`, input.PaymentIntentID))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='fc.payment.applications'
  AND message_key=$1
  AND payload #>> '{data,result}' = 'already_applied'
`, input.PaymentIntentID))

	// Conflicting amount for the same FC intent must not apply a second payment.
	mismatchClient := authoritativePaymentClient{
		status: fc.GetPaymentStatusResult{
			PaymentIntentID: input.PaymentIntentID,
			Status:          fc.SucceededPaymentStatus,
			AmountMinor:     999,
			Currency:        "IRR",
			CompletedAt:     paidAt,
		},
	}
	mismatchUC := f.newPaymentResultUC(t, mismatchClient, nil, nil)
	mismatchInput := input
	mismatchInput.EventID = uuid.NewString()
	mismatchInput.RawPayload = json.RawMessage(`{"late":true,"amount_mismatch":true}`)
	mismatchGot, err := mismatchUC.Process(f.ctx, mismatchInput)
	require.NoError(t, err)
	require.NotEqual(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, mismatchGot.Result)
	assertRepaymentFinancialEffectsUnchanged(t, afterApply, f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID))

	// Conflicting currency likewise must not re-apply.
	currencyClient := authoritativePaymentClient{
		status: fc.GetPaymentStatusResult{
			PaymentIntentID: input.PaymentIntentID,
			Status:          fc.SucceededPaymentStatus,
			AmountMinor:     100,
			Currency:        "USD",
			CompletedAt:     paidAt,
		},
	}
	currencyUC := f.newPaymentResultUC(t, currencyClient, nil, nil)
	currencyInput := input
	currencyInput.EventID = uuid.NewString()
	currencyInput.RawPayload = json.RawMessage(`{"late":true,"currency_mismatch":true}`)
	currencyGot, err := currencyUC.Process(f.ctx, currencyInput)
	require.NoError(t, err)
	require.NotEqual(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, currencyGot.Result)
	assertRepaymentFinancialEffectsUnchanged(t, afterApply, f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID))
}

func assertRepaymentFinancialEffectsUnchanged(t *testing.T, want, got repaymentFinancialSnapshot) {
	t.Helper()

	require.Equal(t, want.AttemptState, got.AttemptState)
	require.Equal(t, want.FinancialOutcomeResolved, got.FinancialOutcomeResolved)
	require.Equal(t, want.IntentID, got.IntentID)
	require.Equal(t, want.SessionID, got.SessionID)
	require.Equal(t, want.AmountMinor, got.AmountMinor)
	require.Equal(t, want.InstallmentStatus, got.InstallmentStatus)
	require.Equal(t, want.PaidAmountMinor, got.PaidAmountMinor)
	require.Equal(t, want.ChargeStatuses, got.ChargeStatuses)
	require.Equal(t, want.ExposureMinor, got.ExposureMinor)
	require.Equal(t, want.RemainingPrincipal, got.RemainingPrincipal)
	require.Equal(t, want.AttemptItems, got.AttemptItems)
}

func TestPostgreSQLExpirationWorkers_ExactlyOneAuditOutbox(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))

	attemptID := uuid.New()
	createdAt := f.now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	leaseUntil := f.now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)
	tx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count, financial_session_create_lease_until
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'created', 'gateway',
	$4, $5, $6, $6, 3, $7
)`, attemptID, contract, installment, uuid.NewString(), "sha256:"+strings.Repeat("f", 64), createdAt, leaseUntil)
	require.NoError(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'principal',$2,100,'debit')`, attemptID, installment)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(f.ctx))

	newExpireUC := func() *paymentattemptexpirationuc.UseCase {
		uc, e := paymentattemptexpirationuc.New(
			f.pool,
			paymentattemptrp.New(),
			outboxrp.New(),
			uuidgen.New(),
			"bnpl.audit.events.v1",
			10,
			model.FinancialSessionCreateExpirationSafetyInterval,
			adapterclock.NewFixed(f.now, time.UTC),
			lognoop.New(),
		)
		require.NoError(t, e)
		return uc
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs <- newExpireUC().ProcessDue(f.ctx) }()
	go func() { defer wg.Done(); errs <- newExpireUC().ProcessDue(f.ctx) }()
	wg.Wait()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='canceled'
  AND failure_code='session_creation_retry_exhausted'
  AND financial_outcome_resolved=false
`, attemptID))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1'
  AND payload::text LIKE '%cancel_session_creation_retry_exhausted%'
`))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
`))

	require.NoError(t, newExpireUC().ProcessDue(f.ctx))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1'
  AND payload::text LIKE '%cancel_session_creation_retry_exhausted%'
`))
}

func TestPostgreSQLExpirationVsReconciliation_ExhaustedNoFCRequest(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))

	attemptID := uuid.New()
	createdAt := f.now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	leaseUntil := f.now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)
	reason := model.FinancialCoreStatusStillUnknownPaymentReconciliationReason
	nextAt := f.now.Add(-time.Second)

	tx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count, financial_session_create_lease_until,
	reconciliation_reason, reconciliation_requested_at, reconciliation_attempt_count,
	next_reconciliation_at
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'reconciliation_required', 'gateway',
	$4, $5, $6, $6, 3, $7, $8, $6, 1, $9
)`, attemptID, contract, installment, uuid.NewString(), "sha256:"+strings.Repeat("1", 64), createdAt, leaseUntil, reason, nextAt)
	require.NoError(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'principal',$2,100,'debit')`, attemptID, installment)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(f.ctx))

	expireUC, err := paymentattemptexpirationuc.New(
		f.pool,
		paymentattemptrp.New(),
		outboxrp.New(),
		uuidgen.New(),
		"bnpl.audit.events.v1",
		10,
		model.FinancialSessionCreateExpirationSafetyInterval,
		adapterclock.NewFixed(f.now, time.UTC),
		lognoop.New(),
	)
	require.NoError(t, err)

	reconUC, err := paymentattemptreconciliationuc.New(
		f.pool,
		paymentattemptrp.New(),
		contractrp.New(),
		outboxrp.New(),
		uuidgen.New(),
		"bnpl.installment_payment_reconciliation_requests",
		"bnpl.audit.events.v1",
		10,
		time.Minute,
		time.Second,
		time.Minute,
		5,
		adapterclock.NewFixed(f.now, time.UTC),
	)
	require.NoError(t, err)

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); errs <- expireUC.ProcessDue(f.ctx) }()
	go func() { defer wg.Done(); errs <- reconUC.ProcessDue(f.ctx) }()
	wg.Wait()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='canceled'
  AND failure_code='session_creation_retry_exhausted'
  AND financial_outcome_resolved=false
  AND next_reconciliation_at IS NULL
`, attemptID))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
`))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1' AND message_key=$1
`, attemptID.String()))
}

func TestPostgreSQLExpirationVsLateFCResult_LockOrdering(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	paidAt := f.now.Add(-time.Minute)
	_, input := f.settlement(t, customer, installment, paidAt, 100)

	var attemptID model.PaymentAttemptID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT id FROM payment_attempts WHERE financial_payment_intent_id=$1`, input.PaymentIntentID).Scan(&attemptID))
	f.exec(t, `
UPDATE payment_attempts
SET state='expired',
    expired_at=$2,
    financial_outcome_resolved=false,
    financial_session_create_attempt_count=3,
    financial_payment_session_id=NULL,
    payment_url=NULL,
    financial_payment_expires_at=NULL,
    next_reconciliation_at=NULL,
    submitted_at=NULL
WHERE id=$1
`, attemptID, f.now)
	input.SourceType = "bnpl_payment_attempt"
	input.SourceID = attemptID.String()

	var (
		capturedMu  sync.Mutex
		capturedPIDs []int32
	)
	capturingPool := &pidCapturingPool{
		inner: f.pool,
		onPID: func(pid int32) {
			capturedMu.Lock()
			capturedPIDs = append(capturedPIDs, pid)
			capturedMu.Unlock()
		},
	}
	client := authoritativePaymentClient{
		status: fc.GetPaymentStatusResult{
			PaymentIntentID: input.PaymentIntentID,
			Status:          fc.SucceededPaymentStatus,
			AmountMinor:     100,
			Currency:        "IRR",
			CompletedAt:     paidAt,
		},
	}
	resultUC := f.newPaymentResultUC(t, client, capturingPool, nil)

	// Hold the payment_attempt row while a concurrent result waits, then release.
	hold, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer hold.Rollback(f.ctx)

	var lockedID uuid.UUID
	require.NoError(t, hold.QueryRow(f.ctx, `SELECT id FROM payment_attempts WHERE id=$1 FOR UPDATE`, attemptID).Scan(&lockedID))

	var holderPID int32
	require.NoError(t, hold.QueryRow(f.ctx, `SELECT pg_backend_pid()`).Scan(&holderPID))

	done := make(chan error, 1)
	go func() {
		_, processErr := resultUC.Process(f.ctx, input)
		done <- processErr
	}()

	processorPID := waitUntilProcessorBlockedByHolder(
		t,
		f.ctx,
		f.pool.Pool,
		holderPID,
		&capturedPIDs,
		&capturedMu,
		5*time.Second,
	)
	require.NotEqual(t, holderPID, processorPID)

	require.NoError(t, hold.Commit(f.ctx))
	require.NoError(t, <-done)

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1 AND state='succeeded' AND financial_outcome_resolved=true
`, attemptID))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status='paid'`, installment))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts WHERE installment_id=$1 AND state='succeeded'
`, installment))
}

func TestPostgreSQLLateFCResultVsExpiration_ReverseLockOrdering(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	paidAt := f.now.Add(-time.Minute)

	// Build an expiration-eligible created attempt that can still accept authoritative success.
	f.exec(t, `UPDATE installments SET installment_order=3 WHERE customer_id=$1 AND id<>$2`, customer, installment)
	f.exec(t, `UPDATE installments SET installment_order=2 WHERE id=$1`, installment)
	f.exec(t, `UPDATE contracts SET requested_amount_minor=200,approved_financed_amount_minor=200,remaining_principal_amount_minor=200,installment_count=3 WHERE customer_id=$1`, customer)
	f.exec(t, `UPDATE credit_accounts SET limit_amount_minor=1000,exposure_amount_minor=200 WHERE customer_id=$1`, customer)

	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))

	attemptID := model.PaymentAttemptID(uuid.New())
	intent := "fc-" + uuid.NewString()
	createdAt := f.now.Add(-model.InstallmentRepaymentPaymentSessionTTL - 2*time.Minute)
	leaseUntil := f.now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)

	tx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	charges, err := installmentchargerp.New().Tx(&postgres.Tx{Tx: tx}).ListByInstallmentIDs(f.ctx, []model.InstallmentID{model.InstallmentID(installment)})
	require.NoError(t, err)
	row := model.Installment{ID: model.InstallmentID(installment), PrincipalAmountMinor: 100, Currency: model.IRRCurrency, Status: model.ScheduledInstallmentStatus}
	items, err := model.BuildPaymentAttemptItems(row, charges)
	require.NoError(t, err)
	cash, ok := model.PayableAmountFromItems(items)
	require.True(t, ok)
	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, financial_payment_intent_id, financial_collection_obligation_id,
	created_at, updated_at, financial_session_create_attempt_count, financial_session_create_lease_until
) VALUES (
	$1, $2, $3, $4, 'IRR', 'installment_repayment', 'created', 'gateway',
	$5, $6, $7, 'collection-1', $8, $8, 3, $9
)`, attemptID, contract, installment, cash, uuid.NewString(), "sha256:"+fmt.Sprintf("%064x", 1), intent, createdAt, leaseUntil)
	require.NoError(t, err)
	for _, item := range items {
		_, err = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,$2,$3,$4,$5)`, attemptID, item.ItemType, item.SourceID, item.AmountMinor, item.Effect)
		require.NoError(t, err)
	}
	require.NoError(t, tx.Commit(f.ctx))

	input := financialcorepaymentresultuc.ProcessInput{
		EventID:         uuid.NewString(),
		EventType:       "financial_core.payment_result.v1",
		EventVersion:    "1",
		SourceService:   "financial-core",
		RawPayload:      json.RawMessage(`{"wake_up":true}`),
		PaymentPurpose:  model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ContractID:      contract,
		InstallmentID:   model.InstallmentID(installment),
		PaymentIntentID: intent,
		PaymentStatus:   model.SucceededFinancialPaymentResultStatus,
		EventOccurredAt: f.now,
		SourceType:      "bnpl_payment_attempt",
		SourceID:        attemptID.String(),
	}

	gate := &lockGatePaymentAttempt{
		PaymentAttempt: paymentattemptrp.New(),
		locked:         make(chan struct{}),
		proceed:        make(chan struct{}),
	}
	client := authoritativePaymentClient{
		status: fc.GetPaymentStatusResult{
			PaymentIntentID: intent,
			Status:          fc.SucceededPaymentStatus,
			AmountMinor:     100,
			Currency:        "IRR",
			CompletedAt:     paidAt,
		},
	}
	resultUC := f.newPaymentResultUC(t, client, nil, gate)

	expireUC, err := paymentattemptexpirationuc.New(
		f.pool,
		paymentattemptrp.New(),
		outboxrp.New(),
		uuidgen.New(),
		"bnpl.audit.events.v1",
		10,
		model.FinancialSessionCreateExpirationSafetyInterval,
		adapterclock.NewFixed(f.now, time.UTC),
		lognoop.New(),
	)
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() {
		_, processErr := resultUC.Process(f.ctx, input)
		done <- processErr
	}()

	select {
	case <-gate.locked:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for FC result processing to lock PaymentAttempt")
	}

	// Expiration uses FOR UPDATE SKIP LOCKED, so it must skip the locked attempt
	// rather than overwrite while the FC result transaction still holds the row.
	require.NoError(t, expireUC.ProcessDue(f.ctx))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts WHERE id=$1 AND state='created' AND NOT financial_outcome_resolved
`, attemptID))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1'
  AND message_key=$1
  AND payload::text LIKE '%expire_local_session_creation%'
`, attemptID.String()))

	close(gate.proceed)
	require.NoError(t, <-done)

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1 AND state='succeeded' AND financial_outcome_resolved=true
`, attemptID))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status='paid'`, installment))

	require.NoError(t, expireUC.ProcessDue(f.ctx))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1 AND state='succeeded' AND financial_outcome_resolved=true
`, attemptID))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.audit.events.v1'
  AND message_key=$1
  AND payload::text LIKE '%expire_local_session_creation%'
`, attemptID.String()))
}

func TestPostgreSQLSubmittedPaymentResultTimeout_StillReconciles(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))

	attemptID := uuid.New()
	sessionID := "fc-session-submitted"
	expiresAt := f.now.Add(-2 * time.Minute)
	reason := model.PaymentResultTimeoutPaymentReconciliationReason
	nextAt := f.now.Add(-time.Second)

	tx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at, submitted_at,
	financial_payment_session_id, financial_payment_expires_at,
	reconciliation_reason, reconciliation_requested_at, reconciliation_attempt_count,
	next_reconciliation_at, financial_session_create_attempt_count
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'reconciliation_required', 'gateway',
	$4, $5, $6, $6, $6, $7, $8, $9, $6, 1, $10, 3
)`, attemptID, contract, installment, uuid.NewString(), "sha256:"+strings.Repeat("2", 64), f.now.Add(-30*time.Minute), sessionID, expiresAt, reason, nextAt)
	require.NoError(t, err)
	_, err = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,'principal',$2,100,'debit')`, attemptID, installment)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(f.ctx))

	reconUC, err := paymentattemptreconciliationuc.New(
		f.pool,
		paymentattemptrp.New(),
		contractrp.New(),
		outboxrp.New(),
		uuidgen.New(),
		"bnpl.installment_payment_reconciliation_requests",
		"bnpl.audit.events.v1",
		10,
		time.Minute,
		time.Second,
		time.Minute,
		5,
		adapterclock.NewFixed(f.now, time.UTC),
	)
	require.NoError(t, err)
	require.NoError(t, reconUC.ProcessDue(f.ctx))

	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
  AND message_key=$1
`, attemptID.String()))
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts WHERE id=$1 AND state='reconciliation_required'
`, attemptID))
}

func TestPostgreSQLExpiredPending_ConflictingIdentityRejected(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	storedIntent := "pi-stored"
	attemptID, _ := f.insertExpiredSessionCreationAttempt(t, installment, 1, &storedIntent)

	resultUC := f.newPaymentResultUC(t, authoritativePaymentClient{}, nil, nil)
	reconUC := f.newReconciliationResultUC(t, resultUC)

	message := reconciliationuc.ProcessInput{
		EventID:       uuid.NewString(),
		EventType:     reconciliationevent.ReconciliationResultEventType,
		EventVersion:  "1",
		SourceService: reconciliationevent.FinancialCoreSourceService,
		RawPayload:    json.RawMessage(`{"pending":"conflict"}`),
		Data: reconciliationevent.ResultEventData{
			RequestEventID:           reconciliationevent.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:     attemptID,
			ReconciliationGeneration: 1,
			Status:                   reconciliationevent.PendingResultStatus,
			FinancialPaymentIntentID: strPtr("pi-other"),
			AmountMinor:              100,
			Currency:                 "IRR",
			OccurredAt:               f.now,
		},
	}

	got, err := reconUC.Process(f.ctx, message)
	require.NoError(t, err)
	require.Equal(t, reconciliationuc.ProcessResultPermanentlyRejected, got.Result)
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='expired'
  AND financial_outcome_resolved=false
  AND financial_payment_intent_id='pi-stored'
`, attemptID))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
`))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status<>'paid'`, installment))
}

func TestPostgreSQLExhaustedBudget_IncompletePendingEnrichesWithoutRetry(t *testing.T) {
	f := newCollectionsFixture(t)
	_, installment := f.seed(t)
	var contract model.ContractID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))

	attemptID := model.PaymentAttemptID(uuid.New())
	// Immutable TTL still unexpired; only the synchronous CreateCheckoutPayment budget is exhausted.
	createdAt := f.now.Add(-2 * time.Minute)
	nextAt := f.now.Add(time.Minute)
	reason := model.FinancialCoreStatusStillUnknownPaymentReconciliationReason

	tx, err := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempts(
	id, contract_id, installment_id, amount_minor, currency, purpose, state, channel,
	idempotency_key, request_fingerprint, created_at, updated_at,
	financial_session_create_attempt_count,
	reconciliation_reason, reconciliation_requested_at, reconciliation_attempt_count,
	next_reconciliation_at
) VALUES (
	$1, $2, $3, 100, 'IRR', 'installment_repayment', 'reconciliation_required', 'gateway',
	$4, $5, $6, $6, 3, $7, $6, 1, $8
)`, attemptID, contract, installment, uuid.NewString(), "sha256:"+strings.Repeat("b", 64), createdAt, reason, nextAt)
	require.NoError(t, err)
	_, err = tx.Exec(f.ctx, `
INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect)
VALUES ($1,'principal',$2,100,'debit')
`, attemptID, installment)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(f.ctx))

	resultUC := f.newPaymentResultUC(t, authoritativePaymentClient{}, nil, nil)
	reconUC := f.newReconciliationResultUC(t, resultUC)

	message := reconciliationuc.ProcessInput{
		EventID:       uuid.NewString(),
		EventType:     reconciliationevent.ReconciliationResultEventType,
		EventVersion:  "1",
		SourceService: reconciliationevent.FinancialCoreSourceService,
		RawPayload:    json.RawMessage(`{"pending":"exhausted_budget"}`),
		Data: reconciliationevent.ResultEventData{
			RequestEventID:           reconciliationevent.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:     attemptID,
			ReconciliationGeneration: 1,
			Status:                   reconciliationevent.PendingResultStatus,
			FinancialPaymentIntentID: strPtr("pi-budget"),
			AmountMinor:              100,
			Currency:                 "IRR",
			OccurredAt:               f.now,
		},
	}

	got, err := reconUC.Process(f.ctx, message)
	require.NoError(t, err)
	require.Equal(t, reconciliationuc.ProcessResultStillReconciliationRequired, got.Result)
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='reconciliation_required'
  AND financial_outcome_resolved=false
  AND financial_payment_intent_id='pi-budget'
  AND next_reconciliation_at IS NULL
  AND expired_at IS NULL
`, attemptID))
	require.Zero(t, f.scalar(t, `
SELECT count(*) FROM integration_outbox
WHERE destination='bnpl.installment_payment_reconciliation_requests'
`))
}

func TestPostgreSQLExpiredPendingEnrichment_ThenLateAuthoritativeSuccess(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	paidAt := f.now.Add(-time.Minute)

	// Prepare contract/installment economics like settlement, then expire without an FC intent.
	f.exec(t, `UPDATE installments SET installment_order=3 WHERE customer_id=$1 AND id<>$2`, customer, installment)
	f.exec(t, `UPDATE installments SET installment_order=2 WHERE id=$1`, installment)
	f.exec(t, `UPDATE contracts SET requested_amount_minor=200,approved_financed_amount_minor=200,remaining_principal_amount_minor=200,installment_count=3 WHERE customer_id=$1`, customer)
	f.exec(t, `UPDATE credit_accounts SET limit_amount_minor=1000,exposure_amount_minor=200 WHERE customer_id=$1`, customer)

	attemptID, contract := f.insertExpiredSessionCreationAttempt(t, installment, 1, nil)
	intentID := "fc-" + uuid.NewString()

	client := authoritativePaymentClient{
		status: fc.GetPaymentStatusResult{
			PaymentIntentID: intentID,
			Status:          fc.SucceededPaymentStatus,
			AmountMinor:     100,
			Currency:        "IRR",
			CompletedAt:     paidAt,
		},
	}
	resultUC := f.newPaymentResultUC(t, client, nil, nil)
	reconUC := f.newReconciliationResultUC(t, resultUC)

	pending := reconciliationuc.ProcessInput{
		EventID:       uuid.NewString(),
		EventType:     reconciliationevent.ReconciliationResultEventType,
		EventVersion:  "1",
		SourceService: reconciliationevent.FinancialCoreSourceService,
		RawPayload:    json.RawMessage(`{"pending":"intent_only"}`),
		Data: reconciliationevent.ResultEventData{
			RequestEventID:           reconciliationevent.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:     attemptID,
			ReconciliationGeneration: 1,
			Status:                   reconciliationevent.PendingResultStatus,
			FinancialPaymentIntentID: &intentID,
			AmountMinor:              100,
			Currency:                 "IRR",
			OccurredAt:               f.now,
		},
	}

	pendingGot, err := reconUC.Process(f.ctx, pending)
	require.NoError(t, err)
	require.Equal(t, reconciliationuc.ProcessResultStillReconciliationRequired, pendingGot.Result)
	require.Equal(t, 1, f.scalar(t, `
SELECT count(*) FROM payment_attempts
WHERE id=$1
  AND state='expired'
  AND financial_outcome_resolved=false
  AND financial_payment_intent_id=$2
  AND next_reconciliation_at IS NULL
`, attemptID, intentID))

	input := financialcorepaymentresultuc.ProcessInput{
		EventID:         uuid.NewString(),
		EventType:       "financial_core.payment_result.v1",
		EventVersion:    "1",
		SourceService:   "financial-core",
		RawPayload:      json.RawMessage(`{"late":true}`),
		PaymentPurpose:  model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ContractID:      contract,
		InstallmentID:   model.InstallmentID(installment),
		PaymentIntentID: intentID,
		PaymentStatus:   model.SucceededFinancialPaymentResultStatus,
		EventOccurredAt: f.now,
		SourceType:      "bnpl_payment_attempt",
		SourceID:        attemptID.String(),
	}

	successGot, err := resultUC.Process(f.ctx, input)
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, successGot.Result)

	afterApply := f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID)
	require.Equal(t, "succeeded", afterApply.AttemptState)
	require.True(t, afterApply.FinancialOutcomeResolved)
	require.Equal(t, "paid", afterApply.InstallmentStatus)

	replaySame, err := resultUC.Process(f.ctx, input)
	require.NoError(t, err)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultEventAlreadyProcessed, replaySame.Result)
	assertRepaymentFinancialEffectsUnchanged(t, afterApply, f.captureRepaymentFinancialSnapshot(t, customer, installment, attemptID, input.EventID))
}

func strPtr(v string) *string { return &v }
