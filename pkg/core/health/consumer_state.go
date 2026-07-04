package health

// ConsumerRuntimeState identifies the supervision state of a message consumer.
//
// Ownership: health owns readiness-oriented consumer supervision. This is
// distinct from messaging.ConsumerStatus, which describes subscriber-loop
// health for messaging adapters. Keep them separate so readiness probes do not
// depend on messaging package types.
type ConsumerRuntimeState string

const (
	// ConsumerStarting indicates the consumer is initializing and not yet ready.
	ConsumerStarting ConsumerRuntimeState = "starting"

	// ConsumerHealthy indicates the consumer is running normally.
	ConsumerHealthy ConsumerRuntimeState = "healthy"

	// ConsumerRetrying indicates the consumer is recovering from a transient failure.
	ConsumerRetrying ConsumerRuntimeState = "retrying"

	// ConsumerFatal indicates the consumer hit a non-recoverable failure.
	ConsumerFatal ConsumerRuntimeState = "fatal"

	// ConsumerStopped indicates the consumer has been shut down.
	ConsumerStopped ConsumerRuntimeState = "stopped"
)

// ConsumerHealth exposes message-consumer runtime supervision state for readiness.
type ConsumerHealth interface {
	// State returns the current consumer supervision state.
	State() ConsumerRuntimeState

	// IsReady reports whether the consumer should pass readiness checks.
	IsReady() bool
}

// ConsumerStateWriter updates message-consumer runtime supervision state.
type ConsumerStateWriter interface {
	// ConsumerHealth exposes the current supervision state.
	ConsumerHealth

	// SetStarting records that the consumer is initializing.
	SetStarting()

	// SetHealthy records that the consumer is running normally.
	SetHealthy()

	// SetRetrying records that the consumer is recovering from a transient failure.
	SetRetrying()

	// SetFatal records that the consumer hit a non-recoverable failure.
	SetFatal()

	// SetStopped records that the consumer has been shut down.
	SetStopped()
}
