package composite

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/parstis/bnpl-backend/pkg/core/task"
)

var (
	// ErrInProcessRuntimeRequired indicates a missing in-process backend.
	ErrInProcessRuntimeRequired = errors.New("in-process task runtime is required")

	// ErrTemporalRuntimeRequired indicates a missing Temporal backend.
	ErrTemporalRuntimeRequired = errors.New("temporal task runtime is required")

	// ErrExecutionModeRequired indicates that an enabled composite task has no route.
	ErrExecutionModeRequired = errors.New("task execution mode is required")

	// ErrUnsupportedExecutionMode indicates an execution mode unknown to this composite.
	ErrUnsupportedExecutionMode = errors.New("unsupported task execution mode")

	// ErrUnknownHandler indicates that Dispatch was called for an unregistered handler.
	ErrUnknownHandler = errors.New("unknown task handler")

	// ErrRuntimeStopped indicates that one child runtime exited before cancellation.
	ErrRuntimeStopped = errors.New("composite child task runtime stopped unexpectedly")
)

// Runtime routes task registration and command dispatch to exactly two child
// runtimes: in-process and Temporal.
type Runtime struct {
	inProcess task.Runtime
	temporal  task.Runtime

	mu     sync.RWMutex
	routes map[string]task.ExecutionMode
}

// New constructs the fixed in-process + Temporal composite runtime.
func New(inProcess task.Runtime, temporal task.Runtime) (*Runtime, error) {
	if inProcess == nil {
		return nil, ErrInProcessRuntimeRequired
	}

	if temporal == nil {
		return nil, ErrTemporalRuntimeRequired
	}

	return &Runtime{
		inProcess: inProcess,
		temporal:  temporal,
		routes:    make(map[string]task.ExecutionMode),
	}, nil
}

// Register routes an enabled task according to ExecutionMode.
//
// Disabled tasks are forwarded to both runtimes. This matters for durable
// runtimes such as Temporal because a previously-created persistent schedule
// may need to be removed when the task becomes disabled.
func (r *Runtime) Register(t task.Task) error {
	const op = "composite.Runtime.Register"

	if !t.Enabled {
		r.mu.Lock()
		delete(r.routes, t.Handler)
		r.mu.Unlock()

		return errors.Join(
			r.inProcess.Register(t),
			r.temporal.Register(t),
		)
	}

	var err error
	switch t.ExecutionMode {
	case task.ExecutionModeInProcess:
		// Also send a disabled copy to Temporal. This removes a durable
		// schedule left behind if the task was routed to Temporal in a
		// previous deployment.
		temporalMetadata := t
		temporalMetadata.Enabled = false
		temporalMetadata.Job = nil
		err = errors.Join(
			r.inProcess.Register(t),
			r.temporal.Register(temporalMetadata),
		)

	case task.ExecutionModeTemporal:
		// The in-process runtime has no durable scheduler state, but feeding it
		// disabled metadata keeps the composite contract symmetric and avoids
		// accidentally registering the same job in both runtimes.
		inProcessMetadata := t
		inProcessMetadata.Enabled = false
		inProcessMetadata.Job = nil
		err = errors.Join(
			r.inProcess.Register(inProcessMetadata),
			r.temporal.Register(t),
		)

	case "":
		return fmt.Errorf("%s: %w: task %q", op, ErrExecutionModeRequired, t.Name)

	default:
		return fmt.Errorf("%s: %w: task %q mode %q", op, ErrUnsupportedExecutionMode, t.Name, t.ExecutionMode)
	}

	if err != nil {
		return err
	}

	r.mu.Lock()
	r.routes[t.Handler] = t.ExecutionMode
	r.mu.Unlock()

	return nil
}

// Dispatch routes a command using the handler route established at registration.
func (r *Runtime) Dispatch(ctx context.Context, command task.Command) error {
	const op = "composite.Runtime.Dispatch"

	r.mu.RLock()
	mode, ok := r.routes[command.Handler]
	r.mu.RUnlock()
	if !ok {
		return fmt.Errorf("%s: %w: %s", op, ErrUnknownHandler, command.Handler)
	}

	switch mode {
	case task.ExecutionModeInProcess:
		return r.inProcess.Dispatch(ctx, command)

	case task.ExecutionModeTemporal:
		return r.temporal.Dispatch(ctx, command)

	default:
		return fmt.Errorf("%s: %w: handler %q mode %q", op, ErrUnsupportedExecutionMode, command.Handler, mode)
	}
}

// Start starts both child runtimes and blocks until the parent context is
// canceled or either child runtime exits unexpectedly.
func (r *Runtime) Start(ctx context.Context) error {
	const op = "composite.Runtime.Start"

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		name string
		err  error
	}

	results := make(chan result, 2)
	start := func(name string, runtime task.Runtime) {
		go func() {
			results <- result{name: name, err: runtime.Start(runCtx)}
		}()
	}

	start("inprocess", r.inProcess)
	start("temporal", r.temporal)

	first := <-results
	if ctx.Err() != nil {
		cancel()
		<-results
		return nil
	}

	cancel()
	second := <-results

	if first.err == nil {
		first.err = ErrRuntimeStopped
	}

	if second.err != nil && !errors.Is(second.err, context.Canceled) {
		return errors.Join(
			fmt.Errorf("%s: %s runtime: %w", op, first.name, first.err),
			fmt.Errorf("%s: %s runtime: %w", op, second.name, second.err),
		)
	}

	return fmt.Errorf("%s: %s runtime: %w", op, first.name, first.err)
}

// Close releases durable child-runtime resources after Start has returned.
func (r *Runtime) Close() error {
	var err error
	err = errors.Join(err, r.inProcess.Close())
	err = errors.Join(err, r.temporal.Close())

	return err
}
