package temporal

import (
	"github.com/parstis/bnpl-backend/pkg/core/log"
)

// sdkLogger adapts corelog.Logger to Temporal's SDK log.Logger contract
// (string message + slog-style keyvals).
type sdkLogger struct {
	logger log.Logger
}

func newSDKLogger(logger log.Logger) *sdkLogger {
	return &sdkLogger{logger: logger}
}

func (l *sdkLogger) Debug(msg string, keyvals ...any) {
	l.logger.With(keyvals...).Debug(log.Msg(msg))
}

func (l *sdkLogger) Info(msg string, keyvals ...any) {
	l.logger.With(keyvals...).Info(log.Msg(msg))
}

func (l *sdkLogger) Warn(msg string, keyvals ...any) {
	l.logger.With(keyvals...).Warn(log.Msg(msg))
}

func (l *sdkLogger) Error(msg string, keyvals ...any) {
	l.logger.With(keyvals...).Error(log.Msg(msg))
}
