//go:build development && integration

package contractuc

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/checkoutdecisionallocationrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/contractrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/creditaccountrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/customerprofilerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/decisionrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentchargerp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/installmentrp"
	"github.com/parstis/bnpl-backend/pkg/adapter/db/postgres/paymentattemptrp"
	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	"github.com/parstis/bnpl-backend/pkg/core/policy/checkout"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/stretchr/testify/require"
)

type activationFixture struct {
	ctx                             context.Context
	pool                            *postgres.Pool
	schema                          string
	customer, contract, installment uuid.UUID
	decision                        uuid.UUID
}

func newActivationFixture(t *testing.T, install bool) *activationFixture {
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
	f := &activationFixture{ctx: ctx, pool: p, schema: schema, customer: uuid.New(), contract: uuid.New(), installment: uuid.New(), decision: uuid.New()}
	f.exec(t, "CREATE SCHEMA "+schema)
	ddl, e := os.ReadFile("../../../adapter/db/postgres/migrationsrp/000_schema.sql")
	require.NoError(t, e)
	// The baseline installs all financial guards before the first business row.
	// Owner-only grants are tested separately under the real managed roles.
	baseline := strings.SplitN(string(ddl), "-- MANAGED DATABASE PERMISSIONS: canonical initial grants (pre-release)", 2)[0]
	baseline = strings.ReplaceAll(baseline, "SET search_path TO bnpl;", "SET search_path TO "+schema+";")
	baseline = strings.ReplaceAll(baseline, "bnpl.", schema+".")
	baseline = strings.ReplaceAll(baseline, "pg_catalog, bnpl", "pg_catalog, "+schema)
	f.exec(t, baseline)
	if !install {
		for _, table := range []string{"installments", "installment_charges", "payment_attempts", "payment_attempt_items"} {
			f.exec(t, "ALTER TABLE "+pgx.Identifier{schema, table}.Sanitize()+" DISABLE TRIGGER USER")
		}
	}
	merchant, account := uuid.New(), uuid.New()
	f.exec(t, `INSERT INTO customer_profiles(id,external_customer_id,jurisdiction,status) VALUES($1,$2,'IR','active')`, f.customer, uuid.New())
	f.exec(t, `INSERT INTO merchant_profiles(id,external_merchant_id,display_name,status,risk_tier,max_tenure_months,jurisdiction) VALUES($1,$2,'test','active','low',12,'IR')`, merchant, uuid.New())
	// Checkout Policy identity is required: activation resolves Reminder/schedule
	// terms through the decision policy ref.
	f.exec(t, `INSERT INTO decisions(id,type,target_type,target_id,outcome,reason_codes,policy_id,policy_version,jurisdiction,actor_type,decided_at,idempotency_key,idempotency_fingerprint) VALUES($1,'credit_limit','customer_profile',$2,'approved',ARRAY['CHECKOUT_APPROVED'],'checkout_contract_approval','atomic_it','IR','system',now(),$3,$4)`, f.decision, f.customer, uuid.NewString(), "sha256:"+strings.Repeat("0", 64))
	f.exec(t, `INSERT INTO credit_accounts(id,customer_id,currency,status,effective_from) VALUES($1,$2,'IRR','active',now())`, account, f.customer)
	f.exec(t, `INSERT INTO contracts(id,decision_id,checkout_key,customer_id,credit_account_id,merchant_profile_id,currency,requested_amount_minor,approved_financed_amount_minor,installment_count,payable_now_amount_minor,remaining_principal_amount_minor,status,expires_at,activated_at) VALUES($1,$2,$3,$4,$5,$6,'IRR',100,100,2,0,100,'active',now(),now())`, f.contract, f.decision, uuid.NewString(), f.customer, account, merchant)
	f.exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) VALUES($1,$2,$3,$4,$5,2,'IRR',100,100,now(),2,'scheduled')`, f.installment, f.decision, f.contract, f.customer, merchant)
	return f
}

func (f *activationFixture) exec(t *testing.T, sql string, args ...any) {
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

func (f *activationFixture) charge(t *testing.T, kind, payer string) uuid.UUID {
	t.Helper()
	id := uuid.New()
	f.exec(t, `INSERT INTO installment_charges(id,installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,$2,$3,'applied',10,'IRR',$4,'platform','test','v1',now(),'{}')`, id, f.installment, kind, payer)
	return id
}

// atomicActivationCheckoutPolicy is a fixture-local Checkout Policy that matches
// the two-installment economics of TestPostgreSQLFirstInstallment_AtomicActivationWithFeesAndDiscount
// without depending on production installment counts (v1=4, v2=3).
type atomicActivationCheckoutPolicy struct{}

func (atomicActivationCheckoutPolicy) Ref() policy.Ref {
	return policy.Ref{ID: "checkout_contract_approval", Version: "atomic_it"}
}

func (atomicActivationCheckoutPolicy) Terms() checkout.Terms {
	return checkout.Terms{
		InstallmentCount:              2,
		DecisionTTL:                   15 * time.Minute,
		MaxFinancedAmountMinor:        2_000_000_000,
		FinancingMode:                 checkout.PartialWithUpfrontFinancing,
		ActivationMode:                checkout.ActivationAfterInitialPayment,
		ScheduleAnchor:                checkout.ScheduleFromContractActivation,
		FirstInstallmentPaymentWindow: 15 * time.Minute,
		Repayment: checkout.RepaymentTerms{
			Cadence: checkout.RepaymentCadence{
				Unit:  checkout.CadenceCalendarDay,
				Every: 30,
			},
			GracePeriodDays: 5,
			Reminder: checkout.ReminderTerms{
				LeadDays: 3,
			},
		},
	}
}

func (atomicActivationCheckoutPolicy) EvaluateEligibility(checkout.EligibilityInput) checkout.EligibilityResult {
	return checkout.EligibilityResult{Outcome: model.ApprovedDecisionOutcome}
}

func (atomicActivationCheckoutPolicy) EvaluateFinancing(checkout.FinancingInput) checkout.FinancingResult {
	return checkout.FinancingResult{Outcome: model.ApprovedDecisionOutcome}
}

func (p atomicActivationCheckoutPolicy) BuildInstallmentSchedule(input checkout.ScheduleInput) (checkout.ScheduleResult, error) {
	// Preserve fixture economics: #1 principal 100 + upfront 10 + fee baked into
	// total 130; #2 principal/total 100. Activation finalization compares these
	// fields exactly before rewriting recurring dues.
	firstDue := input.Now
	secondDue := input.Now.AddDate(0, 1, 0)
	return checkout.ScheduleResult{
		Installments: []checkout.InstallmentDraft{
			{
				Order:                1,
				PrincipalAmountMinor: 100,
				UpfrontAmountMinor:   10,
				BaseFeeAmountMinor:   20,
				TotalAmountMinor:     130,
				DueAt:                firstDue,
			},
			{
				Order:                2,
				PrincipalAmountMinor: 100,
				UpfrontAmountMinor:   0,
				BaseFeeAmountMinor:   0,
				TotalAmountMinor:     100,
				DueAt:                secondDue,
			},
		},
	}, nil
}

func (p atomicActivationCheckoutPolicy) BuildContractPlan(input checkout.ContractPlanInput) (checkout.ContractPlan, error) {
	return checkout.ContractPlan{
		ActivationMode:                   p.Terms().ActivationMode,
		ScheduleAnchor:                   p.Terms().ScheduleAnchor,
		InitialPaymentRequired:           true,
		InitialPaymentAmountMinor:        input.FirstInstallmentTotalAmountMinor,
		ActiveExposureAmountMinor:        input.FinancedAmountMinor - input.FirstInstallmentPrincipalAmountMinor,
		CollectedCashAmountMinor:         input.FirstInstallmentTotalAmountMinor,
		ReceivableAmountMinor:            input.FinancedAmountMinor - input.FirstInstallmentPrincipalAmountMinor,
		FirstInstallmentPaidOnActivation: true,
	}, nil
}

func TestPostgreSQLFirstInstallment_AtomicActivationWithFeesAndDiscount(t *testing.T) {
	f := newActivationFixture(t, true)
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	f.exec(t, `UPDATE contracts SET status='payment_pending',activated_at=NULL,requested_amount_minor=210,approved_financed_amount_minor=200,remaining_principal_amount_minor=200,upfront_amount_minor=10,payable_now_amount_minor=115,expires_at=$2 WHERE id=$1`, f.contract, now.Add(time.Hour))
	f.exec(t, `UPDATE installments SET status='offered',installment_order=1,principal_amount_minor=100,upfront_amount_minor=10,base_fee_amount_minor=20,total_amount_minor=130,due_at=$2 WHERE id=$1`, f.installment, now)
	f.exec(t, `INSERT INTO installments(id,decision_id,contract_id,customer_id,merchant_profile_id,installment_order,currency,principal_amount_minor,total_amount_minor,due_at,grace_period_days,status) SELECT $2,decision_id,contract_id,customer_id,merchant_profile_id,2,'IRR',100,100,$3,2,'offered' FROM installments WHERE id=$1`, f.installment, uuid.New(), now.AddDate(0, 1, 0))
	f.exec(t, `INSERT INTO checkout_decision_allocations(decision_id,checkout_key,customer_id,merchant_profile_id,purchase_channel,purchase_source_id,requested_amount_minor,approved_financed_amount_minor,upfront_amount_minor,available_credit_minor,currency,installment_count,expires_at) SELECT decision_id,checkout_key,customer_id,merchant_profile_id,'pos',$2,210,200,10,1000,'IRR',2,$3 FROM contracts WHERE id=$1`, f.contract, uuid.New(), now.Add(time.Hour))
	fee, discount := uuid.New(), uuid.New()
	for _, c := range []struct {
		id     uuid.UUID
		kind   string
		amount int
	}{{fee, "bnpl_fee", 20}, {discount, "discount", 15}} {
		f.exec(t, `INSERT INTO installment_charges(id,installment_id,charge_type,charge_status,amount_minor,currency,payer_type,recipient_type,policy_id,policy_version,assessed_at,metadata) VALUES($1,$2,$3,'offered',$4,'IRR','customer','platform','test','v1',$5,'{}')`, c.id, f.installment, c.kind, c.amount, now)
	}
	attempt := uuid.New()
	tx, e := f.pool.Pool.Begin(f.ctx)
	require.NoError(t, e)
	defer tx.Rollback(f.ctx)
	_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempts(id,contract_id,installment_id,amount_minor,currency,purpose,state,channel,idempotency_key,request_fingerprint,financial_payment_intent_id,financial_payment_session_id,submitted_at,success_return_url,failure_return_url,cancel_return_url) VALUES($1,$2,$3,115,'IRR','checkout_first_installment','submitted','gateway',$4,$5,'intent-first','session-first',$6,'https://pay.example/ok','https://pay.example/fail','https://pay.example/cancel')`, attempt, f.contract, f.installment, uuid.NewString(), "sha256:"+strings.Repeat("0", 64), now)
	require.NoError(t, e)
	for _, item := range []struct {
		kind   string
		id     uuid.UUID
		amount int
		effect string
	}{{"principal", f.installment, 100, "debit"}, {"upfront", f.installment, 10, "debit"}, {"charge", fee, 20, "debit"}, {"charge", discount, 15, "credit"}} {
		_, e = tx.Exec(f.ctx, `INSERT INTO payment_attempt_items(payment_attempt_id,item_type,source_id,amount_minor,effect) VALUES($1,$2,$3,$4,$5)`, attempt, item.kind, item.id, item.amount, item.effect)
		require.NoError(t, e)
	}
	require.NoError(t, tx.Commit(f.ctx))

	checkoutPolicies, err := checkout.NewRegistry(atomicActivationCheckoutPolicy{})
	require.NoError(t, err)

	uc := &UseCase{
		pool:                       f.pool,
		customerProfile:            customerprofilerp.New(),
		decision:                   decisionrp.New(),
		contract:                   contractrp.New(),
		checkoutDecisionAllocation: checkoutdecisionallocationrp.New(),
		checkoutPolicies:           checkoutPolicies,
		installment:                installmentrp.New(),
		installmentCharge:          installmentchargerp.New(),
		creditAccount:              creditaccountrp.New(),
		paymentAttempt:             paymentattemptrp.New(),
		clock:                      adapterclock.NewFixed(now, time.UTC),
	}
	input := ActivateFromFinancialCorePaymentResultInput{ContractID: model.ContractID(f.contract), InvoiceID: "invoice-first", PaymentIntentID: "intent-first", PaymentSessionID: "session-first", PaidAmountMinor: 115, Currency: model.IRRCurrency, PaidAt: now, EventID: uuid.NewString(), EventOccurredAt: now, PaymentStatus: model.SucceededFinancialPaymentResultStatus}
	out, e := uc.ActivateFromFinancialCorePaymentResult(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, ActivateResultActivated, out.Result)
	var exposure int
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT exposure_amount_minor FROM credit_accounts WHERE customer_id=$1`, f.customer).Scan(&exposure))
	require.Equal(t, 100, exposure)
	var state string
	require.NoError(t, f.pool.Pool.QueryRow(f.ctx, `SELECT charge_status FROM installment_charges WHERE id=$1`, discount).Scan(&state))
	require.Equal(t, "waived", state)
	out, e = uc.ActivateFromFinancialCorePaymentResult(f.ctx, input)
	require.NoError(t, e)
	require.Equal(t, ActivateResultAlreadyActive, out.Result)
	require.NoError(t, f.pool.Conn(f.ctx, func(ctx context.Context, c repo.Conn) error {
		return c.ReadSnapshot(ctx, func(ctx context.Context, tx repo.Tx) error {
			row, e := creditaccountrp.New().Tx(tx).GetByCustomerIDAndCurrency(ctx, model.CustomerID(f.customer), model.IRRCurrency)
			require.NoError(t, e)
			require.EqualValues(t, 100, row.ExposureAmountMinor)
			return nil
		})
	}))
}
