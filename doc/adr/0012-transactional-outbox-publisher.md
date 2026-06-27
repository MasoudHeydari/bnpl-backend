# ADR 0012: Transactional Outbox Publisher

## Status

Accepted

## Context

BNPL domain services must emit integration events (payments, audit, notifications) without dual-write failures between PostgreSQL and Kafka. The transactional outbox pattern stores outbound messages in the same database transaction as business data; a separate process publishes them to the message broker.

## Decision

1. **Schema:** Add `integration_outbox` table in embedded `000_schema.sql` with status, retry, and worker-lock columns.

2. **Port:** `pkg/core/repo.Outbox` with `ClaimPending`, `MarkPublished`, `MarkFailed`, and `ScheduleRetry`.

3. **Repository:** `pkg/adapter/db/postgres/outboxrp` implements claiming with `FOR UPDATE SKIP LOCKED`.

4. **Use case:** `pkg/core/usecase/outboxpublisheruc.PublishDue` claims a batch, publishes via `messaging.Publisher` (topic = `destination` column), and updates row state with exponential backoff between retries.

5. **Scheduling:** Register `outbox_publisher` task in bootstrap when `TASK_OUTBOX_PUBLISHER_ENABLED=true`.

Configuration:

- Use case: `usecases.outbox_publisher` (`OUTBOX_PUBLISHER_*` env vars)
- Task: `infrastructure.task.scheduled.outbox_publisher` (`TASK_OUTBOX_PUBLISHER_*` env vars)

**Not in scope yet:** domain use cases that insert outbox rows inside business transactions.

## Consequences

**Positive**

- Reliable handoff to Kafka when `MESSAGING_PROVIDER=kafka`
- Safe concurrent workers via row locking
- Retry and permanent failure states auditable in SQL

**Negative**

- With `MESSAGING_PROVIDER=noop`, publishes succeed locally but messages are not delivered externally
- No exactly-once consumption guarantee — consumers must be idempotent
- No insert API for domain teams yet

**Operational impact**

- Monitor `integration_outbox` for rows stuck in `failed` or old `pending`
- Worker ID must be unique per concurrent publisher instance

## Alternatives Considered

**Direct Kafka publish from use cases**

- Rejected: risks inconsistency on DB commit failure

**Change Data Capture (Debezium) only**

- Deferred: higher operational complexity for current maturity stage

## Related ADRs

- [0011](0011-in-process-task-runtime.md)
- [0013](0013-messaging-abstraction-kafka-noop.md)
