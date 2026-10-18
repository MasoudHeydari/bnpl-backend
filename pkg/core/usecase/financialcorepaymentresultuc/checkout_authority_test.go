package financialcorepaymentresultuc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestResolveAuthoritativeCheckoutResult_ProcessingIsNotFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptCQ := mock.NewPaymentAttemptConnQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)

	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	intentID := "pi-checkout-1"
	sourceVersion := int64(1)

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Once()
	paymentAttempt.EXPECT().Conn(conn).Return(paymentAttemptCQ).Once()
	paymentAttemptCQ.EXPECT().GetByFinancialPaymentIntentID(ctx, intentID).Return(model.PaymentAttempt{
		ID:                       attemptID,
		ContractID:               contractID,
		Purpose:                  model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                    model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID: &intentID,
	}, nil).Once()

	checkoutClient.EXPECT().GetPaymentStatus(ctx, extcheckoutpayment.GetPaymentStatusInput{
		SourceType:      financialCoreBNPLContractType,
		SourceID:        contractID.String(),
		SourceVersion:   &sourceVersion,
		PaymentIntentID: intentID,
	}).Return(extcheckoutpayment.GetPaymentStatusResult{
		PaymentIntentID: intentID,
		Status:          extcheckoutpayment.ProcessingPaymentStatus,
	}, nil).Once()

	uc := &UseCase{
		pool:                  pool,
		paymentAttempt:        paymentAttempt,
		checkoutPaymentClient: checkoutClient,
	}

	_, err := uc.resolveAuthoritativeCheckoutResult(ctx, ProcessInput{
		PaymentIntentID: intentID,
		PaymentStatus:   model.SucceededFinancialPaymentResultStatus,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), string(extcheckoutpayment.ProcessingPaymentStatus))
}
