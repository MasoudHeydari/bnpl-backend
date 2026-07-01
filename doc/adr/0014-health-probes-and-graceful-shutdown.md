# ADR 0014: Health Probes, Dependency Checks, and Graceful Shutdown

## Status

Accepted

## Context

Kubernetes and load balancers need liveness and readiness probes that behave correctly during graceful shutdown. Readiness must fail while the process drains in-flight work but before the process is killed. Liveness should remain successful until shutdown exceeds its deadline.

The application also needs a composable way to verify runtime dependencies (database, cache, messaging) without embedding SQL or broker clients in HTTP handlers or use case packages.

## Decision

### Core contracts (`pkg/core/health`)

- `LifecycleState` — draining and shutdown-timeout flags for probes and bootstrap
- `DependencyChecker` — `Check(ctx) error` for readiness dependency verification
- `repo.Health` — connection-scoped database ping port (`healthrp` in PostgreSQL adapter)

### Use case (`pkg/core/usecase/healthcheckuc`)

- `Liveness` — no external dependency checks; fails with `cerr.ServiceUnavailable` when shutdown timed out
- `Readiness` — fails when draining or when `DependencyChecker.Check` returns an error

### Infrastructure adapters (`pkg/adapter/infrastructure/health`)

- `AtomicLifecycleState` — implements `LifecycleState`
- `DatabaseDependencyChecker`, `RedisDependencyChecker`, `MessagingDependencyChecker`
- `CompositeDependencyChecker` — runs configured checks from `infrastructure.health.checks`

### HTTP (`pkg/adapter/restful/gin/healthrs`)

- `GET /healthz/live` and `GET /healthz/ready` when `usecases.health.enabled` is true
- Handlers call the use case only; errors serialized via `serdser.SerErr`

### Graceful shutdown (`cmd/bnpl/bootstrap`)

- `Boot` listens for `SIGINT` / `SIGTERM`
- On signal: `LifecycleState.StartDraining()` (readiness fails), bounded shutdown context from `infrastructure.shutdown.timeout`
- Ordered stop: HTTP → task runtime and message consumer → in-flight drain window → publisher, subscriber, cache, database pool
- On shutdown timeout: `LifecycleState.MarkShutdownTimedOut()` (liveness fails)

Configuration:

- `infrastructure.shutdown.timeout` / `SHUTDOWN_TIMEOUT` (default 30s)
- `infrastructure.shutdown.drain_delay` / `SHUTDOWN_DRAIN_DELAY` (default 5s) — decoded and exposed via config API; **not yet applied** in the bootstrap shutdown sequence (readiness draining is immediate on signal)

### Config and version validation

- `versions.config` and `versions.database` semver fields are validated at `config.Load` against binary-supported versions (0.2.0).

## Consequences

**Positive**

- Probe semantics match Kubernetes expectations
- Dependency checks stay in infrastructure; use case stays technology-agnostic
- Graceful shutdown stops background workers before closing brokers and the database pool

**Negative**

- `SHUTDOWN_DRAIN_DELAY` is configured but not wired into bootstrap timing yet
- Readiness uses standard `serdser` error envelopes on failure (not a dedicated probe JSON schema for all error paths)
- Health checks list in YAML is static (not env-expanded per check)

**Operational impact**

- Set `HEALTH_ENABLED=true` and point probes at `/healthz/live` and `/healthz/ready`
- With `infrastructure.health.checks` including `cache` or `messaging`, readiness fails when Redis or Kafka is unreachable even if the noop provider would otherwise allow startup

## Alternatives Considered

**Health logic only in HTTP handlers**

- Rejected; violates layered architecture and duplicates probe rules

**Single `/health` endpoint**

- Rejected; Kubernetes distinguishes liveness and readiness lifecycles

**Gate interface on `App` separate from infrastructure**

- Rejected; lifecycle state lives with other runtime infrastructure wiring

## Related ADRs

- [0001](0001-layered-architecture.md)
- [0002](0002-repository-ports.md)
- [0009](0009-provider-based-infrastructure-configuration.md)
- [0013](0013-messaging-abstraction-kafka-noop.md)
