package circuitbreaker

// Provider identifies which circuit breaker adapter to construct.
type Provider string

const (
	// NoopProvider disables circuit breaking.
	NoopProvider Provider = "noop"

	// GoBreakerProvider uses github.com/sony/gobreaker.
	GoBreakerProvider Provider = "gobreaker"
)
