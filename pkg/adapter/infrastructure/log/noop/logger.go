package noop

import (
	"context"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

// Verify Logger implements the core logging contract.
var _ corelog.Logger = (*Logger)(nil)

// Logger is a no-op implementation of corelog.Logger.
type Logger struct{}

// New creates a new no-op logger.
func New() *Logger {
	return &Logger{}
}

// With returns the same logger instance.
func (l *Logger) With(_ ...any) corelog.Logger {
	return l
}

// WithContext returns the same logger instance.
func (l *Logger) WithContext(context.Context) corelog.Logger {
	return l
}

// Debug discards the log record.
func (l *Logger) Debug(corelog.Event, ...corelog.Field) {}

// Info discards the log record.
func (l *Logger) Info(corelog.Event, ...corelog.Field) {}

// Warn discards the log record.
func (l *Logger) Warn(corelog.Event, ...corelog.Field) {}

// Error discards the log record.
func (l *Logger) Error(corelog.Event, ...corelog.Field) {}
