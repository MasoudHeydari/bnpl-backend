package log

import "context"

// Level identifies the severity of a log record.
type Level string

const (
	// DebugLevel records diagnostic information for development and troubleshooting.
	DebugLevel Level = "debug"

	// InfoLevel records normal operational events.
	InfoLevel Level = "info"

	// WarnLevel records unexpected situations that do not stop operation.
	WarnLevel Level = "warn"

	// ErrorLevel records failures that require attention.
	ErrorLevel Level = "error"
)

// Format identifies the output format of a logger implementation.
type Format string

const (
	// JSONFormat emits structured JSON log records.
	JSONFormat Format = "json"

	// TextFormat emits human-readable text log records.
	TextFormat Format = "text"
)

// Logger defines the minimum logging capabilities required by the application.
//
// Implementations enrich log records with contextual metadata such as trace
// identifiers, request identifiers, service name, and timestamps.
//
// Preferred business usage:
//
//	logger.WithContext(ctx).Info(event.CheckoutSessionCreated, field.SessionID(id))
type Logger interface {
	// With returns a child logger enriched with structured attributes.
	//
	// Prefer typed Field constructors for business attributes. With remains
	// for infrastructure enrichment that already uses slog-style pairs.
	With(args ...any) Logger

	// WithContext returns a child logger enriched with request-scoped
	// identifiers from ctx (trace_id, request_id, correlation_id) when present.
	//
	// The receiver is never mutated. Missing identifiers are skipped.
	WithContext(ctx context.Context) Logger

	// Debug records a diagnostic Event.
	Debug(event Event, fields ...Field)

	// Info records a normal operational Event.
	Info(event Event, fields ...Field)

	// Warn records an unexpected but non-fatal Event.
	Warn(event Event, fields ...Field)

	// Error records a failed operation Event.
	Error(event Event, fields ...Field)
}
