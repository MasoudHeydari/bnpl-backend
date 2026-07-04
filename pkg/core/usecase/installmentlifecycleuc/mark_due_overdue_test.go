package installmentlifecycleuc

import (
	"context"
	"errors"
	"testing"
	"time"

	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
	"github.com/parstis/bnpl-backend/pkg/core/repo"
)

type markDueOverdueDeps struct {
	pool                       *mock.Pool
	conn                       *mock.Conn
	tx                         *mock.Tx
	decision                   *mock.Decision
	checkoutDecisionAllocation *mock.CheckoutDecisionAllocation
	contract                   *mock.Contract
	installment                *mock.Installment
	installmentCharge          *mock.InstallmentCharge
	paymentAttempt             *mock.PaymentAttempt
	inbox                      *mock.Inbox
	outbox                     *mock.Outbox
	outboxTx                   *mock.OutboxTxQueryer
	installmentTx              *mock.InstallmentTxQueryer
	idGenerator                *mock.IDGenerator
	clock                      adapterclock.Fixed
}

func newMarkDueOverdueDeps(t *testing.T, now time.Time) markDueOverdueDeps {
	t.Helper()

	deps := markDueOverdueDeps{
		pool:                       mock.NewPool(t),
		conn:                       mock.NewConn(t),
		tx:                         mock.NewTx(t),
		decision:                   mock.NewDecision(t),
		checkoutDecisionAllocation: mock.NewCheckoutDecisionAllocation(t),
		contract:                   mock.NewContract(t),
		installment:                mock.NewInstallment(t),
		installmentCharge:          mock.NewInstallmentCharge(t),
		paymentAttempt:             mock.NewPaymentAttempt(t),
		inbox:                      mock.NewInbox(t),
		outbox:                     mock.NewOutbox(t),
		outboxTx:                   mock.NewOutboxTxQueryer(t),
		installmentTx:              mock.NewInstallmentTxQueryer(t),
		idGenerator:                mock.NewIDGenerator(t),
		clock:                      adapterclock.NewFixed(now, time.UTC),
	}
	deps.idGenerator.EXPECT().Generate().Return("installment-lifecycle-audit-event").Maybe()
	deps.outboxTx.EXPECT().Create(testifymock.Anything, testifymock.Anything).Return(model.OutboxMessage{}, nil).Maybe()

	return deps
}

func (d markDueOverdueDeps) newUseCase(batchDue, batchOverdue int) *UseCase {
	uc, err := New(
		d.pool,
		d.installment,
		d.installmentCharge,
		d.outbox,
		d.clock,
		"bnpl.audit.events.v1",
		d.idGenerator,
		batchDue,
		batchOverdue,
		metricsnoop.New(),
	)
	if err != nil {
		panic(err)
	}

	return uc
}

func (d markDueOverdueDeps) expectConnTx(ctx context.Context) {
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
	d.installment.EXPECT().Tx(d.tx).Return(d.installmentTx).Once()
	d.outbox.EXPECT().Tx(d.tx).Return(d.outboxTx).Once()
}

func TestMarkDueScheduledInstallments_BeforeDueTimeRemainsScheduled(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dueAt := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	now := dueAt.Add(-time.Minute)
	deps := newMarkDueOverdueDeps(t, now)

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimScheduledDue(ctx, now, 100).Return(nil, nil).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkDueScheduledInstallments(ctx)
	require.NoError(t, err)
	require.Zero(t, output.MarkedDueInstallmentCount)
}

func TestMarkDueScheduledInstallments_AtDueTimeBecomesDue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dueAt := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	now := dueAt
	deps := newMarkDueOverdueDeps(t, now)
	installmentRow := model.Installment{
		ID:     model.InstallmentID(uuid.New()),
		DueAt:  dueAt,
		Status: model.ScheduledInstallmentStatus,
	}
	updated := installmentRow
	updated.Status = model.DueInstallmentStatus

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimScheduledDue(ctx, now, 100).Return([]model.Installment{installmentRow}, nil).Once()
	deps.installmentTx.EXPECT().MarkDue(ctx, installmentRow.ID, now).Return(updated, nil).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkDueScheduledInstallments(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), output.MarkedDueInstallmentCount)
}

func TestMarkDueScheduledInstallments_PaidInstallmentConflictSkipped(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		7,
		10,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newMarkDueOverdueDeps(t, now)
	installmentRow := model.Installment{
		ID:     model.InstallmentID(uuid.New()),
		DueAt:  now,
		Status: model.PaidInstallmentStatus,
	}

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimScheduledDue(ctx, now, 100).Return([]model.Installment{installmentRow}, nil).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkDueScheduledInstallments(ctx)
	require.NoError(t, err)
	require.Zero(t, output.MarkedDueInstallmentCount)
}

func TestMarkOverdueDueInstallments_BeforeGraceEndRemainsDue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dueAt := time.Date(
		2026,
		7,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	now := dueAt.Add(4 * 24 * time.Hour)
	deps := newMarkDueOverdueDeps(t, now)
	installmentRow := model.Installment{
		ID:              model.InstallmentID(uuid.New()),
		DueAt:           dueAt,
		Status:          model.DueInstallmentStatus,
		GracePeriodDays: 5,
	}

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimDueOverdueEligible(ctx, now, 100).Return([]model.Installment{installmentRow}, nil).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkOverdueDueInstallments(ctx)
	require.NoError(t, err)
	require.Zero(t, output.MarkedOverdueInstallmentCount)
}

func TestMarkOverdueDueInstallments_AtGraceEndBecomesOverdue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dueAt := time.Date(
		2026,
		7,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	now := dueAt.Add(5 * 24 * time.Hour)
	deps := newMarkDueOverdueDeps(t, now)
	installmentRow := model.Installment{
		ID:              model.InstallmentID(uuid.New()),
		DueAt:           dueAt,
		Status:          model.DueInstallmentStatus,
		GracePeriodDays: 5,
	}
	updated := installmentRow
	updated.Status = model.OverdueInstallmentStatus

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimDueOverdueEligible(ctx, now, 100).Return([]model.Installment{installmentRow}, nil).Once()
	deps.installmentTx.EXPECT().MarkOverdue(ctx, installmentRow.ID, now).Return(updated, nil).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkOverdueDueInstallments(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), output.MarkedOverdueInstallmentCount)
}

func TestMarkOverdueDueInstallments_ZeroDayGraceSupported(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dueAt := time.Date(
		2026,
		7,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	now := dueAt
	deps := newMarkDueOverdueDeps(t, now)
	installmentRow := model.Installment{
		ID:              model.InstallmentID(uuid.New()),
		DueAt:           dueAt,
		Status:          model.DueInstallmentStatus,
		GracePeriodDays: 0,
	}
	updated := installmentRow
	updated.Status = model.OverdueInstallmentStatus

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimDueOverdueEligible(ctx, now, 100).Return([]model.Installment{installmentRow}, nil).Once()
	deps.installmentTx.EXPECT().MarkOverdue(ctx, installmentRow.ID, now).Return(updated, nil).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkOverdueDueInstallments(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(1), output.MarkedOverdueInstallmentCount)
}

func TestMarkOverdueDueInstallments_MarkOverdueConflictIsIdempotent(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dueAt := time.Date(
		2026,
		7,
		1,
		0,
		0,
		0,
		0,
		time.UTC,
	)
	now := dueAt
	deps := newMarkDueOverdueDeps(t, now)
	installmentRow := model.Installment{
		ID:              model.InstallmentID(uuid.New()),
		DueAt:           dueAt,
		Status:          model.DueInstallmentStatus,
		GracePeriodDays: 0,
	}

	deps.expectConnTx(ctx)
	deps.installmentTx.EXPECT().ClaimDueOverdueEligible(ctx, now, 100).Return([]model.Installment{installmentRow}, nil).Once()
	deps.installmentTx.EXPECT().MarkOverdue(ctx, installmentRow.ID, now).Return(
		model.Installment{},
		cerr.Conflict(errors.New("already overdue")),
	).Once()

	uc := deps.newUseCase(100, 100)

	output, err := uc.MarkOverdueDueInstallments(ctx)
	require.NoError(t, err)
	require.Zero(t, output.MarkedOverdueInstallmentCount)
}

func TestNew_BatchSizeValidation(t *testing.T) {
	t.Parallel()

	newUC := func(markDueBatchSize, markOverdueBatchSize int) (*UseCase, error) {
		return New(
			mock.NewPool(t),
			mock.NewInstallment(t),
			mock.NewInstallmentCharge(t),
			mock.NewOutbox(t),
			mock.NewClock(t),
			"bnpl.audit.events.v1",
			mock.NewIDGenerator(t),
			markDueBatchSize,
			markOverdueBatchSize,
			metricsnoop.New(),
		)
	}

	t.Run("valid batch sizes", func(t *testing.T) {
		t.Parallel()

		uc, err := newUC(100, 50)
		require.NoError(t, err)
		require.Equal(t, 100, uc.MarkDueBatchSize)
		require.Equal(t, 50, uc.MarkOverdueBatchSize)
	})

	t.Run("zero mark due batch size rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(0, 1)
		require.Error(t, err)
		require.Contains(t, err.Error(), "mark due batch size")
	})

	t.Run("zero mark overdue batch size rejected", func(t *testing.T) {
		t.Parallel()

		_, err := newUC(1, 0)
		require.Error(t, err)
		require.Contains(t, err.Error(), "mark overdue batch size")
	})
}
