package trace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTraceIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := WithTraceID(context.Background(), "trace-1")
	require.Equal(t, "trace-1", TraceID(ctx))
	require.Empty(t, TraceID(context.Background()))
}

func TestRequestIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := WithRequestID(context.Background(), "req-1")
	require.Equal(t, "req-1", RequestID(ctx))
	require.Empty(t, RequestID(context.Background()))
}

func TestCorrelationIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := WithCorrelationID(context.Background(), "corr-1")
	require.Equal(t, "corr-1", CorrelationID(ctx))
	require.Empty(t, CorrelationID(context.Background()))
}
