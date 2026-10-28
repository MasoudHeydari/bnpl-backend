//go:build integration && development

package collectionsuc

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/creditaccountrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/inboxrp"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/checkoutsessionuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/contractuc"

	fc "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"

	"github.com/google/uuid"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/contractrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/merchantprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/outboxrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
	"github.com/parstis/bnpl-backend/pkg/core/usecase/installmentrepaymentuc"
	"github.com/stretchr/testify/require"
)

func TestPostgreSQLRepayment_AggregateSettlesAllComponents(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	for _, c := range []struct {
		kind   string
		amount int
		effect string
	}{
		{"bnpl_fee", 20, "debit"}, {"late_fee", 15, "debit"}, {"tax", 5, "debit"}, {"discount", 7, "credit"}, {"waiver", 3, "credit"},
	} {
		id := uuid.New()
		f.exec(t, `INSERT INTO installment_charges(id,installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,$2,$3,'applied',$4,'IRR','customer','platform','test','v1',$5,'{}')`, id, installment, c.kind, c.amount, f.now)

	}
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 130)
	got, e := result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, got.Result)
	require.Equal(t, 5, f.scalar(t, `SELECT count(*) FROM installment_charges WHERE installment_id=$1 AND charge_status IN ('paid','waived')`, installment))
	require.Equal(t, 130, f.scalar(t, `SELECT paid_amount_minor FROM installments WHERE id=$1`, installment))
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	input.EventID = uuid.NewString()
	_, e = result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	require.Equal(t, 5, f.scalar(t, `SELECT count(*) FROM payment_attempt_items i JOIN payment_attempts p ON p.id=i.payment_attempt_id WHERE p.installment_id=$1 AND i.item_type='charge'`, installment))
}

func TestPostgreSQLRepayment_RejectsSecondPaymentAfterPaid(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	f.settleFixture(t, installment, f.now.Add(-time.Hour))

	f.exec(t, `UPDATE contracts SET financial_merchant_entitlement_id='entitlement-1' WHERE customer_id=$1`, customer)
	var contract model.ContractID
	var external model.ExternalCustomerID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT external_customer_id FROM customer_profiles WHERE id=$1`, customer).Scan(&external))
	uc, e := installmentrepaymentuc.New(f.pool, customerprofilerp.New(), merchantprofilerp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), paymentattemptrp.New(), outboxrp.New(), aggregateCheckoutClient{t: t, now: f.now}, f.clk, lognoop.New(), "bnpl.audit.events.v1", integrationIDs{}, "https://pay.example.com/return")
	require.NoError(t, e)
	input := installmentrepaymentuc.CreatePaymentAttemptInput{TenantID: model.TenantID(uuid.New()), ContextID: model.ContextID(uuid.New()), ContractID: contract, InstallmentID: model.InstallmentID(installment), ExternalCustomerID: external, IdempotencyKey: uuid.NewString(), Channel: model.GatewayPaymentChannel}
	ctx := trace.WithTraceID(trace.WithCorrelationID(f.ctx, uuid.NewString()), uuid.NewString())
	_, e = uc.CreatePaymentAttempt(ctx, input)
	require.Error(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM payment_attempts WHERE installment_id=$1`, installment))
}

// Only the external monetary port is faked; assert the generic FC contract
// receives the aggregate, with no BNPL fee-specific operation.
type aggregateCheckoutClient struct {
	fc.Client
	t      *testing.T
	now    time.Time
	amount int64
}

func (c aggregateCheckoutClient) PrepareCollectionObligation(ctx context.Context, in fc.PrepareCollectionObligationInput) (fc.PrepareCollectionObligationResult, error) {
	want := c.amount
	if want == 0 {
		want = 20
	}
	require.EqualValues(c.t, want, in.AmountMinor)
	require.Equal(c.t, "IRR", in.Currency)
	return fc.PrepareCollectionObligationResult{CollectionObligationID: "collection-aggregate", OutstandingAmountMinor: want, Currency: "IRR"}, nil
}
func (c aggregateCheckoutClient) CreateCheckoutPayment(ctx context.Context, in fc.CreateCheckoutPaymentInput) (fc.CreateCheckoutPaymentResult, error) {
	want := c.amount
	if want == 0 {
		want = 20
	}
	require.EqualValues(c.t, want, in.AmountMinor)
	require.Equal(c.t, "collection-aggregate", in.CollectionObligationID)
	return fc.CreateCheckoutPaymentResult{InvoiceID: "invoice-1", PaymentIntentID: "intent-1", PaymentAttemptID: "provider-attempt-1", PaymentSessionID: "session-1", Status: fc.RequiresActionPaymentStatus, AmountMinor: want, Currency: "IRR", PaymentLink: "https://pay.example.com/20", ExpiresAt: c.now.Add(15 * time.Minute)}, nil
}

func TestPostgreSQLRepayment_FullCommandSettlementCompletionAndRecovery(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, first := f.seed(t)
	f.settleFixture(t, first, f.now.Add(-24*time.Hour))
	var target uuid.UUID
	var contract model.ContractID
	var external model.ExternalCustomerID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT id,contract_id FROM installments WHERE customer_id=$1 AND id<>$2`, customer, first).Scan(&target, &contract))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT external_customer_id FROM customer_profiles WHERE id=$1`, customer).Scan(&external))
	f.exec(t, `UPDATE installments SET due_at=$2,grace_period_days=0 WHERE id=$1`, target, f.now.Add(-48*time.Hour))
	f.exec(t, `UPDATE contracts SET requested_amount_minor=200,approved_financed_amount_minor=200,remaining_principal_amount_minor=100,financial_merchant_entitlement_id='entitlement-complete' WHERE id=$1`, contract)
	f.exec(t, `UPDATE credit_accounts SET limit_amount_minor=1000,exposure_amount_minor=100 WHERE customer_id=$1`, customer)
	f.exec(t, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',20,'IRR','customer','platform','test','v1',$2,'{}'),($1,'discount','applied',10,'IRR','customer','platform','test','v1',$2,'{}')`, target, f.now)
	command, e := installmentrepaymentuc.New(f.pool, customerprofilerp.New(), merchantprofilerp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), paymentattemptrp.New(), outboxrp.New(), aggregateCheckoutClient{t: t, now: f.now, amount: 110}, f.clk, lognoop.New(), "bnpl.audit.events.v1", integrationIDs{}, "https://pay.example.com/return")
	require.NoError(t, e)
	ctx := trace.WithTraceID(trace.WithCorrelationID(f.ctx, uuid.NewString()), uuid.NewString())
	request := installmentrepaymentuc.CreatePaymentAttemptInput{TenantID: model.TenantID(uuid.New()), ContextID: model.ContextID(uuid.New()), ContractID: contract, InstallmentID: model.InstallmentID(target), ExternalCustomerID: external, IdempotencyKey: uuid.NewString(), Channel: model.GatewayPaymentChannel}
	attempt, e := command.CreatePaymentAttempt(ctx, request)
	require.NoError(t, e)
	require.EqualValues(t, 110, attempt.PaymentAttempt.AmountMinor)
	replay, e := command.CreatePaymentAttempt(ctx, request)
	require.NoError(t, e)
	require.Equal(t, attempt.PaymentAttempt.ID, replay.PaymentAttempt.ID)
	_, e = f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	intent := *attempt.PaymentAttempt.FinancialPaymentIntentID
	client := authoritativePaymentClient{status: fc.GetPaymentStatusResult{PaymentIntentID: intent, Status: fc.SucceededPaymentStatus, AmountMinor: 110, Currency: "IRR", CompletedAt: f.now.Add(-time.Minute)}}
	result, e := financialcorepaymentresultuc.New(f.pool, &contractuc.UseCase{}, &checkoutsessionuc.UseCase{}, client, inboxrp.New(), outboxrp.New(), paymentattemptrp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), creditaccountrp.New(), integrationIDs{}, sha256fingerprint.New(), "fc.payment.applications", "bnpl.audit.events.v1", f.clk, 0, metricsnoop.New())
	require.NoError(t, e)
	result.WithCollectionsCoordination(customerprofilerp.New(), func(ctx context.Context, id model.CustomerID) error { _, e := f.uc.AssessCustomer(ctx, id); return e })
	event := financialcorepaymentresultuc.ProcessInput{EventID: uuid.NewString(), EventType: "financial_core.payment_result.v1", EventVersion: "1", SourceService: "financial-core", RawPayload: json.RawMessage(`{"settled":true}`), PaymentPurpose: model.InstallmentRepaymentFinancialCorePaymentPurpose, ContractID: contract, InstallmentID: model.InstallmentID(target), PaymentIntentID: intent, PaymentStatus: model.SucceededFinancialPaymentResultStatus, EventOccurredAt: f.now}
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := result.Process(f.ctx, event); failures <- e }()
	}
	wg.Wait()
	close(failures)
	for e := range failures {
		require.NoError(t, e)
	}
	require.Zero(t, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM credit_transactions WHERE reference_id=$1 AND type='exposure_decreased'`, target))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM contracts WHERE id=$1 AND status='completed' AND remaining_principal_amount_minor=0`, contract))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM default_episodes WHERE customer_id=$1 AND status='recovered'`, customer))
	require.Equal(t, 2, f.scalar(t, `SELECT count(*) FROM installment_charges WHERE installment_id=$1 AND charge_status IN ('paid','waived')`, target))
	request.IdempotencyKey = uuid.NewString()
	_, e = command.CreatePaymentAttempt(ctx, request)
	require.Error(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM payment_attempts WHERE installment_id=$1`, target))
}

func TestPostgreSQLRepayment_RollbackAfterFinancialApplicationThenRetry(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	f.exec(t, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',20,'IRR','customer','platform','test','v1',$2,'{}')`, installment, f.now)
	normal, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 120)
	client := authoritativePaymentClient{status: fc.GetPaymentStatusResult{PaymentIntentID: input.PaymentIntentID, Status: fc.SucceededPaymentStatus, AmountMinor: 120, Currency: "IRR", CompletedAt: f.now.Add(-time.Minute)}}
	broken, e := financialcorepaymentresultuc.New(f.pool, &contractuc.UseCase{}, &checkoutsessionuc.UseCase{}, client, inboxrp.New(), rejectingOutbox{outboxrp.New()}, paymentattemptrp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), creditaccountrp.New(), integrationIDs{}, sha256fingerprint.New(), "fc.payment.applications", "bnpl.audit.events.v1", f.clk, 0, metricsnoop.New())
	require.NoError(t, e)
	broken.WithCollectionsCoordination(customerprofilerp.New(), nil)
	_, e = broken.Process(f.ctx, input)
	require.Error(t, e)
	require.Zero(t, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status='paid'`, installment))
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installment_charges WHERE installment_id=$1 AND charge_status='applied'`, installment))
	require.Equal(t, 200, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	_, e = normal.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND status='paid'`, installment))
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
}

func TestPostgreSQLRepayment_ChargeMutationOverlapsActualResult(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 100)
	gate, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer gate.Rollback(f.ctx)
	_, e = gate.Exec(f.ctx, `SELECT id FROM customer_profiles WHERE id=$1 FOR UPDATE`, customer)
	require.NoError(t, e)
	done := make(chan error, 1)
	go func() { _, e := result.Process(f.ctx, input); done <- e }()
	require.Eventually(t, func() bool {
		var count int
		e := f.pool.Pool.QueryRow(f.ctx, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type='Lock' AND query LIKE '%customer_profiles%'`).Scan(&count)
		return e == nil && count > 0
	}, time.Second, 5*time.Millisecond)
	_, e = f.pool.Pool.Exec(f.ctx, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',1,'IRR','customer','platform','test','v1',now(),'{}')`, installment)
	require.Error(t, e)
	require.NoError(t, gate.Commit(f.ctx))
	require.NoError(t, <-done)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
}
