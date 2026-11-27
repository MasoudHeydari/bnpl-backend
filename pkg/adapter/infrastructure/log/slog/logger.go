package slog

import (
	"context"
	"io"
	stdslog "log/slog"
	"os"

	"github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/field"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
)

// Verify that Slog implements the core Logger contract at compile time.
var _ corelog.Logger = (*Slog)(nil)

const (
	corrTraceBit uint8 = 1 << iota
	corrRequestBit
	corrCorrelationBit
)

// Slog adapts the standard library slog.Logger to the core logging contract.
//
// This adapter keeps the core layer independent from the concrete logging
// implementation while preserving structured logs, contextual attributes,
// configurable log levels, and configurable output formats.
type Slog struct {
	logger *stdslog.Logger
	// corrBits tracks which request-scoped correlation keys were already
	// attached so WithContext does not duplicate them.
	corrBits uint8
}

// New constructs a structured logger backed by the standard library slog package.
//
// human is the primary sink (usually stdout). When nil, logs go to os.Stdout.
//
// machine is an optional second sink (usually LOG_FILE). For JSONFormat it
// always receives compact, single-line JSON suitable for collectors, even when
// pretty is true. TextFormat dual-writes the same text to both sinks.
//
// When level is nil, the minimum log level defaults to INFO.
//
// pretty controls human-oriented formatting and is independent of level:
//
//   - JSONFormat + pretty: indented, ANSI-colored JSON on human; compact JSON
//     on machine when set.
//   - JSONFormat + !pretty: compact JSON on human (and machine when set).
//   - TextFormat: standard slog text; pretty is ignored.
//
// Unknown formats fall back to JSON because structured JSON is generally safer
// for production log collection and processing.
func New(
	human io.Writer,
	machine io.Writer,
	level stdslog.Leveler,
	format corelog.Format,
	pretty bool,
) *Slog {
	if human == nil {
		human = os.Stdout
	}

	if level == nil {
		level = stdslog.LevelInfo
	}

	options := &stdslog.HandlerOptions{
		Level: level,
	}

	var handler stdslog.Handler

	switch format {
	case corelog.TextFormat:
		// Pretty printing is a JSON concern; text dual-writes unchanged.
		handler = stdslog.NewTextHandler(outputWriter(human, machine, false), options)

	case corelog.JSONFormat:
		fallthrough

	default:
		handler = stdslog.NewJSONHandler(outputWriter(human, machine, pretty), options)
	}

	return &Slog{
		logger: stdslog.New(handler),
	}
}

// outputWriter selects the io.Writer chain for slog handlers.
//
// Pretty formatting applies only when pretty is true (JSON callers). Text
// callers pass pretty=false so dual-write stays a plain MultiWriter.
func outputWriter(human, machine io.Writer, pretty bool) io.Writer {
	switch {
	case machine == nil && pretty:
		return newPrettyJSONWriter(human)

	case machine == nil:
		return human

	case pretty:
		return newSplitJSONWriter(human, machine)

	default:
		return io.MultiWriter(human, machine)
	}
}

// NewDefault constructs a production-oriented JSON logger.
//
// The default logger writes compact JSON records to stdout and only emits
// records at INFO level or higher.
func NewDefault() *Slog {
	return New(os.Stdout, nil, stdslog.LevelInfo, corelog.JSONFormat, false)
}

// With returns a child logger enriched with structured attributes.
//
// The returned logger inherits all existing attributes and appends args as
// slog key-value pairs or slog.Attr values.
func (l *Slog) With(args ...any) corelog.Logger {
	return &Slog{
		logger:   l.logger.With(args...),
		corrBits: l.corrBits | correlationBitsFromArgs(args...),
	}
}

// WithContext returns a child logger enriched with trace_id, request_id, and
// correlation_id from ctx when present and not already attached.
//
// The receiver is never mutated. A nil or empty ctx is a no-op.
func (l *Slog) WithContext(ctx context.Context) corelog.Logger {
	if ctx == nil {
		return l
	}

	args := make([]any, 0, 6)
	bits := l.corrBits

	if bits&corrTraceBit == 0 {
		if id := trace.ID(ctx); id != "" {
			args = append(args, field.KeyTraceID, id)
			bits |= corrTraceBit
		}
	}

	if bits&corrRequestBit == 0 {
		if id := trace.RequestID(ctx); id != "" {
			args = append(args, field.KeyRequestID, id)
			bits |= corrRequestBit
		}
	}

	if bits&corrCorrelationBit == 0 {
		if id := trace.CorrelationID(ctx); id != "" {
			args = append(args, field.KeyCorrelationID, id)
			bits |= corrCorrelationBit
		}
	}

	if len(args) == 0 {
		if bits == l.corrBits {
			return l
		}

		return &Slog{logger: l.logger, corrBits: bits}
	}

	return &Slog{
		logger:   l.logger.With(args...),
		corrBits: bits,
	}
}

// Debug records a diagnostic Event.
func (l *Slog) Debug(event corelog.Event, fields ...corelog.Field) {
	l.emit(stdslog.LevelDebug, event, fields...)
}

// Info records a normal operational Event.
func (l *Slog) Info(event corelog.Event, fields ...corelog.Field) {
	l.emit(stdslog.LevelInfo, event, fields...)
}

// Warn records an unexpected but non-fatal Event.
func (l *Slog) Warn(event corelog.Event, fields ...corelog.Field) {
	l.emit(stdslog.LevelWarn, event, fields...)
}

// Error records a failed operation Event.
func (l *Slog) Error(event corelog.Event, fields ...corelog.Field) {
	l.emit(stdslog.LevelError, event, fields...)
}

func (l *Slog) emit(level stdslog.Level, event corelog.Event, fields ...corelog.Field) {
	if event == nil {
		return
	}

	message := event.Message()
	attrs := make([]any, 0, 2+len(fields)*2)

	if code := event.Code(); code != "" {
		attrs = append(attrs, field.KeyEvent, code)
	}

	for _, f := range fields {
		key, value, ok := field.Sanitize(f)
		if !ok {
			continue
		}

		attrs = append(attrs, key, value)
	}

	l.logger.Log(context.Background(), level, message, attrs...)
}

func correlationBitsFromArgs(args ...any) uint8 {
	var bits uint8

	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}

		switch key {
		case field.KeyTraceID:
			bits |= corrTraceBit

		case field.KeyRequestID:
			bits |= corrRequestBit

		case field.KeyCorrelationID:
			bits |= corrCorrelationBit
		}
	}

	return bits
}
