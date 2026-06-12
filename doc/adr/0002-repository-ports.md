# ADR 0002: Repository Ports for Database Access

## Status

Accepted

## Context

Database initialization and migration workflows require connection pooling, transactions, raw SQL execution, schema DDL, and migration registration. The migration use case in `pkg/core/usecase/migrationsuc` must coordinate these operations atomically without importing `github.com/jackc/pgx/v5`.

Concrete needs observed in code:

- `InitDBUseCase.dropAndCreateAgain` runs admin DDL inside `Conn.Tx`.
- `InitDBUseCase.initDB` runs embedded bootstrap SQL inside a normal-role transaction.
- `UseCase.MigrateOne` executes filesystem SQL and inserts into `migrations` in one transaction.

Direct `pgx` usage in use cases would couple financial orchestration to a single driver and complicate unit testing.

## Decision

Define repository-style ports in `pkg/core/repo`:

| Port | Purpose |
| --- | --- |
| `Pool`, `Conn`, `Tx`, `Queryer`, `Rows` | Connection lifecycle and SQL execution |
| `Schema`, `SchemaQueryer` | Schema drop/create, role management, privileges |
| `Migrations`, `MigrationsQueryer` | Migration SQL execution and registration |
| `SchemaInitializer` | Embedded bootstrap DDL and seed data |
| `SchemaSettings` | Configuration surface for `InitDBUseCase` |

Implement ports in `pkg/adapter/db/postgres`, `schemarp`, and `migrationsrp`.

Transaction semantics are callback-based: `Conn.Tx(ctx, handler)` commits on `nil`, rolls back on error or panic (`postgres/conn.go`).

## Consequences

**Positive**

- Use cases express transaction boundaries explicitly (`initdb.go` admin and normal phases).
- Migration execution and registration are atomic (`migrationuc.go` + `migrationsrp.Register`).
- Future BNPL repositories (Contract, PaymentAttempt) can extend the same port pattern.

**Negative**

- Adapter wrappers (`postgres.Conn`, `postgres.Tx`) duplicate `Exec`/`Query` forwarding.
- Binding requires type assertions (`tx.(*postgres.Tx)`) at repository factory methods.
- Generic query helpers in `schemarp` interpolate trusted identifiers into DDL because PostgreSQL does not parameterize schema or role names.

**Operational impact**

- Failures surface as wrapped errors through use case layers (`fmt.Errorf` chains in `initdb.go`).
- Pool creation validates connectivity immediately (`postgres.NewPool` acquires a connection at startup).

## Alternatives Considered

**Use `pgx` directly in use cases**

- Rejected: prevents swapping drivers and forces integration tests for all orchestration logic.

**ORM (GORM, ent)**

- Rejected for now: BNPL schema will include financial precision, audit columns, and regulatory constraints that benefit from explicit SQL; current bootstrap already uses raw SQL.

**Single large `Database` interface**

- Rejected: splitting Schema, Migrations, and Queryer keeps initialization concerns separate from future read/write repositories.

## Relationship To Future BNPL Domains

Each BNPL aggregate will need its own repository port in `pkg/core/repo` (for example `ContractRepo`, `InstallmentRepo`) with PostgreSQL implementations under `pkg/adapter/db/postgres`. Shared primitives (`Pool`, `Conn`, `Tx`) remain the transaction boundary for multi-table workflows such as:

- Creating a Contract and its RepaymentPlan in one transaction.
- Recording a PaymentAttempt and updating Installment status atomically.
- Writing Audit rows in the same transaction as state changes.

The current `Schema` and `Migrations` ports cover platform bootstrap only; they do not represent business aggregates.
