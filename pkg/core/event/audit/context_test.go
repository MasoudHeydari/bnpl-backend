package audit

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

func TestEnvelopeIDsFromContext(t *testing.T) {
	t.Parallel()

	t.Run("requires correlation and trace", func(t *testing.T) {
		t.Parallel()

		_, _, err := EnvelopeIDsFromContext(context.Background())
		require.Error(t, err)

		ctx := trace.WithCorrelationID(context.Background(), "corr-1")
		_, _, err = EnvelopeIDsFromContext(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "trace_id")

		ctx = trace.WithTraceID(context.Background(), "trace-1")
		_, _, err = EnvelopeIDsFromContext(ctx)
		require.Error(t, err)
		require.ErrorContains(t, err, "correlation_id")
	})

	t.Run("returns both ids", func(t *testing.T) {
		t.Parallel()

		ctx := trace.WithTraceID(context.Background(), "trace-1")
		ctx = trace.WithCorrelationID(ctx, "corr-1")

		correlationID, traceID, err := EnvelopeIDsFromContext(ctx)
		require.NoError(t, err)
		require.Equal(t, "corr-1", correlationID)
		require.Equal(t, "trace-1", traceID)
	})
}
