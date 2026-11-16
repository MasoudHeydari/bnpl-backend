package observer

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/model"
)

type flowRecord struct {
	level  log.Level
	code   string
	fields map[string]any
}

// flowRecordingLogger records typed Events and Fields emitted by observers.
type flowRecordingLogger struct {
	mu      *sync.Mutex
	records *[]flowRecord
}

func newFlowRecordingLogger() *flowRecordingLogger {
	return &flowRecordingLogger{mu: &sync.Mutex{}, records: new([]flowRecord)}
}

func (l *flowRecordingLogger) With(...any) log.Logger { return l }

func (l *flowRecordingLogger) WithContext(context.Context) log.Logger { return l }

func (l *flowRecordingLogger) Debug(e log.Event, f ...log.Field) { l.record(log.DebugLevel, e, f) }

func (l *flowRecordingLogger) Info(e log.Event, f ...log.Field) { l.record(log.InfoLevel, e, f) }

func (l *flowRecordingLogger) Warn(e log.Event, f ...log.Field) { l.record(log.WarnLevel, e, f) }

func (l *flowRecordingLogger) Error(e log.Event, f ...log.Field) { l.record(log.ErrorLevel, e, f) }

func (l *flowRecordingLogger) record(level log.Level, e log.Event, fields []log.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	values := make(map[string]any, len(fields))
	for _, f := range fields {
		key, value, ok := field.Sanitize(f)
		if !ok {
			continue
		}

		values[key] = value
	}

	*l.records = append(*l.records, flowRecord{level: level, code: e.Code(), fields: values})
}

func (l *flowRecordingLogger) codes() []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	codes := make([]string, 0, len(*l.records))
	for _, r := range *l.records {
		codes = append(codes, r.code)
	}

	return codes
}

func (l *flowRecordingLogger) last() flowRecord {
	l.mu.Lock()
	defer l.mu.Unlock()

	return (*l.records)[len(*l.records)-1]
}

func TestFinancialCorePaymentResult_ContractActivatedIsDistinctFromSubmission(t *testing.T) {
	t.Parallel()

	logger := newFlowRecordingLogger()
	observer, err := NewFinancialCorePaymentResult(logger)
	require.NoError(t, err)

	ctx := context.Background()
	contractID := model.ContractID(uuid.New())

	observer.ApplicationSucceeded(ctx, "event-1", contractID)
	observer.ContractActivated(ctx, "event-1", contractID)

	require.Equal(t, []string{
		event.PaymentApplicationSucceeded.Code(),
		event.ContractActivated.Code(),
	}, logger.codes())

	activated := logger.last()
	require.Equal(t, log.InfoLevel, activated.level)
	require.Equal(t, contractID.String(), activated.fields[field.KeyContractID])
	require.Equal(t, "event-1", activated.fields[field.KeyEventID])
}

func TestFinancialCorePaymentResult_InboxDuplicateIsNotAnApplication(t *testing.T) {
	t.Parallel()

	logger := newFlowRecordingLogger()
	observer, err := NewFinancialCorePaymentResult(logger)
	require.NoError(t, err)

	observer.InboxDuplicateDetected(context.Background(), "event-1", model.ContractID{}, "event_already_processed")

	require.Equal(t, []string{event.PaymentInboxDuplicateDetected.Code()}, logger.codes())
	require.NotContains(t, logger.last().fields, field.KeyContractID)
}

func TestContract_PaymentSubmissionEventsNeverReportSuccess(t *testing.T) {
	t.Parallel()

	logger := newFlowRecordingLogger()
	observer, err := NewContract(logger)
	require.NoError(t, err)

	ctx := context.Background()
	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())

	observer.FinancialCoreRequestStarted(ctx, contractID, attemptID)
	observer.FinancialCoreResponseReceived(ctx, contractID, attemptID)
	observer.PaymentURLObtained(ctx, contractID, attemptID)

	require.Equal(t, []string{
		event.PaymentFinancialCoreRequestStarted.Code(),
		event.PaymentFinancialCoreResponseReceived.Code(),
		event.PaymentURLObtained.Code(),
	}, logger.codes())
	require.Equal(t, "submitted", logger.last().fields[field.KeyStatus])
}

func TestContract_FinancialCoreTimeoutIsDistinctFromFailure(t *testing.T) {
	t.Parallel()

	logger := newFlowRecordingLogger()
	observer, err := NewContract(logger)
	require.NoError(t, err)

	ctx := context.Background()
	contractID := model.ContractID(uuid.New())
	attemptID := model.PaymentAttemptID(uuid.New())

	observer.FinancialCoreFailed(ctx, contractID, attemptID, context.DeadlineExceeded)
	observer.FinancialCoreFailed(ctx, contractID, attemptID, errors.New("provider rejected"))

	require.Equal(t, []string{
		event.PaymentFinancialCoreTimedOut.Code(),
		event.PaymentFinancialCoreFailed.Code(),
	}, logger.codes())
}

func TestOutboxPublisher_FailedMessageLogsAtErrorLevel(t *testing.T) {
	t.Parallel()

	logger := newFlowRecordingLogger()
	observer, err := NewOutboxPublisher(logger)
	require.NoError(t, err)

	outboxID := model.OutboxID(uuid.New())
	observer.MessageFailed(context.Background(), outboxID, "payments.result", 3, "publisher unavailable")

	failed := logger.last()
	require.Equal(t, event.OutboxMessageFailed.Code(), failed.code)
	require.Equal(t, log.ErrorLevel, failed.level)
	require.Equal(t, outboxID.String(), failed.fields[field.KeyOutboxMessageID])
	require.Equal(t, "payments.result", failed.fields[field.KeyTopic])
	require.Equal(t, 3, failed.fields[field.KeyAttemptCount])
}

func TestFlowObserverConstructorsRequireLogger(t *testing.T) {
	t.Parallel()

	_, err := NewContract(nil)
	require.Error(t, err)

	_, err = NewFinancialCorePaymentResult(nil)
	require.Error(t, err)

	_, err = NewInstallmentRepayment(nil)
	require.Error(t, err)

	_, err = NewFinancialCorePaymentReconciliation(nil)
	require.Error(t, err)

	_, err = NewPaymentAttemptReconciliation(nil)
	require.Error(t, err)

	_, err = NewPaymentAttemptExpiration(nil)
	require.Error(t, err)

	_, err = NewOutboxPublisher(nil)
	require.Error(t, err)
}
