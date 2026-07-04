//go:build production

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adaptertrace "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
)

func TestGin_newEngine_HTTPDebugLoggerExcludedInProductionBuild(t *testing.T) {
	t.Parallel()

	engine, err := (Gin{
		Mode:   "test",
		Logger: false,
		HTTPDebugLogger: HTTPDebugLogger{
			Enabled: true, // ignored at compile-time registration
		},
	}).newEngine(adaptertrace.NewULIDProvider(), lognoop.New(), nil)
	require.NoError(t, err)

	// PanicRecovery + TraceID + Language + AdmissionControl + ErrorHandler
	assert.Len(t, engine.Handlers, 5)
}

func TestGin_validate_HTTPDebugLoggerRejectedByProductionBuildProfile(t *testing.T) {
	t.Parallel()

	err := (Gin{
		HTTPDebugLogger: HTTPDebugLogger{Enabled: true},
	}).validate("development")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "gin.http_debug_logger.enabled must be false in production")
}
