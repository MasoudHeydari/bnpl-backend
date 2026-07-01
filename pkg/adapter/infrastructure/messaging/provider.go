package messaging

// Provider identifies which messaging adapter to construct.
type Provider string

const (
	// NoopProvider selects the no-op messaging adapter.
	NoopProvider Provider = "noop"

	// KafkaProvider selects the Kafka-backed messaging adapter.
	KafkaProvider Provider = "kafka"
)
