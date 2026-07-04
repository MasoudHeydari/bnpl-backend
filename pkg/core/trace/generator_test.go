package trace

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIDRoundTrip(t *testing.T) {
	t.Parallel()

	ctx := WithTraceID(context.Background(), "trace-1")
	require.Equal(t, "trace-1", ID(ctx))
	require.Empty(t, ID(context.Background()))
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

type stubProvider struct{}

func (stubProvider) Generate() string         { return "gen" }
func (stubProvider) NewTraceID() string       { return "trace-gen" }
func (stubProvider) NewRequestID() string     { return "req-gen" }
func (stubProvider) NewCorrelationID() string { return "corr-gen" }

func TestEnsureIdentifiers_FillsMissing(t *testing.T) {
	t.Parallel()

	ctx := EnsureIdentifiers(context.Background(), stubProvider{})
	require.Equal(t, "trace-gen", ID(ctx))
	require.Equal(t, "req-gen", RequestID(ctx))
	require.Equal(t, "corr-gen", CorrelationID(ctx))
}

func TestEnsureIdentifiers_PreservesExisting(t *testing.T) {
	t.Parallel()

	ctx := WithTraceID(context.Background(), "trace-keep")
	ctx = WithRequestID(ctx, "req-keep")
	ctx = WithCorrelationID(ctx, "corr-keep")

	ctx = EnsureIdentifiers(ctx, stubProvider{})
	require.Equal(t, "trace-keep", ID(ctx))
	require.Equal(t, "req-keep", RequestID(ctx))
	require.Equal(t, "corr-keep", CorrelationID(ctx))
}

func TestEnsureIdentifiers_ReplacesWhitespaceOnly(t *testing.T) {
	t.Parallel()

	ctx := WithCorrelationID(context.Background(), "   ")
	ctx = EnsureIdentifiers(ctx, stubProvider{})
	require.Equal(t, "corr-gen", CorrelationID(ctx))
}
