package metrics

// Provider selects the metrics adapter constructed at bootstrap.
type Provider string

const (
	// NoopProvider discards all metric observations.
	NoopProvider Provider = "noop"

	// PrometheusProvider records counters and exposes Prometheus text format.
	PrometheusProvider Provider = "prometheus"
)
