package contractuc

import (
	"context"
	"testing"
	"time"

	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

type checkoutSessionDeps struct {
	pool              *mock.Pool
	conn              *mock.Conn
	tx                *mock.Tx
	customerProfile   *mock.CustomerProfile
	customerProfileCQ *mock.CustomerProfileConnQueryer
	merchantProfile   *mock.MerchantProfile
	merchantProfileCQ *mock.MerchantProfileConnQueryer
	contract          *mock.Contract
	contractTx        *mock.ContractTxQueryer
	paymentAttempt    *mock.PaymentAttempt
	paymentAttemptTx  *mock.PaymentAttemptTxQueryer
	outbox            *mock.Outbox
	outboxTx          *mock.OutboxTxQueryer
	checkoutPayment   *mock.CheckoutPaymentClient
	idGenerator       *mock.IdentifierGenerator
	clock             adapterclock.Fixed
}

func newCheckoutSessionDeps(t *testing.T, now time.Time) checkoutSessionDeps {
	t.Helper()

	return checkoutSessionDeps{
		pool:              mock.NewPool(t),
		conn:              mock.NewConn(t),
		tx:                mock.NewTx(t),
		customerProfile:   mock.NewCustomerProfile(t),
		customerProfileCQ: mock.NewCustomerProfileConnQueryer(t),
		merchantProfile:   mock.NewMerchantProfile(t),
		merchantProfileCQ: mock.NewMerchantProfileConnQueryer(t),
		contract:          mock.NewContract(t),
		contractTx:        mock.NewContractTxQueryer(t),
		paymentAttempt:    mock.NewPaymentAttempt(t),
		paymentAttemptTx:  mock.NewPaymentAttemptTxQueryer(t),
		outbox:            mock.NewOutbox(t),
		outboxTx:          mock.NewOutboxTxQueryer(t),
		checkoutPayment:   mock.NewCheckoutPaymentClient(t),
		idGenerator:       mock.NewIdentifierGenerator(t),
		clock:             adapterclock.NewFixed(now, time.UTC),
	}
}

func (d checkoutSessionDeps) newUseCase(t *testing.T) *UseCase {
	t.Helper()

	cancellationClient := mock.NewContractCancellationClient(t)
	sagaRepo, sagaExecutor, sagaDefinition := mustTestCancellationSagaDeps(
		t, d.pool, d.contract, mock.NewCreditAccount(t), cancellationClient, d.clock,
	)

	uc, err := New(
		d.pool,
		d.customerProfile,
		d.merchantProfile,
		mock.NewCreditAccount(t),
		mock.NewDecision(t),
		d.contract,
		mock.NewCheckoutDecisionAllocation(t),
		mustTestCheckoutRegistry(t),
		mock.NewInstallment(t),
		mock.NewInstallmentCharge(t),
		d.paymentAttempt,
		d.checkoutPayment,
		cancellationClient,
		d.outbox,
		d.clock,
		"bnpl.audit.events.v1",
		d.idGenerator,
		"https://bnpl.example/payments/return",
		5*time.Minute,
		100,
		testCancellationSagaSettings(),
		sagaRepo,
		sagaExecutor,
		sagaDefinition,
		mustTestOriginationJurisdictions(t),
		metricsnoop.New(),
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d checkoutSessionDeps) expectConn(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
}

func (d checkoutSessionDeps) expectConnTx(ctx context.Context) {
	d.expectConn(ctx)
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
}

func TestEnsureCheckoutPaymentSession_RecoversFromReconciliationRequired(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-checkout-recovery")
	ctx = trace.WithCorrelationID(ctx, "corr-checkout-recovery")
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newCheckoutSessionDeps(t, now)
	uc := deps.newUseCase(t)

	contractID := model.ContractID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	identitySubjectID := model.IdentitySubjectID(uuid.New())
	merchantID := model.MerchantID(uuid.New())
	expiresAt := now.Add(30 * time.Minute)
	returnURL := "https://bnpl.example/payments/return"
	reason := model.PaymentSessionCreationTimeoutPaymentReconciliationReason
	obligationID := "fc-collection-obligation-123"

	contractRow := model.Contract{
		ID:                    contractID,
		CustomerID:            customerID,
		MerchantProfileID:     merchantProfileID,
		Status:                model.PaymentPendingContractStatus,
		Currency:              model.IRRCurrency,
		PayableNowAmountMinor: 1000,
		ExpiresAt:             expiresAt,
	}
	tenantID := uuid.NewString()
	contextID := uuid.NewString()
	attempt := model.PaymentAttempt{
		ID:                              model.PaymentAttemptID(uuid.New()),
		ContractID:                      contractID,
		Purpose:                         model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                           model.ReconciliationRequiredPaymentAttemptState,
		ReconciliationReason:            &reason,
		AmountMinor:                     1000,
		Currency:                        model.IRRCurrency,
		Channel:                         model.GatewayPaymentChannel,
		SuccessReturnURL:                &returnURL,
		FailureReturnURL:                &returnURL,
		CancelReturnURL:                 &returnURL,
		TenantID:                        &tenantID,
		ContextID:                       &contextID,
		FinancialCollectionObligationID: &obligationID,
		IdempotencyKey:                  "bnpl:contract:" + contractID.String() + ":checkout-payment",
	}

	paymentIntentID := "fc-payment-intent-123"
	paymentSessionID := "fc-payment-session-123"
	paymentAttemptID := "fc-payment-attempt-123"
	invoiceID := "fc-invoice-123"
	paymentLink := "https://financial.example/pay/123"

	deps.expectConn(ctx)
	deps.customerProfile.EXPECT().Conn(deps.conn).Return(deps.customerProfileCQ).Once()
	deps.customerProfileCQ.EXPECT().GetByID(ctx, customerID).Return(model.CustomerProfile{
		ID:                customerID,
		IdentitySubjectID: identitySubjectID,
	}, nil).Once()
	deps.merchantProfile.EXPECT().Conn(deps.conn).Return(deps.merchantProfileCQ).Once()
	deps.merchantProfileCQ.EXPECT().GetByID(ctx, merchantProfileID).Return(model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: merchantID,
	}, nil).Once()

	deps.checkoutPayment.EXPECT().CreateCheckoutPayment(ctx, testifymock.Anything).Return(
		extcheckoutpayment.CreateCheckoutPaymentResult{
			InvoiceID:        invoiceID,
			PaymentIntentID:  paymentIntentID,
			PaymentAttemptID: paymentAttemptID,
			PaymentSessionID: paymentSessionID,
			Status:           extcheckoutpayment.RequiresActionPaymentStatus,
			AmountMinor:      1000,
			Currency:         "IRR",
			PaymentLink:      paymentLink,
			ExpiresAt:        expiresAt,
		},
		nil,
	).Once()

	submittedAttempt := attempt
	submittedAttempt.State = model.SubmittedPaymentAttemptState
	savedContract := contractRow
	savedContract.FinancialInvoiceID = &invoiceID
	savedContract.FinancialPaymentIntentID = &paymentIntentID
	savedContract.FinancialPaymentAttemptID = &paymentAttemptID
	savedContract.FinancialPaymentSessionID = &paymentSessionID
	savedContract.FinancialPaymentLink = &paymentLink
	savedContract.FinancialPaymentExpiresAt = &expiresAt

	deps.expectConnTx(ctx)
	deps.contract.EXPECT().Tx(deps.tx).Return(deps.contractTx).Once()
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()

	deps.contractTx.EXPECT().LockByID(ctx, contractID).Return(contractRow, nil).Once()
	deps.paymentAttemptTx.EXPECT().LockByIDWithItems(ctx, attempt.ID).Return(attempt, nil).Once()
	deps.paymentAttemptTx.EXPECT().MarkSubmittedFromReconciliationRequired(
		ctx,
		attempt.ID,
		paymentIntentID,
		paymentSessionID,
		testifymock.Anything,
		&expiresAt,
		now,
		now,
		now,
	).Return(submittedAttempt, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("audit-event-id").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()
	deps.paymentAttemptTx.EXPECT().GetByContractIDAndPurposeWithItems(
		ctx,
		contractID,
		model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	).Return(submittedAttempt, nil).Once()
	deps.contractTx.EXPECT().SaveCheckoutPaymentSession(
		ctx,
		contractID,
		invoiceID,
		paymentIntentID,
		paymentAttemptID,
		paymentSessionID,
		paymentLink,
		expiresAt,
		false,
		now,
	).Return(savedContract, nil).Once()

	savedContractResult, savedAttempt, output, err := uc.ensureCheckoutPaymentSession(ctx, contractRow, attempt)
	require.NoError(t, err)
	require.Equal(t, model.SubmittedPaymentAttemptState, savedAttempt.State)
	require.Equal(t, paymentLink, output.PaymentLink)
	require.NotNil(t, savedContractResult.FinancialPaymentLink)
	require.Equal(t, paymentLink, *savedContractResult.FinancialPaymentLink)
}
