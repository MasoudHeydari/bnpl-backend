package merchantwebhook

// Provider identifies which merchant webhook delivery adapter to construct.
type Provider string

const (
	// HTTPProvider selects signed outbound HTTP webhook delivery.
	HTTPProvider Provider = "http"

	// NoopProvider selects the no-op webhook publisher that performs no HTTP I/O.
	NoopProvider Provider = "noop"
)
