package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/parstis/bnpl-backend/pkg/adapter/restful/header"
	"github.com/parstis/bnpl-backend/pkg/core/mock"
	"github.com/parstis/bnpl-backend/pkg/core/trace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTraceID_ReusesIncomingHeaders(t *testing.T) {
	t.Parallel()

	provider := mock.NewProvider(t)

	var capturedTraceID, capturedRequestID, capturedCorrelationID string
	e := newTraceEngine(provider, func(c *gin.Context) {
		ctx := c.Request.Context()
		capturedTraceID = trace.TraceID(ctx)
		capturedRequestID = trace.RequestID(ctx)
		capturedCorrelationID = trace.CorrelationID(ctx)
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(header.TraceID, "trace-in")
	req.Header.Set(header.RequestID, "request-in")
	req.Header.Set(header.CorrelationID, "correlation-in")

	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	assert.Equal(t, "trace-in", w.Header().Get(header.TraceID))
	assert.Equal(t, "request-in", w.Header().Get(header.RequestID))
	assert.Equal(t, "correlation-in", w.Header().Get(header.CorrelationID))
	assert.Equal(t, "trace-in", capturedTraceID)
	assert.Equal(t, "request-in", capturedRequestID)
	assert.Equal(t, "correlation-in", capturedCorrelationID)
}

func TestTraceID_GeneratesMissingHeaders(t *testing.T) {
	t.Parallel()

	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-gen").Once()
	provider.EXPECT().NewRequestID().Return("request-gen").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-gen").Once()

	var capturedTraceID, capturedRequestID, capturedCorrelationID string
	e := newTraceEngine(provider, func(c *gin.Context) {
		ctx := c.Request.Context()
		capturedTraceID = trace.TraceID(ctx)
		capturedRequestID = trace.RequestID(ctx)
		capturedCorrelationID = trace.CorrelationID(ctx)
	})

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	assert.Equal(t, "trace-gen", w.Header().Get(header.TraceID))
	assert.Equal(t, "request-gen", w.Header().Get(header.RequestID))
	assert.Equal(t, "correlation-gen", w.Header().Get(header.CorrelationID))
	assert.Equal(t, "trace-gen", capturedTraceID)
	assert.Equal(t, "request-gen", capturedRequestID)
	assert.Equal(t, "correlation-gen", capturedCorrelationID)
}

func TestTraceID_GeneratesOnlyMissingHeaders(t *testing.T) {
	t.Parallel()

	provider := mock.NewProvider(t)
	provider.EXPECT().NewRequestID().Return("request-gen").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-gen").Once()

	e := newTraceEngine(provider, func(c *gin.Context) {})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(header.TraceID, "trace-in")

	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)

	assert.Equal(t, "trace-in", w.Header().Get(header.TraceID))
	assert.Equal(t, "request-gen", w.Header().Get(header.RequestID))
	assert.Equal(t, "correlation-gen", w.Header().Get(header.CorrelationID))
}

func TestTraceID_CallsNextHandler(t *testing.T) {
	t.Parallel()

	provider := mock.NewProvider(t)
	provider.EXPECT().NewTraceID().Return("trace-gen").Once()
	provider.EXPECT().NewRequestID().Return("request-gen").Once()
	provider.EXPECT().NewCorrelationID().Return("correlation-gen").Once()

	nextCalled := false
	e := newTraceEngine(provider, func(c *gin.Context) {
		nextCalled = true
		c.Status(http.StatusNoContent)
	})

	w := httptest.NewRecorder()
	e.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))

	require.True(t, nextCalled)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func newTraceEngine(provider *mock.Provider, handler gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)

	e := gin.New()
	e.Use(TraceID(provider))
	e.GET("/", handler)

	return e
}
