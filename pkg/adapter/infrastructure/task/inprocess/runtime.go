package inprocess

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	coreerrorreporting "github.com/parstis/bnpl-backend/pkg/core/errorreporting"
	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

const defaultMaxConcurrentRuns = 64

// Task runtime registration and lifecycle errors.
var (
	ErrTaskNameRequired       = errors.New("task name is required")
	ErrTaskHandlerRequired    = errors.New("task handler is required")
	ErrTaskJobRequired        = errors.New("task job is required")
	ErrUnsupportedTrigger     = errors.New("unsupported task trigger")
	ErrInvalidTrigger         = errors.New("invalid task trigger")
	ErrCommandHandlerRequired = errors.New("command handler is required")
	ErrUnknownHandler         = errors.New("unknown task handler")
	ErrDuplicateTaskName      = errors.New("duplicate task name")
	ErrDuplicateHandler       = errors.New("duplicate task handler")
	ErrRuntimeBusy            = errors.New("task runtime concurrency limit reached")
	ErrRuntimeAlreadyStarted  = errors.New("task runtime already started")
	ErrRuntimeStopping        = errors.New("task runtime is stopping")
)

// Config configures the in-process task runtime.
type Config struct {
	DefaultTimezone   string
	MaxConcurrentRuns int
}

// Runtime executes scheduled and dispatched tasks inside the current process.
type Runtime struct {
	logger   log.Logger
	reporter coreerrorreporting.Reporter
	cfg      Config

	mu       sync.Mutex
	runCtx   context.Context
	started  bool
	stopping bool
	tasks    []task.Task
	handlers map[string]task.Job
	names    map[string]struct{}

	// runningHandlers counts active executions. Scheduled invocations use this
	// to skip overlap with a previous cron/interval/manual execution.
	runningHandlers map[string]int
	semaphore       chan struct{}
	dispatchWG      sync.WaitGroup
}

// New creates an in-process task runtime.
func New(
	cfg Config,
	logger log.Logger,
	reporter coreerrorreporting.Reporter,
) (*Runtime, error) {
	const op = "inprocess.New"

	if logger == nil {
		return nil, fmt.Errorf("%s: task runtime logger is nil", op)
	}

	if reporter == nil {
		return nil, fmt.Errorf("%s: task runtime reporter is nil", op)
	}

	if cfg.MaxConcurrentRuns <= 0 {
		cfg.MaxConcurrentRuns = defaultMaxConcurrentRuns
	}

	return &Runtime{
		cfg:             cfg,
		logger:          logger,
		reporter:        reporter,
		handlers:        make(map[string]task.Job),
		names:           make(map[string]struct{}),
		runningHandlers: make(map[string]int),
		semaphore:       make(chan struct{}, cfg.MaxConcurrentRuns),
	}, nil
}

// Register validates and stores a scheduled task.
func (r *Runtime) Register(t task.Task) error {
	const op = "inprocess.Runtime.Register"

	if !t.Enabled {
		r.logger.With("task_name", t.Name, "task_handler", t.Handler).
			Info("task registration skipped because task is disabled")
		return nil
	}

	if err := validateTask(t); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrRuntimeAlreadyStarted
	}

	if _, exists := r.names[t.Name]; exists {
		return fmt.Errorf("%s: %w: %s", op, ErrDuplicateTaskName, t.Name)
	}

	if _, exists := r.handlers[t.Handler]; exists {
		return fmt.Errorf("%s: %w: %s", op, ErrDuplicateHandler, t.Handler)
	}

	r.names[t.Name] = struct{}{}
	r.tasks = append(r.tasks, t)
	r.handlers[t.Handler] = t.Job

	r.logger.With(
		"task_name", t.Name,
		"task_handler", t.Handler,
		"trigger_type", string(t.Trigger.Type),
	).Info("task registered")
	return nil
}

// Dispatch executes a registered handler asynchronously with bounded process concurrency.
func (r *Runtime) Dispatch(ctx context.Context, cmd task.Command) error {
	const op = "inprocess.Runtime.Dispatch"

	if cmd.Handler == "" {
		return ErrCommandHandlerRequired
	}

	r.mu.Lock()
	if r.stopping {
		r.mu.Unlock()
		return ErrRuntimeStopping
	}

	job, ok := r.handlers[cmd.Handler]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("%s: %w: %s", op, ErrUnknownHandler, cmd.Handler)
	}

	if !r.tryAcquireHandlerLocked(cmd.Handler) {
		r.mu.Unlock()
		return ErrRuntimeBusy
	}

	// Add while holding the lifecycle lock so Start cannot begin Wait before
	// this dispatch has registered itself.
	r.dispatchWG.Add(1)
	r.mu.Unlock()

	r.logger.With("command_id", cmd.ID, "task_handler", cmd.Handler).
		Info("task dispatch accepted")

	go func() {
		defer r.recoverTaskPanic(ctx, task.Task{
			Name:    cmd.Handler,
			Enabled: true,
			Handler: cmd.Handler,
			Trigger: task.Trigger{Type: task.TriggerManual},
		}, "dispatch")
		defer r.dispatchWG.Done()
		defer r.release(cmd.Handler)

		execCtx, cancel := r.executionContext(ctx)
		defer cancel()
		r.execute(execCtx, task.Task{
			Name:    cmd.Handler,
			Enabled: true,
			Handler: cmd.Handler,
			Trigger: task.Trigger{Type: task.TriggerManual},
			Job:     job,
		})
	}()

	return nil
}

// Start starts scheduled task execution and blocks until ctx is canceled.
func (r *Runtime) Start(ctx context.Context) error {
	const op = "inprocess.Runtime.Start"

	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return ErrRuntimeAlreadyStarted
	}

	r.started = true
	r.stopping = false
	r.runCtx = ctx
	tasks := append([]task.Task(nil), r.tasks...)
	r.mu.Unlock()

	r.logger.With("task_count", len(tasks)).Info("starting in-process task runtime")

	var wg sync.WaitGroup
	cronRunner := cron.New(cron.WithParser(cron.NewParser(
		cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
	)))

	// Register every cron expression before starting interval goroutines. If a
	// cron expression is invalid, Start returns without leaving partially started
	// workers behind.
	for _, t := range tasks {
		if t.Trigger.Type != task.TriggerCron {
			continue
		}

		if err := r.registerCron(ctx, cronRunner, t); err != nil {
			r.resetStartState()
			return err
		}
	}

	for _, t := range tasks {
		switch t.Trigger.Type {
		case task.TriggerManual, task.TriggerCron:
			continue

		case task.TriggerInterval:
			wg.Add(1)
			go func(t task.Task) {
				defer r.recoverTaskPanic(ctx, t, "interval_loop")
				defer wg.Done()
				r.runInterval(ctx, t)
			}(t)

		default:
			r.resetStartState()
			return fmt.Errorf("%s: %w: %s", op, ErrUnsupportedTrigger, t.Trigger.Type)
		}
	}

	cronRunner.Start()
	<-ctx.Done()

	r.mu.Lock()
	r.stopping = true
	r.mu.Unlock()

	r.logger.Info("stopping in-process task runtime")
	stopCtx := cronRunner.Stop()
	<-stopCtx.Done()
	wg.Wait()
	r.dispatchWG.Wait()
	r.logger.Info("in-process task runtime stopped")
	return nil
}

// Close is a no-op because the in-process runtime owns no durable resources
// beyond goroutines stopped when Start returns.
func (r *Runtime) Close() error {
	return nil
}

func (r *Runtime) resetStartState() {
	r.mu.Lock()
	r.started = false
	r.stopping = false
	r.runCtx = nil
	r.mu.Unlock()
}

func (r *Runtime) executionContext(ctx context.Context) (context.Context, context.CancelFunc) {
	r.mu.Lock()
	runCtx := r.runCtx
	r.mu.Unlock()
	if runCtx == nil {
		if ctx == nil {
			return context.WithCancel(context.Background())
		}

		return context.WithCancel(ctx)
	}

	if ctx == nil || ctx == runCtx {
		return context.WithCancel(runCtx)
	}

	return mergeContexts(runCtx, ctx)
}

func (r *Runtime) registerCron(ctx context.Context, cronRunner *cron.Cron, t task.Task) error {
	const op = "inprocess.Runtime.registerCron"

	spec, err := r.cronSpec(t.Trigger)
	if err != nil {
		return err
	}

	if _, err = cronRunner.AddFunc(spec, func() {
		r.executeScheduled(ctx, t)
	}); err != nil {
		return fmt.Errorf("%s: %w: invalid cron expression for task %s: %w", op, ErrInvalidTrigger, t.Name, err)
	}

	return nil
}

func (r *Runtime) runInterval(ctx context.Context, t task.Task) {
	if t.Trigger.RunOnStart {
		r.executeScheduled(ctx, t)
	}

	ticker := time.NewTicker(t.Trigger.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return

		case <-ticker.C:
			r.executeScheduled(ctx, t)
		}
	}
}

func (r *Runtime) executeScheduled(ctx context.Context, t task.Task) {
	defer r.recoverTaskPanic(ctx, t, "scheduled_execution")

	if !r.tryAcquireScheduled(t.Handler) {
		r.logger.With("task_name", t.Name, "task_handler", t.Handler).
			Warn("task execution skipped because a previous run is still active")
		return
	}

	defer r.release(t.Handler)

	execCtx, cancel := r.executionContext(ctx)
	defer cancel()
	r.execute(execCtx, t)
}

func (r *Runtime) tryAcquireScheduled(handler string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stopping {
		return false
	}

	return r.tryAcquireHandlerLocked(handler)
}

// tryAcquireHandlerLocked acquires process and per-handler execution capacity.
// r.mu must be held by the caller.
func (r *Runtime) tryAcquireHandlerLocked(handler string) bool {
	if r.runningHandlers[handler] > 0 {
		return false
	}

	select {
	case r.semaphore <- struct{}{}:
		r.runningHandlers[handler] = 1
		return true

	default:
		return false
	}
}

func (r *Runtime) release(handler string) {
	r.mu.Lock()
	if r.runningHandlers[handler] <= 1 {
		delete(r.runningHandlers, handler)
	} else {
		r.runningHandlers[handler]--
	}

	r.mu.Unlock()
	<-r.semaphore
}

func (r *Runtime) execute(ctx context.Context, t task.Task) {
	defer r.recoverTaskPanic(ctx, t, "job_execution")

	if ctx.Err() != nil {
		return
	}

	logger := r.logger.With(
		"task_name", t.Name,
		"task_handler", t.Handler,
		"trigger_type", string(t.Trigger.Type),
	)
	logger.Info("task execution started")
	if err := t.Job(ctx); err != nil {
		logger.With("error", err.Error()).Error("task execution failed")

		// Context cancellation during normal shutdown is expected and should not
		// become an external error event.
		if !isExpectedContextTermination(ctx, err) {
			r.reporter.CaptureError(ctx, err, taskErrorMetadata(t))
		}

		return
	}

	logger.Info("task execution completed")
}

func (r *Runtime) recoverTaskPanic(ctx context.Context, t task.Task, scope string) {
	recovered := recover()
	if recovered == nil {
		return
	}

	metadata := taskErrorMetadata(t)
	if metadata.Tags == nil {
		metadata.Tags = make(map[string]string)
	}

	metadata.Tags["panic.scope"] = scope

	r.reporter.CapturePanic(ctx, recovered, metadata)

	r.logger.With(
		"task_name", t.Name,
		"task_handler", t.Handler,
		"trigger_type", string(t.Trigger.Type),
		"panic_scope", scope,
		"panic", fmt.Sprint(recovered),
	).Error("task panic recovered")
}

func isExpectedContextTermination(ctx context.Context, err error) bool {
	if ctx == nil || ctx.Err() == nil {
		return false
	}

	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func taskErrorMetadata(t task.Task) coreerrorreporting.Metadata {
	return coreerrorreporting.Metadata{
		Component: "task",
		Tags: map[string]string{
			"task.name":         t.Name,
			"task.handler":      t.Handler,
			"task.trigger_type": string(t.Trigger.Type),
		},
		Fingerprint: []string{"task", t.Handler},
	}
}

func (r *Runtime) cronSpec(trigger task.Trigger) (string, error) {
	const op = "inprocess.Runtime.cronSpec"

	if trigger.Cron == "" {
		return "", fmt.Errorf("%s: %w: cron expression is required", op, ErrInvalidTrigger)
	}

	timezone := trigger.Timezone
	if timezone == "" {
		timezone = r.cfg.DefaultTimezone
	}

	if timezone == "" {
		return trigger.Cron, nil
	}

	if _, err := time.LoadLocation(timezone); err != nil {
		return "", fmt.Errorf("%s: %w: invalid timezone %s: %w", op, ErrInvalidTrigger, timezone, err)
	}

	return "CRON_TZ=" + timezone + " " + trigger.Cron, nil
}

func validateTask(t task.Task) error {
	const op = "inprocess.validateTask"

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
			return fmt.Errorf("%s: %w: cron expression is required", op, ErrInvalidTrigger)
		}

	case task.TriggerInterval:
		if t.Trigger.Interval <= 0 {
			return fmt.Errorf("%s: %w: interval must be positive", op, ErrInvalidTrigger)
		}

	default:
		return fmt.Errorf("%s: %w: %s", op, ErrUnsupportedTrigger, t.Trigger.Type)
	}

	return nil
}
