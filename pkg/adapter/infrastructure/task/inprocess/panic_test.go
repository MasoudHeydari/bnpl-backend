package inprocess_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	taskinprocess "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/task/inprocess"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

type panicCapture struct {
	recovered any
	metadata  coreerrorreporting.Metadata
}

type panicRecordingReporter struct {
	mu     sync.Mutex
	panics []panicCapture
	notify chan struct{}
}

func newPanicRecordingReporter() *panicRecordingReporter {
	return &panicRecordingReporter{notify: make(chan struct{}, 1)}
}

func (*panicRecordingReporter) CaptureError(context.Context, error, coreerrorreporting.Metadata) {}

func (r *panicRecordingReporter) CapturePanic(
	_ context.Context,
	recovered any,
	metadata coreerrorreporting.Metadata,
) {
	r.mu.Lock()
	r.panics = append(r.panics, panicCapture{recovered: recovered, metadata: metadata})
	r.mu.Unlock()

	select {
	case r.notify <- struct{}{}:

	default:
	}
}

func (*panicRecordingReporter) Flush(context.Context) bool { return true }
func (*panicRecordingReporter) Close() error               { return nil }

func (r *panicRecordingReporter) snapshot() []panicCapture {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]panicCapture(nil), r.panics...)
}

func TestDispatch_PanicIsRecoveredAndReported(t *testing.T) {
	reporter := newPanicRecordingReporter()
	runtime, err := taskinprocess.New(
		taskinprocess.Config{MaxConcurrentRuns: 1},
		lognoop.New(),
		reporter,
	)
	require.NoError(t, err)

	require.NoError(t, runtime.Register(task.Task{
		Name:    "panic-task",
		Enabled: true,
		Handler: "panic-handler",
		Trigger: task.Trigger{Type: task.TriggerManual},
		Job: func(context.Context) error {
			panic("boom")
		},
	}))

	require.NoError(t, runtime.Dispatch(context.Background(), task.Command{
		ID:      "cmd-1",
		Handler: "panic-handler",
	}))

	select {
	case <-reporter.notify:

	case <-time.After(time.Second):
		t.Fatal("timed out waiting for recovered panic to be reported")
	}

	captures := reporter.snapshot()
	require.Len(t, captures, 1)
	assert.Equal(t, "boom", captures[0].recovered)
	assert.Equal(t, "task", captures[0].metadata.Component)
	assert.Equal(t, "panic-handler", captures[0].metadata.Tags["task.handler"])
	assert.Equal(t, "job_execution", captures[0].metadata.Tags["panic.scope"])
}
