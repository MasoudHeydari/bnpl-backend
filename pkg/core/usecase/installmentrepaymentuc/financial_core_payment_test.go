package installmentrepaymentuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestEnsureFinancialCoreInstallmentPayment_UsesExistingReceivableAndExternalIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerCQ := mock.NewCustomerProfileConnQueryer(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantCQ := mock.NewMerchantProfileConnQueryer(t)
	contractRepo := mock.NewContract(t)
	contractCQ := mock.NewContractConnQueryer(t)
	installmentRepo := mock.NewInstallment(t)
	installmentCQ := mock.NewInstallmentConnQueryer(t)
	paymentAttemptRepo := mock.NewPaymentAttempt(t)
	paymentAttemptTx := mock.NewPaymentAttemptTxQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)
	idGenerator := mock.NewIdentifierGenerator(t)

	customerInternalID := model.CustomerID(uuid.New())
	identitySubjectID := model.IdentitySubjectID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	merchantID := model.MerchantID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	receivableID := "fc-receivable-60"
	tenantID := "tenant-1"
	contextID := "context-1"
	returnURL := "https://bnpl.example/return"
	dueAt := now.Add(24 * time.Hour)

	attempt := model.PaymentAttempt{
		ID:                 attemptID,
		ContractID:         contractID,
		InstallmentID:      installmentID,
		Purpose:            model.InstallmentRepaymentFinancialCorePaymentPurpose,
		State:              model.CreatedPaymentAttemptState,
		AmountMinor:        20,
		Currency:           model.IRRCurrency,
		Channel:            model.GatewayPaymentChannel,
		IdempotencyKey:     "idem-1",
		RequestFingerprint: "fp-1",
		TenantID:           &tenantID,
		ContextID:          &contextID,
		SuccessReturnURL:   &returnURL,
		FailureReturnURL:   &returnURL,
		CancelReturnURL:    &returnURL,
	}

	// Load contract/parties/installment (read-only Conn).
	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	contractRepo.EXPECT().Conn(conn).Return(contractCQ).Once()
	contractCQ.EXPECT().GetByID(ctx, contractID).Return(model.Contract{
		ID:                    contractID,
		CustomerID:            customerInternalID,
		MerchantProfileID:     merchantProfileID,
		Status:                model.ActiveContractStatus,
		Currency:              model.IRRCurrency,
		FinancialReceivableID: &receivableID,
	}, nil).Once()
	customerProfile.EXPECT().Conn(conn).Return(customerCQ).Once()
	customerCQ.EXPECT().GetByIdentitySubjectID(ctx, identitySubjectID).Return(model.CustomerProfile{
		ID:                customerInternalID,
		IdentitySubjectID: identitySubjectID,
	}, nil).Once()
	merchantProfile.EXPECT().Conn(conn).Return(merchantCQ).Once()
	merchantCQ.EXPECT().GetByID(ctx, merchantProfileID).Return(model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: merchantID,
	}, nil).Once()
	installmentRepo.EXPECT().Conn(conn).Return(installmentCQ).Once()
	installmentCQ.EXPECT().GetByID(ctx, installmentID).Return(model.Installment{
		ID:                   installmentID,
		ContractID:           &contractID,
		PrincipalAmountMinor: 20,
		DueAt:                dueAt,
	}, nil).Once()

	checkoutClient.EXPECT().PrepareCollectionObligation(ctx, testifymock.MatchedBy(
		func(in extcheckoutpayment.PrepareCollectionObligationInput) bool {
			return in.Application == extcheckoutpayment.ExistingReceivableCollectionApplication &&
				in.ReceivableID == receivableID &&
				in.AmountMinor == 20 &&
				in.MerchantID == merchantID.String() &&
				in.CustomerSubjectID == identitySubjectID.String() &&
				in.MerchantID != merchantProfileID.String() &&
				in.CustomerSubjectID != customerInternalID.String()
		},
	)).Return(extcheckoutpayment.PrepareCollectionObligationResult{
		CollectionObligationID: "fc-obligation-1",
		OutstandingAmountMinor: 20,
		Currency:               "IRR",
	}, nil).Once()

	// Persist collection obligation.
	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		},
	).Once()
	paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptTx).Once()
	bound := attempt
	obligationID := "fc-obligation-1"
	bound.FinancialCollectionObligationID = &obligationID
	paymentAttemptTx.EXPECT().BindCollectionObligationIfAbsent(ctx, attemptID, obligationID, now).
		Return(bound, nil).Once()

	checkoutClient.EXPECT().CreateCheckoutPayment(ctx, testifymock.MatchedBy(
		func(in extcheckoutpayment.CreateCheckoutPaymentInput) bool {
			return in.CollectionObligationID == obligationID &&
				in.PayerID == identitySubjectID.String() &&
				in.PayeeID == merchantID.String() &&
				in.AmountMinor == 20
		},
	)).Return(extcheckoutpayment.CreateCheckoutPaymentResult{
		PaymentIntentID:  "fc-intent-1",
		PaymentSessionID: "fc-session-1",
		AmountMinor:      20,
		Currency:         "IRR",
		PaymentLink:      "https://pay.example/1",
		ExpiresAt:        dueAt,
	}, nil).Once()

	// Mark submitted.
	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		},
	).Once()
	paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptTx).Once()
	submitted := bound
	submitted.State = model.SubmittedPaymentAttemptState
	paymentAttemptTx.EXPECT().MarkSubmitted(
		ctx, attemptID, "fc-intent-1", "fc-session-1", testifymock.Anything, testifymock.Anything, now, now,
	).Return(submitted, nil).Once()

	uc, err := New(
		pool,
		customerProfile,
		merchantProfile,
		contractRepo,
		installmentRepo,
		mock.NewInstallmentCharge(t),
		paymentAttemptRepo,
		mock.NewOutbox(t),
		checkoutClient,
		adapterclock.NewFixed(now, time.UTC),
		"bnpl.audit.events.v1",
		idGenerator,
		returnURL,
	)
	require.NoError(t, err)

	out, err := uc.ensureFinancialCoreInstallmentPayment(ctx, attempt, CreatePaymentAttemptInput{
		IdentitySubjectID: identitySubjectID,
		TenantID:          model.TenantID(uuid.MustParse("00000000-0000-4000-8000-0000000000aa")),
		ContextID:         model.ContextID(uuid.MustParse("00000000-0000-4000-8000-0000000000bb")),
	})
	require.NoError(t, err)
	require.Equal(t, model.SubmittedPaymentAttemptState, out.State)
	require.NotNil(t, out.FinancialCollectionObligationID)
	require.Equal(t, obligationID, *out.FinancialCollectionObligationID)
}

func TestEnsureFinancialCoreInstallmentPayment_RejectsMissingReceivable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	contractRepo := mock.NewContract(t)
	contractCQ := mock.NewContractConnQueryer(t)

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	contractRepo.EXPECT().Conn(conn).Return(contractCQ).Once()
	contractCQ.EXPECT().GetByID(ctx, testifymock.Anything).Return(model.Contract{
		ID:     model.ContractID(uuid.New()),
		Status: model.ActiveContractStatus,
	}, nil).Once()

	uc, err := New(
		pool,
		mock.NewCustomerProfile(t),
		mock.NewMerchantProfile(t),
		contractRepo,
		mock.NewInstallment(t),
		mock.NewInstallmentCharge(t),
		mock.NewPaymentAttempt(t),
		mock.NewOutbox(t),
		mock.NewCheckoutPaymentClient(t),
		adapterclock.NewFixed(now, time.UTC),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		"https://bnpl.example/return",
	)
	require.NoError(t, err)

	tenantID := "t"
	contextID := "c"
	returnURL := "https://bnpl.example/return"
	_, err = uc.ensureFinancialCoreInstallmentPayment(ctx, model.PaymentAttempt{
		ID:               model.PaymentAttemptID(uuid.New()),
		ContractID:       model.ContractID(uuid.New()),
		InstallmentID:    model.InstallmentID(uuid.New()),
		State:            model.CreatedPaymentAttemptState,
		AmountMinor:      20,
		Currency:         model.IRRCurrency,
		TenantID:         &tenantID,
		ContextID:        &contextID,
		SuccessReturnURL: &returnURL,
		FailureReturnURL: &returnURL,
		CancelReturnURL:  &returnURL,
	}, CreatePaymentAttemptInput{
		IdentitySubjectID: model.IdentitySubjectID(uuid.New()),
	})
	require.Error(t, err)
}

func TestEnsureFinancialCoreInstallmentPayment_RejectsChargesAgainstPrincipalReceivable(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerCQ := mock.NewCustomerProfileConnQueryer(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantCQ := mock.NewMerchantProfileConnQueryer(t)
	contractRepo := mock.NewContract(t)
	contractCQ := mock.NewContractConnQueryer(t)
	installmentRepo := mock.NewInstallment(t)
	installmentCQ := mock.NewInstallmentConnQueryer(t)

	customerInternalID := model.CustomerID(uuid.New())
	identitySubjectID := model.IdentitySubjectID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	receivableID := "fc-receivable-60"
	tenantID := "tenant-1"
	contextID := "context-1"
	returnURL := "https://bnpl.example/return"

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	contractRepo.EXPECT().Conn(conn).Return(contractCQ).Once()
	contractCQ.EXPECT().GetByID(ctx, contractID).Return(model.Contract{
		ID:                    contractID,
		CustomerID:            customerInternalID,
		MerchantProfileID:     merchantProfileID,
		Status:                model.ActiveContractStatus,
		FinancialReceivableID: &receivableID,
	}, nil).Once()
	customerProfile.EXPECT().Conn(conn).Return(customerCQ).Once()
	customerCQ.EXPECT().GetByIdentitySubjectID(ctx, identitySubjectID).Return(model.CustomerProfile{
		ID:                customerInternalID,
		IdentitySubjectID: identitySubjectID,
	}, nil).Once()
	merchantProfile.EXPECT().Conn(conn).Return(merchantCQ).Once()
	merchantCQ.EXPECT().GetByID(ctx, merchantProfileID).Return(model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: model.MerchantID(uuid.New()),
	}, nil).Once()
	installmentRepo.EXPECT().Conn(conn).Return(installmentCQ).Once()
	installmentCQ.EXPECT().GetByID(ctx, installmentID).Return(model.Installment{
		ID:                   installmentID,
		ContractID:           &contractID,
		PrincipalAmountMinor: 20, // principal only
		DueAt:                now,
	}, nil).Once()

	uc, err := New(
		pool,
		customerProfile,
		merchantProfile,
		contractRepo,
		installmentRepo,
		mock.NewInstallmentCharge(t),
		mock.NewPaymentAttempt(t),
		mock.NewOutbox(t),
		mock.NewCheckoutPaymentClient(t),
		adapterclock.NewFixed(now, time.UTC),
		"bnpl.audit.events.v1",
		mock.NewIdentifierGenerator(t),
		returnURL,
	)
	require.NoError(t, err)

	_, err = uc.ensureFinancialCoreInstallmentPayment(ctx, model.PaymentAttempt{
		ID:               model.PaymentAttemptID(uuid.New()),
		ContractID:       contractID,
		InstallmentID:    installmentID,
		State:            model.CreatedPaymentAttemptState,
		AmountMinor:      22, // principal 20 + late fee 2 — not allowed against principal receivable
		Currency:         model.IRRCurrency,
		TenantID:         &tenantID,
		ContextID:        &contextID,
		SuccessReturnURL: &returnURL,
		FailureReturnURL: &returnURL,
		CancelReturnURL:  &returnURL,
	}, CreatePaymentAttemptInput{IdentitySubjectID: identitySubjectID})
	require.Error(t, err)
}
