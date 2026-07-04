package log

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
// Implementations are expected to enrich log records with contextual metadata
// such as trace identifiers, request identifiers, service name, and timestamps.
type Logger interface {
	// With returns a child logger enriched with structured attributes.
	//
	// The returned logger should inherit all existing attributes and append the
	// provided key-value pairs.
	//
	// Example:
	//
	//	logger := logger.With(
	//		"trace_id", trace.ID(ctx),
	//		"request_id", trace.RequestID(ctx),
	//	)
	With(args ...any) Logger

	// Debug records diagnostic information intended for development and
	// troubleshooting.
	Debug(msg string, args ...any)

	// Info records normal operational events.
	Info(msg string, args ...any)

	// Warn records unexpected situations that do not prevent the application
	// from continuing operation.
	Warn(msg string, args ...any)

	// Error records failures that require attention or indicate that an
	// operation could not be completed successfully.
	Error(msg string, args ...any)
}
