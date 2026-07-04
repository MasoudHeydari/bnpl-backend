package health

import (
	"sync/atomic"

	corehealth "github.com/parstis/bnpl-backend/pkg/core/health"
)

var _ corehealth.ConsumerStateWriter = (*AtomicConsumerState)(nil)

// AtomicConsumerState tracks Kafka consumer runtime supervision state.
type AtomicConsumerState struct {
	state atomic.Value
}

// NewConsumerState returns a consumer state tracker.
func NewConsumerState() *AtomicConsumerState {
	tracker := &AtomicConsumerState{}
	tracker.state.Store(corehealth.ConsumerStarting)

	return tracker
}

// SetStarting marks the consumer as starting.
func (c *AtomicConsumerState) SetStarting() {
	c.state.Store(corehealth.ConsumerStarting)
}

// SetHealthy marks the consumer as healthy.
func (c *AtomicConsumerState) SetHealthy() {
	c.state.Store(corehealth.ConsumerHealthy)
}

// SetRetrying marks the consumer as retrying after a recoverable error.
func (c *AtomicConsumerState) SetRetrying() {
	c.state.Store(corehealth.ConsumerRetrying)
}

// SetFatal marks the consumer as fatally failed.
func (c *AtomicConsumerState) SetFatal() {
	c.state.Store(corehealth.ConsumerFatal)
}

// SetStopped marks the consumer as stopped.
func (c *AtomicConsumerState) SetStopped() {
	c.state.Store(corehealth.ConsumerStopped)
}

// State returns the current consumer state.
func (c *AtomicConsumerState) State() corehealth.ConsumerRuntimeState {
	value, ok := c.state.Load().(corehealth.ConsumerRuntimeState)
	if !ok {
		return corehealth.ConsumerStarting
	}

	return value
}

// IsReady reports whether the consumer is healthy enough for readiness.
func (c *AtomicConsumerState) IsReady() bool {
	return c.State() == corehealth.ConsumerHealthy
}
