package contractuc

import (
	"context"
	"errors"
	"testing"
	"time"

		lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/policy"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type submitPreparedFulfillmentDeps struct {
	pool              *mock.Pool
	conn              *mock.Conn
	decision          *mock.Decision
	decisionCQ        *mock.DecisionConnQueryer
	contract          *mock.Contract
	contractCQ        *mock.ContractConnQueryer
	customerProfile   *mock.CustomerProfile
	customerProfileCQ *mock.CustomerProfileConnQueryer
	checkoutPayment   *mock.CheckoutPaymentClient
	paymentAttempt    *mock.PaymentAttempt
	clock             adapterclock.Fixed
}

func newSubmitPreparedFulfillmentDeps(t *testing.T, now time.Time) submitPreparedFulfillmentDeps {
	t.Helper()

	return submitPreparedFulfillmentDeps{
		pool:              mock.NewPool(t),
		conn:              mock.NewConn(t),
		decision:          mock.NewDecision(t),
		decisionCQ:        mock.NewDecisionConnQueryer(t),
		contract:          mock.NewContract(t),
		contractCQ:        mock.NewContractConnQueryer(t),
		customerProfile:   mock.NewCustomerProfile(t),
		customerProfileCQ: mock.NewCustomerProfileConnQueryer(t),
		checkoutPayment:   mock.NewCheckoutPaymentClient(t),
		paymentAttempt:    mock.NewPaymentAttempt(t),
		clock:             adapterclock.NewFixed(now, time.UTC),
	}
}

func (d submitPreparedFulfillmentDeps) useCase(t *testing.T) *UseCase {
	t.Helper()

	cancellationClient := mock.NewContractCancellationClient(t)
	sagaRepo, sagaExecutor, sagaDefinition := mustTestCancellationSagaDeps(
		t,
		d.pool,
		d.contract,
		mock.NewCreditAccount(t),
		cancellationClient,
		d.clock,
	)

	uc, err := New(
		d.pool,
		d.customerProfile,
		mock.NewMerchantProfile(t),
		mock.NewCreditAccount(t),
		d.decision,
		d.contract,
		mock.NewCheckoutDecisionAllocation(t),
		mustTestCheckoutRegistry(t),
		mock.NewInstallment(t),
		mock.NewInstallmentCharge(t),
		d.paymentAttempt,
		d.checkoutPayment,
		cancellationClient,
		mock.NewOutbox(t),
		d.clock,
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		"https://bnpl.example/payments/return",
		5*time.Minute,
		100,
		testCancellationSagaSettings(),
		sagaRepo,
		sagaExecutor,
		sagaDefinition,
		mustTestOriginationJurisdictions(t),
		lognoop.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	return uc
}

func (d submitPreparedFulfillmentDeps) expectConn(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
}

func (d submitPreparedFulfillmentDeps) expectDecisionPolicy(
	ctx context.Context,
	decisionID model.DecisionID,
	policyRef policy.Ref,
) {
	d.expectConn(ctx)
	d.decision.EXPECT().Conn(d.conn).Return(d.decisionCQ).Once()
	d.decisionCQ.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{
		ID:            decisionID,
		PolicyID:      policyRef.ID,
		PolicyVersion: policyRef.Version,
	}, nil).Once()
}

func matchedCheckoutPaymentSession(t *testing.T) (model.Contract, model.PaymentAttempt) {
	t.Helper()

	now := time.Date(
		2026,
		10,
		13,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	paymentIntentID := "fc-intent-v1"
	paymentSessionID := "fc-session-v1"
	paymentAttemptFCID := "fc-attempt-v1"
	invoiceID := "fc-invoice-v1"
	paymentLink := "https://financial.example/pay/v1"
	expiresAt := now.Add(30 * time.Minute)

	contractRow := model.Contract{
		ID:                        contractID,
		DecisionID:                decisionID,
		Status:                    model.PaymentPendingContractStatus,
		Currency:                  model.IRRCurrency,
		PayableNowAmountMinor:     40,
		ExpiresAt:                 expiresAt,
		FinancialInvoiceID:        &invoiceID,
		FinancialPaymentIntentID:  &paymentIntentID,
		FinancialPaymentAttemptID: &paymentAttemptFCID,
		FinancialPaymentSessionID: &paymentSessionID,
		FinancialPaymentLink:      &paymentLink,
		FinancialPaymentExpiresAt: &expiresAt,
	}
	attempt := model.PaymentAttempt{
		ID:                        model.PaymentAttemptID(uuid.New()),
		ContractID:                contractID,
		Purpose:                   model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                     model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID:  &paymentIntentID,
		FinancialPaymentAttemptID: &paymentAttemptFCID,
		FinancialPaymentSessionID: &paymentSessionID,
		PaymentURL:                &paymentLink,
		FinancialPaymentExpiresAt: &expiresAt,
	}

	return contractRow, attempt
}

func TestSubmitPreparedCheckoutFulfillment_V1PolicyUsesPaymentSessionPath(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newSubmitPreparedFulfillmentDeps(t, time.Date(
		2026,
		10,
		13,
		12,
		0,
		0,
		0,
		time.UTC,
	))
	uc := deps.useCase(t)

	contractRow, attempt := matchedCheckoutPaymentSession(t)
	deps.expectDecisionPolicy(ctx, contractRow.DecisionID, checkoutcatalog.DefaultRef())

	output, err := uc.SubmitPreparedCheckoutFulfillment(ctx, CreateContractOutput{
		Result:          CreateContractResultCreated,
		Contract:        contractRow,
		PaymentRequired: true,
		PaymentAttempt:  attempt,
	})
	require.NoError(t, err)
	require.True(t, output.PaymentRequired)
	require.Equal(t, paymentLinkFromContract(contractRow), output.Payment.PaymentLink)
	require.Equal(t, attempt.ID, output.PaymentAttempt.ID)
}

func paymentLinkFromContract(contractRow model.Contract) string {
	if contractRow.FinancialPaymentLink == nil {
		return ""
	}

	return *contractRow.FinancialPaymentLink
}

func TestSubmitPreparedCheckoutFulfillment_V2PolicyMerchantFundingWithoutPaymentAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		13,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newSubmitPreparedFulfillmentDeps(t, now)
	uc := deps.useCase(t)

	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	entitlementID := "fc-entitlement-v2"
	receivableID := "fc-receivable-v2"

	contractRow := model.Contract{
		ID:                             contractID,
		DecisionID:                     decisionID,
		CustomerID:                     customerID,
		Status:                         model.ActiveContractStatus,
		FinancialMerchantEntitlementID: &entitlementID,
		FinancialReceivableID:          &receivableID,
	}

	deps.expectDecisionPolicy(ctx, decisionID, checkoutcatalog.TrustedRef())

	deps.expectConn(ctx)
	deps.contract.EXPECT().Conn(deps.conn).Return(deps.contractCQ).Once()
	deps.contractCQ.EXPECT().GetByID(ctx, contractID).Return(contractRow, nil).Once()

	deps.expectConn(ctx)
	deps.contract.EXPECT().Conn(deps.conn).Return(deps.contractCQ).Once()
	deps.contractCQ.EXPECT().GetByID(ctx, contractID).Return(contractRow, nil).Once()

	output, err := uc.SubmitPreparedCheckoutFulfillment(ctx, CreateContractOutput{
		Result:          CreateContractResultAlreadyExists,
		Contract:        contractRow,
		PaymentRequired: false,
	})
	require.NoError(t, err)
	require.False(t, output.PaymentRequired)
	require.True(t, output.PaymentAttempt.ID.IsZero())
	require.Empty(t, output.Payment.PaymentLink)
}

func TestSubmitPreparedCheckoutFulfillment_V1PolicyRejectsPaymentRequiredFalse(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newSubmitPreparedFulfillmentDeps(t, time.Date(
		2026,
		10,
		13,
		12,
		0,
		0,
		0,
		time.UTC,
	))
	uc := deps.useCase(t)

	contractRow, attempt := matchedCheckoutPaymentSession(t)
	deps.expectDecisionPolicy(ctx, contractRow.DecisionID, checkoutcatalog.DefaultRef())

	_, err := uc.SubmitPreparedCheckoutFulfillment(ctx, CreateContractOutput{
		Result:          CreateContractResultCreated,
		Contract:        contractRow,
		PaymentRequired: false,
		PaymentAttempt:  attempt,
	})
	requireCerrCode(t, err, cerr.CodeInternalError)
	require.Contains(t, err.Error(), "prepared.PaymentRequired=false disagrees")
}

func TestSubmitPreparedCheckoutFulfillment_V2PolicyRejectsPaymentRequiredTrue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := newSubmitPreparedFulfillmentDeps(t, time.Date(
		2026,
		10,
		13,
		12,
		0,
		0,
		0,
		time.UTC,
	))
	uc := deps.useCase(t)

	decisionID := model.DecisionID(uuid.New())
	contractRow := model.Contract{
		ID:         model.ContractID(uuid.New()),
		DecisionID: decisionID,
		Status:     model.PaymentPendingContractStatus,
	}
	deps.expectDecisionPolicy(ctx, decisionID, checkoutcatalog.TrustedRef())

	_, err := uc.SubmitPreparedCheckoutFulfillment(ctx, CreateContractOutput{
		Result:          CreateContractResultCreated,
		Contract:        contractRow,
		PaymentRequired: true,
		PaymentAttempt: model.PaymentAttempt{
			ID:         model.PaymentAttemptID(uuid.New()),
			ContractID: contractRow.ID,
			Purpose:    model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		},
	})
	requireCerrCode(t, err, cerr.CodeInternalError)
	require.Contains(t, err.Error(), "prepared.PaymentRequired=true disagrees")
}

func requireCerrCode(t *testing.T, err error, code cerr.Code) {
	t.Helper()

	require.Error(t, err)

	var appErr *cerr.Error
	require.True(
		t,
		errors.As(err, &appErr),
		"expected *cerr.Error, got %T: %v",
		err,
		err,
	)
	require.Equal(t, code, appErr.Code)
}
