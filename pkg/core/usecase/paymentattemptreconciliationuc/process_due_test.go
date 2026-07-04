package paymentattemptreconciliationuc

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

type processDueDeps struct {
	pool             *mock.Pool
	conn             *mock.Conn
	tx               *mock.Tx
	paymentAttempt   *mock.PaymentAttempt
	paymentAttemptTx *mock.PaymentAttemptTxQueryer
	contract         *mock.Contract
	contractTx       *mock.ContractTxQueryer
	outbox           *mock.Outbox
	outboxTx         *mock.OutboxTxQueryer
	idGenerator      *mock.IdentifierGenerator
	clock            adapterclock.Fixed
}

func newProcessDueDeps(t *testing.T, now time.Time) processDueDeps {
	t.Helper()

	return processDueDeps{
		pool:             mock.NewPool(t),
		conn:             mock.NewConn(t),
		tx:               mock.NewTx(t),
		paymentAttempt:   mock.NewPaymentAttempt(t),
		paymentAttemptTx: mock.NewPaymentAttemptTxQueryer(t),
		contract:         mock.NewContract(t),
		contractTx:       mock.NewContractTxQueryer(t),
		outbox:           mock.NewOutbox(t),
		outboxTx:         mock.NewOutboxTxQueryer(t),
		idGenerator:      mock.NewIdentifierGenerator(t),
		clock:            adapterclock.NewFixed(now, time.UTC),
	}
}

func (d processDueDeps) newUseCase() *UseCase {
	uc, err := newUseCase(
		d.pool,
		d.paymentAttempt,
		d.contract,
		d.outbox,
		d.idGenerator,
		"bnpl.installment_payment_reconciliation_requests",
		"bnpl.audit.events.v1",
		10,
		5*time.Minute,
		time.Minute,
		time.Second,
		time.Minute,
		5,
		d.clock,
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d processDueDeps) expectConnTx(ctx context.Context) {
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
}

func TestProcessDue_NoEligibleAttempts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()

	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx, now, testifymock.Anything, now, 10,
	).Return(nil, nil).Once()
	deps.paymentAttemptTx.EXPECT().ClaimExpiredSubmittedForReconciliation(
		ctx,
		now,
		time.Minute,
		model.PaymentResultTimeoutPaymentReconciliationReason,
		now,
		testifymock.Anything,
		now,
		10,
	).Return(nil, nil).Once()
	deps.paymentAttemptTx.EXPECT().ClaimStaleCreatedForReconciliation(
		ctx,
		now.Add(-5*time.Minute),
		model.PaymentSessionCreationTimeoutPaymentReconciliationReason,
		now,
		testifymock.Anything,
		now,
		10,
	).Return(nil, nil).Once()

	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_QueuesReconciliationAfterStaleCreatedClaim(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	attemptID := model.PaymentAttemptID(uuid.New())
	reason := model.PaymentSessionCreationTimeoutPaymentReconciliationReason
	claimed := model.PaymentAttempt{
		ID:                   attemptID,
		ContractID:           model.ContractID(uuid.New()),
		InstallmentID:        model.InstallmentID(uuid.New()),
		State:                model.ReconciliationRequiredPaymentAttemptState,
		Purpose:              model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationReason: &reason,
		AmountMinor:          1000,
		Currency:             model.IRRCurrency,
		Channel:              model.GatewayPaymentChannel,
	}

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx, now, testifymock.Anything, now, 10,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimExpiredSubmittedForReconciliation(
		ctx,
		now,
		time.Minute,
		model.PaymentResultTimeoutPaymentReconciliationReason,
		now,
		testifymock.Anything,
		now,
		10,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimStaleCreatedForReconciliation(
		ctx,
		now.Add(-5*time.Minute),
		model.PaymentSessionCreationTimeoutPaymentReconciliationReason,
		now,
		testifymock.Anything,
		now,
		10,
	).Return([]model.PaymentAttempt{claimed}, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("audit-event-id").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(claimed, nil).Once()
	deps.contract.EXPECT().Tx(deps.tx).Return(deps.contractTx).Once()
	deps.contractTx.EXPECT().GetByID(ctx, claimed.ContractID).Return(model.Contract{
		ID:         claimed.ContractID,
		CustomerID: model.CustomerID(uuid.New()),
	}, nil).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()
	deps.paymentAttemptTx.EXPECT().RecordReconciliationRequest(
		ctx,
		attemptID,
		0,
		now,
		testifymock.Anything,
		(*string)(nil),
		now,
	).Return(claimed, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("audit-event-id-2").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	require.NoError(t, uc.ProcessDue(ctx))
}
