# ADR 0011: In-Process Task Runtime and Scheduled Jobs

## Status

Accepted

## Context

The platform needs periodic background work before a separate worker service exists: publishing integration outbox rows, lightweight runtime metrics, and future batch reconciliation. An external scheduler (Kubernetes CronJob) is appropriate for production but heavy for local development.

## Decision

Define `pkg/core/task.Runtime` with `Register`, `Start`, `Dispatch`, and `Stop`. Implement:

| Provider | Package | Behavior |
| --- | --- | --- |
| `noop` | `pkg/adapter/infrastructure/task/noop` | Logs and ignores registration/dispatch |
| `inprocess` | `pkg/adapter/infrastructure/task/inprocess` | Cron, interval, manual, and event triggers in-process |

Scheduled tasks known at startup are declared under `infrastructure.task.scheduled` in YAML:

- `runtime_goroutine_metrics` — logs goroutine count on interval
- `outbox_publisher` — invokes `outboxpublisheruc.PublishDue`

`cmd/bnpl/bootstrap.RegisterTasks` reads enabled tasks from config, resolves handlers via `pkg/adapter/infrastructure/task/jobs.Registry`, and registers them with the runtime. `StartTasks` launches `Runtime.Start` in a background goroutine during HTTP server bootstrap.

On-demand work (future user-triggered jobs) should use `Runtime.Dispatch`, not the scheduled registry.

## Consequences

**Positive**

- No external scheduler required for development
- Trigger types (cron, interval) configurable via environment
- Clear separation between scheduled config tasks and runtime dispatch

**Negative**

- In-process scheduler does not survive process restarts with at-least-once guarantees across replicas — multiple instances run duplicate schedules unless tasks are idempotent (outbox uses DB locking)
- `event` trigger type is defined but has limited production use without a bus
- Task failures are logged; no dead-letter queue

**Operational impact**

- Set `TASK_PROVIDER=noop` to disable background work
- Production may eventually move heavy jobs to dedicated workers

## Alternatives Considered

**Kubernetes CronJob only**

- Rejected for early development velocity; still valid for future production batch jobs

**Embed robfig/cron directly in main**

- Rejected: no abstraction for testing and noop disable path

## Related ADRs

- [0012](0012-transactional-outbox-publisher.md) — outbox publisher job
