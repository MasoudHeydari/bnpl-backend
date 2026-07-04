package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

func TestAccessLogger_ResponseLoggingDisabledDoesNotCapture(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(TraceID(provider), AccessLogger(logger, AccessLoggerOptions{}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP", "secret": "MUST_NEVER_APPEAR"})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "MUST_NEVER_APPEAR")

	records := logger.snapshot()
	require.Len(t, records, 1)
	_, hasBody := attrValue(records[0].attrs, "http.response_body")
	assert.False(t, hasBody)
	assert.False(t, attrsContainString(records[0].attrs, "MUST_NEVER_APPEAR"))
}

func TestAccessLogger_ResponseLoggingAllowlistsApprovedFieldsOnly(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(TraceID(provider), AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true, MaxBodyBytes: 4096},
	}))
	engine.GET("/api/v1/customer-credit", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"data": gin.H{
				"status":              "active",
				"future_secret_field": "MUST_NEVER_APPEAR_IN_LOGS",
			},
			"meta": gin.H{
				"status":  "success",
				"code":    "CUSTOMER_CREDIT_NOT_ASSIGNED",
				"message": "must not be logged",
			},
		})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/customer-credit", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "MUST_NEVER_APPEAR_IN_LOGS")
	require.Contains(t, rec.Body.String(), "must not be logged")

	records := logger.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, "http request completed", records[0].msg)

	body := attrMap(t, records[0].attrs, "http.response_body")
	assert.Equal(t, true, body["body_present"])
	assert.Equal(t, true, body["logged"])
	assert.Equal(t, true, body["filtered"])
	assert.Equal(t, responseBodyReasonLogged, body["reason"])

	fields := body["fields"].(map[string]any)
	meta := fields["meta"].(map[string]any)
	assert.Equal(t, "success", meta["status"])
	assert.Equal(t, "CUSTOMER_CREDIT_NOT_ASSIGNED", meta["code"])
	_, hasMessage := meta["message"]
	assert.False(t, hasMessage)
	_, hasData := fields["data"]
	assert.False(t, hasData)

	assert.False(t, attrsContainString(records[0].attrs, "MUST_NEVER_APPEAR_IN_LOGS"))
	assert.False(t, attrsContainString(records[0].attrs, "must not be logged"))
}

func TestAccessLogger_ResponseLoggingNoPolicyFailsClosed(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/api/v1/unknown-route", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"meta": gin.H{"status": "success", "code": "X"}})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/unknown-route", nil))

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, true, body["body_present"])
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonNoAllowlistPolicy, body["reason"])
	_, hasFields := body["fields"]
	assert.False(t, hasFields)
}

func TestAccessLogger_ResponseLoggingEmptyBody(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, false, body["body_present"])
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonEmptyBody, body["reason"])
}

func TestAccessLogger_ResponseLoggingUnsupportedContentType(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/plain", []byte("UP"))
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, true, body["body_present"])
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonUnsupportedContentType, body["reason"])
}

func TestAccessLogger_ResponseLoggingInvalidJSON(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.Header("Content-Type", "application/json")
		_, _ = c.Writer.WriteString(`{"status":`)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, true, body["body_present"])
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonInvalidJSON, body["reason"])
}

func TestAccessLogger_ResponseLoggingBodyTooLarge(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true, MaxBodyBytes: 32},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		payload := map[string]any{"status": "UP", "padding": strings.Repeat("x", 64)}
		raw, err := json.Marshal(payload)
		require.NoError(t, err)
		c.Data(http.StatusOK, "application/json", raw)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	require.Contains(t, rec.Body.String(), `"status":"UP"`)

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, true, body["body_present"])
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonBodyTooLarge, body["reason"])
	assert.Equal(t, true, body["truncated"])
}

func TestAccessLogger_ResponseLoggingEncodedFailsClosed(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.Header("Content-Encoding", "gzip")
		c.Header("Content-Type", "application/json")
		_, _ = c.Writer.WriteString(`{"status":"UP"}`)
		c.Status(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, false, body["logged"])
	assert.Equal(t, responseBodyReasonEncodedResponseUnsupported, body["reason"])
}

func TestAccessLogger_ResponseLoggingDoesNotLeakSensitiveHeaders(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.Header("Set-Cookie", "session=secret-cookie")
		c.Header("Location", "https://example.test/secret")
		c.Header("Authorization", "Bearer secret")
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	attrs := logger.snapshot()[0].attrs
	assert.False(t, attrsContainString(attrs, "secret-cookie"))
	assert.False(t, attrsContainString(attrs, "https://example.test/secret"))
	assert.False(t, attrsContainString(attrs, "Bearer secret"))

	_, hasResponseHeaders := attrValue(attrs, "http.response_headers")
	assert.False(t, hasResponseHeaders)
}

func TestAccessLogger_ResponseLoggingErrorEnvelopeCodeOnly(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/api/v1/customer-credit", func(c *gin.Context) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "INVALID_ARGUMENT",
			"message": "must not leak",
			"details": gin.H{"fields": []string{"otp"}},
		})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/customer-credit", nil))

	body := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, true, body["logged"])
	fields := body["fields"].(map[string]any)
	assert.Equal(t, "INVALID_ARGUMENT", fields["code"])
	_, hasMessage := fields["message"]
	assert.False(t, hasMessage)
	assert.False(t, attrsContainString(logger.snapshot()[0].attrs, "must not leak"))
}

func TestAccessLogger_ResponseLoggingSingleAccessRecord(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{
		ResponseLogging: ResponseLoggingOptions{Enabled: true},
	}))
	engine.GET("/healthz/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "UP"})
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz/live", nil))

	records := logger.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, "http request completed", records[0].msg)
}
