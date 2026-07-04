package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	"github.com/parstis/bnpl-backend/pkg/core/cerr"
	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
)

type reportingCapture struct {
	err       error
	recovered any
	metadata  coreerrorreporting.Metadata
}

type recordingErrorReporter struct {
	mu      sync.Mutex
	errors  []reportingCapture
	panics  []reportingCapture
	flushes int
	closed  bool
}

func (r *recordingErrorReporter) CaptureError(
	_ context.Context,
	err error,
	metadata coreerrorreporting.Metadata,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.errors = append(r.errors, reportingCapture{err: err, metadata: metadata})
}

func (r *recordingErrorReporter) CapturePanic(
	_ context.Context,
	recovered any,
	metadata coreerrorreporting.Metadata,
) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.panics = append(r.panics, reportingCapture{recovered: recovered, metadata: metadata})
}

func (r *recordingErrorReporter) Flush(context.Context) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.flushes++
	return true
}

func (r *recordingErrorReporter) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	return nil
}

func (r *recordingErrorReporter) snapshot() ([]reportingCapture, []reportingCapture) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]reportingCapture(nil), r.errors...), append([]reportingCapture(nil), r.panics...)
}

func TestErrorHandler_ReportsOnlyServerErrors(t *testing.T) {

	reporter := &recordingErrorReporter{}
	engine := newErrorReportingEngine(reporter)

	engine.GET("/server-error", func(c *gin.Context) {
		_ = c.Error(errors.New("database unavailable"))
		c.Abort()
	})
	engine.GET("/client-error", func(c *gin.Context) {
		_ = c.Error(cerr.NewPublic(
			cerr.CodeInvalidArgument,
			cerr.DefaultMessageKey(cerr.CodeInvalidArgument),
			errors.New("invalid request"),
			cerr.Details{},
		))
		c.Abort()
	})

	serverRecorder := httptest.NewRecorder()
	engine.ServeHTTP(serverRecorder, httptest.NewRequest(http.MethodGet, "/server-error", nil))
	require.Equal(t, http.StatusInternalServerError, serverRecorder.Code)

	clientRecorder := httptest.NewRecorder()
	engine.ServeHTTP(clientRecorder, httptest.NewRequest(http.MethodGet, "/client-error", nil))
	require.Equal(t, http.StatusBadRequest, clientRecorder.Code)

	errorsSeen, panicsSeen := reporter.snapshot()
	require.Len(t, errorsSeen, 1)
	assert.Empty(t, panicsSeen)
	assert.EqualError(t, errorsSeen[0].err, "database unavailable")
	assert.Equal(t, "http", errorsSeen[0].metadata.Component)
	assert.Equal(t, string(cerr.CodeInternalError), errorsSeen[0].metadata.Tags["error_code"])
	assert.Equal(t, "500", errorsSeen[0].metadata.Tags["http.status_code"])
	assert.Equal(t, http.MethodGet, errorsSeen[0].metadata.Tags["http.method"])
	assert.Equal(t, "/server-error", errorsSeen[0].metadata.Tags["http.route"])
}

func TestPanicRecovery_ReportsAndConsumesPanic(t *testing.T) {
	reporter := &recordingErrorReporter{}
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// PanicRecovery is a gin.RecoveryFunc; wire it the same way production does.
	engine.Use(gin.CustomRecovery(PanicRecovery(lognoop.New(), reporter)))
	engine.GET("/panic", func(*gin.Context) { panic("boom") })

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	errorsSeen, panicsSeen := reporter.snapshot()
	assert.Empty(t, errorsSeen)
	require.Len(t, panicsSeen, 1)
	assert.Equal(t, "boom", panicsSeen[0].recovered)
	assert.Equal(t, "http", panicsSeen[0].metadata.Component)
	assert.Equal(t, "PANIC", panicsSeen[0].metadata.Tags["error_code"])
	assert.Equal(t, "/panic", panicsSeen[0].metadata.Tags["http.route"])
}

func TestPanicRecovery_RecoversWithoutReporter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(gin.CustomRecovery(PanicRecovery(lognoop.New(), nil)))
	engine.GET("/panic", func(*gin.Context) { panic("boom") })

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/panic", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
}

func newErrorReportingEngine(reporter coreerrorreporting.Reporter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(ErrorHandler(lognoop.New(), reporter))
	return engine
}
