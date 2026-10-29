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

Starts via `docker-compose.local.yml`:

- PostgreSQL — `127.0.0.1:5432`
- Kafka — `127.0.0.1:9092` (only required when `MESSAGING_PROVIDER=kafka`)

Redis is **not** included in Compose. Start Redis separately or use `CACHE_PROVIDER=noop` (default).

### 3. Build

```bash
make build
# Output: build/bin/bnpl
```

### 4. Database bootstrap

```bash
make init-dev    # requires APP_ENVIRONMENT=development; always recreates schema bnpl
# or
make init-prod   # requires APP_ENVIRONMENT=production; creates only if schema bnpl is missing
```

`init-dev` drops and recreates schema `bnpl`. `init-prod` refuses when `bnpl` already exists (use `make migrate`). See [database.md](database.md).

### 5. Run application

```bash
make run
```

Runs `scripts/entrypoint.sh`, which starts the BNPL process (`exec bnpl`).
It does **not** touch the database. Bootstrap/reset is only via `make init-dev`
(or `make init-prod` / `bnpl db migrate`).

## Makefile Targets

### Build / run

| Target | Action                           |
| --- |----------------------------------|
| `build` | Compile to `build/bin/bnpl`      |
| `run` | `build` + `up` + `entrypoint.sh` (starts app only) |
| `init-dev` | `bnpl db init-dev`               |
| `init-prod` | `bnpl db init-prod`              |
| `clean` | Remove binary and `coverage.out` |

### Test / quality

| Target | Action |
| --- | --- |
| `test` | `go test ./...` |
| `test-integration` | Integration tests with an automatic disposable PostgreSQL 18 container; never uses `bnpl-db` |
| `test-coverage` | Coverage report |
| `lint` | golangci-lint |
| `staticcheck` | staticcheck |
| `revive` | revive |
| `tidy-imports` | goimports |

### Docker / infrastructure

| Target | Action |
| --- | --- |
| `up` / `down` | Compose start/stop |
| `build-image` | `docker build -t bnpl-backend:v0.1.0 .` |
| `run-image` | Run container with default entrypoint |
| `list-images` | List local `bnpl-backend` images |
| `publish-image-to-hamravesh` | Tag and push to Hamravesh registry |
| `clean-db` | Stop DB container and remove volume |

### Setup

| Target | Action |
| --- | --- |
| `mkdir-pgpass` | Create `dist/.db/.pgpass` (not used by Go app) |
| `env` | Print exportable `.env` lines |
| `install-tools` | Install lint and codegen tools |

## CLI Commands

```bash
./build/bin/bnpl [--config path]              # HTTP server + infrastructure
./build/bin/bnpl db init-dev                  # destructive development recreate
./build/bin/bnpl db init-prod                 # one-time production create (refuses if exists)
./build/bin/bnpl db migrate                   # filesystem migrations
./build/bin/bnpl policy-assignment import-trusted \
  --file assets/csv/trusted-subjects/trusted-subjects.csv --actor ops@example
# or: make policy-assignment-import-trusted
# optional overrides: POLICY_ASSIGNMENT_FILE=... POLICY_ASSIGNMENT_ACTOR=...
```

### Trusted customer policy import

`policy-assignment import-trusted` is an operational bulk command for assigning
the code-owned trusted customer policy bundle (`checkout/v2` and
`initial-credit-limit/v2`). Checkout v2 is full-financing with **3** installments
due at one, two, and three Jalali months after purchase. The CSV contains stable
**subject IDs** and is the source of truth for trusted membership. BNPL does not
require or persist national IDs for this workflow.

Shortcut: `make policy-assignment-import-trusted` (defaults:
`POLICY_ASSIGNMENT_FILE=assets/csv/trusted-subjects/trusted-subjects.csv`,
`POLICY_ASSIGNMENT_ACTOR=ops@example`).

The CSV format is intentionally strict and contains exactly one column:

```csv
subject_id
550e8400-e29b-41d4-a716-446655440000
92d2c7b2-6f8b-4f25-8bb2-4deab49af151
```

The CSV is the authoritative source of trusted membership. BNPL does **not**
call Identity Service during this import. Each structurally valid `subject_id`
is processed independently and receives the whole trusted policy bundle
atomically. A persistence failure for one subject is recorded as a durable
failed import item and does **not** roll back successful subjects from the same
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

## Docker Image

Built from multi-stage `Dockerfile`:

1. **Builder:** Go 1.26.2 Alpine, static binary
2. **Runtime:** Alpine, non-root user `bnpluser`, timezone Asia/Tehran

Copied artifacts:

- Binary as `./bnpl`
- `configs/` directory
- `scripts/entrypoint.sh`

Environment:

- `CONFIG_FILE=configs/config.yaml`
- `APP_EXECUTABLE` (path to binary; see entrypoint)

### Entrypoint behavior

`scripts/entrypoint.sh` only starts the BNPL process:

| Step | Behavior |
| --- | --- |
| 1 | Log startup banner |
| 2 | `exec bnpl` — HTTP server + tasks as PID 1 |

Database bootstrap is **not** part of the entrypoint. Use explicit commands:

```bash
make init-dev     # development reset
make init-prod    # production create-if-missing
bnpl db migrate   # incremental migrations
```

See ADR [0006](adr/0006-docker-entrypoint-startup-flow.md).

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

### GitHub Actions

| Workflow | Purpose |
| --- | --- |
| `.github/workflows/ci.yml` | fmt, vet, lint, tests, gosec, Trivy, Docker build |
| `.github/workflows/cd.yml` | Push images to GHCR and Hamravesh; deploy job via SSH |

CD deploy job:

- Checks out `scripts/deploy.sh` and runs on production host
- **`scripts/deploy.sh` is not in this repository** — deploy automation is incomplete

Images tagged with immutable git SHA; `latest` is for reference only.

### GitLab CI

`.gitlab-ci.yml`: test, build, docker push on `main`. Helm and deploy stages commented out.

### Hamravesh registry

```bash
make publish-image-to-hamravesh
```

Uses `HAMRAVESH_CONTAINER_REGISTRY` and `HAMRAVESH_REGISTRY_NAMESPACE` from Makefile.

## Limitations

- Schema bootstrap is explicit (`make init-dev` / `make init-prod` / `bnpl db migrate`); entrypoint never mutates the database
- No `deploy.sh` in repo despite CD reference
- `assets/db/migrations` directory may be missing until first migration is added
- Kafka subscriber processes Financial Core payment results via `financialcorepaymentresultuc`; payment-application results are published through transactional outbox
- No automated rollback documented
- **Not production-ready** — infrastructure is foundational only

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
- Use `init-dev` for development greenfield/reset only; use `init-prod` once for missing production schema

### HTTP server starts but no API routes

- Expected — see [api.md](api.md)

### Container wipes database on restart

- Entrypoint no longer runs DB init; if schema disappears, look for an external Job/`make init-dev` or volume loss
- Prefer `make init-dev` only when you intentionally want a local reset

### Outbox publisher logs errors with noop messaging

- Expected when rows exist and `MESSAGING_PROVIDER=noop` — messages are accepted locally but not delivered to a broker

## TODO

- [ ] Add `scripts/deploy.sh` or remove CD deploy step
- [ ] Document PostgreSQL and backup procedures for production
- [ ] Add Redis to local Compose when cache integration tests are needed

## Related documentation

- [database.md](database.md)
- [configuration.md](configuration.md)
- [security.md](security.md)
