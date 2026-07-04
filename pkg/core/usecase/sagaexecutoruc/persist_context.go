package sagaexecutoruc

import (
	"context"
	"time"
)

// persistStepContext detaches cancellation from the caller so a durable Saga
// transition can be recorded after an external step has already finished, and
// bounds that write with PersistenceTimeout so an unbounded persistence
// operation cannot occupy a worker forever.
//
// HTTP request cancellation must not drop the scheduled retry or compensation
// decision. The external call itself still uses the caller's context and stops
// when that context is canceled. The returned cancel must be called after the
// persistence transaction finishes (success or failure).
func persistStepContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := ctx
	if base == nil {
		base = context.Background()
	} else {
		base = context.WithoutCancel(base)
	}

	if timeout <= 0 {
		return base, func() {}
	}

	return context.WithTimeout(base, timeout)
}
