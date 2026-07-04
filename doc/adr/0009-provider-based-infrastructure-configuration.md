# ADR 0009: Provider-Based Infrastructure Configuration

## Status

Accepted

## Context

The BNPL backend needs logging, outbound HTTP, caching, messaging, background tasks, clocks, and trace identifiers across environments that differ widely:

- Local development may run PostgreSQL only
- Staging may enable Kafka and Redis
- Tests should disable external systems

Hard-wiring concrete adapters in bootstrap would force every environment to depend on every integration and complicate unit testing.

## Decision

Add an `infrastructure:` top-level YAML section decoded into `pkg/adapter/config.Infrastructure`. Each concern exposes a `provider` field; factory methods on `config.Config` construct only the selected implementation:

| Concern | Providers | Factory |
| --- | --- | --- |
| Logger | `noop`, `slog` | `NewLoggerInfrastructure` |
| Outbound HTTP client | `noop`, `standard` | `NewHTTPClientInfrastructure` |
| Cache | `noop`, `redis` | `NewCacheInfrastructure` |
| Messaging | `noop`, `kafka` | `NewMessagingPublisherInfrastructure`, `NewMessagingSubscriberInfrastructure` |
| Task | `noop`, `inprocess` | `NewTaskInfrastructure` |
| Clock | system (dual timezone) | `NewSystemClockInfrastructure` |
| Trace | ULID | `NewTraceInfrastructure` |

Provider-specific settings live in nested YAML blocks (for example `infrastructure.cache.redis`). Unsupported provider values fail at startup with an explicit error. The outbound HTTP standard provider starts with one shared default profile; named per-client overrides are intentionally deferred. Its concurrency limit is implemented as a bounded bulkhead around one reusable `net/http.Client`, not as multiple clients or dedicated worker goroutines. The outbound HTTP execution contract is owned by Core (`pkg/core/httpclient`), while provider selection and concrete `noop`/`standard` implementations remain in the adapter layer. Outbound HTTP metric contracts are similarly owned by `pkg/core/metrics`.

Bootstrap (`cmd/bnpl/bootstrap`) calls these factories during `NewInfrastructures()` before use cases that depend on them.

## Consequences

**Positive**

- Operators enable integrations via environment variables without code changes
- Local development defaults (`noop` cache/messaging) avoid Redis/Kafka dependencies
- Clear extension point for future providers

**Negative**

- Large configuration surface documented in `.env.example`
- Misconfigured provider strings fail at runtime, not via schema validation
- Subscriber and cache may be constructed but unused until features consume them

**Operational impact**

- Production must explicitly set providers and verify connectivity (Redis ping on cache startup)
- Production rejects the outbound HTTP `noop` provider so third-party calls cannot be silently disabled
- Circuit breaker (`noop`, `gobreaker`) is wired via `NewCircuitBreakerInfrastructure` but not yet used by domain call sites

### Amendment (2026-06-30)

| Concern | Providers | Factory |
| --- | --- | --- |
| Circuit breaker | `noop`, `gobreaker` | `NewCircuitBreakerInfrastructure` |
| Shutdown | timeout, drain_delay settings | `GetShutdownTimeout`, `GetShutdownDrainDelay` |
| Health dependency checks | YAML list: `database`, `cache`, `messaging` | `NewDependencyCheckerInfrastructure` |

See ADR [0014](0014-health-probes-and-graceful-shutdown.md).

## Alternatives Considered

**Separate binaries per integration profile**

- Rejected: increases build and deploy complexity

**Compile-time build tags for providers**

- Rejected: harder to test all combinations in CI

## Related ADRs

- [0003](0003-env-expanded-yaml-config.md) — YAML + env expansion
- [0010](0010-cache-abstraction-and-adapters.md) — cache providers
- [0013](0013-messaging-abstraction-kafka-noop.md) — messaging providers
