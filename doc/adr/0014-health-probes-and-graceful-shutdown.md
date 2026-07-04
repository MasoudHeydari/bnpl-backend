# ADR 0014: Health Probes, Dependency Checks, and Graceful Shutdown

## Status

Accepted

## Context

Kubernetes and load balancers need liveness and readiness probes that behave correctly during graceful shutdown. Readiness must fail while the process drains in-flight work but before the process is killed. Liveness should remain successful until shutdown exceeds its deadline.

The application also needs a composable way to verify runtime dependencies (database, cache, messaging) without embedding SQL or broker clients in HTTP handlers or use case packages.

Process startup must construct dependencies in real dependency order, roll back partial initialization, and share one ownership path for cleanup with graceful shutdown.

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
- Database readiness fails closed when the pool or health repository was not wired (nil is not treated as healthy)

### HTTP (`pkg/adapter/restful/gin/healthrs`)

- `GET /healthz/live` and `GET /healthz/ready` when `usecases.health.enabled` is true
- Handlers call the use case only; errors serialized via `serdser.SerErr`

### Process lifecycle (`cmd/bnpl/bootstrap`)

`Boot` listens for `SIGINT` / `SIGTERM` and delegates to `Run`. The CLI layer owns process exit (`os.Exit`); bootstrap does not.

**Construction (`NewApp`)** — dependency order:

1. Infrastructure adapters that do not need the DB pool (logger, metrics, cache, messaging, task runtime, outbound HTTP client)
2. Database pool
3. `DependencyChecker` wired against the live pool (never constructed with a nil pool)
4. External clients and use cases

Partial construction failures release already-created resources through `App.Close`.

**Prepare / start (`Run`)**

1. Register tasks, consumers, routes
2. Bind HTTP (and metrics) listeners before starting background workers so bind failures do not leave workers running
3. Start task runtime + message consumer, then serve
4. Supervise until context cancel, listener failure, or fatal worker exit

**Graceful shutdown** (single global budget: `infrastructure.shutdown.timeout`)

1. `LifecycleState.StartDraining()` — readiness fails immediately
2. `drain_delay` — wait for load-balancer / kube-proxy propagation while dependencies stay up
3. HTTP (then metrics) `Shutdown` — stop accepting; finish in-flight work against live dependencies
4. Cancel worker runtime context; wait for task scheduler and consumer goroutines
5. Release App-owned resources in reverse dependency order: outbound HTTP client → publisher → subscriber → messaging health → cache → external → database pool

`App.Close` / `releaseResources` is the single ownership path for shared dependencies. Startup rollback and graceful shutdown both use it; cleanup is idempotent (`sync.Once`).

Normal `http.ErrServerClosed` and expected `context.Canceled` worker stops are not fatal process errors. Unexpected listener exits and fatal task/consumer exits terminate the supervise loop and enter the same shutdown path. On shutdown timeout: `LifecycleState.MarkShutdownTimedOut()` (liveness fails).

Configuration:

- `infrastructure.shutdown.timeout` / `SHUTDOWN_TIMEOUT` (default 30s) — global graceful-shutdown budget
- `infrastructure.shutdown.drain_delay` / `SHUTDOWN_DRAIN_DELAY` — applied after draining starts and before HTTP shutdown (zero means no wait; deployment defaults live in env examples)

### Config and version validation

- `versions.config` and `versions.database` semver fields are validated at `config.Load` against binary-supported versions (0.2.0).

## Consequences

**Positive**

- Probe semantics match Kubernetes expectations
- Dependency checks stay in infrastructure; use case stays technology-agnostic
- Startup wiring cannot silently skip database readiness via a nil pool
- Shared dependency cleanup cannot diverge between rollback and shutdown
- Drain delay is honored so readiness failure can propagate before listeners stop

**Negative**

- Non-context `Close()` calls after worker stop are not individually deadline-bounded; the process still uses one overall shutdown budget for context-aware steps
- Readiness uses standard `serdser` error envelopes on failure (not a dedicated probe JSON schema for all error paths)
- Health checks list in YAML is static (not env-expanded per check)

**Operational impact**

- Set `HEALTH_ENABLED=true` and point probes at `/healthz/live` and `/healthz/ready`
- With `infrastructure.health.checks` including `cache` or `messaging`, readiness fails when Redis or Kafka is unreachable even if the noop provider would otherwise allow startup
- Size `terminationGracePeriodSeconds` above `SHUTDOWN_TIMEOUT` (+ probe lag)

## Alternatives Considered

**Health logic only in HTTP handlers**

- Rejected; violates layered architecture and duplicates probe rules

**Single `/health` endpoint**

- Rejected; Kubernetes distinguishes liveness and readiness lifecycles

**Gate interface on `App` separate from infrastructure**

- Rejected; lifecycle state lives with other runtime infrastructure wiring

**Generic lifecycle framework (`Startable` / `Supervisor` registry)**

- Rejected; explicit bootstrap methods keep ownership readable without an internal framework

## Related ADRs

- [0001](0001-layered-architecture.md)
- [0002](0002-repository-ports.md)
- [0009](0009-provider-based-infrastructure-configuration.md)
- [0013](0013-messaging-abstraction-kafka-noop.md)
