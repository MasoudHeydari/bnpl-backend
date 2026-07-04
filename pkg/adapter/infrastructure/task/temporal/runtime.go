package temporal

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.temporal.io/api/serviceerror"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	temporalsdk "go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"

	"github.com/parstis/bnpl-backend/pkg/core/log"
	"github.com/parstis/bnpl-backend/pkg/core/task"
)

const (
	workflowTypeName        = "bnpl.task.run.v1"
	activityNamePrefix      = "bnpl.task.activity."
	scheduleIDPrefix        = "bnpl.task.schedule."
	scheduledWorkflowPrefix = "bnpl.task.scheduled."
	dispatchWorkflowPrefix  = "bnpl.task.dispatch."
)

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
	ErrRuntimeAlreadyStarted  = errors.New("task runtime already started")
)

// Config configures the Temporal task runtime.
type Config struct {
	Address                     string
	Namespace                   string
	TaskQueue                   string
	ActivityStartToCloseTimeout time.Duration
	MaxConcurrentActivities     int
	MaxConcurrentWorkflowTasks  int
	TLS                         TLSConfig
}

// TLSConfig configures optional TLS for the Temporal client connection.
type TLSConfig struct {
	Enabled    bool
	ServerName string
}

// Runtime executes task jobs as Temporal Activities coordinated by one generic
// Workflow and Temporal Schedules.
type Runtime struct {
	client client.Client
	worker worker.Worker
	cfg    Config
	logger log.Logger

	mu       sync.Mutex
	started  bool
	tasks    map[string]task.Task
	handlers map[string]struct{}
	fatalErr chan error
}

// New dials Temporal and constructs a worker for the configured task queue.
func New(cfg Config, logger log.Logger) (*Runtime, error) {
	const op = "temporal.New"

	clientOptions := client.Options{
		HostPort:  cfg.Address,
		Namespace: cfg.Namespace,
		Logger:    logger,
	}
	if cfg.TLS.Enabled {
		clientOptions.ConnectionOptions.TLS = &tls.Config{
			MinVersion: tls.VersionTLS12,
			ServerName: strings.TrimSpace(cfg.TLS.ServerName),
		}
	}

	temporalClient, err := client.Dial(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("%s: dial temporal: %w", op, err)
	}

	r := &Runtime{
		client:   temporalClient,
		cfg:      cfg,
		logger:   logger,
		tasks:    make(map[string]task.Task),
		handlers: make(map[string]struct{}),
		fatalErr: make(chan error, 1),
	}

	r.worker = worker.New(temporalClient, cfg.TaskQueue, worker.Options{
		MaxConcurrentActivityExecutionSize:     cfg.MaxConcurrentActivities,
		MaxConcurrentWorkflowTaskExecutionSize: cfg.MaxConcurrentWorkflowTasks,
		DisableRegistrationAliasing:            true,
		OnFatalError: func(err error) {
			select {
			case r.fatalErr <- err:

			default:
			}
		},
	})
	r.worker.RegisterWorkflowWithOptions(runTaskWorkflow, workflow.RegisterOptions{Name: workflowTypeName})

	return r, nil
}

// Register registers a task Activity and stores its schedule definition.
func (r *Runtime) Register(t task.Task) error {
	const op = "temporal.Runtime.Register"

	if strings.TrimSpace(t.Name) == "" {
		return ErrTaskNameRequired
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrRuntimeAlreadyStarted
	}

	if _, exists := r.tasks[t.Name]; exists {
		return fmt.Errorf("%s: %w: %s", op, ErrDuplicateTaskName, t.Name)
	}

	// Disabled tasks are retained so Start can remove a previously persisted
	// Temporal Schedule with the same deterministic schedule ID.
	if !t.Enabled {
		r.tasks[t.Name] = t
		return nil
	}

	if strings.TrimSpace(t.Handler) == "" {
		return ErrTaskHandlerRequired
	}

	if t.Job == nil {
		return ErrTaskJobRequired
	}

	if err := validateTrigger(t.Trigger); err != nil {
		return err
	}

	if _, exists := r.handlers[t.Handler]; exists {
		return fmt.Errorf("%s: %w: %s", op, ErrDuplicateHandler, t.Handler)
	}

	r.worker.RegisterActivityWithOptions(t.Job, activity.RegisterOptions{Name: activityName(t.Handler)})
	r.tasks[t.Name] = t
	r.handlers[t.Handler] = struct{}{}

	r.logger.With(
		"task_name",
		t.Name,
		"task_handler",
		t.Handler,
		"trigger_type",
		string(t.Trigger.Type),
	).Info("temporal task registered")

	return nil
}

// Dispatch starts a Temporal Workflow for an already registered handler.
func (r *Runtime) Dispatch(ctx context.Context, command task.Command) error {
	const op = "temporal.Runtime.Dispatch"

	if strings.TrimSpace(command.Handler) == "" {
		return ErrCommandHandlerRequired
	}

	r.mu.Lock()
	_, ok := r.handlers[command.Handler]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("%s: %w: %s", op, ErrUnknownHandler, command.Handler)
	}

	id := strings.TrimSpace(command.ID)
	if id == "" {
		id = uuid.NewString()
	}

	_, err := r.client.ExecuteWorkflow(
		ctx,
		client.StartWorkflowOptions{
			ID:        dispatchWorkflowPrefix + command.Handler + "." + id,
			TaskQueue: r.cfg.TaskQueue,
		},
		workflowTypeName,
		workflowInput{
			Handler:                     command.Handler,
			ActivityStartToCloseTimeout: r.cfg.ActivityStartToCloseTimeout,
		},
	)
	if err != nil {
		return fmt.Errorf("%s: start temporal task workflow for handler %q: %w", op, command.Handler, err)
	}

	return nil
}

// Start starts the Temporal worker, reconciles persistent schedules, and then
// blocks until cancellation or a fatal worker error.
func (r *Runtime) Start(ctx context.Context) error {
	const op = "temporal.Runtime.Start"

	r.mu.Lock()
	if r.started {
		r.mu.Unlock()
		return ErrRuntimeAlreadyStarted
	}

	r.started = true
	tasks := make([]task.Task, 0, len(r.tasks))
	for _, t := range r.tasks {
		tasks = append(tasks, t)
	}

	r.mu.Unlock()

	if err := r.worker.Start(); err != nil {
		return fmt.Errorf("%s: start temporal worker: %w", op, err)
	}

	for _, t := range tasks {
		if err := r.reconcileSchedule(ctx, t); err != nil {
			r.worker.Stop()
			return err
		}
	}

	r.logger.With(
		"task_count",
		len(tasks),
		"task_queue",
		r.cfg.TaskQueue,
		"namespace",
		r.cfg.Namespace,
	).Info("starting temporal task runtime")

	select {
	case <-ctx.Done():
		r.logger.Info("stopping temporal task runtime")
		r.worker.Stop()
		r.logger.Info("temporal task runtime stopped")
		return nil

	case err := <-r.fatalErr:
		if err == nil {
			err = errors.New("temporal worker reported an empty fatal error")
		}

		return fmt.Errorf("%s: temporal worker fatal error: %w", op, err)
	}
}

// Close closes the Temporal service client.
//
// Start must already have stopped the worker (via context cancel) before
// process resource cleanup reaches this method.
func (r *Runtime) Close() error {
	if r.client != nil {
		r.client.Close()
		r.client = nil
	}

	return nil
}

func (r *Runtime) reconcileSchedule(ctx context.Context, t task.Task) error {
	const op = "temporal.Runtime.reconcileSchedule"

	scheduleID := scheduleIDPrefix + t.Name
	handle := r.client.ScheduleClient().GetHandle(ctx, scheduleID)

	if !t.Enabled || t.Trigger.Type == task.TriggerManual {
		if err := handle.Delete(ctx); err != nil {
			var notFound *serviceerror.NotFound
			if !errors.As(err, &notFound) {
				return fmt.Errorf("%s: delete temporal schedule %q: %w", op, scheduleID, err)
			}
		}

		return nil
	}

	options, err := r.scheduleOptions(t)
	if err != nil {
		return err
	}

	createdHandle, err := r.client.ScheduleClient().Create(ctx, options)
	if err == nil {
		r.logger.With("task_name", t.Name, "schedule_id", createdHandle.GetID()).Info("temporal schedule created")
		return nil
	}

	var alreadyExists *serviceerror.AlreadyExists
	if !errors.As(err, &alreadyExists) {
		return fmt.Errorf("%s: create temporal schedule %q: %w", op, scheduleID, err)
	}

	if err = handle.Update(ctx, client.ScheduleUpdateOptions{
		DoUpdate: func(input client.ScheduleUpdateInput) (*client.ScheduleUpdate, error) {
			current := input.Description.Schedule
			current.Spec = &options.Spec
			current.Action = options.Action
			return &client.ScheduleUpdate{Schedule: &current}, nil
		},
	}); err != nil {
		return fmt.Errorf("%s: update temporal schedule %q: %w", op, scheduleID, err)
	}

	r.logger.With("task_name", t.Name, "schedule_id", scheduleID).Info("temporal schedule updated")
	return nil
}

func (r *Runtime) scheduleOptions(t task.Task) (client.ScheduleOptions, error) {
	const op = "temporal.Runtime.scheduleOptions"

	spec := client.ScheduleSpec{}
	switch t.Trigger.Type {
	case task.TriggerCron:
		if strings.TrimSpace(t.Trigger.Cron) == "" {
			return client.ScheduleOptions{}, fmt.Errorf("%s: %w: task %q cron expression is required", op, ErrInvalidTrigger, t.Name)
		}

		spec.CronExpressions = []string{t.Trigger.Cron}
		spec.TimeZoneName = t.Trigger.Timezone

	case task.TriggerInterval:
		if t.Trigger.Interval <= 0 {
			return client.ScheduleOptions{}, fmt.Errorf("%s: %w: task %q interval must be positive", op, ErrInvalidTrigger, t.Name)
		}

		spec.Intervals = []client.ScheduleIntervalSpec{{Every: t.Trigger.Interval}}

	default:
		return client.ScheduleOptions{}, fmt.Errorf("%s: %w: %s", op, ErrUnsupportedTrigger, t.Trigger.Type)
	}

	return client.ScheduleOptions{
		ID:                 scheduleIDPrefix + t.Name,
		Spec:               spec,
		TriggerImmediately: t.Trigger.RunOnStart,
		Action: &client.ScheduleWorkflowAction{
			ID:        scheduledWorkflowPrefix + t.Name,
			Workflow:  workflowTypeName,
			TaskQueue: r.cfg.TaskQueue,
			Args: []interface{}{workflowInput{
				Handler:                     t.Handler,
				ActivityStartToCloseTimeout: r.cfg.ActivityStartToCloseTimeout,
			}},
		},
	}, nil
}

func validateTrigger(trigger task.Trigger) error {
	const op = "temporal.validateTrigger"

	switch trigger.Type {
	case task.TriggerManual:
		return nil

	case task.TriggerCron:
		if strings.TrimSpace(trigger.Cron) == "" {
			return fmt.Errorf("%s: %w: cron expression is required", op, ErrInvalidTrigger)
		}

		return nil

	case task.TriggerInterval:
		if trigger.Interval <= 0 {
			return fmt.Errorf("%s: %w: interval must be positive", op, ErrInvalidTrigger)
		}

		return nil

	default:
		return fmt.Errorf("%s: %w: %s", op, ErrUnsupportedTrigger, trigger.Type)
	}
}

func activityName(handler string) string {
	return activityNamePrefix + handler
}

// workflowInput contains only deterministic execution metadata. The current
// task.Job contract has no payload argument, so Command.Payload intentionally
// remains unused just as it is by the existing in-process runtime.
type workflowInput struct {
	Handler                     string
	ActivityStartToCloseTimeout time.Duration
}

func runTaskWorkflow(ctx workflow.Context, input workflowInput) error {
	activityCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: input.ActivityStartToCloseTimeout,
		// Existing jobs already own their retry/idempotency semantics. Avoid
		// introducing a second retry layer merely by moving execution to Temporal.
		RetryPolicy: &temporalsdk.RetryPolicy{MaximumAttempts: 1},
	})

	return workflow.ExecuteActivity(activityCtx, activityName(input.Handler)).Get(activityCtx, nil)
}
