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
	"github.com/parstis/bnpl-backend/pkg/core/usecase/financialcorepaymentresultuc"
)

type stubPaymentStatusRecoverer struct {
	fn func(context.Context, financialcorepaymentresultuc.RecoverByGetPaymentStatusInput) (financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput, error)
}

func (s stubPaymentStatusRecoverer) RecoverByGetPaymentStatus(
	ctx context.Context,
	input financialcorepaymentresultuc.RecoverByGetPaymentStatusInput,
) (financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput, error) {
	if s.fn == nil {
		return financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput{}, nil
	}

	return s.fn(ctx, input)
}

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
	observer         *mock.PaymentAttemptReconciliationObserver
	clock            adapterclock.Fixed
	recoverer        stubPaymentStatusRecoverer
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
		observer:         mock.NewPaymentAttemptReconciliationObserver(t),
		clock:            adapterclock.NewFixed(now, time.UTC),
		recoverer:        stubPaymentStatusRecoverer{},
	}
}

func (d processDueDeps) newUseCase() *UseCase {
	uc, err := newUseCase(
		d.pool,
		d.paymentAttempt,
		d.contract,
		d.outbox,
		d.idGenerator,
		d.recoverer,
		"bnpl.audit.events.v1",
		10,
		time.Minute,
		time.Second,
		time.Minute,
		5,
		d.clock,
		d.observer,
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
	now := time.Date(
		2026,
		8,
		31,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()

	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx,
		now,
		testifymock.Anything,
		now,
		10,
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

	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_DoesNotClaimStaleCreatedForSessionRediscovery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(
		2026,
		8,
		31,
		12,
		0,
		0,
		0,
		time.UTC,
	)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx,
		now,
		testifymock.Anything,
		now,
		10,
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

	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_ExhaustedSessionCreationStopsAutomaticFCRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	attemptID := model.PaymentAttemptID(uuid.New())
	reason := model.FinancialCoreStatusStillUnknownPaymentReconciliationReason
	nextRetry := now.Add(-time.Second)
	claimed := model.PaymentAttempt{
		ID:                                 attemptID,
		ContractID:                         model.ContractID(uuid.New()),
		InstallmentID:                      model.InstallmentID(uuid.New()),
		State:                              model.ReconciliationRequiredPaymentAttemptState,
		Purpose:                            model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationReason:               &reason,
		NextReconciliationAt:               &nextRetry,
		AmountMinor:                        1000,
		Currency:                           model.IRRCurrency,
		Channel:                            model.GatewayPaymentChannel,
		FinancialOutcomeResolved:           false,
		FinancialSessionCreateAttemptCount: model.MaxFinancialSessionCreateAttempts,
		CreatedAt:                          now.Add(-6 * time.Minute),
	}

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx,
		now,
		testifymock.Anything,
		now,
		10,
	).Return([]model.PaymentAttempt{claimed}, nil).Once()

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
		9,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(claimed, nil).Once()
	deps.paymentAttemptTx.EXPECT().ClearNextReconciliationAt(ctx, attemptID, now).Return(claimed, nil).Once()

	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_SubmittedPaymentUncertaintyUsesGetPaymentStatus(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	intent := "pi-recover-1"
	var recovered financialcorepaymentresultuc.RecoverByGetPaymentStatusInput
	deps := newProcessDueDeps(t, now)
	deps.recoverer = stubPaymentStatusRecoverer{
		fn: func(
			_ context.Context,
			input financialcorepaymentresultuc.RecoverByGetPaymentStatusInput,
		) (financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput, error) {
			recovered = input
			return financialcorepaymentresultuc.RecoverByGetPaymentStatusOutput{
				Result: financialcorepaymentresultuc.ProcessResultPaymentApplied,
			}, nil
		},
	}
	uc := deps.newUseCase()

	attemptID := model.PaymentAttemptID(uuid.New())
	reason := model.PaymentResultTimeoutPaymentReconciliationReason
	sessionID := "fc-session"
	nextRetry := now.Add(-time.Second)
	claimed := model.PaymentAttempt{
		ID:                                 attemptID,
		ContractID:                         model.ContractID(uuid.New()),
		InstallmentID:                      model.InstallmentID(uuid.New()),
		State:                              model.ReconciliationRequiredPaymentAttemptState,
		Purpose:                            model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationReason:               &reason,
		NextReconciliationAt:               &nextRetry,
		FinancialPaymentIntentID:           &intent,
		FinancialPaymentSessionID:          &sessionID,
		AmountMinor:                        1000,
		Currency:                           model.IRRCurrency,
		Channel:                            model.GatewayPaymentChannel,
		CreatedAt:                          now.Add(-20 * time.Minute),
		FinancialSessionCreateAttemptCount: model.MaxFinancialSessionCreateAttempts,
	}

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx,
		now,
		testifymock.Anything,
		now,
		10,
	).Return([]model.PaymentAttempt{claimed}, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimExpiredSubmittedForReconciliation(
		ctx, now, time.Minute, model.PaymentResultTimeoutPaymentReconciliationReason,
		now, testifymock.Anything, now, 9,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(claimed, nil).Once()
	deps.paymentAttemptTx.EXPECT().RecordReconciliationRequest(
		ctx, attemptID, 0, now, testifymock.Anything, testifymock.Anything, now,
	).Return(claimed, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("request-audit-event-id").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()
	deps.observer.EXPECT().ReconciliationRequested(ctx, attemptID, 1).Once()

	require.NoError(t, uc.ProcessDue(ctx))
	require.Equal(t, attemptID, recovered.PaymentAttemptID)
	require.Equal(t, 1, recovered.Generation)
}

func TestProcessDue_MissingPaymentIntentEscalatesToOperationalReview(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	attemptID := model.PaymentAttemptID(uuid.New())
	reason := model.PaymentResultTimeoutPaymentReconciliationReason
	nextRetry := now.Add(-time.Second)
	claimed := model.PaymentAttempt{
		ID:                   attemptID,
		ContractID:           model.ContractID(uuid.New()),
		InstallmentID:        model.InstallmentID(uuid.New()),
		State:                model.ReconciliationRequiredPaymentAttemptState,
		Purpose:              model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationReason: &reason,
		NextReconciliationAt: &nextRetry,
		AmountMinor:          1000,
		Currency:             model.IRRCurrency,
		Channel:              model.GatewayPaymentChannel,
		IdempotencyKey:       "idem-1",
		CreatedAt:            now.Add(-20 * time.Minute),
	}
	reviewed := claimed
	reviewed.State = model.ManualReviewRequiredPaymentAttemptState

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx, now, testifymock.Anything, now, 10,
	).Return([]model.PaymentAttempt{claimed}, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimExpiredSubmittedForReconciliation(
		ctx, now, time.Minute, model.PaymentResultTimeoutPaymentReconciliationReason,
		now, testifymock.Anything, now, 9,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(claimed, nil).Once()
	deps.paymentAttemptTx.EXPECT().MarkManualReviewRequired(
		ctx, attemptID, testifymock.Anything, now, now,
	).Return(reviewed, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("manual-audit-event-id").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()
	deps.observer.EXPECT().ManualReviewRequired(ctx, attemptID, manualReviewReasonMissingPaymentIntent).Once()

	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_ExpiresEligibleSessionCreationCancelsWhenBudgetExhausted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	attemptID := model.PaymentAttemptID(uuid.New())
	reason := model.PaymentSessionCreationTimeoutPaymentReconciliationReason
	nextRetry := now.Add(-time.Second)
	leaseUntil := now.Add(-model.FinancialSessionCreateExpirationSafetyInterval - time.Second)
	claimed := model.PaymentAttempt{
		ID:                                 attemptID,
		ContractID:                         model.ContractID(uuid.New()),
		InstallmentID:                      model.InstallmentID(uuid.New()),
		State:                              model.ReconciliationRequiredPaymentAttemptState,
		Purpose:                            model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationReason:               &reason,
		NextReconciliationAt:               &nextRetry,
		AmountMinor:                        1000,
		Currency:                           model.IRRCurrency,
		Channel:                            model.GatewayPaymentChannel,
		FinancialOutcomeResolved:           false,
		FinancialSessionCreateAttemptCount: model.MaxFinancialSessionCreateAttempts,
		FinancialSessionCreateLeaseUntil:   &leaseUntil,
		CreatedAt:                          now.Add(-model.InstallmentRepaymentPaymentSessionTTL - time.Minute),
	}
	canceled := claimed
	canceled.State = model.CanceledPaymentAttemptState
	failure := model.SessionCreationRetryExhaustedFailureCode
	canceled.FailureCode = &failure
	canceledAt := now
	canceled.CanceledAt = &canceledAt
	canceled.NextReconciliationAt = nil
	canceled.FinancialSessionCreateLeaseUntil = nil

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx, now, testifymock.Anything, now, 10,
	).Return([]model.PaymentAttempt{claimed}, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimExpiredSubmittedForReconciliation(
		ctx, now, time.Minute, model.PaymentResultTimeoutPaymentReconciliationReason,
		now, testifymock.Anything, now, 9,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(claimed, nil).Once()
	deps.paymentAttemptTx.EXPECT().CancelSessionCreationRetryExhausted(
		ctx, attemptID, model.ReconciliationRequiredPaymentAttemptState, now, now,
	).Return(canceled, nil).Once()
	deps.idGenerator.EXPECT().Generate().Return("expire-audit-event-id").Once()
	deps.outboxTx.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	deps.observer.EXPECT().AttemptClosedLocally(ctx, attemptID, attemptOutcomeCanceledRetryExhausted).Once()

	require.NoError(t, uc.ProcessDue(ctx))
}

func TestProcessDue_UnderBudgetSessionCreationStopsAutomaticFCRequest(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	deps := newProcessDueDeps(t, now)
	uc := deps.newUseCase()

	attemptID := model.PaymentAttemptID(uuid.New())
	reason := model.FinancialCoreStatusStillUnknownPaymentReconciliationReason
	nextRetry := now.Add(-time.Second)
	claimed := model.PaymentAttempt{
		ID:                                 attemptID,
		ContractID:                         model.ContractID(uuid.New()),
		InstallmentID:                      model.InstallmentID(uuid.New()),
		State:                              model.ReconciliationRequiredPaymentAttemptState,
		Purpose:                            model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationReason:               &reason,
		NextReconciliationAt:               &nextRetry,
		AmountMinor:                        1000,
		Currency:                           model.IRRCurrency,
		Channel:                            model.GatewayPaymentChannel,
		FinancialOutcomeResolved:           false,
		FinancialSessionCreateAttemptCount: 2,
		CreatedAt:                          now.Add(-6 * time.Minute),
	}

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimDueReconciliationRequired(
		ctx, now, testifymock.Anything, now, 10,
	).Return([]model.PaymentAttempt{claimed}, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().ClaimExpiredSubmittedForReconciliation(
		ctx, now, time.Minute, model.PaymentResultTimeoutPaymentReconciliationReason,
		now, testifymock.Anything, now, 9,
	).Return(nil, nil).Once()

	deps.expectConnTx(ctx)
	deps.paymentAttempt.EXPECT().Tx(deps.tx).Return(deps.paymentAttemptTx).Once()
	deps.outbox.EXPECT().Tx(deps.tx).Return(deps.outboxTx).Once()
	deps.paymentAttemptTx.EXPECT().LockByID(ctx, attemptID).Return(claimed, nil).Once()
	deps.paymentAttemptTx.EXPECT().ClearNextReconciliationAt(ctx, attemptID, now).Return(claimed, nil).Once()

	require.NoError(t, uc.ProcessDue(ctx))
}
