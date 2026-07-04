package noop_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
)

func TestWithReturnsSameLogger(t *testing.T) {
	logger := lognoop.New()

	child := logger.With(
		"trace_id", "trace-123",
		"request_id", "request-123",
	)

	require.Same(t, logger, child)
}

func TestLoggingMethodsDoNotPanic(t *testing.T) {
	logger := lognoop.New()

	require.NotPanics(t, func() {
		logger.Debug("debug")
		logger.Info("info")
		logger.Warn("warn")
		logger.Error("error")

		logger.Debug("debug", "key", "value")
		logger.Info("info", "key", "value")
		logger.Warn("warn", "key", "value")
		logger.Error("error", "key", "value")
	})
}
