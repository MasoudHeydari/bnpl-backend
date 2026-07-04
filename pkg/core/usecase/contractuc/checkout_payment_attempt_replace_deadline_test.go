package contractuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestReplaceTerminalCheckoutPaymentAttempt_RejectsWhenClockAdvancesDuringLockWait(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deadline := time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)
	start := deadline.Add(-time.Second)
	clk := &adapterclock.Fixed{Time: start, Location: time.UTC}

	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())
	installmentID := model.InstallmentID(uuid.New())
	successURL := "https://bnpl.example/ok"
	failureURL := "https://bnpl.example/fail"
	cancelURL := "https://bnpl.example/cancel"

	contractRow := model.Contract{
		ID:        contractID,
		Status:    model.PaymentPendingContractStatus,
		ExpiresAt: deadline,
	}
	attempt := model.PaymentAttempt{
		ID:               attemptID,
		ContractID:       contractID,
		InstallmentID:    installmentID,
		State:            model.FailedPaymentAttemptState,
		AmountMinor:      1000,
		Currency:         model.IRRCurrency,
		Channel:          model.GatewayPaymentChannel,
		IdempotencyKey:   "idem-old",
		SuccessReturnURL: &successURL,
		FailureReturnURL: &failureURL,
		CancelReturnURL:  &cancelURL,
		Items: []model.PaymentAttemptItem{{
			ItemType:    model.PrincipalPaymentAttemptItemType,
			SourceID:    installmentID.UUID(),
			AmountMinor: 1000,
			Effect:      model.DebitPaymentAttemptItemEffect,
		}},
	}

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	contractRepo := mock.NewContract(t)
	contractTx := mock.NewContractTxQueryer(t)
	paymentAttemptRepo := mock.NewPaymentAttempt(t)
	paymentAttemptTx := mock.NewPaymentAttemptTxQueryer(t)

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
	paymentAttemptRepo.EXPECT().Tx(tx).Return(paymentAttemptTx).Once()

	// Simulate lock wait: domain clock advances past the BNPL deadline before unlock.
	contractTx.EXPECT().LockByID(ctx, contractID).RunAndReturn(
		func(context.Context, model.ContractID) (model.Contract, error) {
			clk.Time = deadline.Add(2 * time.Second)
			return contractRow, nil
		},
	).Once()

	uc := &UseCase{
		pool:           pool,
		contract:       contractRepo,
		paymentAttempt: paymentAttemptRepo,
		clock:          clk,
	}

	_, err := uc.replaceTerminalCheckoutPaymentAttempt(ctx, contractRow, attempt)
	require.Error(t, err)
	require.True(t, cerr.IsCode(err, cerr.CodeConflict))
	var appErr *cerr.Error
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, cerr.MessageKey("error.checkout_payment_session_expired"), appErr.MessageKey)
}
