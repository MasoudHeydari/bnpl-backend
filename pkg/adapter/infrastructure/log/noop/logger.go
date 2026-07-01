package noop

import corelog "github.com/parstis/bnpl-backend/pkg/core/log"

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

// Debug discards the log record.
func (l *Logger) Debug(string, ...any) {}

// Info discards the log record.
func (l *Logger) Info(string, ...any) {}

// Warn discards the log record.
func (l *Logger) Warn(string, ...any) {}

// Error discards the log record.
func (l *Logger) Error(string, ...any) {}
