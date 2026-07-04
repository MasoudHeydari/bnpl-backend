package metrics

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	defaultMetricsPath              = "/metrics"
	defaultMetricsReadHeaderTimeout = 5 * time.Second
)

// ServerOptions configures the dedicated operational metrics HTTP listener.
//
// This server is intentionally separate from the application REST API. It must
// not register business routes or auth/actor middleware. Treat /metrics as an
// internal scrape endpoint and keep it off public ingress (for example via
// Kubernetes Service / NetworkPolicy).
type ServerOptions struct {
	// Addr is the host:port listen address (for example "0.0.0.0:9090").
	Addr string

	// Path is the scrape path. Empty defaults to /metrics.
	Path string

	// Handler serves the Prometheus text exposition payload.
	Handler http.Handler

	// ReadHeaderTimeout mitigates Slowloris-style attacks on the metrics listener.
	// Non-positive values use a safe default.
	ReadHeaderTimeout time.Duration
}

// Server is a standalone HTTP listener for Prometheus scrape traffic.
//
// It owns only the operational scrape surface. Lifecycle (start/stop) is driven
// by bootstrap using the same graceful-shutdown contract as the API server.
type Server struct {
	srv *http.Server
}

// NewServer constructs a metrics HTTP server.
//
// Returns an error when options are incomplete or the scrape path is invalid.
// Callers must not start a server when metrics exposition is disabled.
func NewServer(opts ServerOptions) (*Server, error) {
	const op = "metrics.NewServer"

	if opts.Handler == nil {
		return nil, fmt.Errorf("%s: metrics server handler is required", op)
	}

	if strings.TrimSpace(opts.Addr) == "" {
		return nil, fmt.Errorf("%s: metrics server address is required", op)
	}

	path, err := NormalizePath(opts.Path)
	if err != nil {
		return nil, err
	}

	timeout := opts.ReadHeaderTimeout
	if timeout <= 0 {
		timeout = defaultMetricsReadHeaderTimeout
	}

	mux := http.NewServeMux()
	mux.Handle(path, opts.Handler)

	return &Server{
		srv: &http.Server{
			Addr:              opts.Addr,
			Handler:           mux,
			ReadHeaderTimeout: timeout,
		},
	}, nil
}

// Addr returns the configured listen address.
func (s *Server) Addr() string {
	if s == nil || s.srv == nil {
		return ""
	}

	return s.srv.Addr
}

// ListenAndServe starts the metrics listener.
//
// Bind failures propagate to the caller so process startup cannot succeed with a
// silently dead scrape endpoint.
func (s *Server) ListenAndServe() error {
	return s.srv.ListenAndServe()
}

// Serve serves metrics on an already-bound listener.
//
// Bootstrap binds first so a busy metrics port fails before background workers
// start.
func (s *Server) Serve(ln net.Listener) error {
	return s.srv.Serve(ln)
}

// Shutdown stops accepting new connections and waits for in-flight scrapes.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

// NormalizePath validates and normalizes a metrics scrape path.
//
// Empty values become /metrics. Paths must be absolute, must not contain query
// or fragment separators, and must not end with "/" unless they are exactly "/".
func NormalizePath(path string) (string, error) {
	const op = "metrics.NormalizePath"

	path = strings.TrimSpace(path)
	if path == "" {
		return defaultMetricsPath, nil
	}

	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("%s: metrics path %q must start with '/'", op, path)
	}

	if strings.ContainsAny(path, "?#") {
		return "", fmt.Errorf("%s: metrics path %q must not contain '?' or '#'", op, path)
	}

	if strings.Contains(path, "//") {
		return "", fmt.Errorf("%s: metrics path %q contains an empty segment", op, path)
	}

	if path != "/" && strings.HasSuffix(path, "/") {
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "/"
		}
	}

	return path, nil
}
