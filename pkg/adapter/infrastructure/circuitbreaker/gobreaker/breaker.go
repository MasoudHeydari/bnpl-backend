package gobreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/sony/gobreaker/v2"

	"github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
)

// Config contains runtime settings for the gobreaker adapter.
type Config struct {
	// MaxRequests is the maximum number of requests allowed to pass when the
	// circuit is half-open.
	MaxRequests uint32

	// Interval is the cyclic period used by gobreaker to clear internal counts
	// while the circuit is closed.
	Interval time.Duration

	// Timeout is the duration the circuit remains open before moving to
	// half-open.
	Timeout time.Duration

	// FailureThreshold is the minimum number of consecutive failed executions
	// required before the circuit transitions from closed to open.
	FailureThreshold uint32
}

// CircuitBreaker adapts sony/gobreaker to corecb.CircuitBreaker.
//
// A separate gobreaker instance is created lazily for each operation name.
type CircuitBreaker struct {
	mu       sync.Mutex
	settings gobreaker.Settings
	breakers map[string]*gobreaker.CircuitBreaker[struct{}]
}

// New creates a gobreaker-backed circuit breaker adapter.
func New(cfg Config) *CircuitBreaker {
	return &CircuitBreaker{
		settings: gobreaker.Settings{
			MaxRequests: cfg.MaxRequests,
			Interval:    cfg.Interval,
			Timeout:     cfg.Timeout,
			ReadyToTrip: func(counts gobreaker.Counts) bool {
				return counts.ConsecutiveFailures >= cfg.FailureThreshold
			},
		},
		breakers: make(map[string]*gobreaker.CircuitBreaker[struct{}]),
	}
}

// Execute runs fn under the named circuit breaker.
//
// If gobreaker rejects the call because the circuit is open or half-open
// capacity is exhausted, Execute returns corecb.ErrOpen.
func (c *CircuitBreaker) Execute(
	ctx context.Context,
	name string,
	fn func(ctx context.Context) error,
) error {
	cb := c.get(name)

	_, err := cb.Execute(
		func() (struct{}, error) {
			if err := ctx.Err(); err != nil {
				return struct{}{}, err
			}

			return struct{}{}, fn(ctx)
		},
	)

	if errors.Is(err, gobreaker.ErrOpenState) ||
		errors.Is(err, gobreaker.ErrTooManyRequests) {
		return circuitbreaker.ErrOpen
	}

	return err
}

// get returns the circuit breaker for name, creating it on first use.
func (c *CircuitBreaker) get(name string) *gobreaker.CircuitBreaker[struct{}] {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cb, ok := c.breakers[name]; ok {
		return cb
	}

	settings := c.settings
	settings.Name = name

	cb := gobreaker.NewCircuitBreaker[struct{}](settings)
	c.breakers[name] = cb

	return cb
}
