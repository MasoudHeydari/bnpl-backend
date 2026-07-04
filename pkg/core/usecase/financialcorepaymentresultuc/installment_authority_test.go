package financialcorepaymentresultuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestResolveAuthoritativeInstallmentResult_ProcessingDoesNotMutateOutcome(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	intentID := "fc-intent-processing"
	tenantID := "tenant-1"
	contextID := "context-1"
	attemptID := model.PaymentAttemptID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())

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
		InstallmentID:            installmentID,
		Purpose:                  model.InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                    model.SubmittedPaymentAttemptState,
		TenantID:                 &tenantID,
		ContextID:                &contextID,
		FinancialPaymentIntentID: &intentID,
	}, nil).Once()

	sourceVersion := int64(1)
	checkoutClient.EXPECT().GetPaymentStatus(ctx, extcheckoutpayment.GetPaymentStatusInput{
		SourceType:      financialCorePaymentAttemptSourceType,
		SourceID:        attemptID.String(),
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

	_, err := uc.resolveAuthoritativeInstallmentResult(ctx, ProcessInput{
		PaymentIntentID: intentID,
		PaymentStatus:   model.SucceededFinancialPaymentResultStatus, // stale/wake-up claim must not win
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), string(extcheckoutpayment.ProcessingPaymentStatus))
}

func TestResolveAuthoritativeInstallmentResult_SucceededUsesFCAmount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	intentID := "fc-intent-succeeded"
	tenantID := "tenant-1"
	contextID := "context-1"
	attemptID := model.PaymentAttemptID(uuid.New())
	contractID := model.ContractID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	paidAt := time.Date(
		2026,
		9,
		21,
		10,
		0,
		0,
		0,
		time.UTC,
	)

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
		InstallmentID:            installmentID,
		Purpose:                  model.InstallmentRepaymentFinancialCorePaymentPurpose,
		TenantID:                 &tenantID,
		ContextID:                &contextID,
		FinancialPaymentIntentID: &intentID,
	}, nil).Once()

	checkoutClient.EXPECT().GetPaymentStatus(ctx, testifymock.Anything).Return(extcheckoutpayment.GetPaymentStatusResult{
		PaymentIntentID: intentID,
		Status:          extcheckoutpayment.SucceededPaymentStatus,
		AmountMinor:     20,
		Currency:        "IRR",
		CompletedAt:     paidAt,
	}, nil).Once()

	uc := &UseCase{
		pool:                  pool,
		paymentAttempt:        paymentAttempt,
		checkoutPaymentClient: checkoutClient,
	}

	out, err := uc.resolveAuthoritativeInstallmentResult(ctx, ProcessInput{
		PaymentIntentID: intentID,
		PaymentStatus:   model.FailedFinancialPaymentResultStatus,
		PaidAmountMinor: 999,
	})
	require.NoError(t, err)
	require.Equal(t, model.SucceededFinancialPaymentResultStatus, out.PaymentStatus)
	require.Equal(t, uint64(20), out.PaidAmountMinor)
	require.Equal(t, model.IRRCurrency, out.Currency)
	require.Equal(t, paidAt.UTC(), out.PaidAt)
	require.Equal(t, contractID, out.ContractID)
	require.Equal(t, installmentID, out.InstallmentID)
}
