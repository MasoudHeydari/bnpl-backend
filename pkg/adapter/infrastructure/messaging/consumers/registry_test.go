package consumers

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errorreportingnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/errorreporting/noop"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/messaging"
)

type registryPanicCapture struct {
	recovered any
	metadata  coreerrorreporting.Metadata
}

type registryRecordingReporter struct {
	mu     sync.Mutex
	panics []registryPanicCapture
}

func (*registryRecordingReporter) CaptureError(context.Context, error, coreerrorreporting.Metadata) {}

func (r *registryRecordingReporter) CapturePanic(
	_ context.Context,
	recovered any,
	metadata coreerrorreporting.Metadata,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.panics = append(r.panics, registryPanicCapture{recovered: recovered, metadata: metadata})
}

func (*registryRecordingReporter) Flush(context.Context) bool { return true }
func (*registryRecordingReporter) Close() error               { return nil }

func (r *registryRecordingReporter) snapshot() []registryPanicCapture {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]registryPanicCapture(nil), r.panics...)
}

func TestRegistryHandle_PanicIsRecoveredReportedAndRetryable(t *testing.T) {
	reporter := &registryRecordingReporter{}
	registry, err := NewRegistry(lognoop.New(), reporter)
	require.NoError(t, err)
	require.NoError(t, registry.Register("payment_result", func(context.Context, messaging.Message) error {
		panic("boom")
	}))

	err = registry.Handle(context.Background(), messaging.Message{
		Topic:     "payment_result",
		Partition: 3,
		Offset:    42,
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrHandlerPanicked)
	assert.False(t, messaging.IsNonRetryable(err), "panic must leave the message retryable/unacknowledged")

	captures := reporter.snapshot()
	require.Len(t, captures, 1)
	assert.Equal(t, "boom", captures[0].recovered)
	assert.Equal(t, "messaging_consumer", captures[0].metadata.Component)
	assert.Equal(t, "payment_result", captures[0].metadata.Tags["messaging.topic"])
	assert.EqualValues(t, 3, captures[0].metadata.Contexts["message"]["partition"])
	assert.EqualValues(t, 42, captures[0].metadata.Contexts["message"]["offset"])
}

func TestRegistryHandle_NormalErrorIsPreserved(t *testing.T) {
	expected := errors.New("handler failed")
	registry, err := NewRegistry(lognoop.New(), errorreportingnoop.New())
	require.NoError(t, err)
	require.NoError(t, registry.Register("payment_result", func(context.Context, messaging.Message) error {
		return expected
	}))

	err = registry.Handle(context.Background(), messaging.Message{Topic: "payment_result"})
	assert.ErrorIs(t, err, expected)
}
