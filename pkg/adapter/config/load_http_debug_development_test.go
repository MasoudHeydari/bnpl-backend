//go:build development

package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_HTTPDebugLoggerAndResponseLoggingEnvironmentVariables(t *testing.T) {
	t.Setenv("GIN_HTTP_DEBUG_LOGGER_ENABLED", "true")
	t.Setenv("GIN_HTTP_DEBUG_LOGGER_MAX_REQUEST_BODY_BYTES", "8192")
	t.Setenv("GIN_HTTP_DEBUG_LOGGER_MAX_RESPONSE_BODY_BYTES", "4096")
	t.Setenv("GIN_RESPONSE_LOGGING_ENABLED", "true")
	t.Setenv("GIN_RESPONSE_LOGGING_MAX_BODY_BYTES", "2048")

	content := validVersionsYAML + validMinimalValidationYAML + `
gin:
  logger: true
  http_debug_logger:
    enabled: ${GIN_HTTP_DEBUG_LOGGER_ENABLED}
    max_request_body_bytes: ${GIN_HTTP_DEBUG_LOGGER_MAX_REQUEST_BODY_BYTES}
    max_response_body_bytes: ${GIN_HTTP_DEBUG_LOGGER_MAX_RESPONSE_BODY_BYTES}
  response_logging:
    enabled: ${GIN_RESPONSE_LOGGING_ENABLED}
    max_body_bytes: ${GIN_RESPONSE_LOGGING_MAX_BODY_BYTES}
`

	cfg, err := Load(writeConfigFile(t, content))
	require.NoError(t, err)
	assert.True(t, cfg.Gin.HTTPDebugLogger.Enabled)
	assert.Equal(t, int64(8192), cfg.Gin.HTTPDebugLogger.MaxRequestBodyBytes)
	assert.Equal(t, int64(4096), cfg.Gin.HTTPDebugLogger.MaxResponseBodyBytes)
	assert.True(t, cfg.Gin.ResponseLogging.Enabled)
	assert.Equal(t, int64(2048), cfg.Gin.ResponseLogging.MaxBodyBytes)
}
