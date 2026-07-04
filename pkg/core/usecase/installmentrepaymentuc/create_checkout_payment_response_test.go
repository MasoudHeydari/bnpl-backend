package installmentrepaymentuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

type installmentFCResponseHarness struct {
	pool               *mock.Pool
	conn               *mock.Conn
	tx                 *mock.Tx
	customerProfile    *mock.CustomerProfile
	customerCQ         *mock.CustomerProfileConnQueryer
	merchantProfile    *mock.MerchantProfile
	merchantCQ         *mock.MerchantProfileConnQueryer
	contractRepo       *mock.Contract
	contractCQ         *mock.ContractConnQueryer
	installmentRepo    *mock.Installment
	installmentCQ      *mock.InstallmentConnQueryer
	paymentAttemptRepo *mock.PaymentAttempt
	paymentAttemptTx   *mock.PaymentAttemptTxQueryer
	outboxRepo         *mock.Outbox
	outboxTx           *mock.OutboxTxQueryer
	idGenerator        *mock.IdentifierGenerator
	checkoutClient     *mock.CheckoutPaymentClient

	now                time.Time
	attempt            model.PaymentAttempt
	externalCustomerID model.ExternalCustomerID
	uc                 *UseCase
}

func newInstallmentFCResponseHarness(t *testing.T) *installmentFCResponseHarness {
	t.Helper()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	h := &installmentFCResponseHarness{
		pool:               mock.NewPool(t),
		conn:               mock.NewConn(t),
		tx:                 mock.NewTx(t),
		customerProfile:    mock.NewCustomerProfile(t),
		customerCQ:         mock.NewCustomerProfileConnQueryer(t),
		merchantProfile:    mock.NewMerchantProfile(t),
		merchantCQ:         mock.NewMerchantProfileConnQueryer(t),
		contractRepo:       mock.NewContract(t),
		contractCQ:         mock.NewContractConnQueryer(t),
		installmentRepo:    mock.NewInstallment(t),
		installmentCQ:      mock.NewInstallmentConnQueryer(t),
		paymentAttemptRepo: mock.NewPaymentAttempt(t),
		paymentAttemptTx:   mock.NewPaymentAttemptTxQueryer(t),
		outboxRepo:         mock.NewOutbox(t),
		outboxTx:           mock.NewOutboxTxQueryer(t),
		idGenerator:        mock.NewIdentifierGenerator(t),
		checkoutClient:     mock.NewCheckoutPaymentClient(t),
		now:                now,
	}

	customerInternalID := model.CustomerID(uuid.New())
	h.externalCustomerID = model.ExternalCustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	receivableID := "fc-receivable-1"
	obligationID := "fc-obligation-1"
	tenantID := "tenant-1"
	contextID := "context-1"
	returnURL := "https://bnpl.example/return"
	createdAt := now.Add(-2 * time.Minute)

	h.attempt = model.PaymentAttempt{
		ID:                              attemptID,
		ContractID:                      contractID,
		InstallmentID:                   installmentID,
		Purpose:                         model.InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                           model.CreatedPaymentAttemptState,
		AmountMinor:                     20,
		Currency:                        model.IRRCurrency,
		Channel:                         model.GatewayPaymentChannel,
		IdempotencyKey:                  "idem-response-1",
		RequestFingerprint:              "fp-1",
		TenantID:                        &tenantID,
		ContextID:                       &contextID,
		SuccessReturnURL:                &returnURL,
		FailureReturnURL:                &returnURL,
		CancelReturnURL:                 &returnURL,
		FinancialCollectionObligationID: &obligationID,
		CreatedAt:                       createdAt,
	}
	h.attempt.Items = []model.PaymentAttemptItem{{ItemType: model.PrincipalPaymentAttemptItemType, SourceID: h.attempt.InstallmentID.UUID(), AmountMinor: h.attempt.AmountMinor, Effect: model.DebitPaymentAttemptItemEffect}}

	h.pool.EXPECT().Conn(testifymock.Anything, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, h.conn)
		},
	).Maybe()
	h.contractRepo.EXPECT().Conn(h.conn).Return(h.contractCQ).Maybe()
	h.contractCQ.EXPECT().GetByID(testifymock.Anything, contractID).Return(model.Contract{
		ID:                             contractID,
		CustomerID:                     customerInternalID,
		MerchantProfileID:              merchantProfileID,
		Status:                         model.ActiveContractStatus,
		Currency:                       model.IRRCurrency,
		FinancialMerchantEntitlementID: &receivableID,
	}, nil).Maybe()
	h.customerProfile.EXPECT().Conn(h.conn).Return(h.customerCQ).Maybe()
	h.customerCQ.EXPECT().GetByExternalCustomerID(testifymock.Anything, h.externalCustomerID).Return(model.CustomerProfile{
		ID:                 customerInternalID,
		ExternalCustomerID: h.externalCustomerID,
	}, nil).Maybe()
	h.merchantProfile.EXPECT().Conn(h.conn).Return(h.merchantCQ).Maybe()
	h.merchantCQ.EXPECT().GetByID(testifymock.Anything, merchantProfileID).Return(model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: model.MerchantID(uuid.New()),
	}, nil).Maybe()
	h.installmentRepo.EXPECT().Conn(h.conn).Return(h.installmentCQ).Maybe()
	h.installmentCQ.EXPECT().GetByID(testifymock.Anything, installmentID).Return(model.Installment{
		ID:                   installmentID,
		ContractID:           &contractID,
		PrincipalAmountMinor: 20,
		DueAt:                now.Add(-48 * time.Hour),
	}, nil).Maybe()

	uc, err := New(
		h.pool,
		h.customerProfile,
		h.merchantProfile,
		h.contractRepo,
		h.installmentRepo,
		mock.NewInstallmentCharge(t),
		h.paymentAttemptRepo,
		h.outboxRepo,
		h.checkoutClient,
		adapterclock.NewFixed(now, time.UTC),
		lognoop.New(),
		"bnpl.audit.events.v1",
		h.idGenerator,
		returnURL,
	)
	require.NoError(t, err)
	h.uc = uc

	return h
}

func (h *installmentFCResponseHarness) input() CreatePaymentAttemptInput {
	return CreatePaymentAttemptInput{
		ExternalCustomerID: h.externalCustomerID,
		TenantID:           model.TenantID(uuid.MustParse("00000000-0000-4000-8000-0000000000aa")),
		ContextID:          model.ContextID(uuid.MustParse("00000000-0000-4000-8000-0000000000bb")),
	}
}

func (h *installmentFCResponseHarness) expectReserveSessionCreate(ctx context.Context) model.PaymentAttempt {
	h.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()
	h.paymentAttemptTx.EXPECT().LockByID(ctx, h.attempt.ID).Return(h.attempt, nil).Once()

	leaseToken := uuid.New()
	leaseUntil := h.now.Add(model.FinancialSessionCreateLeaseDuration)
	reserved := h.attempt
	reserved.FinancialSessionCreateAttemptCount = h.attempt.FinancialSessionCreateAttemptCount + 1
	reserved.FinancialSessionCreateLeaseToken = &leaseToken
	reserved.FinancialSessionCreateLeaseUntil = &leaseUntil
	h.paymentAttemptTx.EXPECT().IncrementFinancialSessionCreateAttemptCount(
		ctx,
		h.attempt.ID,
		model.MaxFinancialSessionCreateAttempts,
		h.now,
	).Return(reserved, nil).Once()

	return reserved
}

// expectReleaseSessionCreateLease after a failed CreateCheckoutPayment under budget.
// Finalization uses a detached timeout context, so matchers accept any context.
func (h *installmentFCResponseHarness) expectReleaseSessionCreateLease(reserved model.PaymentAttempt) {
	h.conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()
	h.outboxRepo.EXPECT().Tx(h.tx).Return(h.outboxTx).Once()
	h.paymentAttemptTx.EXPECT().LockByID(testifymock.Anything, h.attempt.ID).Return(reserved, nil).Once()
	h.paymentAttemptTx.EXPECT().ClearFinancialSessionCreateLease(
		testifymock.Anything,
		reserved.ID,
		*reserved.FinancialSessionCreateLeaseToken,
		h.now.UTC(),
	).Return(reserved, nil).Once()
}

func (h *installmentFCResponseHarness) validFCResult() extcheckoutpayment.CreateCheckoutPaymentResult {
	return extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  "fc-intent-1",
		PaymentSessionID: "fc-session-1",
		Status:           extcheckoutpayment.RequiresActionPaymentStatus,
		AmountMinor:      20,
		Currency:         "IRR",
		PaymentLink:      "https://pay.example/session-1",
		ExpiresAt:        h.attempt.CreatedAt.Add(DefaultPaymentSessionTTL),
	}
}

func TestEnsureFinancialCoreInstallmentPayment_ValidActionableResponseMarksSubmitted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newInstallmentFCResponseHarness(t)
	result := h.validFCResult()

	h.expectReserveSessionCreate(ctx)
	h.checkoutClient.EXPECT().CreateCheckoutPayment(ctx, testifymock.Anything).Return(result, nil).Once()
	h.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()

	submitted := h.attempt
	submitted.State = model.SubmittedPaymentAttemptState
	link := result.PaymentLink
	expires := result.ExpiresAt.UTC()
	submitted.PaymentURL = &link
	submitted.FinancialPaymentExpiresAt = &expires
	h.paymentAttemptTx.EXPECT().MarkSubmitted(
		ctx,
		h.attempt.ID,
		result.PaymentIntentID,
		result.PaymentSessionID,
		&link,
		testifymock.MatchedBy(func(got *time.Time) bool {
			return got != nil && got.UTC().Equal(expires)
		}),
		h.now,
		h.now,
	).Return(submitted, nil).Once()

	out, err := h.uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
	require.NoError(t, err)
	require.Equal(t, model.SubmittedPaymentAttemptState, out.State)
	require.NotNil(t, out.PaymentURL)
	require.Equal(t, result.PaymentLink, *out.PaymentURL)
}

func TestEnsureFinancialCoreInstallmentPayment_InvalidFCResponseLeavesCreated(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		mutate   func(*extcheckoutpayment.CreateCheckoutPaymentResult)
		wantCode cerr.Code
	}{
		{
			name: "missing payment intent",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.PaymentIntentID = ""
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "missing payment session",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.PaymentSessionID = ""
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "missing payment link",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.PaymentLink = ""
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "http payment link",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.PaymentLink = "http://pay.example/1"
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "userinfo payment link",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.PaymentLink = "https://u:p@pay.example/1"
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "missing fc expiry",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.ExpiresAt = time.Time{}
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "expired fc expiry",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.ExpiresAt = now
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "amount mismatch",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.AmountMinor = 21
			},
			wantCode: cerr.CodeConflict,
		},
		{
			name: "currency mismatch",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.Currency = "USD"
			},
			wantCode: cerr.CodeConflict,
		},
		{
			name: "unsupported processing status",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.Status = extcheckoutpayment.ProcessingPaymentStatus
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
		{
			name: "unsupported succeeded status",
			mutate: func(r *extcheckoutpayment.CreateCheckoutPaymentResult) {
				r.Status = extcheckoutpayment.SucceededPaymentStatus
			},
			wantCode: cerr.CodeServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			h := newInstallmentFCResponseHarness(t)
			result := h.validFCResult()
			tt.mutate(&result)

			reserved := h.expectReserveSessionCreate(ctx)
			h.checkoutClient.EXPECT().CreateCheckoutPayment(ctx, testifymock.Anything).Return(result, nil).Once()
			h.expectReleaseSessionCreateLease(reserved)
			// MarkSubmitted must never run for an invalid actionable session.

			out, err := h.uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
			require.Error(t, err)
			require.True(t, cerr.IsCode(err, tt.wantCode))
			require.Equal(t, model.PaymentAttempt{}, out)
			require.Equal(t, model.CreatedPaymentAttemptState, h.attempt.State)
			require.Equal(t, "idem-response-1", h.attempt.IdempotencyKey)
			require.NotNil(t, h.attempt.FinancialCollectionObligationID)
		})
	}
}

func TestEnsureFinancialCoreInstallmentPayment_IdempotentReplayAccepted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newInstallmentFCResponseHarness(t)
	result := h.validFCResult()
	result.ReusedIdempotentResult = true

	h.expectReserveSessionCreate(ctx)
	h.checkoutClient.EXPECT().CreateCheckoutPayment(ctx, testifymock.Anything).Return(result, nil).Once()
	h.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()

	submitted := h.attempt
	submitted.State = model.SubmittedPaymentAttemptState
	link := result.PaymentLink
	h.paymentAttemptTx.EXPECT().MarkSubmitted(
		ctx,
		h.attempt.ID,
		result.PaymentIntentID,
		result.PaymentSessionID,
		&link,
		testifymock.Anything,
		h.now,
		h.now,
	).Return(submitted, nil).Once()

	out, err := h.uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
	require.NoError(t, err)
	require.Equal(t, model.SubmittedPaymentAttemptState, out.State)
}

func TestEnsureFinancialCoreInstallmentPayment_PersistenceFailureAfterValidResponseIsAmbiguous(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newInstallmentFCResponseHarness(t)
	result := h.validFCResult()

	h.expectReserveSessionCreate(ctx)
	h.checkoutClient.EXPECT().CreateCheckoutPayment(ctx, testifymock.Anything).Return(result, nil).Once()
	h.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()
	h.paymentAttemptTx.EXPECT().MarkSubmitted(
		ctx,
		h.attempt.ID,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		testifymock.Anything,
		h.now,
		h.now,
	).Return(model.PaymentAttempt{}, errors.New("db write failed")).Once()

	out, err := h.uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
	require.Error(t, err)
	require.Equal(t, model.PaymentAttempt{}, out)
	// Durable local state remains created; callers must reconcile, not invent failure.
	require.Equal(t, model.CreatedPaymentAttemptState, h.attempt.State)
}

func TestEnsureFinancialCoreInstallmentPayment_RetryBudgetExhaustedCancelsLocally(t *testing.T) {
	t.Parallel()

	ctx := trace.WithTraceID(trace.WithCorrelationID(context.Background(), "corr-retry-budget"), "trace-retry-budget")
	h := newInstallmentFCResponseHarness(t)
	h.attempt.FinancialSessionCreateAttemptCount = model.MaxFinancialSessionCreateAttempts
	h.idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()

	h.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()
	h.outboxRepo.EXPECT().Tx(h.tx).Return(h.outboxTx).Once()
	h.paymentAttemptTx.EXPECT().LockByID(ctx, h.attempt.ID).Return(h.attempt, nil).Once()
	canceled := h.attempt
	canceled.State = model.CanceledPaymentAttemptState
	failure := model.SessionCreationRetryExhaustedFailureCode
	canceled.FailureCode = &failure
	canceled.CanceledAt = &h.now
	canceled.FinancialOutcomeResolved = false
	h.paymentAttemptTx.EXPECT().CancelSessionCreationRetryExhausted(
		ctx,
		h.attempt.ID,
		model.CreatedPaymentAttemptState,
		h.now,
		h.now,
	).Return(canceled, nil).Once()
	h.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	out, err := h.uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Equal(t, model.CanceledPaymentAttemptState, out.State)
	require.False(t, out.FinancialOutcomeResolved)
	require.NotNil(t, out.FailureCode)
	require.Equal(t, model.SessionCreationRetryExhaustedFailureCode, *out.FailureCode)
}

func TestEnsureFinancialCoreInstallmentPayment_ThirdFailureReturnsBudgetExhausted(t *testing.T) {
	t.Parallel()

	ctx := trace.WithTraceID(trace.WithCorrelationID(context.Background(), "corr-third-fail"), "trace-third-fail")
	h := newInstallmentFCResponseHarness(t)
	h.attempt.FinancialSessionCreateAttemptCount = model.MaxFinancialSessionCreateAttempts - 1
	reserved := h.expectReserveSessionCreate(ctx)
	h.checkoutClient.EXPECT().CreateCheckoutPayment(ctx, testifymock.Anything).
		Return(extcheckoutpayment.CreateCheckoutPaymentResult{}, errors.New("FC deadline exceeded")).Once()

	h.conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()
	h.outboxRepo.EXPECT().Tx(h.tx).Return(h.outboxTx).Once()
	h.paymentAttemptTx.EXPECT().LockByID(testifymock.Anything, reserved.ID).Return(reserved, nil).Once()

	canceled := reserved
	canceled.State = model.CanceledPaymentAttemptState
	reason := model.SessionCreationRetryExhaustedFailureCode
	canceled.FailureCode = &reason
	canceled.CanceledAt = &h.now
	h.paymentAttemptTx.EXPECT().CancelSessionCreationRetryExhausted(
		testifymock.Anything, reserved.ID, model.CreatedPaymentAttemptState, h.now, h.now,
	).Return(canceled, nil).Once()
	h.idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()
	h.outboxTx.EXPECT().Create(testifymock.Anything, testifymock.Anything).
		Return(model.OutboxMessage{}, nil).Once()

	_, err := h.uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
	require.Error(t, err)
	var publicErr *cerr.Error
	require.ErrorAs(t, err, &publicErr)
	require.Equal(t, cerr.CodeConflict, publicErr.Code)
	require.Equal(t, cerr.MessageKey("error.payment_session_create_retry_budget_exhausted"), publicErr.MessageKey)
}

func TestFinalizeFailedSessionCreateAttempt_StaleLeaseCannotCancelNewOwner(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	h := newInstallmentFCResponseHarness(t)
	staleToken := uuid.New()
	newToken := uuid.New()

	stale := h.attempt
	stale.FinancialSessionCreateAttemptCount = 1
	stale.FinancialSessionCreateLeaseToken = &staleToken
	current := h.attempt
	current.FinancialSessionCreateAttemptCount = model.MaxFinancialSessionCreateAttempts
	current.FinancialSessionCreateLeaseToken = &newToken

	h.conn.EXPECT().Tx(testifymock.Anything, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, h.tx)
		},
	).Once()
	h.paymentAttemptRepo.EXPECT().Tx(h.tx).Return(h.paymentAttemptTx).Once()
	h.outboxRepo.EXPECT().Tx(h.tx).Return(h.outboxTx).Once()
	h.paymentAttemptTx.EXPECT().LockByID(testifymock.Anything, stale.ID).
		Return(current, nil).Once()

	canceled, err := h.uc.finalizeFailedSessionCreateAttempt(ctx, stale, h.now)
	require.NoError(t, err)
	require.False(t, canceled)
}

func TestEnsureFinancialCoreInstallmentPayment_RequestedExpiryElapsedDoesNotCallFC(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	createdAt := time.Date(2026, 10, 20, 11, 0, 0, 0, time.UTC)
	now := createdAt.Add(DefaultPaymentSessionTTL).Add(time.Minute)

	h := newInstallmentFCResponseHarness(t)
	h.attempt.CreatedAt = createdAt

	uc, err := New(
		h.pool,
		h.customerProfile,
		h.merchantProfile,
		h.contractRepo,
		h.installmentRepo,
		mock.NewInstallmentCharge(t),
		h.paymentAttemptRepo,
		mock.NewOutbox(t),
		h.checkoutClient,
		adapterclock.NewFixed(now, time.UTC),
		lognoop.New(),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		"https://bnpl.example/return",
	)
	require.NoError(t, err)

	_, err = uc.ensureFinancialCoreInstallmentPayment(ctx, h.attempt, h.input())
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	require.Equal(t, model.CreatedPaymentAttemptState, h.attempt.State)
	require.Equal(t, "idem-response-1", h.attempt.IdempotencyKey)
}
