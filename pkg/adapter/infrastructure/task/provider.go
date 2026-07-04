package task

// Provider identifies which task runtime adapter to construct.
type Provider string

const (
	// NoopProvider selects the no-op task runtime adapter.
	NoopProvider Provider = "noop"

	// InProcessProvider selects the in-process task runtime adapter.
	InProcessProvider Provider = "inprocess"

	// TemporalProvider selects the Temporal task runtime adapter.
	TemporalProvider Provider = "temporal"

	// CompositeProvider selects the fixed in-process + Temporal composite runtime.
	CompositeProvider Provider = "composite"
)
