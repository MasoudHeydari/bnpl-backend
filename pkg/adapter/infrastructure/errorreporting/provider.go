package errorreporting

// Provider identifies an error-reporting implementation.
type Provider string

const (
	// NoopProvider disables external error reporting while preserving wiring.
	NoopProvider Provider = "noop"

	// SentryProvider reports unexpected failures to Sentry.
	SentryProvider Provider = "sentry"
)
