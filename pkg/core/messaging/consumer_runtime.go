package messaging

// ConsumerStatus identifies the runtime health of a message consumer loop.
//
// Ownership: messaging owns subscriber-loop health reporting. This is distinct
// from health.ConsumerRuntimeState, which is used by readiness/dependency
// checks. The value sets intentionally overlap, but the packages stay separate
// so messaging adapters are not forced through the health port.
type ConsumerStatus string

const (
	// ConsumerStatusStarting indicates the consumer loop is initializing.
	ConsumerStatusStarting ConsumerStatus = "starting"

	// ConsumerStatusHealthy indicates the consumer loop is running normally.
	ConsumerStatusHealthy ConsumerStatus = "healthy"

	// ConsumerStatusRetrying indicates the consumer loop is recovering from a
	// transient failure.
	ConsumerStatusRetrying ConsumerStatus = "retrying"

	// ConsumerStatusFatal indicates the consumer loop hit a non-recoverable failure.
	ConsumerStatusFatal ConsumerStatus = "fatal"

	// ConsumerStatusStopped indicates the consumer loop has been shut down.
	ConsumerStatusStopped ConsumerStatus = "stopped"
)

// ConsumerRuntimeState exposes live message-consumer health for adapters that
// report loop status without depending on the health package.
type ConsumerRuntimeState interface {
	// ConsumerStatus returns the current consumer-loop health status.
	ConsumerStatus() ConsumerStatus
}
