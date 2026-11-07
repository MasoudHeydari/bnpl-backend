# Operations

## Prerequisites

| Tool | Version / notes |
| --- | --- |
| Go | 1.26.2 (`go.mod`) |
| Docker + Compose | Local PostgreSQL and optional Kafka/Redis |
| PostgreSQL | Required for HTTP server and DB commands |

Optional dev tools (Makefile `install-tools`):

- golangci-lint v2.12.2
- staticcheck, revive, goimports
- mockery, protoc (proto dir may be empty)

## Local Development

### 1. Environment

```bash
cp .env.example .env
# Edit POSTGRES_* and GIN_* as needed
```

For PostgreSQL-only local runs, keep noop providers (defaults in `.env.example`):

```bash
CACHE_PROVIDER=noop
MESSAGING_PROVIDER=noop
```

### 2. Infrastructure

```bash
make up
```

Starts the full local stack via `docker-compose.local.yml` (creates the Grafana
password secret and Promtail log dirs when missing):

- PostgreSQL / Kafka / Temporal / Redpanda / Loki / Promtail / Grafana published on
  `COMPOSE_PUBLISH_HOST` from `.env` (default in `.env.example`: `192.168.1.104`)
- Envoy uses host networking (see envoy config); BNPL itself still binds via `GIN_HOST`

Redis is **not** included in Compose. Start Redis separately or use `CACHE_PROVIDER=noop` (default).

### 3. Build

```bash
make build
# Output: build/bin/bnpl
```

### 4. Database bootstrap

Schema initialization is handled by the **separate DB Admin binary**, not
the Runtime server. Use the corresponding development or production Make
targets, for example:

```bash
make db-init-development
# production on an empty schema only, through a separately authorized DB Admin job:
make db-init-production
```

Development initialization is destructive. Production initialization refuses
an existing managed schema. See
[database deployment](architecture/database-deployment.md).

### 5. Run application

```bash
make run
```

The development Runtime starts with its application configuration and does
not initialize or migrate the database. `cmd/bnpl-dbadmin` owns those
operations.

## Makefile Targets

Key targets (verify exact optional flags in the repository Makefile):

| Target | Action |
| --- | --- |
| `make build-development` / `make build-production` | Build Runtime with matching build profile |
| `make build-dbadmin-development` / `make build-dbadmin-production` | Build privileged DB Admin separately |
| `make run` | Run local development Runtime |
| `make db-init-development` / `make db-init-production` | Initialize the managed schema with DB Admin |
| `make db-migrate-development` / `make db-migrate-production` | Apply forward migrations through DB Admin |
| `make test` | Unit tests with development tags |
| `make test-integration` | Isolated PostgreSQL integration tests |
| `make test-all` | Unit and integration tests |
| `make test-e2e` | Separate end-to-end suite, not part of `test-all` |
| `make db-invariants-verify-production` | Verify DB financial invariants against deployment |
| `make db-privileges-verify-production` | Verify restricted DB privileges |

Runtime serves HTTP and business background work. Database administrative
operations must run with separate credentials and their own deployment
permissions.

### Trusted customer policy import

`policy-assignment import-trusted` is an operational bulk command for assigning
the code-owned trusted customer policy bundle (`checkout/v2` and
`initial-credit-limit/v2`). Checkout v2 is full-financing with **3** installments
due at one, two, and three Jalali months after purchase. The CSV contains stable
**external customer IDs** (Customer Reference Service UUIDs) and is the source of truth for trusted membership. BNPL does not
require or persist national IDs for this workflow.

Shortcut: `make policy-assignment-import-trusted` (defaults:
`POLICY_ASSIGNMENT_FILE=assets/csv/trusted-customers/trusted-customers.csv`,
`POLICY_ASSIGNMENT_ACTOR=ops@example`).

The CSV format is intentionally strict and contains exactly one column:

```csv
external_customer_id
550e8400-e29b-41d4-a716-446655440000
92d2c7b2-6f8b-4f25-8bb2-4deab49af151
```

The CSV is the authoritative source of trusted membership. BNPL does **not**
call Customer Reference Service during this import. Each valid `external_customer_id`
is processed independently and receives the whole trusted policy bundle
atomically. A persistence failure for one customer is recorded as a durable
failed import item and does **not** roll back successful customers from the same
file. Import batches and row outcomes are persisted in
`policy_assignment_imports` and `policy_assignment_import_items`.

The default HTTP server handles `SIGINT` and `SIGTERM` with graceful shutdown (see below).

## Graceful Shutdown

On `SIGINT` or `SIGTERM`:

1. Readiness fails immediately (`LifecycleState.StartDraining`)
2. Bounded shutdown runs under one budget (`SHUTDOWN_TIMEOUT`, default 30s)
3. `SHUTDOWN_DRAIN_DELAY` waits for load-balancer / kube-proxy propagation
4. HTTP (then metrics) servers stop accepting and finish in-flight work
5. Task scheduler and message consumer stop (runtime context canceled)
6. Outbound HTTP client, publisher, subscriber, messaging health, cache, external clients, and database pool close (database last)
7. If shutdown exceeds timeout, liveness fails (`MarkShutdownTimedOut`)

HTTP listen bind happens before background workers start so a busy port fails without leaving workers running.

## Kubernetes Health Probes

When `HEALTH_ENABLED=true` (default):

| Probe | Path | Purpose |
| --- | --- | --- |
| Liveness | `GET /healthz/live` | Process alive; ignores external dependencies |
| Readiness | `GET /healthz/ready` | Safe to receive traffic; checks draining + configured dependencies |

Example probe configuration:

```yaml
livenessProbe:
  httpGet:
    path: /healthz/live
    port: 8000
  initialDelaySeconds: 5
  periodSeconds: 10
readinessProbe:
  httpGet:
    path: /healthz/ready
    port: 8000
  initialDelaySeconds: 5
  periodSeconds: 5
```

Readiness includes checks listed in `infrastructure.health.checks` (default: database, cache, messaging). With `CACHE_PROVIDER=noop` and `MESSAGING_PROVIDER=noop`, cache and messaging checks still run but succeed against noop adapters.

## Docker Images and Runtime Entrypoint

Runtime and DB Admin are built as distinct development/production images.
Runtime contains the business binary and `configs/runtime.yaml`, not DB
administrator credentials or DDL commands. DB Admin contains only the
administrative entrypoint and `configs/dbadmin.yaml`.

The Runtime entrypoint starts `bnpl` as PID 1. It does not create, migrate,
drop or reset the database. See
[database deployment](architecture/database-deployment.md) for documented
build targets, release images, and production initialization steps.

## Provider Requirements by Environment

| Provider setting | External dependency | When required |
| --- | --- | --- |
| `CACHE_PROVIDER=noop` | None | Default local setup |
| `CACHE_PROVIDER=redis` | Redis at `CACHE_REDIS_ADDR` | Startup fails on failed `PING` |
| `MESSAGING_PROVIDER=noop` | None | Default local setup |
| `MESSAGING_PROVIDER=kafka` | Kafka at `KAFKA_BROKER_1` | Lazy connect; publish/consume fails without broker |
| `TASK_PROVIDER=noop` | None | Disables scheduled jobs |
| `TASK_PROVIDER=inprocess` | None | Runs goroutine metrics, outbox publisher, contract/installment jobs when enabled |

## Financial Core payment results

When `MESSAGING_PROVIDER=kafka`, the consumer on `KAFKA_TOPIC_PAYMENT_RESULTS` decodes Financial Core payment-result events and delegates to `financialcorepaymentresultuc.Process` (all routing, purpose resolution, and unsupported-purpose handling occur inside the use case — the Kafka handler only decodes and maps transport fields).

Processing semantics:

1. Kafka adapter maps known external payment statuses to typed values; unknown raw statuses are preserved (`RawPaymentStatus`) and durably fail inbox **after acquisition** (`unsupported_payment_result_status`) — never `already_applied`, no outbound acknowledgement
2. Inbox fingerprint uses **normalized canonical business fields** including raw payment status for unknown values; same trimmed `(source_service, event_id)` with different fingerprint → conflict row
3. Same `(source_service, event_id)` with different fingerprint → `integration_inbox_conflicts` row; original terminal inbox row unchanged (no infinite retry loop)
4. Acquire `integration_inbox` row via `AcquireForProcessing` in the same transaction as domain updates (`financialcorepaymentresultuc`)
5. **Process results:** `payment_applied`, `already_applied`, `reconciliation_required`, `failure_recorded`, `cancellation_recorded`, `event_already_processed`, `terminal_result_unmatched`, `payload_conflict_recorded`, `permanently_rejected`

Kafka adapter validates only the minimum trusted envelope before calling `financialcorepaymentresultuc`. Payment business fields are validated after inbox acquisition.
6. Failed/canceled results mark inbox processed and local attempt terminal state — **no** `applied` outbox acknowledgement
7. Reconciliation-required outcomes are persisted (inbox + local `PaymentAttempt` when present + outbox acknowledgement where applicable) — **not** dropped as non-retryable before commit
8. Financial Core **`financial_payment_intent_id`** is primary correlation; provider attempt ID is recorded for audit but mismatch alone is not rejection
9. Duplicate success requires matching intent + attempt + amount + currency + installment — amount equality alone is not payment identity
10. Kafka offset is acknowledged only after successful commit (applied, duplicate processed, reconciliation persisted, conflict recorded, or durable permanent invalid event)
11. Infrastructure/transaction failures remain retryable (DB, serialization, outbox insert, lock timeout). Business reconciliation outcomes (`reconciliation_required`, `permanently_rejected`, `payload_conflict_recorded`, missing contract/installment on succeeded payment) commit durably and acknowledge Kafka — not retried indefinitely.
12. Installment `already_applied` is determined before current exposure/principal validation; equivalent replays do not fail because exposure was already reduced.
13. Checkout reconciliation uses `persistCheckoutPaymentReconciliation` (no payment-attempt repository); installment reconciliation uses `persistInstallmentPaymentReconciliation` (requires payment-attempt repository).
14. Optional Financial Core provider-attempt IDs persist as SQL `NULL`, not empty strings.

Outbox publisher task delivers business acknowledgements to `KAFKA_TOPIC_PAYMENT_APPLICATION_RESULTS` using `financial_payment_intent_id` as the Kafka message key.

**Monetary truth vs application truth:** Financial Core inbound events report whether money succeeded/failed/canceled. Outbound `bnpl.payment_application_result.v1` reports whether BNPL applied that successful payment to its obligation (`applied`, `already_applied`, `reconciliation_required`). Kafka offset commit is transport-level and distinct from the business acknowledgement.

**Exact replay vs new duplicate success:** `event_already_processed` is returned when the same inbox event is replayed. `already_applied` is returned only when a new inbound success is proven to match the same financial identity as an already-applied payment.

See ADR [0016](adr/0016-bnpl-payment-attempt-ownership.md).

## Graceful Shutdown Configuration

| Key | Default | Purpose |
| --- | --- | --- |
| `SHUTDOWN_TIMEOUT` | `30s` | Maximum time for full shutdown sequence |
| `SHUTDOWN_DRAIN_DELAY` | `5s` | Delay after readiness fails and before HTTP shutdown (LB propagation) |

## Docker Compose

File: `docker-compose.local.yml`

- App runs on **host** (`make run`), not in Compose
- DB credentials from `.env` (`POSTGRES_USER`, `POSTGRES_PASSWORD`)
- Application connects via `POSTGRES_HOST=127.0.0.1` and pgpass lines

## Deployment Workflow

The repository contains CI/CD configuration; do not assume it proves the
actual deployed gateway, Kubernetes network policies, or Financial Core
integration contracts.

For the first production release, require:

1. A **trusted-Gateway negative test**: direct untrusted access to BNPL must
   be denied. Envoy must validate JWTs, strip incoming `X-Auth-*` headers,
   and inject authenticated values. Confirm gateway-to-service source
   restrictions and transport protection.
2. An empty-schema install through the separate DB Admin Production job,
   followed by DB invariant and privilege verification. No Runtime admin
   secrets.
3. Kafka subscription validation: payment results and payment
   reconciliation results only. There is no installment
   payment-session-created consumer. Confirm task jobs are enabled where
   required for outbox publishing, expiry, reconciliation, and collections.
4. `make test-all` **and** `make test-e2e` with disposable infrastructure,
   plus controlled FC idempotency/expiry/result-replay integration tests.
5. A rollback/restore plan for the chosen deployment and verifiable health
   and alerting controls. Do not claim release readiness based solely on
   local test success.

Follow [security](security.md) and
[database deployment](architecture/database-deployment.md) for details.

## Limitations

The repository does not itself prove deployment-specific Envoy strip/JWT,
NetworkPolicy, workload identity, live Financial Core behavior, or external
payment-event delivery. These must be verified in the target environment.
Outbox is deliberately at-least-once and requires consumer idempotency.

## Troubleshooting

### `config.Load(...): reading config file`

- Verify `CONFIG_FILE` or pass `--config`
- Ensure `configs/config.yaml` exists (copied in Docker image)

### `no matching password line`

- Align pgpass lines with `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, role names

### Pool creation fails

- Run `make up`; check `docker compose ps`
- Verify PostgreSQL on `127.0.0.1:5432`

### `connect redis cache` on startup

- Set `CACHE_PROVIDER=noop` or start Redis at `CACHE_REDIS_ADDR`

### `db migrate` fails

- Ensure `MIGRATIONS_FILES_DIR` exists and contains SQL files
- Use DB Admin development init for disposable reset only and DB Admin production init for a new empty managed schema

### HTTP server starts but business APIs are inaccessible

- Check Gateway access, route registration and `AuthContext.Attach`; the `/api/v1` business routes are implemented. See [api.md](api.md)

### Container wipes database on restart

- Entrypoint no longer runs DB init; if schema disappears, look for an external Job/`make init-dev` or volume loss
- Prefer `make init-dev` only when you intentionally want a local reset

### Outbox publisher logs errors with noop messaging

- Expected when rows exist and `MESSAGING_PROVIDER=noop` — messages are accepted locally but not delivered to a broker

## Remaining Operational Work

- Confirm the actual CD deploy implementation against the chosen production environment.
- Document backup and rollback procedures for the production database and first deployment.
- Record evidence for trusted Gateway/NetworkPolicy, FC integration and consumer/topic wiring.

## Related documentation

- [database.md](database.md)
- [configuration.md](configuration.md)
- [security.md](security.md)
