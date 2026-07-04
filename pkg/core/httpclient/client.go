package httpclient

import (
	"errors"
	"net/http"
)

var (
	// ErrDisabled indicates that outbound HTTP execution is intentionally
	// disabled by the selected infrastructure provider. No network I/O occurs.
	ErrDisabled = errors.New("outbound http client is disabled")

	// ErrSaturated indicates that all execution slots are occupied and the
	// bounded waiting queue is also full. Callers should make any retry or
	// fallback decision at the integration layer where operation semantics and
	// idempotency guarantees are known.
	ErrSaturated = errors.New("outbound http client is saturated")

	// ErrQueueWaitTimeout indicates that a request entered the bounded queue but
	// could not obtain an execution slot within the configured wait timeout.
	ErrQueueWaitTimeout = errors.New("outbound http client queue wait timeout")

	// ErrNilRequest indicates that Client.Do received a nil request or a request
	// without a URL.
	ErrNilRequest = errors.New("outbound http request is nil")
)

// Client executes outbound HTTP/HTTPS requests through the application's
// shared outbound transport infrastructure.
//
// Implementations must be safe for concurrent use. Callers own response-body
// consumption and must close every non-nil response body. Implementations may
// keep concurrency capacity reserved until the body reaches EOF or is closed.
//
// This is an infrastructure port, not a business-service port. Core use cases
// should normally depend on a narrower domain-specific interface implemented by
// an external adapter, while that adapter may use Client internally.
type Client interface {
	// Do executes req and returns the HTTP response or a transport-level error.
	//
	// Implementations must honor req.Context() cancellation and deadlines.
	Do(req *http.Request) (*http.Response, error)

	// Close releases resources owned by the client implementation.
	//
	// Implementations that own no resources may return nil.
	Close() error
}
