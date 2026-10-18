package sagaexecutoruc

import "context"

// persistStepContext detaches cancellation from the caller so a durable Saga
// transition can be recorded after an external step has already finished.
//
// HTTP request cancellation must not drop the scheduled retry or compensation
// decision. The external call itself still uses the caller's context and stops
// when that context is canceled.
func persistStepContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}

	return context.WithoutCancel(ctx)
}
