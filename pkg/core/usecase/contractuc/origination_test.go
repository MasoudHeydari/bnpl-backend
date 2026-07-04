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
	"github.com/parstis/bnpl-backend/pkg/core/domaincapability"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	checkoutv1 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v1"
	checkoutv2 "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/v2"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type originationDeps struct {
	pool                 *mock.Pool
	conn                 *mock.Conn
	tx                   *mock.Tx
	customerProfile      *mock.CustomerProfile
	customerProfileTx    *mock.CustomerProfileTxQueryer
	merchantProfile      *mock.MerchantProfile
	merchantProfileTx    *mock.MerchantProfileTxQueryer
	creditAccount        *mock.CreditAccount
	creditAccountTx      *mock.CreditAccountTxQueryer
	decision             *mock.Decision
	decisionCQ           *mock.DecisionConnQueryer
	decisionTx           *mock.DecisionTxQueryer
	contract             *mock.Contract
	contractTx           *mock.ContractTxQueryer
	contractCQ           *mock.ContractConnQueryer
	allocation           *mock.CheckoutDecisionAllocation
	allocationTx         *mock.CheckoutDecisionAllocationTxQueryer
	installment          *mock.Installment
	installmentTx        *mock.InstallmentTxQueryer
	installmentCharge    *mock.InstallmentCharge
	installmentChargeTx  *mock.InstallmentChargeTxQueryer
	paymentAttempt       *mock.PaymentAttempt
	paymentAttemptTx     *mock.PaymentAttemptTxQueryer
	outbox               *mock.Outbox
	outboxTx             *mock.OutboxTxQueryer
	checkoutPayment      *mock.CheckoutPaymentClient
	contractCancellation *mock.ContractCancellationClient
	jurisdictions        domaincapability.OriginationJurisdictions
	clock                adapterclock.Fixed
}

func newOriginationDeps(t *testing.T, now time.Time) originationDeps {
	t.Helper()

	return originationDeps{
		pool:                 mock.NewPool(t),
		conn:                 mock.NewConn(t),
		tx:                   mock.NewTx(t),
		customerProfile:      mock.NewCustomerProfile(t),
		customerProfileTx:    mock.NewCustomerProfileTxQueryer(t),
		merchantProfile:      mock.NewMerchantProfile(t),
		merchantProfileTx:    mock.NewMerchantProfileTxQueryer(t),
		creditAccount:        mock.NewCreditAccount(t),
		creditAccountTx:      mock.NewCreditAccountTxQueryer(t),
		decision:             mock.NewDecision(t),
		decisionCQ:           mock.NewDecisionConnQueryer(t),
		decisionTx:           mock.NewDecisionTxQueryer(t),
		contract:             mock.NewContract(t),
		contractTx:           mock.NewContractTxQueryer(t),
		contractCQ:           mock.NewContractConnQueryer(t),
		allocation:           mock.NewCheckoutDecisionAllocation(t),
		allocationTx:         mock.NewCheckoutDecisionAllocationTxQueryer(t),
		installment:          mock.NewInstallment(t),
		installmentTx:        mock.NewInstallmentTxQueryer(t),
		installmentCharge:    mock.NewInstallmentCharge(t),
		installmentChargeTx:  mock.NewInstallmentChargeTxQueryer(t),
		paymentAttempt:       mock.NewPaymentAttempt(t),
		paymentAttemptTx:     mock.NewPaymentAttemptTxQueryer(t),
		outbox:               mock.NewOutbox(t),
		outboxTx:             mock.NewOutboxTxQueryer(t),
		checkoutPayment:      mock.NewCheckoutPaymentClient(t),
		contractCancellation: mock.NewContractCancellationClient(t),
		clock:                adapterclock.NewFixed(now, time.UTC),
	}
}

func (d originationDeps) newUseCase(t *testing.T) *UseCase {
	t.Helper()

	jurisdictions := d.jurisdictions
	if jurisdictions == nil {
		jurisdictions = mustTestOriginationJurisdictions(t)
	}

	sagaRepo, sagaExecutor, sagaDefinition := mustTestCancellationSagaDeps(
		t,
		d.pool,
		d.contract,
		d.creditAccount,
		d.contractCancellation,
		d.clock,
	)

	uc, err := New(
		d.pool,
		d.customerProfile,
		d.merchantProfile,
		d.creditAccount,
		d.decision,
		d.contract,
		d.allocation,
		mustTestCheckoutRegistry(t),
		d.installment,
		d.installmentCharge,
		d.paymentAttempt,
		d.checkoutPayment,
		d.contractCancellation,
		d.outbox,
		d.clock,
		"audit-events",
		mock.NewIdentifierGenerator(t),
		"https://bnpl.example/payments/return",
		5*time.Minute,
		100,
		testCancellationSagaSettings(),
		sagaRepo,
		sagaExecutor,
		sagaDefinition,
		jurisdictions,
		lognoop.New(),
		metricsnoop.New(),
	)
	require.NoError(t, err)

	return uc
}

func (d originationDeps) expectDecisionPolicyConn(
	ctx context.Context,
	decisionID model.DecisionID,
	policyID string,
	policyVersion string,
) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.decision.EXPECT().Conn(d.conn).Return(d.decisionCQ).Once()
	d.decisionCQ.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{
		ID:            decisionID,
		PolicyID:      policyID,
		PolicyVersion: policyVersion,
	}, nil).Once()
}

func (d originationDeps) expectPaymentPendingTxBindings(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
	d.customerProfile.EXPECT().Tx(d.tx).Return(d.customerProfileTx).Once()
	d.allocation.EXPECT().Tx(d.tx).Return(d.allocationTx).Once()
	d.contract.EXPECT().Tx(d.tx).Return(d.contractTx).Once()
	d.creditAccount.EXPECT().Tx(d.tx).Return(d.creditAccountTx).Once()
	d.installment.EXPECT().Tx(d.tx).Return(d.installmentTx).Once()
	d.installmentCharge.EXPECT().Tx(d.tx).Return(d.installmentChargeTx).Once()
	d.paymentAttempt.EXPECT().Tx(d.tx).Return(d.paymentAttemptTx).Once()
	d.outbox.EXPECT().Tx(d.tx).Return(d.outboxTx).Once()
}

func (d originationDeps) expectMerchantFundingTxBindings(ctx context.Context) {
	d.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, d.conn)
		},
	).Once()
	d.conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, d.tx)
		},
	).Once()
	d.customerProfile.EXPECT().Tx(d.tx).Return(d.customerProfileTx).Once()
	d.decision.EXPECT().Tx(d.tx).Return(d.decisionTx).Once()
	d.allocation.EXPECT().Tx(d.tx).Return(d.allocationTx).Once()
	d.contract.EXPECT().Tx(d.tx).Return(d.contractTx).Once()
	d.creditAccount.EXPECT().Tx(d.tx).Return(d.creditAccountTx).Once()
	d.installment.EXPECT().Tx(d.tx).Return(d.installmentTx).Once()
}

func matchingCheckoutPaymentAttempt(
	input CreateContractInput,
	contractRow model.Contract,
	first model.Installment,
	returnURL string,
) model.PaymentAttempt {
	tenant := input.TenantID.String()
	contextID := input.ContextID.String()

	return model.PaymentAttempt{
		ID:                 model.PaymentAttemptID(uuid.New()),
		ContractID:         contractRow.ID,
		InstallmentID:      first.ID,
		Purpose:            model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:              model.CreatedPaymentAttemptState,
		TenantID:           &tenant,
		ContextID:          &contextID,
		Currency:           contractRow.Currency,
		AmountMinor:        contractRow.PayableNowAmountMinor,
		Channel:            model.GatewayPaymentChannel,
		IdempotencyKey:     checkoutPaymentIdempotencyKey(contractRow.ID),
		RequestFingerprint: checkoutPaymentRequestFingerprint(input, contractRow),
		SuccessReturnURL:   &returnURL,
		FailureReturnURL:   &returnURL,
		CancelReturnURL:    &returnURL,
		Items: []model.PaymentAttemptItem{{
			ItemType:    model.PrincipalPaymentAttemptItemType,
			Effect:      model.DebitPaymentAttemptItemEffect,
			AmountMinor: contractRow.PayableNowAmountMinor,
			SourceID:    first.ID.UUID(),
		}},
	}
}

func TestPrepareCheckoutContract_PaymentBased_NewContractBlockedWhenOriginationDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newOriginationDeps(t, now)

	blocked := mock.NewOriginationJurisdictions(t)
	blocked.EXPECT().
		RequireOriginationEnabled(model.IRJurisdiction).
		Return(domaincapability.ErrOriginationNotEnabled).
		Once()
	deps.jurisdictions = blocked

	decisionID := model.DecisionID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

	v1 := checkoutv1.New().Ref()
	deps.expectDecisionPolicyConn(ctx, decisionID, v1.ID, v1.Version)
	deps.expectPaymentPendingTxBindings(ctx)

	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.allocationTx.EXPECT().
		LockByDecisionID(ctx, decisionID).
		Return(model.CheckoutDecisionAllocation{
			DecisionID:                  decisionID,
			CustomerID:                  customerID,
			MerchantProfileID:           merchantProfileID,
			Currency:                    model.IRRCurrency,
			RequestedAmountMinor:        100_000,
			ApprovedFinancedAmountMinor: 100_000,
			InstallmentCount:            1,
			ExpiresAt:                   now.Add(time.Hour),
		}, nil).
		Once()
	deps.installmentTx.EXPECT().
		LockByDecisionID(ctx, decisionID).
		Return([]model.Installment{{
			ID:                   installmentID,
			DecisionID:           decisionID,
			InstallmentOrder:     1,
			Currency:             model.IRRCurrency,
			PrincipalAmountMinor: 100_000,
			TotalAmountMinor:     100_000,
			Status:               model.OfferedInstallmentStatus,
		}}, nil).
		Once()
	deps.contractTx.EXPECT().
		GetByDecisionID(ctx, decisionID).
		Return(model.Contract{}, cerr.ResourceNotFound(errors.New("missing"))).
		Once()
	deps.merchantProfile.EXPECT().Tx(deps.tx).Return(deps.merchantProfileTx).Once()
	deps.merchantProfileTx.EXPECT().
		GetByID(ctx, merchantProfileID).
		Return(model.MerchantProfile{
			ID:           merchantProfileID,
			Jurisdiction: model.IRJurisdiction,
		}, nil).
		Once()

	uc := deps.newUseCase(t)
	_, err := uc.PrepareCheckoutContract(ctx, CreateContractInput{
		DecisionID:         decisionID,
		ExternalCustomerID: externalCustomerID,
		TenantID:           tenantID,
		ContextID:          contextID,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.False(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestPrepareCheckoutContract_PaymentBased_ExistingContractReplayAfterOriginationDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newOriginationDeps(t, now)
	// Replay must not consult origination admission.
	deps.jurisdictions = mock.NewOriginationJurisdictions(t)

	decisionID := model.DecisionID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	expiresAt := now.Add(time.Hour)

	input := CreateContractInput{
		DecisionID:         decisionID,
		ExternalCustomerID: externalCustomerID,
		TenantID:           tenantID,
		ContextID:          contextID,
	}
	existing := model.Contract{
		ID:                    contractID,
		DecisionID:            decisionID,
		CustomerID:            customerID,
		MerchantProfileID:     merchantProfileID,
		Status:                model.PaymentPendingContractStatus,
		Currency:              model.IRRCurrency,
		PayableNowAmountMinor: 100_000,
		ExpiresAt:             expiresAt,
	}
	first := model.Installment{
		ID:                   installmentID,
		DecisionID:           decisionID,
		InstallmentOrder:     1,
		Currency:             model.IRRCurrency,
		PrincipalAmountMinor: 100_000,
		TotalAmountMinor:     100_000,
		Status:               model.OfferedInstallmentStatus,
	}
	attempt := matchingCheckoutPaymentAttempt(input, existing, first, "https://bnpl.example/payments/return")

	v1 := checkoutv1.New().Ref()
	deps.expectDecisionPolicyConn(ctx, decisionID, v1.ID, v1.Version)
	deps.expectPaymentPendingTxBindings(ctx)

	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.allocationTx.EXPECT().
		LockByDecisionID(ctx, decisionID).
		Return(model.CheckoutDecisionAllocation{
			DecisionID:                  decisionID,
			CustomerID:                  customerID,
			MerchantProfileID:           merchantProfileID,
			Currency:                    model.IRRCurrency,
			RequestedAmountMinor:        100_000,
			ApprovedFinancedAmountMinor: 100_000,
			InstallmentCount:            1,
			ExpiresAt:                   expiresAt,
		}, nil).
		Once()
	deps.installmentTx.EXPECT().
		LockByDecisionID(ctx, decisionID).
		Return([]model.Installment{first}, nil).
		Once()
	deps.contractTx.EXPECT().
		GetByDecisionID(ctx, decisionID).
		Return(existing, nil).
		Once()
	deps.installmentChargeTx.EXPECT().
		ListByInstallmentIDs(ctx, []model.InstallmentID{installmentID}).
		Return(nil, nil).
		Once()
	deps.paymentAttemptTx.EXPECT().
		GetByContractIDAndPurposeWithItems(
			ctx,
			contractID,
			model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		).
		Return(attempt, nil).
		Once()

	uc := deps.newUseCase(t)
	output, err := uc.PrepareCheckoutContract(ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateContractResultAlreadyExists, output.Result)
	require.Equal(t, contractID, output.Contract.ID)
	require.Equal(t, attempt.ID, output.PaymentAttempt.ID)
}

func TestPrepareCheckoutContract_PaymentBased_RaceLoserReplaysWithoutRequiringOriginationAgain(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newOriginationDeps(t, now)

	jurisdictions := mock.NewOriginationJurisdictions(t)
	jurisdictions.EXPECT().
		RequireOriginationEnabled(model.IRJurisdiction).
		Return(nil).
		Once()
	deps.jurisdictions = jurisdictions

	decisionID := model.DecisionID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	creditAccountID := model.CreditAccountID(uuid.New())
	contractID := model.ContractID(uuid.New())
	tenantID := model.TenantID(uuid.New())
	contextID := model.ContextID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	expiresAt := now.Add(time.Hour)

	input := CreateContractInput{
		DecisionID:         decisionID,
		ExternalCustomerID: externalCustomerID,
		TenantID:           tenantID,
		ContextID:          contextID,
	}
	allocation := model.CheckoutDecisionAllocation{
		DecisionID:                  decisionID,
		CheckoutKey:                 "checkout-key",
		CustomerID:                  customerID,
		MerchantProfileID:           merchantProfileID,
		Currency:                    model.IRRCurrency,
		RequestedAmountMinor:        100_000,
		ApprovedFinancedAmountMinor: 100_000,
		InstallmentCount:            1,
		ExpiresAt:                   expiresAt,
	}
	first := model.Installment{
		ID:                   installmentID,
		DecisionID:           decisionID,
		InstallmentOrder:     1,
		Currency:             model.IRRCurrency,
		PrincipalAmountMinor: 100_000,
		TotalAmountMinor:     100_000,
		Status:               model.OfferedInstallmentStatus,
	}
	winner := model.Contract{
		ID:                    contractID,
		DecisionID:            decisionID,
		CustomerID:            customerID,
		MerchantProfileID:     merchantProfileID,
		Status:                model.PaymentPendingContractStatus,
		Currency:              model.IRRCurrency,
		PayableNowAmountMinor: 100_000,
		ExpiresAt:             expiresAt,
	}
	attempt := matchingCheckoutPaymentAttempt(input, winner, first, "https://bnpl.example/payments/return")

	v1 := checkoutv1.New().Ref()
	deps.expectDecisionPolicyConn(ctx, decisionID, v1.ID, v1.Version)
	deps.expectPaymentPendingTxBindings(ctx)

	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.allocationTx.EXPECT().LockByDecisionID(ctx, decisionID).Return(allocation, nil).Once()
	deps.installmentTx.EXPECT().LockByDecisionID(ctx, decisionID).Return([]model.Installment{first}, nil).Once()
	deps.contractTx.EXPECT().
		GetByDecisionID(ctx, decisionID).
		Return(model.Contract{}, cerr.ResourceNotFound(errors.New("missing"))).
		Once()
	deps.merchantProfile.EXPECT().Tx(deps.tx).Return(deps.merchantProfileTx).Once()
	deps.merchantProfileTx.EXPECT().
		GetByID(ctx, merchantProfileID).
		Return(model.MerchantProfile{ID: merchantProfileID, Jurisdiction: model.IRJurisdiction}, nil).
		Once()
	deps.creditAccountTx.EXPECT().
		LockByCustomerIDAndCurrency(ctx, customerID, model.IRRCurrency).
		Return(model.CreditAccount{
			ID:               creditAccountID,
			CustomerID:       customerID,
			Currency:         model.IRRCurrency,
			LimitAmountMinor: 500_000,
			Status:           model.ActiveCreditAccountStatus,
		}, nil).
		Once()
	deps.contractTx.EXPECT().
		Create(
			ctx,
			decisionID,
			allocation.CheckoutKey,
			customerID,
			creditAccountID,
			merchantProfileID,
			(*model.MerchantBranchProfileID)(nil),
			model.IRRCurrency,
			allocation.RequestedAmountMinor,
			allocation.ApprovedFinancedAmountMinor,
			allocation.UpfrontAmountMinor,
			allocation.InstallmentCount,
			uint64(100_000),
			allocation.ApprovedFinancedAmountMinor,
			model.PaymentPendingContractStatus,
			allocation.ExpiresAt,
			now,
			now,
		).
		Return(model.Contract{}, false, nil).
		Once()
	deps.contractTx.EXPECT().
		GetByDecisionID(ctx, decisionID).
		Return(winner, nil).
		Once()
	deps.installmentChargeTx.EXPECT().
		ListByInstallmentIDs(ctx, []model.InstallmentID{installmentID}).
		Return(nil, nil).
		Once()
	deps.paymentAttemptTx.EXPECT().
		GetByContractIDAndPurposeWithItems(
			ctx,
			contractID,
			model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		).
		Return(attempt, nil).
		Once()

	uc := deps.newUseCase(t)
	output, err := uc.PrepareCheckoutContract(ctx, input)
	require.NoError(t, err)
	require.Equal(t, CreateContractResultAlreadyExists, output.Result)
	require.Equal(t, contractID, output.Contract.ID)
}

func TestPrepareCheckoutContract_MerchantFunding_NewContractBlockedWhenOriginationDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newOriginationDeps(t, now)

	blocked := mock.NewOriginationJurisdictions(t)
	blocked.EXPECT().
		RequireOriginationEnabled(model.IRJurisdiction).
		Return(domaincapability.ErrOriginationNotEnabled).
		Once()
	deps.jurisdictions = blocked

	decisionID := model.DecisionID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())

	v2 := checkoutv2.New().Ref()
	deps.expectDecisionPolicyConn(ctx, decisionID, v2.ID, v2.Version)
	deps.expectMerchantFundingTxBindings(ctx)

	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().
		GetByDecisionID(ctx, decisionID).
		Return(model.Contract{}, cerr.ResourceNotFound(errors.New("missing"))).
		Once()
	deps.decisionTx.EXPECT().
		LockLiveCheckoutDecisionByID(ctx, decisionID, now).
		Return(model.Decision{
			ID:            decisionID,
			PolicyID:      v2.ID,
			PolicyVersion: v2.Version,
		}, nil).
		Once()
	deps.allocationTx.EXPECT().
		LockByDecisionID(ctx, decisionID).
		Return(model.CheckoutDecisionAllocation{
			DecisionID:                  decisionID,
			CustomerID:                  customerID,
			MerchantProfileID:           merchantProfileID,
			Currency:                    model.IRRCurrency,
			RequestedAmountMinor:        100_000,
			ApprovedFinancedAmountMinor: 100_000,
			InstallmentCount:            3,
			ExpiresAt:                   now.Add(time.Hour),
		}, nil).
		Once()
	deps.merchantProfile.EXPECT().Tx(deps.tx).Return(deps.merchantProfileTx).Once()
	deps.merchantProfileTx.EXPECT().
		GetByID(ctx, merchantProfileID).
		Return(model.MerchantProfile{
			ID:           merchantProfileID,
			Jurisdiction: model.IRJurisdiction,
		}, nil).
		Once()

	uc := deps.newUseCase(t)
	_, err := uc.PrepareCheckoutContract(ctx, CreateContractInput{
		DecisionID:         decisionID,
		ExternalCustomerID: externalCustomerID,
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeInternalError))
	require.False(t, cerr.IsCode(err, cerr.CodeInvalidArgument))
}

func TestPrepareCheckoutContract_MerchantFunding_ExistingContractResumeAfterOriginationDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newOriginationDeps(t, now)
	deps.jurisdictions = mock.NewOriginationJurisdictions(t)

	decisionID := model.DecisionID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	customerID := model.CustomerID(uuid.New())
	contractID := model.ContractID(uuid.New())

	v2 := checkoutv2.New().Ref()
	deps.expectDecisionPolicyConn(ctx, decisionID, v2.ID, v2.Version)
	deps.expectMerchantFundingTxBindings(ctx)

	existing := model.Contract{
		ID:         contractID,
		DecisionID: decisionID,
		CustomerID: customerID,
		Status:     model.PaymentPendingContractStatus,
		Currency:   model.IRRCurrency,
	}

	deps.customerProfileTx.EXPECT().
		LockByExternalCustomerID(ctx, externalCustomerID).
		Return(model.CustomerProfile{ID: customerID, ExternalCustomerID: externalCustomerID}, nil).
		Once()
	deps.contractTx.EXPECT().
		GetByDecisionID(ctx, decisionID).
		Return(existing, nil).
		Once()

	uc := deps.newUseCase(t)
	output, err := uc.PrepareCheckoutContract(ctx, CreateContractInput{
		DecisionID:         decisionID,
		ExternalCustomerID: externalCustomerID,
	})
	require.NoError(t, err)
	require.Equal(t, CreateContractResultAlreadyExists, output.Result)
	require.Equal(t, contractID, output.Contract.ID)
	require.False(t, output.PaymentRequired)
}

func TestSubmitPreparedCheckoutFulfillment_MerchantFunding_ResumeAfterOriginationDisabled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		10,
		16,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newOriginationDeps(t, now)
	// Servicing an existing merchant-funding Contract must not consult origination.
	deps.jurisdictions = mock.NewOriginationJurisdictions(t)

	decisionID := model.DecisionID(uuid.New())
	contractID := model.ContractID(uuid.New())
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

	v2 := checkoutcatalog.TrustedRef()
	deps.expectDecisionPolicyConn(ctx, decisionID, v2.ID, v2.Version)

	deps.pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, deps.conn)
		},
	).Twice()
	deps.contract.EXPECT().Conn(deps.conn).Return(deps.contractCQ).Twice()
	deps.contractCQ.EXPECT().GetByID(ctx, contractID).Return(contractRow, nil).Twice()

	uc := deps.newUseCase(t)
	output, err := uc.SubmitPreparedCheckoutFulfillment(ctx, CreateContractOutput{
		Result:          CreateContractResultAlreadyExists,
		Contract:        contractRow,
		PaymentRequired: false,
	})
	require.NoError(t, err)
	require.False(t, output.PaymentRequired)
	require.Equal(t, contractID, output.Contract.ID)
}
