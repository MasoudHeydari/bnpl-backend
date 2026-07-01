package circuitbreaker

import (
	"context"
	"errors"
)

// ErrOpen is returned when the circuit breaker rejects execution because the
// protected dependency is considered unhealthy.
//
// Callers may use errors.Is to detect this error and decide whether to degrade,
// retry later, or return a dependency-unavailable response.
var ErrOpen = errors.New("circuit breaker is open")

// CircuitBreaker protects calls to external or failure-prone dependencies.
//
// The name argument identifies the protected dependency or operation. Concrete
// implementations may keep independent circuit state per name.
//
// Implementations must execute fn only when the circuit allows it. If the
// circuit is open, Execute should return ErrOpen without calling fn.
type CircuitBreaker interface {
	// Execute runs fn under the circuit identified by name.
	//
	// The provided context must be passed to fn so cancellation and deadlines are
	// preserved across the protected call.
	Execute(ctx context.Context, name string, fn func(ctx context.Context) error) error
}
