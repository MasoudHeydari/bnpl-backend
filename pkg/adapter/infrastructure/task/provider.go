package task

// Provider identifies which task runtime adapter to construct.
type Provider string

const (
	// NoopProvider selects the no-op task runtime adapter.
	NoopProvider Provider = "noop"

	// InProcessProvider selects the in-process task runtime adapter.
	InProcessProvider Provider = "inprocess"
)
