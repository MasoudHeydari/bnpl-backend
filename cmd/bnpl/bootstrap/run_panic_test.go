package bootstrap

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
)

func TestPanicSafeHTTPHandler_RecoversAndReports(t *testing.T) {
	reporter := &bootstrapPanicReporter{panics: make(chan any, 1)}
	handler := panicSafeHTTPHandler(
		"metrics_http",
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			panic("metrics-boom")
		}),
		lognoop.New(),
		reporter,
	)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)

	select {
	case recovered := <-reporter.panics:
		require.Equal(t, "metrics-boom", recovered)

	case <-time.After(time.Second):
		t.Fatal("recovered metrics panic was not reported")
	}
}

func TestServeWithPanicRecovery_RecoversReportsAndReturnsError(t *testing.T) {
	reporter := &bootstrapPanicReporter{panics: make(chan any, 1)}

	err := serveWithPanicRecovery(
		context.Background(),
		"http_server",
		func() error {
			panic("serve-boom")
		},
		lognoop.New(),
		reporter,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "http_server panic recovered")

	select {
	case recovered := <-reporter.panics:
		require.Equal(t, "serve-boom", recovered)

	case <-time.After(time.Second):
		t.Fatal("recovered listener panic was not reported")
	}
}
