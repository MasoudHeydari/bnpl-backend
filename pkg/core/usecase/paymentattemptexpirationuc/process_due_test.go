package paymentattemptexpirationuc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

func TestProcessDue_ExpiresFullyStaleCreatedAttempt(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 3, 15, 12, 20, 0, 0, time.UTC)
	createdAt := now.Add(-model.InstallmentRepaymentPaymentSessionTTL - time.Minute)
	leaseUntil := now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptTx := mock.NewPaymentAttemptTxQueryer(t)
	outbox := mock.NewOutbox(t)
	outboxTx := mock.NewOutboxTxQueryer(t)
	idGenerator := mock.NewIdentifierGenerator(t)
	logger := mock.NewLogger(t)
	clk := adapterclock.NewFixed(now, time.UTC)

	attemptID := model.PaymentAttemptID(uuid.New())
	candidate := model.PaymentAttempt{
		ID:                               attemptID,
		ContractID:                       model.ContractID(uuid.New()),
		InstallmentID:                    model.InstallmentID(uuid.New()),
		Purpose:                          model.InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                            model.CreatedPaymentAttemptState,
		CreatedAt:                        createdAt,
		FinancialSessionCreateLeaseUntil: &leaseUntil,
		AmountMinor:                      1000,
		Currency:                         model.IRRCurrency,
		Channel:                          model.GatewayPaymentChannel,
	}
	expiredAt := now
	expired := candidate
	expired.State = model.ExpiredPaymentAttemptState
	expired.ExpiredAt = &expiredAt

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Twice()
	conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		},
	).Twice()

	paymentAttempt.EXPECT().Tx(tx).Return(paymentAttemptTx).Twice()
	outbox.EXPECT().Tx(tx).Return(outboxTx).Once()

	paymentAttemptTx.EXPECT().ClaimCandidatesForSessionCreationExpiration(
		ctx,
		now,
		model.FinancialSessionCreateExpirationSafetyInterval,
		1,
	).Return([]model.PaymentAttempt{candidate}, nil).Once()
	paymentAttemptTx.EXPECT().ClaimCandidatesForSessionCreationExpiration(
		ctx,
		now,
		model.FinancialSessionCreateExpirationSafetyInterval,
		1,
	).Return(nil, nil).Once()

	paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(candidate, nil).Once()
	paymentAttemptTx.EXPECT().MarkExpired(
		ctx,
		attemptID,
		model.CreatedPaymentAttemptState,
		now,
		now,
	).Return(expired, nil).Once()

	idGenerator.EXPECT().Generate().Return(uuid.NewString()).Once()
	outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	logger.EXPECT().With(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(logger).Once()
	logger.EXPECT().Info("payment attempt session creation expiration cycle completed").Once()

	uc, err := New(
		pool,
		paymentAttempt,
		outbox,
		idGenerator,
		"bnpl.audit.events.v1",
		10,
		0,
		clk,
		logger,
	)
	require.NoError(t, err)
	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_SkipsActiveLease(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 3, 15, 12, 20, 0, 0, time.UTC)
	createdAt := now.Add(-model.InstallmentRepaymentPaymentSessionTTL - time.Minute)
	leaseUntil := now.Add(10 * time.Second)

	pool := mock.NewPool(t)
	conn := mock.NewConn(t)
	tx := mock.NewTx(t)
	paymentAttempt := mock.NewPaymentAttempt(t)
	paymentAttemptTx := mock.NewPaymentAttemptTxQueryer(t)
	outbox := mock.NewOutbox(t)
	idGenerator := mock.NewIdentifierGenerator(t)
	logger := mock.NewLogger(t)
	clk := adapterclock.NewFixed(now, time.UTC)

	attemptID := model.PaymentAttemptID(uuid.New())
	candidate := model.PaymentAttempt{
		ID:                               attemptID,
		ContractID:                       model.ContractID(uuid.New()),
		InstallmentID:                    model.InstallmentID(uuid.New()),
		Purpose:                          model.InstallmentRepaymentFinancialCorePaymentPurpose,
		State:                            model.CreatedPaymentAttemptState,
		CreatedAt:                        createdAt,
		FinancialSessionCreateLeaseUntil: &leaseUntil,
		AmountMinor:                      1000,
		Currency:                         model.IRRCurrency,
		Channel:                          model.GatewayPaymentChannel,
	}

	pool.EXPECT().Conn(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.ConnHandler) error {
			return handler(ctx, conn)
		},
	).Twice()
	conn.EXPECT().Tx(ctx, testifymock.Anything).RunAndReturn(
		func(ctx context.Context, handler repo.TxHandler) error {
			return handler(ctx, tx)
		},
	).Twice()

	paymentAttempt.EXPECT().Tx(tx).Return(paymentAttemptTx).Twice()

	paymentAttemptTx.EXPECT().ClaimCandidatesForSessionCreationExpiration(
		ctx,
		now,
		model.FinancialSessionCreateExpirationSafetyInterval,
		1,
	).Return([]model.PaymentAttempt{candidate}, nil).Once()
	paymentAttemptTx.EXPECT().ClaimCandidatesForSessionCreationExpiration(
		ctx,
		now,
		model.FinancialSessionCreateExpirationSafetyInterval,
		1,
	).Return(nil, nil).Once()

	paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(candidate, nil).Once()

	logger.EXPECT().With(testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything, testifymock.Anything).Return(logger).Once()
	logger.EXPECT().Info("payment attempt session creation expiration cycle completed").Once()

	uc, err := New(
		pool,
		paymentAttempt,
		outbox,
		idGenerator,
		"bnpl.audit.events.v1",
		10,
		0,
		clk,
		logger,
	)
	require.NoError(t, err)
	require.NoError(t, uc.ProcessDue(ctx))
}
