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

func TestResolveAuthoritativeCheckoutResult_ProcessingIgnoresStaleSucceededWakeup(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	intentID := "fc-checkout-intent-processing"
	tenantID := "tenant-1"
	contextID := "context-1"
	attemptID := model.PaymentAttemptID(uuid.New())
	contractID := model.ContractID(uuid.New())

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptCQ := mock.NewPaymentAttemptConnQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)

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
		TenantID:                 &tenantID,
		ContextID:                &contextID,
		FinancialPaymentIntentID: &intentID,
	}, nil).Once()

	checkoutClient.EXPECT().GetPaymentStatus(ctx, extcheckoutpayment.GetPaymentStatusInput{
		TenantID:        tenantID,
		ContextID:       contextID,
		SourceService:   financialCoreBNPLSourceService,
		SourceType:      financialCoreBNPLContractType,
		SourceID:        contractID.String(),
		SourceVersion:   1,
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
