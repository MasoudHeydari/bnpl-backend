package slog

import (
	"io"
	"log/slog"
	"os"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// Verify Slog implements the core Logger contract at compile time.
var _ corelog.Logger = (*Slog)(nil)

// Slog adapts slog.Logger to the core logging contract.
//
// It keeps the core layer independent from the standard library logging
// implementation while still allowing structured logs, contextual attributes,
// and configurable output formats.
type Slog struct {
	logger *slog.Logger
}

// New constructs a structured logger backed by slog.
//
// When w is nil, logs are written to os.Stdout. JSONFormat emits JSON records;
// TextFormat emits human-readable text records. Unknown formats fall back to
// JSON because it is safer for production log collection.
func New(w io.Writer, level slog.Leveler, format corelog.Format) *Slog {
	if w == nil {
		w = os.Stdout
	}

	if level == nil {
		level = slog.LevelInfo
	}

	var handler slog.Handler
	switch format {
	case corelog.TextFormat:
		handler = slog.NewTextHandler(w, &slog.HandlerOptions{
			Level: level,
		})
	default:
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{
			Level: level,
		})
	}

	return &Slog{
		logger: slog.New(handler),
	}
}

// NewDefault constructs a production-oriented JSON logger writing to stdout.
func NewDefault() *Slog {
	return New(os.Stdout, slog.LevelInfo, corelog.JSONFormat)
}

// With returns a child logger enriched with structured attributes.
//
// The returned logger inherits existing attributes and appends args as slog
// key-value pairs or slog.Attr values.
func (l *Slog) With(args ...any) corelog.Logger {
	return &Slog{
		logger: l.logger.With(args...),
	}
}

// Debug records diagnostic information.
func (l *Slog) Debug(msg string, args ...any) {
	l.logger.Debug(msg, args...)
}

// Info records a normal operational event.
func (l *Slog) Info(msg string, args ...any) {
	l.logger.Info(msg, args...)
}

// Warn records an unexpected but non-fatal condition.
func (l *Slog) Warn(msg string, args ...any) {
	l.logger.Warn(msg, args...)
}

// Error records a failed operation or condition requiring attention.
func (l *Slog) Error(msg string, args ...any) {
	l.logger.Error(msg, args...)
}
