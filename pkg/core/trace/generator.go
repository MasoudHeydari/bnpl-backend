package trace

import "context"

// IDGenerator generates opaque unique identifiers for tracing.
// Implementations may use ULID, UUID, XID, or another strategy. Callers must
// treat generated values as opaque strings and must not rely on their format.
type IDGenerator interface {
	// Generate returns a new opaque unique identifier.
	Generate() string
}

// Provider supplies tracing identifiers needed at transport and use-case boundaries.
// Keeping this as a core contract allows adapters to provide infrastructure
// implementations while application code depends only on this interface.
type Provider interface {
	// IDGenerator provides the shared opaque ID generation strategy.
	IDGenerator

	// NewTraceID returns a new distributed-trace identifier.
	NewTraceID() string

	// NewRequestID returns a new per-request identifier.
	NewRequestID() string

	// NewCorrelationID returns a new cross-service correlation identifier.
	NewCorrelationID() string
}

type (
	traceIDKey       struct{}
	requestIDKey     struct{}
	correlationIDKey struct{}
)

// WithTraceID returns a child context containing traceID.
func WithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// ID returns the trace ID stored in ctx.
func ID(ctx context.Context) string {
	traceID, ok := ctx.Value(traceIDKey{}).(string)
	if !ok {
		return ""
	}

	return traceID
}

// WithRequestID returns a child context containing requestID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestID returns the request ID stored in ctx.
func RequestID(ctx context.Context) string {
	requestID, ok := ctx.Value(requestIDKey{}).(string)
	if !ok {
		return ""
	}

	return requestID
}

// WithCorrelationID returns a child context containing correlationID.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, correlationID)
}

// CorrelationID returns the correlation ID stored in ctx.
func CorrelationID(ctx context.Context) string {
	correlationID, ok := ctx.Value(correlationIDKey{}).(string)
	if !ok {
		return ""
	}

	return correlationID
}
