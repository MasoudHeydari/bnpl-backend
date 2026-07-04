package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/parstis/bnpl-backend/pkg/core/mock"
)

func TestAccessLogger_UsesApplicationLoggerWithBoundedRequestMetadata(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(TraceID(provider), AccessLogger(logger, AccessLoggerOptions{}))
	engine.GET("/contracts/:contract_id", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req := httptest.NewRequest(
		http.MethodGet,
		"/contracts/contract-123?token=must-not-be-logged",
		nil,
	)
	req.Header.Set("User-Agent", "sensitive-client-details")
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	records := logger.snapshot()
	require.Len(t, records, 1)

	record := records[0]
	assert.Equal(t, "info", record.level)
	assert.Equal(t, "http request completed", record.msg)
	assert.Equal(t, "trace-1", attrString(t, record.attrs, "trace_id"))
	assert.Equal(t, "request-1", attrString(t, record.attrs, "request_id"))
	assert.Equal(t, "correlation-1", attrString(t, record.attrs, "correlation_id"))
	assert.Equal(t, http.MethodGet, attrString(t, record.attrs, "http.method"))
	assert.Equal(t, "/contracts/:contract_id", attrString(t, record.attrs, "http.route"))

	status, ok := attrValue(record.attrs, "http.status_code")
	require.True(t, ok)
	assert.Equal(t, http.StatusNoContent, status)

	_, hasPath := attrValue(record.attrs, "http.path")
	_, hasQuery := attrValue(record.attrs, "http.raw_query")
	_, hasUserAgent := attrValue(record.attrs, "http.user_agent")
	assert.False(t, hasPath)
	assert.False(t, hasQuery)
	assert.False(t, hasUserAgent)
}

func TestAccessLogger_SeesStatusWrittenByErrorHandler(t *testing.T) {
	logger := newRecordingLogger()
	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-1").Once()
	provider.EXPECT().NewRequestID().Return("request-1").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-1").Once()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(
		TraceID(provider),
		AccessLogger(logger, AccessLoggerOptions{}),
		ErrorHandler(logger, nil),
	)
	engine.GET("/failure", func(c *gin.Context) {
		_ = c.Error(errors.New("database exploded"))
		c.Abort()
	})

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/failure", nil))

	require.Equal(t, http.StatusInternalServerError, rec.Code)

	var accessRecord *logRecord
	records := logger.snapshot()
	for i := range records {
		if records[i].msg == "http request completed" {
			accessRecord = &records[i]
			break
		}
	}

	require.NotNil(t, accessRecord)
	status, ok := attrValue(accessRecord.attrs, "http.status_code")
	require.True(t, ok)
	assert.Equal(t, http.StatusInternalServerError, status)
	assert.Equal(t, "info", accessRecord.level)
}

func TestAccessLogger_UsesStableRouteForUnmatchedRequest(t *testing.T) {
	logger := newRecordingLogger()

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(AccessLogger(logger, AccessLoggerOptions{}))

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing/123", nil))

	records := logger.snapshot()
	require.Len(t, records, 1)
	assert.Equal(t, unmatchedRoute, attrString(t, records[0].attrs, "http.route"))
}
