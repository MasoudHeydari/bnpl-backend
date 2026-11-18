package checkoutpayment

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

type expiryLogRecord struct {
	level string
	msg   string
	args  []any
}

type expiryRecordingLogger struct {
	mu      *sync.Mutex
	records *[]expiryLogRecord
	attrs   []any
}

func newExpiryRecordingLogger() *expiryRecordingLogger {
	return &expiryRecordingLogger{
		mu:      &sync.Mutex{},
		records: &[]expiryLogRecord{},
	}
}

func (l *expiryRecordingLogger) With(args ...any) corelog.Logger {
	attrs := append(append([]any{}, l.attrs...), args...)
	return &expiryRecordingLogger{mu: l.mu, records: l.records, attrs: attrs}
}

func (l *expiryRecordingLogger) WithContext(context.Context) corelog.Logger {
	return l
}

func (l *expiryRecordingLogger) Debug(event corelog.Event, fields ...corelog.Field) {
	l.recordLog("debug", event, fields...)
}

func (l *expiryRecordingLogger) Info(event corelog.Event, fields ...corelog.Field) {
	l.recordLog("info", event, fields...)
}

func (l *expiryRecordingLogger) Warn(event corelog.Event, fields ...corelog.Field) {
	l.recordLog("warn", event, fields...)
}

func (l *expiryRecordingLogger) Error(event corelog.Event, fields ...corelog.Field) {
	l.recordLog("error", event, fields...)
}

func (l *expiryRecordingLogger) recordLog(level string, event corelog.Event, fields ...corelog.Field) {
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := ""
	attrs := append([]any{}, l.attrs...)
	if event != nil {
		msg = event.Message()
	}

	for _, f := range fields {
		if f == nil {
			continue
		}

		attrs = append(attrs, f.Key(), f.Value())
	}

	*l.records = append(*l.records, expiryLogRecord{level: level, msg: msg, args: attrs})
}

func (l *expiryRecordingLogger) warnings() []expiryLogRecord {
	l.mu.Lock()
	defer l.mu.Unlock()

	out := make([]expiryLogRecord, 0, len(*l.records))
	for _, record := range *l.records {
		if record.level == "warn" {
			out = append(out, record)
		}
	}

	return out
}

func TestWarnIfExpiresAtLaterThanRequested(t *testing.T) {
	t.Parallel()

	requested := time.Date(2026, 10, 20, 12, 15, 0, 0, time.UTC)

	t.Run("equal does not warn", func(t *testing.T) {
		t.Parallel()

		logger := newExpiryRecordingLogger()
		WarnIfExpiresAtLaterThanRequested(logger, requested, requested, "payment_attempt_id", "pa-1")
		require.Empty(t, logger.warnings())
	})

	t.Run("earlier does not warn", func(t *testing.T) {
		t.Parallel()

		logger := newExpiryRecordingLogger()
		WarnIfExpiresAtLaterThanRequested(
			logger,
			requested,
			requested.Add(-time.Minute),
			"payment_attempt_id", "pa-1",
		)
		require.Empty(t, logger.warnings())
	})

	t.Run("later emits structured warning", func(t *testing.T) {
		t.Parallel()

		logger := newExpiryRecordingLogger()
		actual := requested.Add(30 * time.Minute)
		WarnIfExpiresAtLaterThanRequested(
			logger,
			requested,
			actual,
			"payment_attempt_id", "pa-1",
			"payment_operation_type", "installment_repayment",
		)

		warnings := logger.warnings()
		require.Len(t, warnings, 1)
		require.Equal(t, LatePaymentSessionExpiryWarning, warnings[0].msg)

		args := map[any]any{}
		for i := 0; i+1 < len(warnings[0].args); i += 2 {
			args[warnings[0].args[i]] = warnings[0].args[i+1]
		}

		require.Equal(t, "pa-1", args["payment_attempt_id"])
		require.Equal(t, "installment_repayment", args["payment_operation_type"])
		require.Equal(t, requested.Format(time.RFC3339Nano), args["requested_expires_at"])
		require.Equal(t, actual.Format(time.RFC3339Nano), args["fc_expires_at"])
		require.Equal(t, (30 * time.Minute).Seconds(), args["expiry_deviation_seconds"])
	})
}
