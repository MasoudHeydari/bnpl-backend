package financialcorepaymentreconciliationuc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	testifymock "github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	sha256fingerprint "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/fingerprint/sha256"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/event/installmentpaymentreconciliation"
	extcheckoutpayment "github.com/parstis/bnpl-backend/pkg/core/external/checkoutpayment"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

type pendingLogRecord struct {
	level string
	msg   string
	args  []any
}

type pendingRecordingLogger struct {
	records *[]pendingLogRecord
	attrs   []any
}

func newPendingRecordingLogger() *pendingRecordingLogger {
	return &pendingRecordingLogger{records: &[]pendingLogRecord{}}
}

func (l *pendingRecordingLogger) With(args ...any) corelog.Logger {
	attrs := append(append([]any{}, l.attrs...), args...)
	return &pendingRecordingLogger{records: l.records, attrs: attrs}
}

func (l *pendingRecordingLogger) Debug(msg string, args ...any) {
	*l.records = append(*l.records, pendingLogRecord{level: "debug", msg: msg, args: append(append([]any{}, l.attrs...), args...)})
}

func (l *pendingRecordingLogger) Info(msg string, args ...any) {
	*l.records = append(*l.records, pendingLogRecord{level: "info", msg: msg, args: append(append([]any{}, l.attrs...), args...)})
}

func (l *pendingRecordingLogger) Warn(msg string, args ...any) {
	*l.records = append(*l.records, pendingLogRecord{level: "warn", msg: msg, args: append(append([]any{}, l.attrs...), args...)})
}

func (l *pendingRecordingLogger) Error(msg string, args ...any) {
	*l.records = append(*l.records, pendingLogRecord{level: "error", msg: msg, args: append(append([]any{}, l.attrs...), args...)})
}

func (l *pendingRecordingLogger) warnCount(msg string) int {
	count := 0
	for _, record := range *l.records {
		if record.level == "warn" && record.msg == msg {
			count++
		}
	}

	return count
}

func TestHandlePending_CompleteValidSessionTransitionsToSubmitted(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	url := "https://pay.example/ps-1"
	expires := now.Add(20 * time.Minute)
	logger := newPendingRecordingLogger()

	idGenerator := mock.NewIdentifierGenerator(t)
	idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()

	uc := &UseCase{
		fingerprintHasher:                         sha256fingerprint.New(),
		logger:                                    logger,
		clock:                                     adapterclock.NewFixed(now, time.UTC),
		idGenerator:                               idGenerator,
		reconciliationAcknowledgementsDestination: "bnpl.reconciliation.acks.v1",
		auditDestination:                          "bnpl.audit.events.v1",
		retryBaseDelay:                            time.Second,
		retryMaxDelay:                             time.Minute,
	}

	attempt := model.PaymentAttempt{
		ID:                         attemptID,
		State:                      model.ReconciliationRequiredPaymentAttemptState,
		AmountMinor:                1000,
		Currency:                   model.IRRCurrency,
		Purpose:                    model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationAttemptCount: 1,
		CreatedAt:                  now.Add(-5 * time.Minute),
	}

	input := ProcessInput{
		EventID:       "evt-pending-1",
		EventType:     installmentpaymentreconciliation.ReconciliationResultEventType,
		EventVersion:  installmentpaymentreconciliation.ReconciliationResultEventVersion,
		SourceService: installmentpaymentreconciliation.FinancialCoreSourceService,
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            installmentpaymentreconciliation.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    installmentpaymentreconciliation.PendingResultStatus,
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &url,
			ExpiresAt:                 &expires,
			AmountMinor:               1000,
			Currency:                  "IRR",
			OccurredAt:                now,
		},
	}

	inboxQ := mock.NewInboxTxQueryer(t)
	outboxQ := mock.NewOutboxTxQueryer(t)
	paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)

	submitted := attempt
	submitted.State = model.SubmittedPaymentAttemptState
	submitted.FinancialPaymentIntentID = ptrString("pi-1")
	submitted.FinancialPaymentSessionID = ptrString("ps-1")
	submitted.PaymentURL = &url
	submitted.FinancialPaymentExpiresAt = &expires

	paymentAttemptQ.EXPECT().MarkSubmittedFromReconciliationRequired(
		ctx,
		attemptID,
		"pi-1",
		"ps-1",
		testifymock.MatchedBy(func(v *string) bool { return v != nil && *v == url }),
		testifymock.MatchedBy(func(v *time.Time) bool { return v != nil && v.UTC().Equal(expires.UTC()) }),
		now,
		now,
		now,
	).Return(submitted, nil).Once()

	outboxQ.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Twice()
	inboxQ.EXPECT().MarkProcessed(ctx, inboxID, now, now).Return(model.Inbox{ID: inboxID}, nil).Once()

	var output ProcessOutput
	err := uc.handlePending(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		inboxID,
		model.Inbox{ID: inboxID, Status: model.ReceivedInboxStatus},
		input,
		attempt,
		attempt.State,
		now,
		&output,
	)
	require.NoError(t, err)
	require.Equal(t, ProcessResultResolvedSubmitted, output.Result)
	require.Equal(t, 1, logger.warnCount(extcheckoutpayment.LatePaymentSessionExpiryWarning))
}

func TestHandlePending_IncompleteSessionKeepsReconciliationRequired(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())

	idGenerator := mock.NewIdentifierGenerator(t)
	idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()

	uc := &UseCase{
		fingerprintHasher:                         sha256fingerprint.New(),
		logger:                                    lognoop.New(),
		clock:                                     adapterclock.NewFixed(now, time.UTC),
		idGenerator:                               idGenerator,
		reconciliationAcknowledgementsDestination: "bnpl.reconciliation.acks.v1",
		auditDestination:                          "bnpl.audit.events.v1",
		retryBaseDelay:                            time.Second,
		retryMaxDelay:                             time.Minute,
	}

	attempt := model.PaymentAttempt{
		ID:                         attemptID,
		State:                      model.ReconciliationRequiredPaymentAttemptState,
		AmountMinor:                1000,
		Currency:                   model.IRRCurrency,
		Purpose:                    model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationAttemptCount: 1,
		CreatedAt:                  now.Add(-5 * time.Minute),
	}

	input := ProcessInput{
		EventID:       "evt-pending-incomplete",
		EventType:     installmentpaymentreconciliation.ReconciliationResultEventType,
		EventVersion:  installmentpaymentreconciliation.ReconciliationResultEventVersion,
		SourceService: installmentpaymentreconciliation.FinancialCoreSourceService,
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            installmentpaymentreconciliation.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    installmentpaymentreconciliation.PendingResultStatus,
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			AmountMinor:               1000,
			Currency:                  "IRR",
			OccurredAt:                now,
		},
	}

	inboxQ := mock.NewInboxTxQueryer(t)
	outboxQ := mock.NewOutboxTxQueryer(t)
	paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)

	retried := attempt
	paymentAttemptQ.EXPECT().ScheduleReconciliationRetry(
		ctx,
		attemptID,
		testifymock.Anything,
		testifymock.Anything,
		now,
	).Return(retried, nil).Once()
	outboxQ.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Twice()
	inboxQ.EXPECT().MarkProcessed(ctx, inboxID, now, now).Return(model.Inbox{ID: inboxID}, nil).Once()

	var output ProcessOutput
	err := uc.handlePending(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		inboxID,
		model.Inbox{ID: inboxID, Status: model.ReceivedInboxStatus},
		input,
		attempt,
		attempt.State,
		now,
		&output,
	)
	require.NoError(t, err)
	require.Equal(t, ProcessResultStillReconciliationRequired, output.Result)
}

func TestHandlePending_AmountMismatchPermanentlyRejects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	url := "https://pay.example/ps-1"
	expires := now.Add(20 * time.Minute)

	idGenerator := mock.NewIdentifierGenerator(t)
	idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()

	uc := &UseCase{
		fingerprintHasher:                         sha256fingerprint.New(),
		logger:                                    lognoop.New(),
		clock:                                     adapterclock.NewFixed(now, time.UTC),
		idGenerator:                               idGenerator,
		reconciliationAcknowledgementsDestination: "bnpl.reconciliation.acks.v1",
		auditDestination:                          "bnpl.audit.events.v1",
		retryBaseDelay:                            time.Second,
		retryMaxDelay:                             time.Minute,
	}

	attempt := model.PaymentAttempt{
		ID:                         attemptID,
		State:                      model.ReconciliationRequiredPaymentAttemptState,
		AmountMinor:                1000,
		Currency:                   model.IRRCurrency,
		Purpose:                    model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationAttemptCount: 1,
		CreatedAt:                  now.Add(-5 * time.Minute),
	}

	input := ProcessInput{
		EventID: "evt-pending-amount",
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            installmentpaymentreconciliation.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    installmentpaymentreconciliation.PendingResultStatus,
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &url,
			ExpiresAt:                 &expires,
			AmountMinor:               999,
			Currency:                  "IRR",
			OccurredAt:                now,
		},
	}

	inboxQ := mock.NewInboxTxQueryer(t)
	outboxQ := mock.NewOutboxTxQueryer(t)
	paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)

	inboxQ.EXPECT().MarkFailed(ctx, inboxID, "pending_economics_conflict", testifymock.Anything, now, now).
		Return(model.Inbox{ID: inboxID, Status: model.FailedInboxStatus}, nil).Once()
	outboxQ.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	var output ProcessOutput
	err := uc.handlePending(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		inboxID,
		model.Inbox{ID: inboxID, Status: model.ReceivedInboxStatus},
		input,
		attempt,
		attempt.State,
		now,
		&output,
	)
	require.NoError(t, err)
	require.Equal(t, ProcessResultPermanentlyRejected, output.Result)
}

func TestHandlePending_IdentityConflictPermanentlyRejects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	url := "https://pay.example/ps-1"
	expires := now.Add(20 * time.Minute)

	idGenerator := mock.NewIdentifierGenerator(t)
	idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()

	uc := &UseCase{
		fingerprintHasher:                         sha256fingerprint.New(),
		logger:                                    lognoop.New(),
		clock:                                     adapterclock.NewFixed(now, time.UTC),
		idGenerator:                               idGenerator,
		reconciliationAcknowledgementsDestination: "bnpl.reconciliation.acks.v1",
		auditDestination:                          "bnpl.audit.events.v1",
		retryBaseDelay:                            time.Second,
		retryMaxDelay:                             time.Minute,
	}

	attempt := model.PaymentAttempt{
		ID:                         attemptID,
		State:                      model.ReconciliationRequiredPaymentAttemptState,
		AmountMinor:                1000,
		Currency:                   model.IRRCurrency,
		Purpose:                    model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationAttemptCount: 1,
		CreatedAt:                  now.Add(-5 * time.Minute),
		FinancialPaymentIntentID:   ptrString("pi-stored"),
	}

	input := ProcessInput{
		EventID: "evt-pending-identity",
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            installmentpaymentreconciliation.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    installmentpaymentreconciliation.PendingResultStatus,
			FinancialPaymentIntentID:  ptrString("pi-other"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &url,
			ExpiresAt:                 &expires,
			AmountMinor:               1000,
			Currency:                  "IRR",
			OccurredAt:                now,
		},
	}

	inboxQ := mock.NewInboxTxQueryer(t)
	outboxQ := mock.NewOutboxTxQueryer(t)
	paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)

	inboxQ.EXPECT().MarkFailed(ctx, inboxID, "pending_session_identity_conflict", testifymock.Anything, now, now).
		Return(model.Inbox{ID: inboxID, Status: model.FailedInboxStatus}, nil).Once()
	outboxQ.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	var output ProcessOutput
	err := uc.handlePending(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		inboxID,
		model.Inbox{ID: inboxID, Status: model.ReceivedInboxStatus},
		input,
		attempt,
		attempt.State,
		now,
		&output,
	)
	require.NoError(t, err)
	require.Equal(t, ProcessResultPermanentlyRejected, output.Result)
}

func TestHandlePending_SucceededAttemptIsNotDowngraded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)
	attemptID := model.PaymentAttemptID(uuid.New())
	inboxID := model.InboxID(uuid.New())
	url := "https://pay.example/ps-1"
	expires := now.Add(20 * time.Minute)

	idGenerator := mock.NewIdentifierGenerator(t)
	idGenerator.EXPECT().Generate().Return(uuid.NewString()).Maybe()

	uc := &UseCase{
		fingerprintHasher:                         sha256fingerprint.New(),
		logger:                                    lognoop.New(),
		clock:                                     adapterclock.NewFixed(now, time.UTC),
		idGenerator:                               idGenerator,
		reconciliationAcknowledgementsDestination: "bnpl.reconciliation.acks.v1",
		auditDestination:                          "bnpl.audit.events.v1",
		retryBaseDelay:                            time.Second,
		retryMaxDelay:                             time.Minute,
	}

	attempt := model.PaymentAttempt{
		ID:                         attemptID,
		State:                      model.SucceededPaymentAttemptState,
		AmountMinor:                1000,
		Currency:                   model.IRRCurrency,
		Purpose:                    model.InstallmentRepaymentFinancialCorePaymentPurpose,
		ReconciliationAttemptCount: 1,
		CreatedAt:                  now.Add(-5 * time.Minute),
	}

	input := ProcessInput{
		EventID:       "evt-pending-after-success",
		RawPayload:    json.RawMessage(`{}`),
		Data: installmentpaymentreconciliation.ResultEventData{
			RequestEventID:            installmentpaymentreconciliation.RequestEventID(attemptID, 1),
			BNPLPaymentAttemptID:      attemptID,
			ReconciliationGeneration:  1,
			Status:                    installmentpaymentreconciliation.PendingResultStatus,
			FinancialPaymentIntentID:  ptrString("pi-1"),
			FinancialPaymentSessionID: ptrString("ps-1"),
			PaymentURL:                &url,
			ExpiresAt:                 &expires,
			AmountMinor:               1000,
			Currency:                  "IRR",
			OccurredAt:                now,
		},
	}

	inboxQ := mock.NewInboxTxQueryer(t)
	outboxQ := mock.NewOutboxTxQueryer(t)
	paymentAttemptQ := mock.NewPaymentAttemptTxQueryer(t)

	inboxQ.EXPECT().MarkProcessed(ctx, inboxID, now, now).Return(model.Inbox{ID: inboxID}, nil).Once()
	outboxQ.EXPECT().Create(ctx, testifymock.Anything).Return(model.OutboxMessage{}, nil).Once()

	var output ProcessOutput
	err := uc.handlePending(
		ctx,
		inboxQ,
		outboxQ,
		paymentAttemptQ,
		inboxID,
		model.Inbox{ID: inboxID, Status: model.ReceivedInboxStatus},
		input,
		attempt,
		attempt.State,
		now,
		&output,
	)
	require.NoError(t, err)
	require.Equal(t, ProcessResultAlreadyProcessed, output.Result)
}
