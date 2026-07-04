package contractuc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	checkoutcatalog "github.com/parstis/bnpl-backend/pkg/core/policy/checkout/catalog"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestEnsureMerchantEntitlement_PersistsEntitlementAndReceivableWithExternalIDs(t *testing.T) {
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
	contractTx := mock.NewContractTxQueryer(t)
	decisionRepo := mock.NewDecision(t)
	decisionCQ := mock.NewDecisionConnQueryer(t)
	installmentRepo := mock.NewInstallment(t)
	installmentCQ := mock.NewInstallmentConnQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)
	checkoutPolicies := mustTestCheckoutRegistry(t)
	policyRef := checkoutcatalog.DefaultRef()

	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	customerInternalID := model.CustomerID(uuid.New())
	identitySubjectID := model.IdentitySubjectID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	merchantID := model.MerchantID(uuid.New())
	paymentIntentID := "fc-intent-activation"
	installmentID := model.InstallmentID(uuid.New())

	// purchase=100, financed=80, upfront=20, first principal=20
	// activation cash=40, remaining receivable=60
	contractRow := model.Contract{
		ID:                            contractID,
		CustomerID:                    customerInternalID,
		MerchantProfileID:             merchantProfileID,
		DecisionID:                    decisionID,
		Status:                        model.ActiveContractStatus,
		Currency:                      model.IRRCurrency,
		RequestedAmountMinor:          100,
		ApprovedFinancedAmountMinor:   80,
		UpfrontAmountMinor:            20,
		PayableNowAmountMinor:         40,
		RemainingPrincipalAmountMinor: 60,
		FinancialPaymentIntentID:      &paymentIntentID,
	}

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	contractRepo.EXPECT().Conn(conn).Return(contractCQ).Once()
	contractCQ.EXPECT().GetByID(ctx, contractID).Return(contractRow, nil).Once()
	decisionRepo.EXPECT().Conn(conn).Return(decisionCQ).Once()
	decisionCQ.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{
		ID:            decisionID,
		PolicyID:      policyRef.ID,
		PolicyVersion: policyRef.Version,
	}, nil).Once()
	customerProfile.EXPECT().Conn(conn).Return(customerCQ).Once()
	customerCQ.EXPECT().GetByID(ctx, customerInternalID).Return(model.CustomerProfile{
		ID:                customerInternalID,
		IdentitySubjectID: identitySubjectID,
	}, nil).Once()
	merchantProfile.EXPECT().Conn(conn).Return(merchantCQ).Once()
	merchantCQ.EXPECT().GetByID(ctx, merchantProfileID).Return(model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: merchantID,
	}, nil).Once()
	installmentRepo.EXPECT().Conn(conn).Return(installmentCQ).Once()
	installmentCQ.EXPECT().ListByContractID(ctx, contractID).Return([]model.Installment{
		{
			ID:                   installmentID,
			ContractID:           &contractID,
			InstallmentOrder:     1,
			PrincipalAmountMinor: 20,
			TotalAmountMinor:     40,
		},
		{
			ID:                   model.InstallmentID(uuid.New()),
			ContractID:           &contractID,
			InstallmentOrder:     2,
			PrincipalAmountMinor: 20,
		},
	}, nil).Once()

	checkoutClient.EXPECT().RecordMerchantEntitlement(ctx, testifymock.MatchedBy(
		func(in extcheckoutpayment.RecordMerchantEntitlementInput) bool {
			return in.MerchantID == merchantID.String() &&
				in.CustomerSubjectID == identitySubjectID.String() &&
				in.MerchantID != merchantProfileID.String() &&
				in.CustomerSubjectID != customerInternalID.String() &&
				in.CollectedCashAmountMinor == 40 &&
				in.ReceivableAmountMinor == 60 &&
				in.MerchantPayableAmountMinor == 100 &&
				in.CollectedCashPaymentIntentID == paymentIntentID &&
				in.SourceID == contractID.String()
		},
	)).Return(extcheckoutpayment.RecordMerchantEntitlementResult{
		MerchantEntitlementID: "fc-entitlement-1",
		ReceivableID:          "fc-receivable-60",
	}, nil).Once()

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
	contractRepo.EXPECT().Tx(tx).Return(contractTx).Once()
	contractTx.EXPECT().BindMerchantEntitlementIfAbsent(ctx, contractID, "fc-entitlement-1", now).
		Return(contractRow, nil).Once()
	contractTx.EXPECT().BindReceivableIfAbsent(ctx, contractID, "fc-receivable-60", now).
		Return(contractRow, nil).Once()

	uc := &UseCase{
		pool:                  pool,
		customerProfile:       customerProfile,
		merchantProfile:       merchantProfile,
		decision:              decisionRepo,
		contract:              contractRepo,
		checkoutPolicies:      checkoutPolicies,
		installment:           installmentRepo,
		checkoutPaymentClient: checkoutClient,
		clock:                 adapterclock.NewFixed(now, time.UTC),
	}

	err := uc.EnsureMerchantEntitlement(ctx, contractID)
	require.NoError(t, err)
}

func TestRecordMerchantEntitlement_ActivationPendingCallsFinancialCoreOnceAndDoesNotRetry(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	fcErr := errors.New("financial core unavailable")

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	customerProfile := mock.NewCustomerProfile(t)
	customerCQ := mock.NewCustomerProfileConnQueryer(t)
	merchantProfile := mock.NewMerchantProfile(t)
	merchantCQ := mock.NewMerchantProfileConnQueryer(t)
	contractRepo := mock.NewContract(t)
	contractCQ := mock.NewContractConnQueryer(t)
	decisionRepo := mock.NewDecision(t)
	decisionCQ := mock.NewDecisionConnQueryer(t)
	installmentRepo := mock.NewInstallment(t)
	installmentCQ := mock.NewInstallmentConnQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)
	checkoutPolicies := mustTestCheckoutRegistry(t)
	policyRef := checkoutcatalog.TrustedRef()

	contractID := model.ContractID(uuid.New())
	decisionID := model.DecisionID(uuid.New())
	customerInternalID := model.CustomerID(uuid.New())
	identitySubjectID := model.IdentitySubjectID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	merchantID := model.MerchantID(uuid.New())

	contractRow := model.Contract{
		ID:                            contractID,
		CustomerID:                    customerInternalID,
		MerchantProfileID:             merchantProfileID,
		DecisionID:                    decisionID,
		Status:                        model.PaymentPendingContractStatus,
		Currency:                      model.IRRCurrency,
		RequestedAmountMinor:          100,
		ApprovedFinancedAmountMinor:   100,
		UpfrontAmountMinor:            0,
		PayableNowAmountMinor:         0,
		RemainingPrincipalAmountMinor: 100,
	}

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	contractRepo.EXPECT().Conn(conn).Return(contractCQ).Once()
	contractCQ.EXPECT().GetByID(ctx, contractID).Return(contractRow, nil).Once()
	decisionRepo.EXPECT().Conn(conn).Return(decisionCQ).Once()
	decisionCQ.EXPECT().GetByID(ctx, decisionID).Return(model.Decision{
		ID:            decisionID,
		PolicyID:      policyRef.ID,
		PolicyVersion: policyRef.Version,
	}, nil).Once()
	customerProfile.EXPECT().Conn(conn).Return(customerCQ).Once()
	customerCQ.EXPECT().GetByID(ctx, customerInternalID).Return(model.CustomerProfile{
		ID:                customerInternalID,
		IdentitySubjectID: identitySubjectID,
	}, nil).Once()
	merchantProfile.EXPECT().Conn(conn).Return(merchantCQ).Once()
	merchantCQ.EXPECT().GetByID(ctx, merchantProfileID).Return(model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: merchantID,
	}, nil).Once()
	installmentRepo.EXPECT().Conn(conn).Return(installmentCQ).Once()
	installmentCQ.EXPECT().ListByContractID(ctx, contractID).Return([]model.Installment{
		{
			ID:                   model.InstallmentID(uuid.New()),
			ContractID:           &contractID,
			InstallmentOrder:     1,
			PrincipalAmountMinor: 34,
			TotalAmountMinor:     34,
		},
		{
			ID:                   model.InstallmentID(uuid.New()),
			ContractID:           &contractID,
			InstallmentOrder:     2,
			PrincipalAmountMinor: 33,
			TotalAmountMinor:     33,
		},
		{
			ID:                   model.InstallmentID(uuid.New()),
			ContractID:           &contractID,
			InstallmentOrder:     3,
			PrincipalAmountMinor: 33,
			TotalAmountMinor:     33,
		},
	}, nil).Once()

	checkoutClient.EXPECT().RecordMerchantEntitlement(ctx, testifymock.MatchedBy(
		func(in extcheckoutpayment.RecordMerchantEntitlementInput) bool {
			return in.MerchantID == merchantID.String() &&
				in.CustomerSubjectID == identitySubjectID.String() &&
				in.CollectedCashAmountMinor == 0 &&
				in.ReceivableAmountMinor == 100 &&
				in.MerchantPayableAmountMinor == 100 &&
				in.CollectedCashPaymentIntentID == "" &&
				in.SourceID == contractID.String()
		},
	)).Return(extcheckoutpayment.RecordMerchantEntitlementResult{}, fcErr).Once()

	uc := &UseCase{
		pool:                  pool,
		customerProfile:       customerProfile,
		merchantProfile:       merchantProfile,
		decision:              decisionRepo,
		contract:              contractRepo,
		checkoutPolicies:      checkoutPolicies,
		installment:           installmentRepo,
		checkoutPaymentClient: checkoutClient,
	}

	_, err := uc.recordMerchantEntitlement(ctx, contractID, model.PaymentPendingContractStatus)
	require.ErrorIs(t, err, fcErr)
}
