# ADR 0010: Cache Abstraction and Adapters

## Status

Accepted

## Context

Future BNPL workflows need short-lived shared state: idempotency keys, OTP verification, rate limits, and distributed locks. The core layer must not depend on Redis client libraries or in-process maps directly.

## Decision

Define `pkg/core/cache.Cache` as a byte-oriented cache port with `Get`, `Set`, `SetNX`, `Delete`, `Exists`, `TTL`, `Increment`, and `Close`. Implement adapters:

| Adapter | Package | Selectable via config |
| --- | --- | --- |
| No-op | `pkg/adapter/infrastructure/cache/noop` | Yes (`CACHE_PROVIDER=noop`) |
| Redis | `pkg/adapter/infrastructure/cache/redis` | Yes (`CACHE_PROVIDER=redis`) |
| In-memory | `pkg/adapter/infrastructure/cache/memory` | **No** — tests and single-process use only |

Provider selection is configured under `infrastructure.cache.provider`. When `redis` is selected, startup validates connectivity with `PING` before accepting traffic.

The noop adapter always misses on read and does not retain state; it must not be used for production coordination semantics.

## Consequences

**Positive**

- Domain code can depend on `cache.Cache` without Redis imports
- Redis-specific behavior (Lua increment + TTL) is isolated in the adapter
- noop enables local runs without Redis

**Negative**

- `memory` adapter exists but is not wired in config — manual injection only
- No cache key naming convention enforced at core layer
- Serialization is caller responsibility

**Operational impact**

- Production cache requires Redis HA planning separate from this repository
- `CACHE_PROVIDER=redis` causes startup failure when Redis is down

## Alternatives Considered

**Use Redis directly in use cases**

- Rejected: couples financial logic to infrastructure

**Expose memory as third config provider**

- Deferred: risks accidental use in multi-instance deployments

## Related ADRs

- [0009](0009-provider-based-infrastructure-configuration.md)
