package inprocess

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

var (
	// ErrTaskNameRequired indicates that a task has no stable name.
	ErrTaskNameRequired = errors.New("task name is required")

	// ErrTaskHandlerRequired indicates that a task has no handler identifier.
	ErrTaskHandlerRequired = errors.New("task handler is required")

	// ErrTaskJobRequired indicates that an in-process task has no executable job.
	ErrTaskJobRequired = errors.New("task job is required")

	// ErrUnsupportedTrigger indicates that the trigger type is not supported.
	ErrUnsupportedTrigger = errors.New("unsupported task trigger")

	// ErrInvalidTrigger indicates that the trigger is incorrectly configured.
	ErrInvalidTrigger = errors.New("invalid task trigger")

	// ErrCommandHandlerRequired indicates that a dispatched command has no handler.
	ErrCommandHandlerRequired = errors.New("command handler is required")

	// ErrUnknownHandler indicates that no registered job matches the command handler.
	ErrUnknownHandler = errors.New("unknown task handler")
)

// Config configures the in-process task runtime.
type Config struct {
	// DefaultTimezone is used for cron triggers without task-specific timezone.
	DefaultTimezone string
}

// Runtime executes scheduled and dispatched tasks inside the current process.
//
// Runtime supports two execution models:
//
//   - scheduled tasks through Register and Start
//   - on-demand asynchronous commands through Dispatch
//
// Scheduling is local to the current process. Cron tasks are executed by
// robfig/cron, interval tasks are executed by goroutines, and manual tasks are
// registered only for explicit Dispatch calls.
//
// Dispatch resolves task.Command.Handler through the in-memory handler
// registry and executes the matched Job asynchronously.
//
// In multi-instance deployments, every instance owns its own scheduler and
// handler registry. Scheduled jobs must therefore be idempotent or protected by
// distributed coordination.
//
// The Handler-based dispatch model is intentionally aligned with future
// providers such as Temporal, where the same Handler value can resolve to a
// workflow or activity instead of an in-process function.
type Runtime struct {
	// logger is used for runtime lifecycle, registration,
	// execution, dispatch, and failure logging.
	logger log.Logger

	// cfg contains runtime-level configuration shared across all tasks.
	cfg Config

	// mu protects access to registered tasks and handlers.
	mu sync.Mutex

	// tasks contains registered scheduled tasks.
	//
	// Disabled tasks are not stored.
	//
	// Start copies this slice before scheduling so the runtime does not hold
	// the lock while cron and interval workers are running.
	tasks []task.Task

	// handlers maps stable handler names to executable jobs.
	//
	// Dispatch uses this registry to resolve task.Command.Handler into a Job.
	//
	// This mirrors future Temporal-style execution where Handler can map to a
	// workflow or activity name instead of an in-process function.
	handlers map[string]task.Job
}

// New creates an in-process task runtime.
func New(cfg Config, logger log.Logger) *Runtime {
	return &Runtime{
		cfg:      cfg,
		logger:   logger,
		handlers: make(map[string]task.Job),
	}
}

// Register validates and stores a scheduled task.
//
// Disabled tasks are skipped. Enabled tasks are also added to the handler
// registry so they can be executed later through Dispatch.
func (r *Runtime) Register(t task.Task) error {
	if !t.Enabled {
		r.logger.With(
			"task_name", t.Name,
			"task_handler", t.Handler,
		).Info("task registration skipped because task is disabled")

		return nil
	}

	if err := validateTask(t); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.tasks = append(r.tasks, t)
	r.handlers[t.Handler] = t.Job

	r.logger.With(
		"task_name", t.Name,
		"task_handler", t.Handler,
		"trigger_type", string(t.Trigger.Type),
	).Info("task registered")

	return nil
}

// Dispatch executes a registered handler asynchronously.
//
// Dispatch is intended for Celery-style or Temporal-style on-demand execution.
// The in-process implementation resolves the command by Handler and runs the
// matching Job in a goroutine.
func (r *Runtime) Dispatch(ctx context.Context, cmd task.Command) error {
	if cmd.Handler == "" {
		return ErrCommandHandlerRequired
	}

	r.mu.Lock()
	job, ok := r.handlers[cmd.Handler]
	r.mu.Unlock()

	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownHandler, cmd.Handler)
	}

	r.logger.With(
		"command_id", cmd.ID,
		"task_handler", cmd.Handler,
	).Info("task dispatch accepted")

	go func() {
		r.execute(ctx, task.Task{
			Name:    cmd.Handler,
			Enabled: true,
			Handler: cmd.Handler,
			Trigger: task.Trigger{
				Type: task.TriggerManual,
			},
			Job: job,
		})
	}()

	return nil
}

// Start starts scheduled task execution.
//
// Start blocks until ctx is canceled.
func (r *Runtime) Start(ctx context.Context) error {
	r.mu.Lock()
	tasks := append([]task.Task(nil), r.tasks...)
	r.mu.Unlock()

	r.logger.With("task_count", len(tasks)).Info("starting in-process task runtime")

	var wg sync.WaitGroup

	cronRunner := cron.New(
		cron.WithParser(cron.NewParser(
			cron.Minute |
				cron.Hour |
				cron.Dom |
				cron.Month |
				cron.Dow |
				cron.Descriptor,
		)),
	)

	for _, t := range tasks {
		switch t.Trigger.Type {
		case task.TriggerManual:
			continue

		case task.TriggerCron:
			if err := r.registerCron(cronRunner, t); err != nil {
				return err
			}

		case task.TriggerInterval:
			wg.Add(1)
			go func(t task.Task) {
				defer wg.Done()
				r.runInterval(ctx, t)
			}(t)

		default:
			return fmt.Errorf("%w: %s", ErrUnsupportedTrigger, t.Trigger.Type)
		}
	}

	cronRunner.Start()

	<-ctx.Done()

	r.logger.Info("stopping in-process task runtime")

	stopCtx := cronRunner.Stop()
	<-stopCtx.Done()

	wg.Wait()

	r.logger.Info("in-process task runtime stopped")

	return nil
}

// registerCron registers a cron-triggered task.
func (r *Runtime) registerCron(cronRunner *cron.Cron, t task.Task) error {
	spec, err := r.cronSpec(t.Trigger)
	if err != nil {
		return err
	}

	if _, err = cronRunner.AddFunc(spec, func() {
		r.execute(context.Background(), t)
	}); err != nil {
		return fmt.Errorf("%w: invalid cron expression for task %s: %w", ErrInvalidTrigger, t.Name, err)
	}

	return nil
}

// runInterval executes an interval-triggered task until ctx is canceled.
func (r *Runtime) runInterval(ctx context.Context, t task.Task) {
	if t.Trigger.RunOnStart {
		r.execute(ctx, t)
	}

	ticker := time.NewTicker(t.Trigger.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			r.execute(ctx, t)
		}
	}
}

// execute runs a task and logs its lifecycle.
func (r *Runtime) execute(ctx context.Context, t task.Task) {
	logger := r.logger.With(
		"task_name", t.Name,
		"task_handler", t.Handler,
		"trigger_type", string(t.Trigger.Type),
	)

	logger.Info("task execution started")

	if err := t.Job(ctx); err != nil {
		logger.With("error", err.Error()).Error("task execution failed")
		return
	}

	logger.Info("task execution completed")
}

// cronSpec returns a robfig/cron-compatible cron expression.
func (r *Runtime) cronSpec(trigger task.Trigger) (string, error) {
	if trigger.Cron == "" {
		return "", fmt.Errorf("%w: cron expression is required", ErrInvalidTrigger)
	}

	timezone := trigger.Timezone
	if timezone == "" {
		timezone = r.cfg.DefaultTimezone
	}

	if timezone == "" {
		return trigger.Cron, nil
	}

	if _, err := time.LoadLocation(timezone); err != nil {
		return "", fmt.Errorf("%w: invalid timezone %s: %w", ErrInvalidTrigger, timezone, err)
	}

	return "CRON_TZ=" + timezone + " " + trigger.Cron, nil
}

// validateTask validates a task before registration.
func validateTask(t task.Task) error {
	if t.Name == "" {
		return ErrTaskNameRequired
	}

	if t.Handler == "" {
		return ErrTaskHandlerRequired
	}

	if t.Job == nil {
		return ErrTaskJobRequired
	}

	switch t.Trigger.Type {
	case task.TriggerManual:
		return nil

	case task.TriggerCron:
		if t.Trigger.Cron == "" {
			return fmt.Errorf("%w: cron expression is required", ErrInvalidTrigger)
		}

	case task.TriggerInterval:
		if t.Trigger.Interval <= 0 {
			return fmt.Errorf("%w: interval must be positive", ErrInvalidTrigger)
		}

	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedTrigger, t.Trigger.Type)
	}

	return nil
}
