package slog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"

	stdslog "log/slog"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/event"
	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	adapterslog "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/slog"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

func TestWithContext_AttachesPresentIdentifiers(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := adapterslog.New(&buf, nil, stdslog.LevelInfo, corelog.JSONFormat, false)

	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-1")
	ctx = trace.WithRequestID(ctx, "request-1")
	ctx = trace.WithCorrelationID(ctx, "corr-1")

	logger.WithContext(ctx).Info(event.CheckoutSessionCreated, field.Outcome("created"))

	record := decodeLastJSON(t, buf.Bytes())
	require.Equal(t, "trace-1", record["trace_id"])
	require.Equal(t, "request-1", record["request_id"])
	require.Equal(t, "corr-1", record["correlation_id"])
	require.Equal(t, event.CheckoutSessionCreated.Code(), record["event"])
	require.Equal(t, "created", record["outcome"])
}

func TestWithContext_MissingIdentifiersAreSkipped(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := adapterslog.New(&buf, nil, stdslog.LevelInfo, corelog.JSONFormat, false)

	ctx := trace.WithTraceID(context.Background(), "trace-only")
	logger.WithContext(ctx).Info(corelog.Msg("ops.ping"))

	record := decodeLastJSON(t, buf.Bytes())
	require.Equal(t, "trace-only", record["trace_id"])
	_, hasRequest := record["request_id"]
	_, hasCorr := record["correlation_id"]
	require.False(t, hasRequest)
	require.False(t, hasCorr)
}

func TestWithContext_DoesNotDuplicateCorrelationFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := adapterslog.New(&buf, nil, stdslog.LevelInfo, corelog.JSONFormat, false)

	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-1")
	ctx = trace.WithRequestID(ctx, "request-1")
	ctx = trace.WithCorrelationID(ctx, "corr-1")

	child := logger.WithContext(ctx).WithContext(ctx)
	child.Info(corelog.Msg("ops.once"))

	raw := buf.String()
	require.Equal(t, 1, bytes.Count([]byte(raw), []byte(`"trace_id"`)))
	require.Equal(t, 1, bytes.Count([]byte(raw), []byte(`"request_id"`)))
	require.Equal(t, 1, bytes.Count([]byte(raw), []byte(`"correlation_id"`)))
}

func TestWithContext_PreservesPriorAttributes(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := adapterslog.New(&buf, nil, stdslog.LevelInfo, corelog.JSONFormat, false)

	ctx := trace.WithTraceID(context.Background(), "trace-1")
	logger.With("component", "checkout").WithContext(ctx).Info(corelog.Msg("ops.enriched"))

	record := decodeLastJSON(t, buf.Bytes())
	require.Equal(t, "checkout", record["component"])
	require.Equal(t, "trace-1", record["trace_id"])
}

func TestWithContext_ConcurrentSafe(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := adapterslog.New(&buf, nil, stdslog.LevelInfo, corelog.JSONFormat, false)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			ctx := trace.WithTraceID(context.Background(), "trace-concurrent")
			logger.WithContext(ctx).Info(corelog.Msg("ops.concurrent"), field.AttemptCount(i))
		}(i)
	}

	wg.Wait()
	require.NotEmpty(t, buf.Bytes())
}

func TestEmit_DropsUnknownFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := adapterslog.New(&buf, nil, stdslog.LevelInfo, corelog.JSONFormat, false)

	logger.Info(corelog.Msg("ops.safe"), corelog.TextAttr("password", "secret"), field.Outcome("ok"))

	record := decodeLastJSON(t, buf.Bytes())
	_, hasPassword := record["password"]
	require.False(t, hasPassword)
	require.Equal(t, "ok", record["outcome"])
}

func decodeLastJSON(t *testing.T, raw []byte) map[string]any {
	t.Helper()

	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	require.NotEmpty(t, lines)

	var record map[string]any
	require.NoError(t, json.Unmarshal(lines[len(lines)-1], &record))
	return record
}
