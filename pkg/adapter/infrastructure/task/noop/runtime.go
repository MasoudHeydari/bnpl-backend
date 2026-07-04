package noop

import (
	"context"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

// Runtime is a no-op implementation of task.Runtime.
//
// It accepts scheduled task registrations and dispatched commands, but never
// executes any job.
type Runtime struct {
	logger log.Logger
}

// New creates a no-op task runtime.
func New(logger log.Logger) *Runtime {
	return &Runtime{
		logger: logger,
	}
}

// Register accepts a scheduled task registration and ignores it.
func (r *Runtime) Register(t task.Task) error {
	r.logger.With(
		"task_name", t.Name,
		"task_handler", t.Handler,
	).Info("task registration ignored by noop runtime")

	return nil
}

// Dispatch accepts an asynchronous command and ignores it.
func (r *Runtime) Dispatch(ctx context.Context, cmd task.Command) error {
	r.logger.With(
		"command_id", cmd.ID,
		"task_handler", cmd.Handler,
	).Info("task dispatch ignored by noop runtime")

	return nil
}

// Start blocks until ctx is canceled.
//
// No scheduled tasks are executed while the runtime is running.
func (r *Runtime) Start(ctx context.Context) error {
	r.logger.Info("starting noop task runtime")

	<-ctx.Done()

	r.logger.Info("stopping noop task runtime")

	return nil
}

// Close is a no-op because the noop runtime owns no durable resources.
func (r *Runtime) Close() error {
	return nil
}
