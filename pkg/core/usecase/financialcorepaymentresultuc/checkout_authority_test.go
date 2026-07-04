package financialcorepaymentresultuc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

// The FC event is only a wake-up signal: checkout settlement must use the
// contract owned by the persisted PaymentAttempt even if Kafka omits it.
func TestResolveAuthoritativeCheckoutResult_BackfillsContractForSettlementLock(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptCQ := mock.NewPaymentAttemptConnQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)
	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	intentID := "pi-checkout-backfill"
	sourceVersion := int64(1)

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
		return fn(ctx, conn)
	}).Once()
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
		Status:          extcheckoutpayment.FailedPaymentStatus,
	}, nil).Once()
	uc := &UseCase{pool: pool, paymentAttempt: paymentAttempt, checkoutPaymentClient: checkoutClient}

	out, err := uc.resolveAuthoritativeCheckoutResult(ctx, ProcessInput{
		PaymentIntentID: intentID,
		PaymentPurpose:  model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
	})
	require.NoError(t, err)
	require.Equal(t, contractID, out.ContractID)
	require.Equal(t, model.FailedFinancialPaymentResultStatus, out.PaymentStatus)
}

func TestResolveAuthoritativeCheckoutResult_RejectsConflictingContract(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptCQ := mock.NewPaymentAttemptConnQueryer(t)
	checkoutClient := mock.NewCheckoutPaymentClient(t)
	contractID := model.ContractID(uuid.New())
	intentID := "pi-checkout-conflict"

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(func(ctx context.Context, fn repo.ConnHandler) error {
		return fn(ctx, conn)
	}).Once()
	paymentAttempt.EXPECT().Conn(conn).Return(paymentAttemptCQ).Once()
	paymentAttemptCQ.EXPECT().GetByFinancialPaymentIntentID(ctx, intentID).Return(model.PaymentAttempt{
		ID:                       model.PaymentAttemptID(uuid.New()),
		ContractID:               contractID,
		Purpose:                  model.CheckoutFirstInstallmentFinancialCorePaymentPurpose,
		State:                    model.SubmittedPaymentAttemptState,
		FinancialPaymentIntentID: &intentID,
	}, nil).Once()
	uc := &UseCase{pool: pool, paymentAttempt: paymentAttempt, checkoutPaymentClient: checkoutClient}

	_, err := uc.resolveAuthoritativeCheckoutResult(ctx, ProcessInput{
		PaymentIntentID: intentID,
		ContractID:      model.ContractID(uuid.New()),
	})
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
}

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
