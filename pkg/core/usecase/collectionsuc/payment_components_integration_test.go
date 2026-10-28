//go:build integration && development

package collectionsuc

import (
	"context"
	"fmt"
	"testing"
	"time"

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
	result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 130)
	f.exec(t, `UPDATE payment_attempts SET amount_minor=130 WHERE installment_id=$1`, installment)
	for _, c := range []struct {
		kind   string
		amount int
		effect string
	}{
		{"bnpl_fee", 20, "debit"}, {"late_fee", 15, "debit"}, {"tax", 5, "debit"}, {"discount", 7, "credit"}, {"waiver", 3, "credit"},
	} {
		id := uuid.New()
		f.exec(t, `INSERT INTO installment_charges(id,installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,$2,$3,'applied',$4,'IRR','customer','platform','test','v1',$5,'{}')`, id, installment, c.kind, c.amount, f.now)
		f.exec(t, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) SELECT id,'charge',$2,$3,$4 FROM payment_attempts WHERE installment_id=$1`, installment, id, c.amount, c.effect)
	}
	got, e := result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, got.Result)
	require.Equal(t, 5, f.scalar(t, `SELECT count(*) FROM installment_charges WHERE installment_id=$1 AND charge_status='paid'`, installment))
	require.Equal(t, 130, f.scalar(t, `SELECT paid_amount_minor FROM installments WHERE id=$1`, installment))
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	input.EventID = uuid.NewString()
	_, e = result.Process(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, 100, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
	require.Equal(t, 5, f.scalar(t, `SELECT count(*) FROM payment_attempt_items i JOIN payment_attempts p ON p.id=i.payment_attempt_id WHERE p.installment_id=$1 AND i.item_type='charge'`, installment))
}

func TestPostgreSQLRepayment_HistoricalChargeOnlyDoesNotReleasePrincipalAgain(t *testing.T) {
	for _, completed := range []bool{false, true} {
		t.Run(fmt.Sprint("completed=", completed), func(t *testing.T) {
			f := newCollectionsFixture(t)
			customer, installment := f.seed(t)
			result, input := f.settlement(t, customer, installment, f.now.Add(-time.Minute), 20)
			originalPaidAt := f.now.Add(-24 * time.Hour)
			f.exec(t, `UPDATE installments SET status='paid',paid_at=$2,paid_amount_minor=100 WHERE id=$1`, installment, originalPaidAt)
			if completed {
				f.exec(t, `UPDATE contracts SET status='completed',completed_at=$2,remaining_principal_amount_minor=0 WHERE customer_id=$1`, customer, f.now)
				f.exec(t, `UPDATE credit_accounts SET exposure_amount_minor=0 WHERE customer_id=$1`, customer)
			}
			id := uuid.New()
			f.exec(t, `INSERT INTO installment_charges(id,installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,$2,'late_fee','applied',20,'IRR','customer','platform','test','v1',$3,'{}')`, id, installment, f.now)
			// Fixture represents a new immutable residual-charge attempt, not a rewrite of
			// an accepted principal payment. Production command builds this same snapshot.
			f.exec(t, `UPDATE payment_attempts SET amount_minor=20 WHERE installment_id=$1`, installment)
			f.exec(t, `UPDATE payment_attempt_items SET item_type='charge',source_id=$2,amount_minor=20 WHERE payment_attempt_id IN (SELECT id FROM payment_attempts WHERE installment_id=$1)`, installment, id)
			_, e := f.uc.AssessCustomer(f.ctx, customer)
			require.NoError(t, e)
			got, e := result.Process(f.ctx, input)
			require.NoError(t, e)
			require.Equal(t, financialcorepaymentresultuc.ProcessResultPaymentApplied, got.Result)
			require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installment_charges WHERE id=$1 AND charge_status='paid'`, id))
			require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM installments WHERE id=$1 AND paid_at=$2 AND paid_amount_minor=100`, installment, originalPaidAt))
			want := 200
			if completed {
				want = 0
			}
			require.Equal(t, want, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
			require.Zero(t, f.scalar(t, `SELECT count(*) FROM customer_restrictions WHERE customer_id=$1 AND status='active'`, customer))
			input.EventID = uuid.NewString()
			_, e = result.Process(f.ctx, input)
			require.NoError(t, e)
			require.Equal(t, want, f.scalar(t, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, customer))
		})
	}
}

func TestPostgreSQLRepayment_CreateHistoricalChargeOnlyAttempt(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	f.exec(t, `UPDATE installments SET status='paid',paid_at=$2,paid_amount_minor=100 WHERE id=$1`, installment, f.now.Add(-time.Hour))
	f.exec(t, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',20,'IRR','customer','platform','test','v1',$2,'{}')`, installment, f.now)
	f.exec(t, `UPDATE contracts SET financial_merchant_entitlement_id='entitlement-1' WHERE customer_id=$1`, customer)
	var contract model.ContractID
	var external model.ExternalCustomerID
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT contract_id FROM installments WHERE id=$1`, installment).Scan(&contract))
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT external_customer_id FROM customer_profiles WHERE id=$1`, customer).Scan(&external))
	uc, e := installmentrepaymentuc.New(f.pool, customerprofilerp.New(), merchantprofilerp.New(), contractrp.New(), installmentrp.New(), installmentchargerp.New(), paymentattemptrp.New(), outboxrp.New(), aggregateCheckoutClient{t: t, now: f.now}, f.clk, lognoop.New(), "bnpl.audit.events.v1", integrationIDs{}, "https://pay.example.com/return")
	require.NoError(t, e)
	input := installmentrepaymentuc.CreatePaymentAttemptInput{TenantID: model.TenantID(uuid.New()), ContextID: model.ContextID(uuid.New()), ContractID: contract, InstallmentID: model.InstallmentID(installment), ExternalCustomerID: external, IdempotencyKey: uuid.NewString(), Channel: model.GatewayPaymentChannel}
	ctx := trace.WithTraceID(trace.WithCorrelationID(f.ctx, uuid.NewString()), uuid.NewString())
	got, e := uc.CreatePaymentAttempt(ctx, input)
	require.NoError(t, e)
	require.EqualValues(t, 20, got.PaymentAttempt.AmountMinor)
	require.True(t, model.IsChargeOnlyPayment(got.PaymentAttempt.Items))
	again, e := uc.CreatePaymentAttempt(ctx, input)
	require.NoError(t, e)
	require.Equal(t, got.PaymentAttempt.ID, again.PaymentAttempt.ID)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM payment_attempts WHERE installment_id=$1`, installment))
}

// Only the external monetary port is faked; assert the generic FC contract
// receives the aggregate, with no BNPL fee-specific operation.
type aggregateCheckoutClient struct {
	fc.Client
	t   *testing.T
	now time.Time
}

func (c aggregateCheckoutClient) PrepareCollectionObligation(ctx context.Context, in fc.PrepareCollectionObligationInput) (fc.PrepareCollectionObligationResult, error) {
	require.EqualValues(c.t, 20, in.AmountMinor)
	require.Equal(c.t, "IRR", in.Currency)
	return fc.PrepareCollectionObligationResult{CollectionObligationID: "residual-collection", OutstandingAmountMinor: 20, Currency: "IRR"}, nil
}
func (c aggregateCheckoutClient) CreateCheckoutPayment(ctx context.Context, in fc.CreateCheckoutPaymentInput) (fc.CreateCheckoutPaymentResult, error) {
	require.EqualValues(c.t, 20, in.AmountMinor)
	require.Equal(c.t, "residual-collection", in.CollectionObligationID)
	return fc.CreateCheckoutPaymentResult{InvoiceID: "invoice-1", PaymentIntentID: "intent-1", PaymentAttemptID: "provider-attempt-1", PaymentSessionID: "session-1", Status: fc.RequiresActionPaymentStatus, AmountMinor: 20, Currency: "IRR", PaymentLink: "https://pay.example.com/20", ExpiresAt: c.now.Add(15 * time.Minute)}, nil
}

func TestPostgreSQLRepaymentReminder_HistoricalChargeDebtRemainsVisible(t *testing.T) {
	f := newCollectionsFixture(t)
	customer, installment := f.seed(t)
	f.exec(t, `UPDATE installments SET status='paid',paid_amount_minor=100,paid_at=$2 WHERE customer_id=$1`, customer, f.now.Add(-time.Hour))
	f.exec(t, `UPDATE contracts SET status='completed',completed_at=$2,remaining_principal_amount_minor=0 WHERE customer_id=$1`, customer, f.now)
	f.exec(t, `INSERT INTO installment_charges(installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,'late_fee','applied',20,'IRR','customer','platform','test','v1',$2,'{}')`, installment, f.now)
	opened, e := f.uc.AssessCustomer(f.ctx, customer)
	require.NoError(t, e)
	require.Equal(t, AssessTransitionDefaulted, opened.Transition)
	got, e := f.reminders(t, f.now.Add(time.Hour), outboxrp.New()).Dispatch(f.ctx)
	require.NoError(t, e)
	require.Equal(t, 1, got.CreatedCount)
	require.Equal(t, 1, f.scalar(t, `SELECT count(*) FROM repayment_reminders WHERE customer_id=$1 AND reminder_type='collections_default'`, customer))
}
