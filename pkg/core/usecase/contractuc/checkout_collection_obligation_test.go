package contractuc

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

func TestEnsureCheckoutCollectionObligation_UsesPreRecognitionCashAndExternalIDs(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		9,
		21,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	paymentAttemptRepo := mock.NewPaymentAttempt(t)
	paymentAttemptTx := mock.NewPaymentAttemptTxQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)

	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	customerInternalID := model.CustomerID(uuid.New())
	externalCustomerID := model.ExternalCustomerID(uuid.New())
	merchantProfileID := model.MerchantProfileID(uuid.New())
	merchantID := model.MerchantID(uuid.New())
	expiresAt := now.Add(30 * time.Minute)

	contractRow := model.Contract{
		ID:                    contractID,
		CustomerID:            customerInternalID,
		MerchantProfileID:     merchantProfileID,
		Currency:              model.IRRCurrency,
		PayableNowAmountMinor: 40, // upfront 20 + first principal 20
		ExpiresAt:             expiresAt,
	}
	attempt := model.PaymentAttempt{
		ID:          attemptID,
		ContractID:  contractID,
		Purpose:     model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:       model.CreatedPaymentAttemptState,
		AmountMinor: 40,
		Currency:    model.IRRCurrency,
		Items: []model.PaymentAttemptItem{
			{ItemType: model.UpfrontPaymentAttemptItemType, Effect: model.DebitPaymentAttemptItemEffect, AmountMinor: 20},
			{ItemType: model.PrincipalPaymentAttemptItemType, Effect: model.DebitPaymentAttemptItemEffect, AmountMinor: 20},
		},
	}
	customer := model.CustomerProfile{
		ID:                 customerInternalID,
		ExternalCustomerID: externalCustomerID,
	}
	merchant := model.MerchantProfile{
		ID:         merchantProfileID,
		MerchantID: merchantID,
	}

	checkoutClient.EXPECT().PrepareCollectionObligation(ctx, testifymock.MatchedBy(
		func(in extcheckoutpayment.PrepareCollectionObligationInput) bool {
			return in.Application == extcheckoutpayment.PreRecognitionCashCollectionApplication &&
				in.AmountMinor == 40 &&
				in.MerchantEntitlementID == "" &&
				in.MerchantID == merchantID.String() &&
				in.CustomerID == externalCustomerID.String() &&
				in.MerchantID != merchantProfileID.String() &&
				in.CustomerID != customerInternalID.String() &&
				in.SourceID == contractID.String()
		},
	)).Return(extcheckoutpayment.PrepareCollectionObligationResult{
		CollectionObligationID: "fc-obligation-activation",
		OutstandingAmountMinor: 40,
		Currency:               "IRR",
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
	paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptTx).Once()
	paymentAttemptTx.EXPECT().LockByIDWithItems(ctx, attemptID).Return(attempt, nil).Once()

	bound := attempt
	obligationID := "fc-obligation-activation"
	bound.FinancialCollectionObligationID = &obligationID
	paymentAttemptTx.EXPECT().BindCollectionObligationIfAbsent(ctx, attemptID, obligationID, now).
		Return(bound, nil).Once()

	uc := &UseCase{
		pool:                  pool,
		paymentAttempt:        paymentAttemptRepo,
		checkoutPaymentClient: checkoutClient,
		clock:                 adapterclock.NewFixed(now, time.UTC),
	}

	out, err := uc.ensureCheckoutCollectionObligation(
		ctx,
		contractRow,
		attempt,
		customer,
		merchant,
	)
	require.NoError(t, err)
	require.NotNil(t, out.FinancialCollectionObligationID)
	require.Equal(t, obligationID, *out.FinancialCollectionObligationID)
}
