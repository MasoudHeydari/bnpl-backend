package slog

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	stdslog "log/slog"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

func TestExpandEscapedWhitespaceInJSONStrings(t *testing.T) {
	t.Parallel()

	input := []byte(`{
  "msg": "database query",
  "sql": "SELECT 1;\nSELECT 2;\tDONE"
}`)

	got := expandEscapedWhitespaceInJSONStrings(input)
	require.Contains(t, string(got), "SELECT 1;\nSELECT 2;\tDONE")
	require.NotContains(t, string(got), `\n`)
	require.NotContains(t, string(got), `\t`)
	require.True(t, bytes.Contains(got, []byte("\"msg\": \"database query\"")))
}

func TestJSON_CompactIndependentOfLevel(t *testing.T) {
	t.Parallel()

	for _, level := range []stdslog.Level{stdslog.LevelDebug, stdslog.LevelInfo} {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := New(&buf, nil, level, corelog.JSONFormat, false)
			logger.Info(corelog.Msg("ops.compact"), field.Outcome("ok"))

			line := strings.TrimSpace(buf.String())
			require.NotContains(t, line, "\n")
			require.NotContains(t, line, "\x1b[")
			require.True(t, json.Valid([]byte(line)))

			var record map[string]any
			require.NoError(t, json.Unmarshal([]byte(line), &record))
			require.Equal(t, "ops.compact", record["msg"])
			require.Equal(t, "ok", record["outcome"])
		})
	}
}

func TestJSON_PrettyIndependentOfLevel(t *testing.T) {
	t.Parallel()

	for _, level := range []stdslog.Level{stdslog.LevelDebug, stdslog.LevelInfo} {
		t.Run(level.String(), func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			logger := New(&buf, nil, level, corelog.JSONFormat, true)
			logger.Info(corelog.Msg("ops.pretty"), field.Outcome("ok"))

			out := buf.String()
			require.Contains(t, out, "\n")
			require.Contains(t, out, "\x1b[")
			require.Contains(t, out, "ops.pretty")
			require.Contains(t, out, "ok")
		})
	}
}

func TestJSON_PrettyStdoutCompactFile(t *testing.T) {
	t.Parallel()

	var human, machine bytes.Buffer
	logger := New(&human, &machine, stdslog.LevelDebug, corelog.JSONFormat, true)

	ctx := context.Background()
	ctx = trace.WithTraceID(ctx, "trace-1")
	ctx = trace.WithRequestID(ctx, "request-1")
	ctx = trace.WithCorrelationID(ctx, "corr-1")

	logger.WithContext(ctx).Debug(corelog.Msg("ops.split"), field.Outcome("split"))

	humanOut := human.String()
	require.Contains(t, humanOut, "\n")
	require.Contains(t, humanOut, "\x1b[")
	require.Contains(t, humanOut, "ops.split")

	machineLine := strings.TrimSpace(machine.String())
	require.Equal(t, 1, strings.Count(machine.String(), "\n"))
	require.NotContains(t, machineLine, "\n")
	require.NotContains(t, machineLine, "\x1b[")
	require.True(t, json.Valid([]byte(machineLine)))

	var record map[string]any
	require.NoError(t, json.Unmarshal([]byte(machineLine), &record))
	require.Equal(t, "ops.split", record["msg"])
	require.Equal(t, "split", record["outcome"])
	require.Equal(t, "trace-1", record["trace_id"])
	require.Equal(t, "request-1", record["request_id"])
	require.Equal(t, "corr-1", record["correlation_id"])
}

func TestJSON_CompactDualWriteNoDuplicatePerSink(t *testing.T) {
	t.Parallel()

	var human, machine bytes.Buffer
	logger := New(&human, &machine, stdslog.LevelInfo, corelog.JSONFormat, false)
	logger.Info(corelog.Msg("ops.dual"))

	require.Equal(t, 1, strings.Count(human.String(), "\n"))
	require.Equal(t, 1, strings.Count(machine.String(), "\n"))
	require.Equal(t, strings.TrimSpace(human.String()), strings.TrimSpace(machine.String()))
	require.True(t, json.Valid([]byte(strings.TrimSpace(machine.String()))))
}

func TestText_PrettyIgnored(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := New(&buf, nil, stdslog.LevelDebug, corelog.TextFormat, true)
	logger.Info(corelog.Msg("ops.text"))

	out := buf.String()
	require.Contains(t, out, "ops.text")
	require.NotContains(t, out, "\x1b[")
	require.NotContains(t, out, `{`)
}

func TestJSON_ConcurrentWrites(t *testing.T) {
	t.Parallel()

	var human, machine bytes.Buffer
	logger := New(&human, &machine, stdslog.LevelDebug, corelog.JSONFormat, true)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			logger.Info(corelog.Msg("ops.concurrent"), field.AttemptCount(i))
		}(i)
	}

	wg.Wait()

	lines := nonEmptyLines(machine.Bytes())
	require.Len(t, lines, 32)
	for _, line := range lines {
		require.True(t, json.Valid(line))
		require.NotContains(t, string(line), "\x1b[")
	}
}

func nonEmptyLines(raw []byte) [][]byte {
	parts := bytes.Split(raw, []byte("\n"))
	out := make([][]byte, 0, len(parts))
	for _, part := range parts {
		part = bytes.TrimSpace(part)
		if len(part) == 0 {
			continue
		}
		out = append(out, part)
	}
	return out
}
