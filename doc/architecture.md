# Architecture

## Purpose

This document describes the **current** Go backend structure and how it is intended to support a future Buy Now Pay Later (BNPL) platform. The repository today is mostly infrastructure and bootstrap code — not a complete BNPL engine.

## Current State vs Intended Direction

| Area | Current state | Intended direction |
| --- | --- | --- |
| HTTP API | Gin server, empty `/api/v1` group | REST APIs for checkout, contracts, repayments, admin |
| Database | `migrations` table only; destructive init | Full BNPL schema with forward migrations |
| Use cases | `migrationsuc` only | Credit, installment, payment, collections use cases |
| Events | Kafka in Compose, unused in Go | Domain events for audit and integrations |
| Auth | None | Role-based access for admin and merchant APIs |
| Observability | Gin logger/recovery | Structured logs, metrics, tracing |

**Do not assume business aggregates (Contract, Installment, MerchantProfile, etc.) exist in code.** They are planned domains listed for architectural alignment only.

## BNPL Platform Vision

The long-term system will handle:

- Borrower credit evaluation and credit limit management
- Installment plan generation and contract lifecycle
- Checkout financing decisions
- Payment tracking and delinquency handling
- Collections and legal case escalation
- Merchant and branch management
- Auditability and regulatory compliance

Planned domain names (not yet implemented):

Contract · Decision · RepaymentPlan · Installment · PaymentAttempt · CreditLimit · CollectionsCase · LegalCase · MerchantProfile · MerchantBranchProfile · Restriction · Block · Audit

Product context (non-English): [pintoday.md](pintoday.md), [base_doc.md](base_doc.md).

## Runtime Entrypoints

Single binary `bnpl` (Cobra):

| Command | Behavior |
| --- | --- |
| `bnpl` (default) | HTTP server |
| `bnpl db init-dev` | Destructive bootstrap + dev seed SQL |
| `bnpl db init-prod` | Destructive bootstrap + prod seed SQL |
| `bnpl db migrate` | Filesystem SQL migrations (partially wired) |

See ADR [0005](adr/0005-single-binary-cobra-commands.md).

## Layers

### Core (`pkg/core`)

| Package | Role |
| --- | --- |
| `model` | `Secret`, `Migration` — no BNPL business models yet |
| `repo` | Port interfaces: Pool, Conn, Tx, Schema, Migrations |
| `cerr` | HTTP status–coupled error wrappers |
| `usecase/migrationsuc` | Schema bootstrap and migration orchestration |

**Boundary:** No imports of `pgx`, `gin`, `cobra`, or YAML.

### Adapter (`pkg/adapter`, `cmd/bnpl`)

| Package | Role |
| --- | --- |
| `config` | YAML load, env expansion, dependency factories |
| `restful/gin` | Gin wrapper, middleware |
| `restful/gin/routes` | Route registration (placeholder) |
| `restful/gin/serdser` | Binding and JSON error helpers |
| `cmd/bnpl/command` | CLI wiring |

### Infrastructure (`pkg/adapter/db/postgres`)

| Package | Role |
| --- | --- |
| `postgres` | `pgxpool` adapters for Pool, Conn, Tx |
| `schemarp` | Schema/role DDL |
| `migrationsrp` | Embedded bootstrap SQL, migration SQL |

## Dependency Direction

```
cmd/bnpl/command  →  pkg/adapter/config  →  pkg/core/usecase
                                              ↓
                                        pkg/core/repo  ←  pkg/adapter/db/postgres
```

No reverse dependencies from core into adapters.

## Request Flow (HTTP)

Current flow only — no business handlers:

1. `startWebServer` loads config (`config.Load`)
2. Opens normal-role pool (`ConnectionPool(NormalRole)`)
3. Builds Gin engine (`config.Gin.NewEngine`)
4. `routes.Register` creates `/api/v1` group — **no handlers attached**
5. Gin listens on `gin.host:gin.port`

The pool is opened but unused by routes in the current implementation.

## Database Flow

### Runtime (intended for future handlers)

1. Normal-role pool from config
2. Acquire connection via `Pool.Conn`
3. Execute SQL through `Conn` or `Tx`

### Bootstrap (`init-dev` / `init-prod`)

1. Admin pool → transaction → drop/create schema, role, privileges
2. Normal pool → transaction → embedded DDL + seed SQL

Both paths are **destructive**. See [database.md](database.md) and ADR [0008](adr/0008-destructive-database-initialization-strategy.md).

### Migrations (`db migrate`)

1. Read files from configured directory (currently empty path / missing directory)
2. Per file: transaction → execute SQL → insert into `migrations` table

Bootstrap and migrations are separate mechanisms.

## Configuration Flow

1. Resolve path: `--config` → `CONFIG_FILE` → `configs/config.yaml`
2. Read YAML, `os.ExpandEnv`, decode to `config.Config`
3. Factories build pools, schema repos, Gin engine, use cases

Known wiring gaps:

- `gin.trusted_proxies` in YAML — not decoded; Gin hardcodes `127.0.0.1`
- `usecases.migration.files-dir` — not decoded into `Migrations.migDirPath`
- `versions` in YAML — ignored

Details: [configuration.md](configuration.md)

## How Infrastructure Supports Future BNPL Domains

| Today's decision | Future benefit |
| --- | --- |
| Repository ports | Swap PostgreSQL, mock repos in unit tests for credit rules |
| Transaction callbacks | Atomic contract + installment creation |
| Layered packages | Add `usecase/contractuc` without touching `pgx` |
| Embedded + filesystem SQL split | Bootstrap for greenfield; migrations for production evolution |
| `model.Secret` | Pattern for gateway keys and webhook secrets |
| `cerr` + `serdser` | Consistent HTTP errors for merchant APIs |
| Single binary | Ops commands for collections batch jobs alongside HTTP |

## Open Gaps

### HTTP

- [ ] Register health/readiness endpoints
- [ ] Implement `/api/v1` handlers
- [ ] Wire authentication middleware
- [ ] Use database pool in route handlers

### Business domains

- [ ] Define core models and repository ports for Contract, Decision, CreditLimit
- [ ] Implement use cases and PostgreSQL repositories
- [ ] Add API schemas and validation

### Database

- [ ] Wire `MIGRATIONS_FILES_DIR` from YAML
- [ ] Create `assets/db/migrations` with initial forward migrations
- [ ] Populate dev/prod seed SQL or document intentional emptiness
- [ ] Production-safe, non-destructive migration workflow
- [ ] Reduce admin credential use to dedicated jobs

### Configuration

- [ ] Decode `trusted_proxies` from YAML
- [ ] Post-load validation for required fields
- [ ] Remove or gate config debug printing in CLI

### Integrations

- [ ] Kafka producer/consumer (env vars exist, code absent)
- [ ] Redis (not referenced in Go code)
- [ ] Structured logging (`LOG_LEVEL` unused)

### Observability

- [ ] Metrics (request latency, pool stats, migration duration)
- [ ] Distributed tracing
- [ ] Audit event persistence

### Security

- [ ] AuthN/AuthZ for APIs
- [ ] Secret manager integration
- [ ] Audit log for sensitive operations

See [security.md](security.md) for current secret handling limits.

## Architecture Decision Records

| ADR | Topic |
| --- | --- |
| [0001](adr/0001-layered-architecture.md) | Layered architecture |
| [0002](adr/0002-repository-ports.md) | Repository ports |
| [0003](adr/0003-env-expanded-yaml-config.md) | Env-expanded YAML |
| [0004](adr/0004-embedded-schema-bootstrap.md) | Embedded bootstrap SQL |
| [0005](adr/0005-single-binary-cobra-commands.md) | Single binary / Cobra |
| [0006](adr/0006-docker-entrypoint-startup-flow.md) | Docker entrypoint |
| [0007](adr/0007-secret-handling-strategy.md) | Secret handling |
| [0008](adr/0008-destructive-database-initialization-strategy.md) | Destructive init |

## Package Map

| Path | Responsibility |
| --- | --- |
| `cmd/bnpl/main.go` | Delegates to `command.Execute()` |
| `cmd/bnpl/command/` | Cobra tree, server and DB commands |
| `pkg/adapter/config/` | Config load and wiring |
| `pkg/adapter/restful/gin/` | Gin adapter |
| `pkg/adapter/restful/gin/routes/` | Route registration |
| `pkg/adapter/restful/gin/serdser/` | HTTP bind/error helpers |
| `pkg/core/model/` | Shared structs |
| `pkg/core/repo/` | Port interfaces |
| `pkg/core/cerr/` | Error wrappers |
| `pkg/core/usecase/migrationsuc/` | DB init and migrate orchestration |
| `pkg/adapter/db/postgres/` | pgx adapters |
| `pkg/adapter/db/postgres/schemarp/` | Schema admin SQL |
| `pkg/adapter/db/postgres/migrationsrp/` | Bootstrap embed + migration repo |
