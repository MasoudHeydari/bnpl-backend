package task

import (
	"context"
	"time"
)

// Job represents executable background work.
//
// Jobs are the in-memory executable form of a task.
//
// Implementations should:
//
//   - respect context cancellation
//   - tolerate retries whenever possible
//   - avoid unbounded execution times
//   - be safe to execute more than once when feasible
//
// Infrastructure implementations are responsible for determining when
// and how jobs are executed.
type Job func(ctx context.Context) error

// TriggerType identifies how a scheduled task is triggered.
type TriggerType string

const (
	// TriggerManual indicates that a task is executed only when explicitly
	// triggered by the application.
	TriggerManual TriggerType = "manual"

	// TriggerCron indicates that a task is executed according to a cron
	// expression.
	TriggerCron TriggerType = "cron"

	// TriggerInterval indicates that a task is executed repeatedly with
	// a fixed interval between executions.
	TriggerInterval TriggerType = "interval"

	// TriggerEvent indicates that execution is driven by external events.
	//
	// This trigger type is primarily intended for workflow engines,
	// messaging systems, and event-driven orchestration.
	TriggerEvent TriggerType = "event"
)

// Trigger describes when a task should be executed.
//
// The fields used depend on the selected TriggerType.
//
// Examples:
//
//	Trigger{
//		Type: TriggerCron,
//		Cron: "0 2 * * *",
//	}
//
//	Trigger{
//		Type: TriggerInterval,
//		Interval: 15 * time.Minute,
//		RunOnStart: true,
//	}
type Trigger struct {
	// Type defines the scheduling strategy.
	Type TriggerType

	// Cron contains a cron expression when Type is TriggerCron.
	//
	// Example:
	//
	//	0 2 * * *
	//
	// Executes every day at 02:00.
	Cron string

	// Interval defines the execution frequency when
	// Type is TriggerInterval.
	Interval time.Duration

	// Timezone specifies the timezone used when evaluating
	// cron expressions.
	//
	// An empty value indicates the scheduler default timezone.
	Timezone string

	// RunOnStart indicates whether the task should execute immediately
	// when the scheduler starts.
	//
	// This option is typically useful for interval-based tasks.
	RunOnStart bool
}

// Task describes a scheduled unit of work.
//
// Tasks are generally registered during application bootstrap and become
// eligible for execution once a Scheduler is started.
//
// A Task combines:
//
//   - task identity
//   - scheduling metadata
//   - execution metadata
//   - an executable Job implementation
//
// Some implementations may ignore Job entirely and instead resolve execution
// through Handler identifiers.
type Task struct {
	// Name uniquely identifies the task.
	//
	// Names are primarily intended for:
	//
	//   - logging
	//   - metrics
	//   - observability
	//   - operational debugging
	Name string

	// Enabled determines whether the task should be registered.
	Enabled bool

	// Trigger defines when the task should execute.
	Trigger Trigger

	// Handler identifies the business operation represented by the task.
	//
	// Handler names should remain stable across implementations.
	//
	// Examples:
	//
	//	"runtime_goroutine_metrics"
	//	"payment_reconciliation"
	//
	// Workflow engines such as Temporal may use Handler names to resolve
	// workflows or activities.
	Handler string

	// Job contains the executable task implementation.
	//
	// In-process implementations typically execute this function directly.
	//
	// Distributed workflow systems may ignore Job entirely and instead
	// resolve execution using Handler and provider-specific registries.
	Job Job
}

// Command describes an asynchronous task execution request.
//
// Unlike Task, a Command is not scheduled.
//
// Commands are dispatched on demand and are intended for:
//
//   - asynchronous processing
//   - workflow execution
//   - queue-based job execution
//   - event-driven background work
//
// The Command abstraction allows business code to request execution
// without knowing whether the underlying implementation uses:
//
//   - goroutines
//   - message queues
//   - Temporal workflows
//   - external workers
type Command struct {
	// ID uniquely identifies the execution request.
	//
	// Implementations may use this field for:
	//
	//   - idempotency
	//   - tracing
	//   - correlation
	//   - workflow identifiers
	ID string

	// Handler identifies the operation to execute.
	//
	// Handler names should correspond to registered task handlers.
	Handler string

	// Payload contains provider-specific execution data.
	//
	// The payload should be serializable when used with distributed
	// orchestration systems.
	Payload any
}

// Scheduler registers and executes scheduled tasks.
//
// Scheduler implementations are typically long-lived and started during
// application bootstrap.
type Scheduler interface {
	// Register registers a scheduled task.
	//
	// Registration does not imply execution.
	//
	// Tasks become eligible for execution after Start is called.
	Register(task Task) error

	// Start starts the scheduler.
	//
	// Start blocks until:
	//
	//   - the provided context is canceled
	//   - a fatal runtime error occurs
	Start(ctx context.Context) error
}

// Dispatcher dispatches asynchronous task executions.
//
// Dispatchers are responsible for delivering work to the underlying
// execution system.
type Dispatcher interface {
	// Dispatch submits a command for execution.
	//
	// Implementations may execute the command:
	//
	//   - immediately
	//   - asynchronously
	//   - through queues
	//   - through workflow engines
	Dispatch(ctx context.Context, command Command) error
}

// Runtime provides scheduling and dispatch capabilities.
//
// Most task providers are expected to implement Runtime.
//
// Examples:
//
//   - inprocess
//   - Temporal
//   - distributed worker systems
//   - noop implementations
type Runtime interface {
	Scheduler
	Dispatcher
}
