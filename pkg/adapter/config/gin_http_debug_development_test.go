//go:build development

package config

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	going "github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	adaptertrace "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/trace"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
)

func TestGin_newEngine_HTTPDebugLoggerDisabledByDefault(t *testing.T) {
	t.Parallel()

	engine, err := (Gin{Mode: "test"}).newEngine(
		adaptertrace.NewULIDProvider(),
		lognoop.New(),
		nil,
	)
	require.NoError(t, err)
	assert.Len(t, engine.Handlers, 5) // PanicRecovery + TraceID + Language + AdmissionControl + ErrorHandler
}

func TestGin_newEngine_HTTPDebugLoggerMiddlewareCountWhenEnabled(t *testing.T) {
	t.Parallel()

	provider := adaptertrace.NewULIDProvider()
	logger := lognoop.New()

	engine, err := (Gin{
		Mode: "test",
		HTTPDebugLogger: HTTPDebugLogger{
			Enabled:             true,
			MaxRequestBodyBytes: 1024,
		},
	}).newEngine(provider, logger, nil)
	require.NoError(t, err)
	assert.Len(t, engine.Handlers, 6)

	engine, err = (Gin{
		Mode:   "test",
		Logger: true,
		HTTPDebugLogger: HTTPDebugLogger{
			Enabled: true,
		},
	}).newEngine(provider, logger, nil)
	require.NoError(t, err)
	assert.Len(t, engine.Handlers, 7)
}

func TestGin_newEngine_HTTPDebugLoggerEmitsOnlyWhenEnabled(t *testing.T) {
	provider := adaptertrace.NewULIDProvider()

	t.Run("disabled", func(t *testing.T) {
		logger := newConfigRecordingLogger()
		engine, err := (Gin{Mode: "test"}).newEngine(provider, logger, nil)
		require.NoError(t, err)

		engine.GET("/ping", func(c *going.Context) {
			c.Status(http.StatusNoContent)
		})

		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
		require.Equal(t, http.StatusNoContent, rec.Code)
		assert.Empty(t, logger.snapshot())
	})

	t.Run("enabled", func(t *testing.T) {
		logger := newConfigRecordingLogger()
		engine, err := (Gin{
			Mode: "test",
			HTTPDebugLogger: HTTPDebugLogger{
				Enabled:             true,
				MaxRequestBodyBytes: 128,
			},
		}).newEngine(provider, logger, nil)
		require.NoError(t, err)

		engine.POST("/echo", func(c *going.Context) {
			body, readErr := io.ReadAll(c.Request.Body)
			require.NoError(t, readErr)
			assert.Equal(t, `{"ok":true}`, string(body))
			c.Status(http.StatusCreated)
		})

		req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(`{"ok":true}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		engine.ServeHTTP(rec, req)

		require.Equal(t, http.StatusCreated, rec.Code)
		records := logger.snapshot()
		require.Len(t, records, 1)
		assert.Equal(t, "http request debug exchange", records[0].msg)
		assert.NotEmpty(t, rec.Header().Get(header.TraceID))
	})
}
