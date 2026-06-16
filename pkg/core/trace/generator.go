package trace

import "context"

// IDGenerator generates opaque unique identifiers for tracing.
// Implementations may use ULID, UUID, XID, or another strategy. Callers must
// treat generated values as opaque strings and must not rely on their format.
type IDGenerator interface {
	Generate() string
}

// Provider supplies tracing identifiers needed at transport and use-case boundaries.
// Keeping this as a core contract allows adapters to provide infrastructure
// implementations while application code depends only on this interface.
type Provider interface {
	NewTraceID() string
	NewRequestID() string
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

// TraceID returns the trace ID stored in ctx.
func TraceID(ctx context.Context) string {
	traceID, _ := ctx.Value(traceIDKey{}).(string)
	return traceID
}

// WithRequestID returns a child context containing requestID.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// RequestID returns the request ID stored in ctx.
func RequestID(ctx context.Context) string {
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// WithCorrelationID returns a child context containing correlationID.
func WithCorrelationID(ctx context.Context, correlationID string) context.Context {
	return context.WithValue(ctx, correlationIDKey{}, correlationID)
}

// CorrelationID returns the correlation ID stored in ctx.
func CorrelationID(ctx context.Context) string {
	correlationID, _ := ctx.Value(correlationIDKey{}).(string)
	return correlationID
}
