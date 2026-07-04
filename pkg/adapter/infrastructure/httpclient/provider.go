package httpclient

// Provider identifies the outbound HTTP client implementation created during
// bootstrap.
type Provider string

const (
	// NoopProvider disables all outbound HTTP calls. It is intended for local
	// development and tests and is forbidden in production configuration.
	NoopProvider Provider = "noop"

	// StandardProvider uses net/http with bounded concurrency, bounded queueing,
	// transport connection pooling, timeouts, logging, and metrics.
	StandardProvider Provider = "standard"
)
