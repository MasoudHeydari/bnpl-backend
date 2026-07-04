//go:build production

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Production build smoke: safe response logging works without the Development
// HTTPDebugLogger symbol being present in this compilation unit.
func TestAccessLogger_ResponseLoggingAvailableInProductionBuild(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "error": "ignored"})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"UP"`)

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, true, body["logged"])
	fields := body["fields"].(map[string]any)
	assert.Equal(t, "UP", fields["status"])
	_, hasError := fields["error"]
	assert.False(t, hasError)
	assert.False(t, attrsContainString(logger.snapshot()[0].attrs, "ignored"))
}
