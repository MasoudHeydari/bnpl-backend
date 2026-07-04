//go:build development

package middleware

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

func TestHTTPDebugLogger_PreservesBodyForHandler(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	payload := `{"identity_subject_id":"550e8400-e29b-41d4-a716-446655440000","otp":"123456"}`
	var handlerBody string

	engine := newHTTPDebugEngine(
		provider,
		logger,
		HTTPDebugLoggerOptions{MaxRequestBodyBytes: 1024},
		func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			handlerBody = string(body)
			c.Status(http.StatusNoContent)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, payload, handlerBody)

	records := logger.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, "debug", records[0].level)
	assert.Equal(t, "http request debug exchange", records[0].msg)
	assert.Equal(t, "trace-1", attrString(t, records[0].attrs, "trace_id"))
	assert.Equal(t, "request-1", attrString(t, records[0].attrs, "request_id"))
	assert.Equal(t, "correlation-1", attrString(t, records[0].attrs, "correlation_id"))
}

func TestHTTPDebugLogger_DoesNotRedactHeadersQueryOrJSON(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	payload := map[string]any{
		"password": "test-password-not-real",
		"otp":      "654321",
		"token":    "test-token-abc",
		"safe":     "ok",
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)

	engine := newHTTPDebugEngine(
		provider,
		logger,
		HTTPDebugLoggerOptions{},
		func(c *gin.Context) {
			_, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			c.Header("Set-Cookie", "session=test-session-cookie")
			c.Header("Location", "https://example.test/redirect?token=test-redirect-token")
			c.JSON(http.StatusCreated, gin.H{
				"access_token": "test-response-token",
				"otp":          "111222",
			})
		},
	)

	req := httptest.NewRequest(
		http.MethodPost,
		"/?access_token=tok&otp=999888&safe=1",
		bytes.NewReader(body),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-secret-token")
	req.Header.Set("Cookie", "session=abc")
	req.Header.Set("X-Auth-Subject-Id", "550e8400-e29b-41d4-a716-446655440000")

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), "test-response-token")

	record := logger.snapshot()[0]
	headers := attrMap(t, record.attrs, "http.request_headers")
	assert.Equal(t, []string{"Bearer test-secret-token"}, headerValue(t, headers, "Authorization"))
	assert.Equal(t, []string{"session=abc"}, headerValue(t, headers, "Cookie"))
	assert.Equal(
		t,
		[]string{"550e8400-e29b-41d4-a716-446655440000"},
		headerValue(t, headers, "X-Auth-Subject-Id"),
	)

	queryValues, err := url.ParseQuery(attrString(t, record.attrs, "http.query"))
	require.NoError(t, err)
	assert.Equal(t, []string{"tok"}, queryValues["access_token"])
	assert.Equal(t, []string{"999888"}, queryValues["otp"])

	loggedBody := attrMap(t, record.attrs, "http.request_body")
	assert.Equal(t, "test-password-not-real", loggedBody["password"])
	assert.Equal(t, "654321", loggedBody["otp"])
	assert.Equal(t, "test-token-abc", loggedBody["token"])

	responseHeaders := attrMap(t, record.attrs, "http.response_headers")
	assert.Equal(t, []string{"session=test-session-cookie"}, headerValue(t, responseHeaders, "Set-Cookie"))
	assert.Equal(
		t,
		[]string{"https://example.test/redirect?token=test-redirect-token"},
		headerValue(t, responseHeaders, "Location"),
	)

	responseBody := attrMap(t, record.attrs, "http.response_body")
	assert.Equal(t, "test-response-token", responseBody["access_token"])
	assert.Equal(t, "111222", responseBody["otp"])
}

func TestHTTPDebugLogger_PreservesCheckoutTokenInPathAndReferer(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	token := "cs_0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	referer := "https://bnpl.example/api/v1/hosted/checkout-sessions/" + token + "/confirm"

	engine := newHTTPDebugEngine(
		provider,
		logger,
		HTTPDebugLoggerOptions{},
		func(c *gin.Context) {
			c.Status(http.StatusOK)
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosted/checkout-sessions/"+token, nil)
	req.Header.Set("Referer", referer)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	record := logger.snapshot()[0]
	assert.Equal(t, referer, attrString(t, record.attrs, "http.referer"))
	assert.Equal(t, "/api/v1/hosted/checkout-sessions/"+token, attrString(t, record.attrs, "http.path"))
	assert.Contains(t, attrString(t, record.attrs, "http.path"), token)
}

func TestHTTPDebugLogger_TruncatesLoggedBodiesButPreservesWireBytes(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	fullBody := strings.Repeat("a", 40)
	responseBody := strings.Repeat("b", 40)
	var handlerBody string

	engine := newHTTPDebugEngine(
		provider,
		logger,
		HTTPDebugLoggerOptions{MaxRequestBodyBytes: 10, MaxResponseBodyBytes: 10},
		func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			handlerBody = string(body)
			c.Header("Content-Type", "text/plain")
			_, err = c.Writer.WriteString(responseBody)
			require.NoError(t, err)
			c.Status(http.StatusOK)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(fullBody))
	req.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	assert.Equal(t, fullBody, handlerBody)
	assert.Equal(t, responseBody, rec.Body.String())
	require.True(t, attrBool(t, logger.snapshot()[0].attrs, "http.request_body_truncated"))
	require.True(t, attrBool(t, logger.snapshot()[0].attrs, "http.response_body_truncated"))

	loggedRequest := attrMap(t, logger.snapshot()[0].attrs, "http.request_body")
	assert.True(t, loggedRequest["truncated"].(bool))
	assert.Equal(t, int64(10), loggedRequest["limit_bytes"])
}

func TestHTTPDebugLogger_BinaryBodyUsesBase64(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	binary := []byte{0x00, 0xff, 0x10, 0x80}

	engine := newHTTPDebugEngine(
		provider,
		logger,
		HTTPDebugLoggerOptions{},
		func(c *gin.Context) {
			_, err := io.ReadAll(c.Request.Body)
			require.NoError(t, err)
			c.Data(http.StatusOK, "application/octet-stream", binary)
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(binary))
	req.Header.Set("Content-Type", "application/octet-stream")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, binary, rec.Body.Bytes())

	requestBody := attrMap(t, logger.snapshot()[0].attrs, "http.request_body")
	assert.Equal(t, "base64", requestBody["encoding"])
	decoded, err := base64.StdEncoding.DecodeString(requestBody["value"].(string))
	require.NoError(t, err)
	assert.Equal(t, binary, decoded)

	responseBody := attrMap(t, logger.snapshot()[0].attrs, "http.response_body")
	assert.Equal(t, "base64", responseBody["encoding"])
}

func TestHTTPDebugLogger_WriteAndWriteStringCaptured(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	engine := newHTTPDebugEngine(
		provider,
		logger,
		HTTPDebugLoggerOptions{},
		func(c *gin.Context) {
			c.Header("Content-Type", "text/plain")
			_, err := c.Writer.Write([]byte("part-a-"))
			require.NoError(t, err)
			_, err = c.Writer.WriteString("part-b")
			require.NoError(t, err)
			c.Status(http.StatusOK)
		},
	)

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, "part-a-part-b", rec.Body.String())
	assert.Equal(t, "part-a-part-b", attrString(t, logger.snapshot()[0].attrs, "http.response_body"))
}

func TestHTTPDebugLogger_DisabledPathHasNoCaptureWhenNotRegistered(t *testing.T) {
	// When the middleware is not registered, handlers still work and no debug
	// records are emitted. Registration itself is gated by config.
	logger := newRecordingLogger()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, "ok", rec.Body.String())
	assert.Empty(t, logger.snapshot())
}

func TestHTTPDebugLogger_SeesErrorHandlerSerializedStatus(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(
		TraceID(provider),
		HTTPDebugLogger(logger, HTTPDebugLoggerOptions{}),
		ErrorHandler(logger, nil),
	)
	engine.GET("/failure", func(c *gin.Context) {
		_ = c.Error(errors.New("database exploded"))
		c.Abort()
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/failure", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	records := logger.snapshot()
	var debugRecord *logRecord
	for i := range records {
		if records[i].msg == "http request debug exchange" {
			debugRecord = &records[i]
			break
		}
	}

	require.NotNil(t, debugRecord)
	status, ok := attrValue(debugRecord.attrs, "http.status_code")
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "error", debugRecord.level)
}

func newHTTPDebugEngine(
	provider *mock.Provider,
	logger corelog.Logger,
	opts HTTPDebugLoggerOptions,
	handler gin.HandlerFunc,
) *gin.Engine {
	gin.SetMode(gin.TestMode)

	e := gin.New()
	e.Use(TraceID(provider), HTTPDebugLogger(logger, opts))
	e.Any("/", handler)
	e.GET("/api/v1/hosted/checkout-sessions/:token", handler)

	return e
}
