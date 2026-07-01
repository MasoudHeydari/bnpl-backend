package noop

import (
	"context"

	corecb "github.com/parstis/bnpl-backend/pkg/core/circuitbreaker"
)

// CircuitBreaker is a no-op implementation of corecb.CircuitBreaker.
//
// It always delegates directly to the protected function and keeps no state.
type CircuitBreaker struct{}

// New creates a no-op circuit breaker.
func New() corecb.CircuitBreaker {
	return CircuitBreaker{}
}

// Execute runs fn immediately without applying circuit breaker rules.
func (CircuitBreaker) Execute(
	ctx context.Context,
	_ string,
	fn func(ctx context.Context) error,
) error {
	return fn(ctx)
}
