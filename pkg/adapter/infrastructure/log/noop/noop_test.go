package noop_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
)

func TestWithReturnsSameLogger(t *testing.T) {
	logger := lognoop.New()

	child := logger.With(
		"trace_id", "trace-123",
		"request_id", "request-123",
	)

	require.Same(t, logger, child)
	require.Same(t, logger, logger.WithContext(context.Background()))
}

func TestLoggingMethodsDoNotPanic(t *testing.T) {
	logger := lognoop.New()

	require.NotPanics(t, func() {
		logger.Debug(corelog.Msg("debug"))
		logger.Info(corelog.Msg("info"))
		logger.Warn(corelog.Msg("warn"))
		logger.Error(corelog.Msg("error"))

		logger.Debug(corelog.Msg("debug"), corelog.TextAttr("key", "value"))
		logger.Info(corelog.Msg("info"), corelog.TextAttr("key", "value"))
		logger.Warn(corelog.Msg("warn"), corelog.TextAttr("key", "value"))
		logger.Error(corelog.Msg("error"), corelog.TextAttr("key", "value"))
	})
}
