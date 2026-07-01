package log

// Provider identifies which logger adapter to construct.
type Provider string

const (
	// NoopProvider selects the no-op logger adapter.
	NoopProvider Provider = "noop"

	// SlogProvider selects the structured logger adapter.
	SlogProvider Provider = "slog"
)
