package saga

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/model"
)

// Executor runs already-persisted Saga instances.
//
// The interface intentionally exposes only execution by durable identifier so
// business use cases can trigger a low-latency execution attempt without
// depending on a concrete scheduler, worker runtime, or executor implementation.
// Durable retries remain the responsibility of the background Saga executor.
type Executor interface {
	// ExecuteByID claims and executes one runnable Saga instance.
	ExecuteByID(ctx context.Context, id model.SagaID) error
}
