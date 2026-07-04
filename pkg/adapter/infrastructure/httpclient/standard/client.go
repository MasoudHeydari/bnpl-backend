package standard

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	coreclock "github.com/parstis/bnpl-backend/pkg/core/clock"
	corehttpclient "github.com/parstis/bnpl-backend/pkg/core/httpclient"
	corelog "github.com/parstis/bnpl-backend/pkg/core/log"
	coremetrics "github.com/parstis/bnpl-backend/pkg/core/metrics"
	coretrace "github.com/parstis/bnpl-backend/pkg/core/trace"
)

var (
	_ corehttpclient.Client                = (*Client)(nil)
	_ corehttpclient.SecureProfileExecutor = (*Client)(nil)
)

// Config contains the single default outbound HTTP profile used by all
// third-party HTTP integrations in the first implementation phase.
//
// Per-client overrides are intentionally not modeled yet. The default profile
// keeps configuration simple while the runtime types remain reusable when
// named client profiles are introduced later.
type Config struct {
	// MaxInFlight is the maximum number of requests allowed to occupy an
	// outbound execution slot concurrently. A slot remains occupied until the
	// response body reaches EOF or is closed.
	MaxInFlight int

	// MaxQueueSize bounds how many callers may wait for an execution slot.
	// Zero means fail fast when every in-flight slot is occupied.
	MaxQueueSize int

	// QueueWaitTimeout caps how long a queued request may wait for an execution
	// slot. Request context cancellation/deadline always takes precedence.
	QueueWaitTimeout time.Duration

	// Timeout is net/http.Client's end-to-end request timeout. It includes
	// connection setup, redirects, response headers, and response body reads.
	Timeout time.Duration

	// DialTimeout limits TCP connection establishment.
	DialTimeout time.Duration

	// TLSHandshakeTimeout limits TLS handshake completion.
	TLSHandshakeTimeout time.Duration

	// ResponseHeaderTimeout limits the wait for response headers after the
	// request has been written.
	ResponseHeaderTimeout time.Duration

	// IdleConnTimeout controls how long pooled idle connections are retained.
	IdleConnTimeout time.Duration

	// MaxIdleConns is the total number of idle keep-alive connections retained
	// across all hosts.
	MaxIdleConns int

	// MaxIdleConnsPerHost limits idle keep-alive connections retained per host.
	MaxIdleConnsPerHost int

	// MaxConnsPerHost limits total connections per host, including dialing,
	// active, and idle connections. This is transport-level protection in
	// addition to the global MaxInFlight bulkhead.
	MaxConnsPerHost int
}

// Client is a concurrency-safe shared HTTP client.
//
// It does not create one net/http.Client per request and it does not run a
// goroutine worker pool. Instead, a semaphore provides the worker-pool behavior
// the service needs: bounded in-flight execution plus a bounded waiting queue.
// This preserves net/http connection reuse and propagates context cancellation
// naturally.
type Client struct {
	client *http.Client

	slots chan struct{}
	queue chan struct{}

	queueWaitTimeout time.Duration
	clock            coreclock.Clock
	logger           corelog.Logger
	metrics          coremetrics.OutboundHTTPMetrics

	profilesMu sync.RWMutex
	profiles   map[string]*secureProfile

	closeOnce sync.Once
}

// New constructs the standard outbound HTTP client and its reusable transport.
func New(
	cfg Config,
	clk coreclock.Clock,
	logger corelog.Logger,
	metrics coremetrics.OutboundHTTPMetrics,
) (*Client, error) {
	const op = "standard.New"

	if clk == nil {
		return nil, fmt.Errorf("%s: http client clock is nil", op)
	}

	if logger == nil {
		return nil, fmt.Errorf("%s: http client logger is nil", op)
	}

	if metrics == nil {
		return nil, fmt.Errorf("%s: http client metrics is nil", op)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: cfg.DialTimeout}).DialContext
	transport.TLSHandshakeTimeout = cfg.TLSHandshakeTimeout
	transport.ResponseHeaderTimeout = cfg.ResponseHeaderTimeout
	transport.IdleConnTimeout = cfg.IdleConnTimeout
	transport.MaxIdleConns = cfg.MaxIdleConns
	transport.MaxIdleConnsPerHost = cfg.MaxIdleConnsPerHost
	transport.MaxConnsPerHost = cfg.MaxConnsPerHost
	transport.ForceAttemptHTTP2 = true

	c := &Client{
		client: &http.Client{
			Transport: transport,
			Timeout:   cfg.Timeout,
		},
		slots:            make(chan struct{}, cfg.MaxInFlight),
		queue:            make(chan struct{}, cfg.MaxQueueSize),
		queueWaitTimeout: cfg.QueueWaitTimeout,
		clock:            clk,
		logger:           logger,
		metrics:          metrics,
		profiles:         make(map[string]*secureProfile),
	}

	logger.With(
		"max_in_flight", cfg.MaxInFlight,
		"max_queue_size", cfg.MaxQueueSize,
		"queue_wait_timeout", cfg.QueueWaitTimeout.String(),
		"request_timeout", cfg.Timeout.String(),
		"max_conns_per_host", cfg.MaxConnsPerHost,
	).Info("outbound http client initialized")

	return c, nil
}

// Do executes req through the bounded outbound client.
//
// If all execution slots are busy, the caller enters the bounded queue. When
// both capacities are exhausted, ErrSaturated is returned immediately. No
// retry policy is applied here because retry safety depends on the semantics of
// the external API and its idempotency guarantees.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	return c.do(req, func(r *http.Request) (*http.Response, error) {
		return c.client.Do(r)
	})
}

func (c *Client) do(
	req *http.Request,
	execute func(*http.Request) (*http.Response, error),
) (*http.Response, error) {
	if req == nil || req.URL == nil {
		return nil, corehttpclient.ErrNilRequest
	}

	ctx := req.Context()
	logger := c.requestLogger(ctx, req)
	enteredAt := c.clock.Now()

	queued, queueWait, err := c.acquire(ctx)
	if err != nil {
		duration := c.clock.Now().Sub(enteredAt)
		outcome := outcomeForError(err)
		c.observeCompletion(req.Method, outcome, duration)
		c.logAcquireFailure(logger, err, queued, queueWait)

		return nil, err
	}

	if queued {
		logger = logger.With("queue_wait", queueWait.String())
		logger.Debug("outbound http request acquired execution slot after queueing")
	}

	requestStartedAt := c.clock.Now()
	resp, err := execute(req)
	finishedAt := c.clock.Now()
	transportDuration := finishedAt.Sub(requestStartedAt)
	totalDuration := finishedAt.Sub(enteredAt)

	if err != nil {
		c.releaseSlot()

		outcome := outcomeForError(err)
		c.observeCompletion(req.Method, outcome, totalDuration)
		c.logRequestError(logger, err, transportDuration)

		return nil, err
	}

	outcome := outcomeForStatus(resp.StatusCode)
	c.observeCompletion(req.Method, outcome, totalDuration)
	c.logResponse(logger, resp.StatusCode, transportDuration)

	// Keep the bulkhead slot until the body is consumed or closed. Releasing at
	// header receipt would under-count long/streaming responses and allow more
	// live outbound work than MaxInFlight promises.
	if resp.Body == nil {
		c.releaseSlot()

		return resp, nil
	}

	resp.Body = &releaseBody{
		ReadCloser: resp.Body,
		release:    c.releaseSlot,
	}

	return resp, nil
}

// Close closes idle keep-alive connections owned by the shared transport and
// any secure profiles registered on this client.
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.client.CloseIdleConnections()
		c.closeSecureProfiles()
		c.logger.Info("outbound http client closed idle connections")
	})
	return nil
}

// acquire reserves an execution slot. It returns whether the request was
// queued and the observed queue wait duration.
func (c *Client) acquire(ctx context.Context) (bool, time.Duration, error) {
	select {
	case c.slots <- struct{}{}:
		c.metricsInFlight(1)
		return false, 0, nil

	default:
	}

	// Reserve queue capacity without blocking. This is the explicit
	// backpressure boundary: once the queue is full, fail fast.
	select {

	case c.queue <- struct{}{}:
		c.metricsQueueDepth(1)

	default:
		c.metricsSaturated()
		return false, 0, corehttpclient.ErrSaturated
	}

	queuedAt := c.clock.Now()
	timer := time.NewTimer(c.queueWaitTimeout)
	defer timer.Stop()

	leaveQueue := func() time.Duration {
		<-c.queue
		c.metricsQueueDepth(-1)
		wait := c.clock.Now().Sub(queuedAt)
		c.metricsQueueWait(wait)
		return wait
	}

	select {

	case c.slots <- struct{}{}:
		wait := leaveQueue()
		c.metricsInFlight(1)
		return true, wait, nil

	case <-ctx.Done():
		wait := leaveQueue()
		return true, wait, ctx.Err()

	case <-timer.C:
		wait := leaveQueue()
		return true, wait, corehttpclient.ErrQueueWaitTimeout
	}
}

// releaseSlot returns one occupied execution slot to the bounded bulkhead and
// updates the in-flight gauge. Every successful acquire must release exactly once.
func (c *Client) releaseSlot() {
	<-c.slots
	c.metricsInFlight(-1)
}

// requestLogger builds a request-scoped logger using only bounded/safe request
// metadata plus trace correlation identifiers. Sensitive URL/body/header data is
// intentionally excluded.
func (c *Client) requestLogger(ctx context.Context, req *http.Request) corelog.Logger {
	args := []any{
		"http_method", req.Method,
		"http_scheme", req.URL.Scheme,
		"http_host", req.URL.Host,
	}

	// Trace identifiers are safe correlation metadata. URL path/query, request
	// or response bodies, Authorization, cookies, and arbitrary headers are
	// deliberately excluded because they may contain PII or secrets.
	if value := coretrace.ID(ctx); value != "" {
		args = append(args, "trace_id", value)
	}

	if value := coretrace.RequestID(ctx); value != "" {
		args = append(args, "request_id", value)
	}

	if value := coretrace.CorrelationID(ctx); value != "" {
		args = append(args, "correlation_id", value)
	}

	return c.logger.With(args...)
}

// logAcquireFailure records failures that happen before network execution, such
// as saturation, queue timeout, or request-context cancellation.
func (c *Client) logAcquireFailure(logger corelog.Logger, err error, queued bool, queueWait time.Duration) {
	entry := logger.With("error", safeErrorMessage(err), "queued", queued)
	if queued {
		entry = entry.With("queue_wait", queueWait.String())
	}

	switch {
	case errors.Is(err, context.Canceled):
		entry.Debug("outbound http request canceled while waiting for execution slot")

	case errors.Is(err, context.DeadlineExceeded):
		entry.Warn("outbound http request deadline exceeded while waiting for execution slot")

	case errors.Is(err, corehttpclient.ErrSaturated):
		entry.Warn("outbound http request rejected because client capacity is saturated")

	case errors.Is(err, corehttpclient.ErrQueueWaitTimeout):
		entry.Warn("outbound http request timed out while waiting in bounded queue")

	default:
		entry.Warn("outbound http request failed before execution")
	}
}

// logRequestError records transport failures after an execution slot has been
// acquired. safeErrorMessage is used so net/url errors cannot leak full URLs.
func (c *Client) logRequestError(logger corelog.Logger, err error, duration time.Duration) {
	entry := logger.With("duration", duration.String(), "error", safeErrorMessage(err))
	switch {
	case errors.Is(err, context.Canceled):
		entry.Debug("outbound http request canceled")

	case errors.Is(err, context.DeadlineExceeded), isTimeoutError(err):
		entry.Warn("outbound http request timed out")

	default:
		entry.Warn("outbound http request failed")
	}
}

// logResponse records response metadata after headers arrive. The response body
// and headers are intentionally not logged by this generic infrastructure.
func (c *Client) logResponse(logger corelog.Logger, statusCode int, duration time.Duration) {
	entry := logger.With(
		"http_status_code", statusCode,
		"duration", duration.String(),
	)
	if statusCode >= http.StatusInternalServerError {
		entry.Warn("outbound http response received with server error status")
		return
	}

	entry.Debug("outbound http response headers received")
}

// observeCompletion records one request completion using only normalized bounded
// metric dimensions.
func (c *Client) observeCompletion(method, outcome string, duration time.Duration) {
	c.metrics.OutboundHTTPRequestCompleted(context.Background(), normalizeMethod(method), outcome, duration)
}

// metricsInFlight adjusts the current in-flight request gauge.
func (c *Client) metricsInFlight(delta int) {
	c.metrics.OutboundHTTPInFlightChanged(context.Background(), delta)
}

// metricsQueueDepth adjusts the current bounded-queue depth gauge.
func (c *Client) metricsQueueDepth(delta int) {
	c.metrics.OutboundHTTPQueueDepthChanged(context.Background(), delta)
}

// metricsQueueWait records one queue-wait observation.
func (c *Client) metricsQueueWait(duration time.Duration) {
	c.metrics.OutboundHTTPQueueWaitObserved(context.Background(), duration)
}

// metricsSaturated records one fail-fast rejection caused by exhausted execution
// and queue capacity.
func (c *Client) metricsSaturated() {
	c.metrics.OutboundHTTPSaturated(context.Background())
}

// normalizeMethod maps arbitrary request methods to a bounded label set. Unknown
// extension methods collapse to OTHER instead of creating unbounded time series.
func normalizeMethod(method string) string {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodGet:
		return http.MethodGet

	case http.MethodHead:
		return http.MethodHead

	case http.MethodPost:
		return http.MethodPost

	case http.MethodPut:
		return http.MethodPut

	case http.MethodPatch:
		return http.MethodPatch

	case http.MethodDelete:
		return http.MethodDelete

	case http.MethodConnect:
		return http.MethodConnect

	case http.MethodOptions:
		return http.MethodOptions

	case http.MethodTrace:
		return http.MethodTrace

	default:
		return "OTHER"
	}
}

// outcomeForStatus collapses HTTP status codes into bounded status classes used
// by metrics.
func outcomeForStatus(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return "2xx"

	case statusCode >= 300 && statusCode < 400:
		return "3xx"

	case statusCode >= 400 && statusCode < 500:
		return "4xx"

	case statusCode >= 500 && statusCode < 600:
		return "5xx"

	default:
		return "other_status"
	}
}

// outcomeForError classifies transport/acquisition failures into a bounded
// outcome label set.
func outcomeForError(err error) string {
	switch {
	case errors.Is(err, corehttpclient.ErrSaturated):
		return "saturated"

	case errors.Is(err, corehttpclient.ErrQueueWaitTimeout):
		return "queue_timeout"

	case errors.Is(err, context.Canceled):
		return "canceled"

	case errors.Is(err, context.DeadlineExceeded), isTimeoutError(err):
		return "timeout"

	default:
		return "network_error"
	}
}

// isTimeoutError reports whether err or one of its wrappers implements a network
// timeout.
func isTimeoutError(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// safeErrorMessage strips net/url.Error's URL field before logging. The
// standard library includes the full URL in url.Error.Error(), which may carry
// sensitive path segments or query parameters.
func safeErrorMessage(err error) string {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return urlErr.Err.Error()
	}

	return err.Error()
}

// releaseBody releases the client's in-flight slot exactly once when the
// response stream is fully consumed or explicitly closed.
type releaseBody struct {
	io.ReadCloser
	once    sync.Once
	release func()
}

// Read delegates to the underlying body and releases the bulkhead slot when the
// response stream terminates with EOF or another terminal read error.
func (b *releaseBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		// EOF and terminal body-read errors both end useful transport work. Close
		// remains idempotent and may still be called by the consumer afterwards.
		b.once.Do(b.release)
	}

	return n, err
}

// Close closes the underlying response body and releases the bulkhead slot. The
// sync.Once guard makes this safe after EOF-triggered release.
func (b *releaseBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.release)
	return err
}
