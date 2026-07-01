# ADR 0001: Layered Architecture

## Status

Accepted

## Context

The BNPL platform must evolve from infrastructure bootstrap into a regulated financial system with many business domains (Contract, Decision, RepaymentPlan, Audit, and others). The current codebase already separates concerns across packages:

- `pkg/core/model`, `pkg/core/repo`, and `pkg/core/cerr` contain no imports of `pgx`, `gin`, `cobra`, or YAML libraries.
- `pkg/core/usecase/migrationsuc` orchestrates database initialization without SQL literals.
- `pkg/adapter/config`, `pkg/adapter/restful/gin`, and `cmd/bnpl/command` handle transport and wiring.
- `pkg/adapter/db/postgres` implements repository ports with `pgx`.

Without an explicit layering rule, business logic could accumulate in CLI handlers or SQL adapters, making future BNPL domains harder to test and audit.

## Decision

Adopt a layered architecture with strict inward dependency flow:

| Layer | Packages | Responsibility |
| --- | --- | --- |
| Core | `pkg/core/model`, `pkg/core/repo`, `pkg/core/cerr` | Domain-shaped types, port interfaces, shared error semantics |
| Application | `pkg/core/usecase/*` | Orchestration and transaction boundaries |
| Adapter | `cmd/bnpl`, `pkg/adapter/config`, `pkg/adapter/restful/gin` | CLI, HTTP, configuration translation |
| Infrastructure | `pkg/adapter/db/postgres`, `pkg/adapter/infrastructure/*` | PostgreSQL, cache, messaging, logging, tasks, trace |

Adapters and infrastructure depend on core interfaces; core packages never depend on adapters.

Evidence: `migrationsuc.InitDBUseCase` depends on `repo.SchemaSettings` and `repo.SchemaInitializer`, not on `pgx` types. `outboxpublisheruc.UseCase` depends on `repo.Outbox` and `messaging.Publisher`. `bootstrap.Boot` loads config and wires adapters without SQL in `cmd/`.

### Amendment (2026-06-27)

Infrastructure concerns (cache, messaging, logging, tasks, trace, clock) now live under `pkg/adapter/infrastructure/*` with ports in `pkg/core`. The infrastructure layer table row above was expanded accordingly. Core packages `pkg/core/cache`, `pkg/core/messaging`, `pkg/core/task`, `pkg/core/log`, `pkg/core/clock`, and `pkg/core/trace` define ports only.

## Consequences

**Positive**

- Future BNPL use cases (credit evaluation, installment generation) can be tested with mocked `repo` interfaces.
- PostgreSQL can be replaced or supplemented without rewriting application orchestration.
- Code review can enforce boundaries: business rules must not appear in `cmd/` or `postgres/` packages.

**Negative**

- More constructor and wiring code at startup (`config.Config` exposes multiple factory methods).
- Type assertions in adapters (for example `c.(*postgres.Conn)` in `schemarp.Repo.Conn`) couple adapters to concrete implementations at the binding site.

**Operational impact**

- Operators interact only with the CLI and config; layering is invisible at runtime.
- On-call engineers trace failures through layers: CLI → use case → repository adapter.

## Alternatives Considered

**Flat package layout (`internal/db`, `internal/http`)**

- Rejected: works for small services but makes domain boundaries implicit as the BNPL surface grows.

**Hexagonal architecture with explicit `ports/` and `adapters/` top-level directories**

- Rejected for now: the current `pkg/core` + `pkg/adapter` split already expresses the same idea with less directory churn.

**Framework-first design (Gin handlers calling SQL directly)**

- Rejected: would block testability and auditability required for financial workflows.

## Relationship To Future BNPL Domains

Each planned domain (Contract, Installment, PaymentAttempt, Audit) should follow the same pattern:

1. Define aggregates and repository ports in `pkg/core`.
2. Implement orchestration in `pkg/core/usecase/<domain>`.
3. Add HTTP handlers in `pkg/adapter/restful/gin/routes` that call use cases only.
4. Implement persistence in `pkg/adapter/db/postgres/<domain>rp`.

The current `migrationsuc` package is the reference implementation for this pattern until business use cases are added.
