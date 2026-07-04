package standard

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	adapterclock "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/clock"
	lognoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/log/noop"
	metricsnoop "github.com/parstis/bnpl-backend/pkg/adapter/infrastructure/metrics/noop"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/stretchr/testify/require"
)

func testClient(logger corelog.Logger, maxInFlight, maxQueue int, queueWait time.Duration) *Client {
	client, err := New(
		Config{
			MaxInFlight:           maxInFlight,
			MaxQueueSize:          maxQueue,
			QueueWaitTimeout:      queueWait,
			Timeout:               2 * time.Second,
			DialTimeout:           time.Second,
			TLSHandshakeTimeout:   time.Second,
			ResponseHeaderTimeout: time.Second,
			IdleConnTimeout:       time.Minute,
			MaxIdleConns:          10,
			MaxIdleConnsPerHost:   5,
			MaxConnsPerHost:       5,
		},
		adapterclock.NewSystem(time.UTC, time.UTC),
		logger,
		metricsnoop.New(),
	)
	if err != nil {
		panic(err)
	}

	return client
}

func TestClient_BoundedQueueRejectsWhenSaturated(t *testing.T) {
	t.Parallel()

	firstStarted := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case firstStarted <- struct{}{}:

		default:
		}

		<-releaseFirst
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testClient(lognoop.New(), 1, 1, time.Second)
	defer client.Close()

	firstDone := make(chan error, 1)
	go func() {
		resp, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
		if resp != nil {
			_ = resp.Body.Close()
		}

		firstDone <- err
	}()

	select {
	case <-firstStarted:

	case <-time.After(time.Second):
		t.Fatal("first request did not start")
	}

	secondDone := make(chan error, 1)
	go func() {
		resp, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
		if resp != nil {
			_ = resp.Body.Close()
		}

		secondDone <- err
	}()

	require.Eventually(t, func() bool { return len(client.queue) == 1 }, time.Second, 5*time.Millisecond)

	resp, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
	if resp != nil {
		_ = resp.Body.Close()
	}

	require.ErrorIs(t, err, corehttpclient.ErrSaturated)

	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
}

func TestClient_QueueWaitTimeout(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case started <- struct{}{}:

		default:
		}

		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testClient(lognoop.New(), 1, 1, 30*time.Millisecond)
	defer client.Close()

	firstDone := make(chan error, 1)
	go func() {
		resp, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
		if resp != nil {
			_ = resp.Body.Close()
		}

		firstDone <- err
	}()
	<-started

	_, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
	require.ErrorIs(t, err, corehttpclient.ErrQueueWaitTimeout)

	close(release)
	require.NoError(t, <-firstDone)
}

func TestClient_QueuedRequestHonorsContextCancellation(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case started <- struct{}{}:

		default:
		}

		<-release
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := testClient(lognoop.New(), 1, 1, time.Second)
	defer client.Close()

	firstDone := make(chan error, 1)
	go func() {
		resp, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
		if resp != nil {
			_ = resp.Body.Close()
		}

		firstDone <- err
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	require.NoError(t, err)
	cancel()

	_, err = client.Do(req)
	require.ErrorIs(t, err, context.Canceled)

	close(release)
	require.NoError(t, <-firstDone)
}

func TestClient_HoldsSlotUntilResponseBodyClosed(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	client := testClient(lognoop.New(), 1, 0, time.Second)
	defer client.Close()

	first, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
	require.NoError(t, err)
	require.NotNil(t, first)

	_, err = client.Do(mustRequest(t, http.MethodGet, server.URL))
	require.ErrorIs(t, err, corehttpclient.ErrSaturated)

	require.NoError(t, first.Body.Close())

	second, err := client.Do(mustRequest(t, http.MethodGet, server.URL))
	require.NoError(t, err)
	require.NoError(t, second.Body.Close())
}

func TestClient_LogsSafeRequestMetadataWithoutURLPathQueryOrAuthorization(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	logger := &recordingLogger{state: &logState{}}
	client := testClient(logger, 1, 1, time.Second)
	defer client.Close()

	req := mustRequest(t, http.MethodGet, server.URL+"/customers/super-secret-id?token=very-secret")
	req.Header.Set("Authorization", "Bearer secret-token")
	resp, err := client.Do(req)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())

	logged := logger.state.String()
	require.NotContains(t, logged, "super-secret-id")
	require.NotContains(t, logged, "very-secret")
	require.NotContains(t, logged, "secret-token")
	require.Contains(t, logged, "http_method")
	require.Contains(t, logged, "http_host")
}

func TestClient_TransportErrorLogDoesNotExposeFullURL(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	baseURL := server.URL
	server.Close()

	logger := &recordingLogger{state: &logState{}}
	client := testClient(logger, 1, 1, time.Second)
	defer client.Close()

	_, err := client.Do(mustRequest(t, http.MethodGet, baseURL+"/private/super-secret-id?token=very-secret"))
	require.Error(t, err)

	logged := logger.state.String()
	require.NotContains(t, logged, "super-secret-id")
	require.NotContains(t, logged, "very-secret")
	require.Contains(t, logged, "outbound http request failed")
}

func mustRequest(t *testing.T, method, url string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)
	return req
}

type logState struct {
	mu      sync.Mutex
	records []string
}

func (s *logState) append(msg string, args []any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = append(s.records, fmt.Sprintf("%s %v", msg, args))
}

func (s *logState) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.Join(s.records, "\n")
}

type recordingLogger struct {
	state *logState
	base  []any
}

func (l *recordingLogger) With(args ...any) corelog.Logger {
	base := append([]any{}, l.base...)
	base = append(base, args...)
	return &recordingLogger{state: l.state, base: base}
}

func (l *recordingLogger) Debug(msg string, args ...any) { l.record(msg, args...) }
func (l *recordingLogger) Info(msg string, args ...any)  { l.record(msg, args...) }
func (l *recordingLogger) Warn(msg string, args ...any)  { l.record(msg, args...) }
func (l *recordingLogger) Error(msg string, args ...any) { l.record(msg, args...) }

func (l *recordingLogger) record(msg string, args ...any) {
	all := append([]any{}, l.base...)
	all = append(all, args...)
	l.state.append(msg, all)
}

var _ corelog.Logger = (*recordingLogger)(nil)
