# Database

## Overview

PostgreSQL is accessed through `pgx` / `pgxpool`. Responsibilities are split across:

| Component | Location |
| --- | --- |
| Pool, connection, transaction adapters | `pkg/adapter/db/postgres` |
| Schema administration | `pkg/adapter/db/postgres/schemarp` |
| Bootstrap embed + migration SQL | `pkg/adapter/db/postgres/migrationsrp` |
| Integration outbox repository | `pkg/adapter/db/postgres/outboxrp` |
| Health ping repository | `pkg/adapter/db/postgres/healthrp` |
| Orchestration | `pkg/core/usecase/migrationsuc`, `outboxpublisheruc`, `healthcheckuc` |

## Managed Schema

Fixed schema name: `bnpl` (`migrationsuc.SchemaName()`).

All application objects are expected to live in this schema. Bootstrap sets `search_path` for role `bnpl`.

## PostgreSQL Roles

| Role | Constant | Purpose |
| --- | --- | --- |
| `admin` | `repo.AdminRole` | Drop/create schema, role DDL, grants |
| `bnpl` | `repo.NormalRole` | Runtime queries, bootstrap SQL, migrations, outbox |

### Admin role usage

Used exclusively during **destructive initialization**:

- `DROP SCHEMA IF EXISTS bnpl CASCADE`
- `CREATE SCHEMA bnpl`
- `CREATE ROLE bnpl` (if missing)
- `GRANT ALL ON SCHEMA bnpl TO bnpl`
- `ALTER ROLE bnpl SET search_path = bnpl`

**Operational risk:** Admin credentials are powerful. Production runtime pods should not require admin access once schema exists.

### Application role usage

- HTTP server connection pool
- Embedded bootstrap SQL execution
- Filesystem migration execution (`db migrate`)
- Outbox claim/publish/update operations

## pgpass Usage

The application does **not** read a `.pgpass` file. Credentials are pgpass-**format lines** in configuration:

- `POSTGRES_ADMIN_PGPASS`
- `POSTGRES_NORMAL_PGPASS`

Format: `host:port:database:user:password`

Matching logic in `Database.GetRolePassword` requires exact alignment with `database.host`, `database.port`, `database.name`, and role name.

See [security.md](security.md) and ADR [0007](adr/0007-secret-handling-strategy.md).

## Bootstrap vs Migrations

These are **separate concerns**:

| Aspect | Bootstrap (`init-dev`) | Bootstrap (`init-prod`) | Migrations (`db migrate`) |
| --- | --- | --- | --- |
| Trigger | CLI or Docker entrypoint | CLI | CLI |
| Required env | `APP_ENVIRONMENT=development` | `APP_ENVIRONMENT=production` | any |
| SQL source | `go:embed` in binary | `go:embed` in binary | Files on disk |
| Destructive | **Yes** — drops schema | **No** — create-only when missing | **No** — applies incremental SQL |
| If schema exists | Recreate | **Conflict error** (use migrate) | Apply pending files |
| Tracks versions | No (recreates from scratch) | No | Yes — `migrations` table |
| Production safe | **No** | Only for first-time create | Intended evolution path |

## Schema Initialization Process

Implemented by `InitDBUseCase` (`initdb.go`).

### Phase 1: Administrative bootstrap (destructive)

1. Open admin-role pool
2. Transaction:
   - Drop schema `bnpl` if exists (**CASCADE**)
   - Create schema `bnpl`
   - Ensure role `bnpl` exists with configured password
   - Grant privileges; set default `search_path`
3. Commit

### Phase 2: Schema content

1. Open normal-role pool
2. Transaction:
   - Execute embedded `000_schema.sql`
   - Execute `000_dev.sql` or `000_prod.sql`
3. Commit

## Embedded Bootstrap SQL

Embedded in `pkg/adapter/db/postgres/migrationsrp/settler.go` via `go:embed`.

| File | Current contents |
| --- | --- |
| `000_schema.sql` | `migrations`, `integration_outbox`, `integration_inbox`, `payment_attempts`, BNPL domain tables + indexes |
| `000_dev.sql` | Development seed data including physical/online merchant selling points and source mappings |
| `000_prod.sql` | Empty operational seed; production merchant/source mappings are managed as business data |

`assets/db/init/` contains symlinks to these files for reference; runtime uses embedded copies only.

`init-dev` includes deterministic demo data; `init-prod` creates the same schema without development business data.

## Merchant selling points and purchase sources

`merchant_profiles` represents the merchant business identity and is intentionally channel-neutral. A merchant may operate only physical locations, only online storefronts, or both.

`merchant_branch_profiles` represents a merchant selling point. `branch_type` is either `physical` or `online`; the branch-level funding, settlement, currency, risk, amount, and tenure overrides apply uniformly to both kinds of selling point.

Purchase-source identifiers are normalized into dedicated mapping tables:

| Table | Purpose | Resolution rule |
| --- | --- | --- |
| `merchant_pos_terminals` | Maps a unique external `pos_gps_id` to a physical selling point | Enabled mapping + `branch_type=physical` |
| `merchant_online_stores` | Maps a unique external `online_store_id` to an online selling point | Enabled mapping + `branch_type=online` |

This keeps a POS terminal or online storefront separate from branch policy. One physical selling point may own multiple POS terminals, and a merchant may expose multiple online selling points/storefronts. An online-only merchant simply has no physical selling point.

`checkout_decision_allocations` snapshots `purchase_channel`, `purchase_source_id`, and optional `merchant_order_id` together with the resolved merchant/branch allocation. Changing a POS/store mapping later therefore does not reinterpret an already-created Decision.

## Integration Outbox

Table: `bnpl.integration_outbox` (created by `000_schema.sql`).

| Column | Purpose |
| --- | --- |
| `id` | UUID primary key |
| `destination` | Publish topic / logical destination (e.g. Kafka topic) |
| `message_key` | Kafka partition key (Financial Core `financial_payment_intent_id` for payment-application events) |
| `event_type` | Machine-readable event name |
| `aggregate_type` | Optional entity type for traceability |
| `payload` | JSONB event body |
| `status` | `pending`, `processing`, `published`, `failed` |
| `attempt_count`, `max_attempts` | Retry tracking |
| `next_attempt_at` | Scheduled retry time |
| `locked_at`, `locked_by` | Worker claim metadata |
| `last_error` | Last delivery failure |
| `created_at`, `published_at` | Timestamps |

### Repository (`outboxrp`)

Port: `pkg/core/repo/outbox.go`

Operations:

- `ClaimPending` — `FOR UPDATE SKIP LOCKED` batch claim for a worker
- `MarkPublished`, `MarkFailed`, `ScheduleRetry` — post-publish state updates

### Use case (`outboxpublisheruc`)

`PublishDue`:

1. Claims due pending rows up to configured batch size
2. Publishes each via `messaging.Publisher` using `message_key` as the Kafka key (preserves ordering per payment intent)
3. Marks published or schedules exponential backoff retry

Delivery is **at-least-once**. Financial Core consumers must deduplicate `bnpl.payment_application_result.v1` by outbound `event_id`. The publisher never regenerates event IDs on retry.

Domain use cases insert outbox rows through `repo.CreateOutboxMessage` in the same transaction as inbox and domain state. Only `financialcorepaymentresultuc` builds payment-application acknowledgement payloads.

## Integration Inbox

Table: `bnpl.integration_inbox` (created by `000_schema.sql`).

Durable at-least-once processing of inbound Financial Core events. Independent from `payment_attempts`.

| Column | Purpose |
| --- | --- |
| `event_id` | Unique externally supplied event identifier |
| `event_type`, `event_version`, `source_service` | Event metadata |
| `correlation_id`, `causation_id` | Tracing |
| `payload` | Original JSON payload retained |
| `payload_fingerprint` | Self-describing fingerprint (`sha256:<64-hex>`) of normalized canonical business fields (not raw JSON formatting). **Deployment note:** pre-production databases storing legacy raw digests (without `sha256:` prefix) must be reset or explicitly rewritten before replaying old integration events — old and new formats are not treated as equivalent. |
| `consumer_name` | Integration consumer that owns the row (`financial_core_payment_result`) |
| `subject_type`, `subject_id` | Primary BNPL subject (`contract`, `installment`, `payment_attempt`) |
| `external_reference_type`, `external_reference` | External correlation (`financial_payment_intent`) |
| `status` | `received`, `processed`, `reconciliation_required`, `failed` |
| `received_at`, `processed_at`, `failed_at` | Processing timestamps |
| `failure_code`, `failure_message` | Failure details |

### Inbox status semantics

| Status | Meaning |
| --- | --- |
| `received` | Event is inside an active processing transaction or was durably accepted but not completed |
| `processed` | Successfully applied or proven idempotent replay — never rerun normal processing |
| `reconciliation_required` | Durable record that cannot be applied automatically — never rerun normal processing |
| `failed` | Structurally invalid or permanently unprocessable trusted event — not automatically retried |

Infrastructure failures (DB unavailable, serialization/outbox insert failure, lock timeout) roll back the transaction and remain Kafka-retryable. They must not be committed as durable `failed` business events.

Unique constraint on `(source_service, event_id)` prevents double application. Kafka consumers acknowledge only after inbox (and domain) state commits.

See ADR [0016](adr/0016-bnpl-payment-attempt-ownership.md).

## Integration Inbox Conflicts

Table: `bnpl.integration_inbox_conflicts` (created by `000_schema.sql`).

When the same `(source_service, event_id)` is redelivered with a different canonical payload fingerprint, the original inbox row is **not** mutated. A conflict row is inserted instead:

| Column | Purpose |
| --- | --- |
| `inbox_id` | Reference to the original inbox row |
| `existing_payload_fingerprint` | Fingerprint stored on the inbox row |
| `incoming_payload_fingerprint` | Fingerprint of the conflicting redelivery |
| `incoming_payload` | Conflicting JSON retained for audit |
| `failure_code` | `event_identity_payload_mismatch` |

Unique on `(source_service, event_id, incoming_payload_fingerprint)` makes repeated conflicting redeliveries idempotent.

## Payment Attempts (BNPL-local)

Table: `bnpl.payment_attempts` (created by `000_schema.sql`).

Local repayment orchestration for installments 2+. **Not** Financial Core provider payment attempts.

| Constraint / index | Purpose |
| --- | --- |
| `UNIQUE (installment_id, idempotency_key)` | Client idempotency |
| Partial unique on `financial_payment_intent_id` / `financial_payment_attempt_id` | Financial Core correlation (`financial_payment_attempt_id` is `NULL` when provider attempt is unknown — empty strings are not stored) |
| `contracts.successful_financial_payment_attempt_id` | Final successful Financial Core provider attempt after checkout activation (session attempt remains in `financial_payment_attempt_id`) |
| Partial unique `WHERE state = 'succeeded'` on `installment_id` | At most one applied success per installment |

States: `created`, `submitted`, `succeeded`, `failed`, `canceled`, `reconciliation_required`.

## Payment Attempt Items

Table: `bnpl.payment_attempt_items` (created by `000_schema.sql`).

**Child rows of the `PaymentAttempt` aggregate** — not an independent aggregate. Snapshotted payable obligation lines captured atomically when a repayment attempt starts (`CreateWithItems`). Used for payment-result validation and charge settlement instead of live charge recalculation.

| Column | Purpose |
| --- | --- |
| `payment_attempt_id` | Parent attempt |
| `item_type` | `principal` or `charge` |
| `source_id` | Installment id (principal) or charge id at snapshot time |
| `amount_minor`, `effect` | Debit/credit line amounts |

Principal items use `installment.principal_amount_minor` only — fees, penalties, discounts, and waivers are separate charge items. Only **customer-payable** charges are included; merchant/platform payer charges are excluded. Attempt total = sum(debit items) − sum(credit items), verified against `payment_attempts.amount_minor` before persist.

On successful payment application, only **snapshotted customer debit charge** items are marked paid (by charge source IDs). Credit items and post-snapshot charges are never settled through this path.

## init-dev Behavior

Command: `bnpl db init-dev`

Requires `runtime.environment=development` (`APP_ENVIRONMENT=development`).

1. **Destroys** managed schema `bnpl` and all contained objects (even if empty)
2. Recreates schema and role privileges
3. Applies embedded DDL and dev seed SQL

**Use for:** local development resets, CI, greenfield development environments.

**Do not use for:** production databases.

## init-prod Behavior

Command: `bnpl db init-prod`

Requires `runtime.environment=production` (`APP_ENVIRONMENT=production`).

1. If schema `bnpl` **already exists** → fail with conflict; do **not** drop data; use `db migrate`
2. If schema `bnpl` is **missing** → create schema and role privileges, then apply embedded DDL and prod seed SQL

**Use for:** one-time greenfield production bootstrap only.

See ADR [0008](adr/0008-destructive-database-initialization-strategy.md).

## Migration Lifecycle

Separate from bootstrap. Implemented in `migrationsuc.UseCase`.

Per migration file:

1. Read file from `MIGRATIONS_FILES_DIR` (wired from YAML `usecases.migration.files-dir`)
2. Normal-role connection → transaction
3. Execute SQL
4. `INSERT INTO migrations(name, created_at)`
5. Commit

Execution and registration share one transaction — avoids applied-but-untracked migrations.

### Current migration directory state

- Default path: `assets/db/migrations` (from `.env.example`)
- Directory may not exist until first migration is added
- `db migrate` fails when the directory is missing or empty

## Schema Versioning

| Mechanism | Status |
| --- | --- |
| `migrations` table | Created by bootstrap DDL |
| `versions.database` in YAML | Validated at config load against binary 0.2.0 |
| Semantic migration file naming | Convention only; not enforced |

## Connection Management

Pool settings from config map to `pgxpool.Config`:

- `MaxConns`, `MinConns`
- `MaxConnLifetime`, `MaxConnIdleTime`
- `HealthCheckPeriod`, `ConnectTimeout`
- `ssl_mode` from `POSTGRES_SSL_MODE`

`postgres.NewPool` acquires a connection immediately after creation (fail-fast).

Commands defer `pool.Close()` after use.

## Transaction Boundaries

Callback pattern: `Conn.Tx(ctx, handler)`

- `nil` return → commit
- non-`nil` error → rollback
- panic → rollback, error returned

Used for bootstrap phases, per-migration execution, and outbox publish transactions.

## Operational Risks

| Risk | Mitigation (current) | Target state |
| --- | --- | --- |
| Data loss on `init-*` | Documentation, ADR 0008 | Non-destructive `db migrate` only in prod |
| Docker entrypoint starts HTTP only | Schema bootstrap via `make init-dev` / `bnpl db migrate` | Migrate-on-deploy job |
| Admin creds in app config | Role separation at runtime | Admin only in migration job |
| Password in DDL logs | Restrict PostgreSQL `log_statement` | Parameterized role management alternative |
| Outbox without producers | Table only; publisher idle | Domain inserts in same TX as business writes |
| No backup/restore docs in repo | Operator responsibility | Document RPO/RTO with BNPL domains |

## TODO

- [ ] Add `assets/db/migrations` with first forward migration
- [ ] Add `db migrate` dry-run or status command
- [ ] Document PostgreSQL backup requirements for production
- [ ] Domain use case to insert outbox rows transactionally
- [ ] Add dev/prod seed content or document intentional absence

## Related documentation

- [architecture.md](architecture.md) — database and outbox flow
- [operations.md](operations.md) — Makefile and Docker commands
- [security.md](security.md) — credential handling
- ADR [0012](adr/0012-transactional-outbox-publisher.md)
