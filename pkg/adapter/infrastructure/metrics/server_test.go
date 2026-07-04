package metrics

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNormalizePath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty defaults", input: "", want: "/metrics"},
		{name: "whitespace defaults", input: "  ", want: "/metrics"},
		{name: "valid", input: "/metrics", want: "/metrics"},
		{name: "nested", input: "/internal/metrics", want: "/internal/metrics"},
		{name: "trim trailing slash", input: "/metrics/", want: "/metrics"},
		{name: "missing slash", input: "metrics", wantErr: true},
		{name: "query rejected", input: "/metrics?x=1", wantErr: true},
		{name: "fragment rejected", input: "/metrics#x", wantErr: true},
		{name: "empty segment", input: "//metrics", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizePath(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestNewServerRequiresHandlerAndAddr(t *testing.T) {
	t.Parallel()

	_, err := NewServer(ServerOptions{Addr: "127.0.0.1:0", Handler: nil})
	require.Error(t, err)

	_, err = NewServer(ServerOptions{Addr: "", Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})})
	require.Error(t, err)
}

func TestServerListenServeAndShutdown(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	})
	server, err := NewServer(ServerOptions{
		Addr:    addr,
		Path:    "/metrics",
		Handler: handler,
	})
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	require.Eventually(t, func() bool {
		resp, getErr := http.Get("http://" + addr + "/metrics")
		if getErr != nil {
			return false
		}

		defer resp.Body.Close()
		body, readErr := io.ReadAll(resp.Body)
		return readErr == nil && resp.StatusCode == http.StatusOK && string(body) == "ok"
	}, 2*time.Second, 20*time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, server.Shutdown(shutdownCtx))

	select {
	case listenErr := <-errCh:
		require.ErrorIs(t, listenErr, http.ErrServerClosed)

	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ListenAndServe to return")
	}
}

func TestServerBindErrorPropagates(t *testing.T) {
	t.Parallel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	server, err := NewServer(ServerOptions{
		Addr: ln.Addr().String(),
		Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		}),
	})
	require.NoError(t, err)

	err = server.ListenAndServe()
	require.Error(t, err)
	require.NotErrorIs(t, err, http.ErrServerClosed)
}
