# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed

- Reject checkout amounts outside PostgreSQL `BIGINT` / Financial Core `int64` range and use checked first-installment arithmetic.
- Include all checkout payment-session inputs in the Financial Core request fingerprint.
- Re-check persisted checkout payment sessions under row lock to prevent concurrent overwrite and detect conflicting provider results.
- Validate checkout return URLs and bound tenant, context, provider, and URL input lengths.
- Validate failed/canceled checkout events against the stored contract payment identity before marking inbox events processed.
- Treat invalid Gin trusted-proxy configuration as a startup error instead of silently ignoring it.

### Added

- External partner terminal authentication: `X-Terminal-Id` / `X-API-Key` (`PartnerTerminalAuth`) on `/api/v1/partner` checkout routes; merchant-panel terminal lifecycle (`GET`/`POST /api/v1/merchant/terminal`, credential rotate/retire) requires `bnpl:merchant-terminals:manage:merchant`, configured through `usecases.merchant_terminal`.
- Merchant `CheckoutSession` integration boundary with separate external/internal create contracts, trusted customer identity binding, pre-registered return destinations, merchant-safe status polling, and user/partner cancellation semantics.
- Temporal-backed task runtime with persistent Temporal Schedules and a generic Workflow/Activity bridge for existing `task.Job` handlers.
- Fixed `composite` task runtime that routes each enabled task to either `inprocess` or `temporal` via explicit execution metadata.
- Local Temporal development server in `docker-compose.local.yml`.
- `pkg/core/usecase/financialcorepaymentresultuc` — single owner of Financial Core payment-result inbox/outbox processing.
- `pkg/core/usecase/installmentofferuc`, `installmentlifecycleuc`, `installmentrepaymentuc` — split from monolithic `installmentuc`.
- `payment_attempt_items` table — child rows of `PaymentAttempt` aggregate (managed via `PaymentAttempt` repository, not a separate port)
- `integration_inbox_conflicts` table for payload identity mismatches on reused event IDs
- `ProcessResult` process-level enum in `financialcorepaymentresultuc` (separate from payment-application outbox acknowledgement)
- Canonical inbox fingerprinting from typed business fields in `financialcorepaymentresultuc`
- `pkg/core/identifier.Generator` for payment-application outbox event IDs (ULID adapter).
- `InboxEvent` / `AcquireForProcessing` generic inbox API with `payload_fingerprint` replay protection.
- `MarkCanceled` on `PaymentAttempt`; `MarkSnapshottedChargeIDsPaid` on installment charges.
- `ReduceActiveExposureIfSufficient` replaces clamping `SubtractActiveExposure`.

### Changed

- Partner integration docs live under `doc/integration/` only: `internal-partner-checkout.md` (Damz-style) and `external-partner-checkout.md` (Digikala/Technolife-style). Removed `doc/frontend/` and the older `partner-checkout-sessions` / `integrations/` layout.
- Task configuration schema bumped to `0.3.0`; standalone task providers own all jobs while `composite` requires per-job execution routing.
- Disabled task definitions are registered as metadata so durable providers can remove schedules left by previous deployments.
- Production fails closed on `infrastructure.task.provider=noop` during both configuration validation and task-runtime initialization; contract-cancellation Saga also keeps its stricter non-noop startup guard in every environment.
- Payment architecture refactor: inbox/outbox orchestration moved from `contractuc`/`installmentuc` to `financialcorepaymentresultuc`.
- `installmentuc` removed; offer/lifecycle/repayment split into dedicated use cases.
- `contractuc` no longer depends on inbox/outbox/idGenerator/payment-application destination.
- Kafka payment-result handler decodes and delegates to `financialcorepaymentresultuc.Process` only (routing inside use case).
- `integration_inbox` uses generic `subject_*` and `external_reference_*` columns plus `payload_fingerprint`.
- Payment attempt creation snapshots items atomically via `BuildPaymentAttemptItems` + `CreateWithItems`; principal uses `PrincipalAmountMinor`; customer-only payable charges with debit/credit semantics
- Later installment success validates snapshot integrity, settles only snapshotted customer debit charges, and verifies charge update counts
- Failed/canceled payment results return `failure_recorded` / `cancellation_recorded` — no payment-application outbox event
- Unsupported payment purpose durably fails inbox (`unsupported_payment_purpose`); empty purpose with `bnpl_contract` source treated as checkout (compatibility rule in use case)
- Inbox payload conflicts persist in `integration_inbox_conflicts` without mutating terminal inbox rows
- Bootstrap exposes `identifier.Generator` (not `trace.IDGenerator`) for payment integration event IDs
- Removed standalone `PaymentAttemptItem` repository port and `paymentattemptitemrp` adapter
- Regenerated mockery interfaces for payment attempt aggregate and inbox conflict APIs
- Outbound payment-application event envelope (`paymentapplication.Event` + `EventData`) with explicit version and producer
- `integration_outbox.message_key` for Kafka partition ordering (`financial_payment_intent_id` for payment-application events)
- `repo.CreateOutboxMessage` replaces positional outbox create signature
- Process results `event_already_processed` and `payload_conflict_recorded` distinct from payment-application `already_applied`
- Every trusted inbound **succeeded** payment with valid intent produces `applied`, `already_applied`, or `reconciliation_required` outbox acknowledgement atomically with domain state
- Validation-before-mutation for installment success; charge row locking via `LockByIDs`; no post-mutation reconciliation commits
- Idempotency fingerprint excludes mutable payable amount (stable client identity only)
- Kafka adapter validates only minimum envelope before inbox; business validation moved to `financialcorepaymentresultuc`
- Financial Core provider-attempt ID optional; payment intent remains primary correlation
- Checkout activation business conflicts (`Conflict`, `ErrPaymentRequiresReconciliation`) become durable reconciliation + outbound ack
- Outbound reconciliation events use nullable monetary metadata; `ValidateEvent` before outbox insert
- `BNPLAppliedAt` uses persisted application timestamps for `already_applied`
- Process result `terminal_result_unmatched` for failed/canceled events without matching local attempt
- Outbox create omits `max_attempts` insert (schema default applies)
- Installment repayment idempotency lookup occurs before mutable charge validation
- `pkg/core/fingerprint` port with SHA-256 adapter; persisted fingerprints use `sha256:<digest>` format
- Payment-application event contract moved from `pkg/core/integration/paymentapplication` to `pkg/core/event/paymentapplication`
- Checkout/installment unsupported payment statuses validated after inbox acquisition; durable inbox failure uses preserved raw status
- Integration identity normalized before inbox persistence and fingerprinting
- Envelope compatibility defaults isolated in Kafka adapter mapper
- Missing contract/installment on succeeded payment → reconciliation (not retryable not-found)
- Separate checkout vs installment reconciliation persistence (no nil payment-attempt repository)
- `MarkSucceeded` accepts nullable provider-attempt ID (`NULL` in PostgreSQL when absent)
- Installment `already_applied` determined before exposure/principal validation; explicit `MarkSucceeded` conflict handling
- `contractuc.ReconciliationError` replaces error-message substring classification
- Exact first-installment amount enforced in `contractuc` for all callers
- Intent-first checkout correlation; optional provider-attempt ID in activation input
- Fingerprint values validated before persistence; legacy raw dev fingerprints require DB reset/rewrite
- Reconciliation reasons `contract_not_found`, `installment_not_found` added
- Succeeded payment without `financial_payment_intent_id` → inbox permanent failure (`missing_payment_intent`, `permanently_rejected`); no payment-application outbox
- `MarkReconciliationRequired` transition conflicts classified from locked attempt state (not silently ignored)
- Installment `already_applied` requires paid installment and compatible contract state (not only succeeded `PaymentAttempt`)
- Recovery `already_applied` uses `Installment.PaidAt` (or other persisted application timestamp), not acknowledgement time
- Checkout duplicate/already-applied validates currency and exact amount via shared `validateActiveContractPaymentIdentity`
- `contracts.successful_financial_payment_attempt_id` stores final successful Financial Core provider attempt (session attempt remains in `financial_payment_attempt_id`)
- Canonical known payment statuses fingerprinted as typed values (`succeeded`/`failed`/`canceled`); unknown statuses remain distinct after normalization
- `fingerprint.Generator` Core port restored to minimal `Generate` only; use cases validate `Value` before persistence (no panic-based helpers)
- First-installment activation marks only snapshotted customer debit charges paid via `MarkSnapshottedChargeIDsPaid` (not all offered charges)
- Removed unsafe `MarkOfferedChargesPaidByInstallmentID` and `MarkPayableChargesPaidByInstallmentID` repository methods

### Removed

- `pkg/core/usecase/installmentuc` (split into offer/lifecycle/repayment packages).
- `contractuc` payment inbox/outbox/validation/process files; `installmentuc` payment application files.
- `pkg/core/integration/paymentapplication/reason_helpers.go` (merged into `reason.go`).

### Notes

- No incremental migration files (pre-production); schema updated in `000_schema.sql` only.
- No reserved exposure column exists; exposure reservation release on expiry is not implemented (only active exposure on activation).

## [0.2.0] - 2026-06-30

### Added

- Provider-based `infrastructure:` configuration section (logger, cache, messaging, task, clock, trace, circuit breaker, shutdown, health checks).
- Core infrastructure ports: `cache`, `clock`, `log`, `messaging`, `task`, `trace`, `circuitbreaker`, `health`.
- Cache adapters: noop, Redis (`go-redis`); in-memory adapter for tests (not config-selectable).
- Logging adapters: noop, slog (JSON/text).
- Messaging adapters: noop, Kafka (IBM Sarama producer and consumer group) with `messaging.Health` for broker checks.
- Circuit breaker adapters: noop, gobreaker (`sony/gobreaker/v2`).
- In-process task runtime with cron/interval triggers; noop task provider.
- Scheduled tasks: runtime goroutine metrics, outbox publisher.
- `integration_outbox` table, `outboxrp` repository, and `outboxpublisheruc` use case.
- `healthrp` repository, `healthcheckuc` use case, and `GET /healthz/live` / `GET /healthz/ready` HTTP probes.
- Composite readiness dependency checks (database, cache, messaging) via `infrastructure.health.checks`.
- `cmd/bnpl/bootstrap` package: application wiring, graceful shutdown on SIGINT/SIGTERM, background workers.
- Config and database schema semver validation at `config.Load` (`versions.config` / `versions.database` = 0.2.0).
- Trace-ID middleware (`X-Trace-Id`, `X-Request-Id`, `X-Correlation-Id`) and ULID trace provider.
- HTTP `serdser` success/error envelopes and Gin pagination helpers.
- Core and PostgreSQL pagination utilities.
- Expanded `cerr` error codes (including `SERVICE_UNAVAILABLE`) and HTTP status mapping.
- Mockery configuration (`.mockery.yaml`) and generated mocks under `pkg/core/mock`.
- ADRs 0009–0014 for infrastructure, cache, tasks, outbox, messaging, and health/shutdown decisions.

### Changed

- HTTP server startup moved from `command.startWebServer` to `bootstrap.Boot` with full infrastructure wiring.
- `gin.trusted_proxies` and `usecases.migration.files-dir` are decoded from YAML.
- `000_schema.sql` includes `integration_outbox` DDL and indexes.
- Docker entrypoint: empty `DB_INIT_COMMAND` skips DB init; added init retry loop.
- Config path resolution: CLI flag → `CONFIG_FILE` → `configs/config.yaml`.
- Kafka message subscriber starts with bootstrap (placeholder handler until domain handlers exist).
- README and all `doc/` guides updated to reflect 0.2.0 platform state.

### Infrastructure

- Redis cache startup validates connectivity with `PING`.
- Kafka producer configurable acks, retries, and consumer topic list.
- Dual clock timezones: system (UTC) and business (Asia/Tehran default).
- Outbox publisher exponential backoff via use case configuration.
- Graceful shutdown: HTTP → tasks/consumer → drain → publisher/subscriber/cache/DB; lifecycle probes during drain.

### Tests

- **Bootstrap:** `app_test`, `infrastructure_test`, `repo_test`, `route_test`, `shutdown_test`, `usecase_test`.
- **Config:** `database_test`, `gin_test`, `infrastructure_test`, `load_test`, `shutdown_test`, `usecase_test`, `vers_test`.
- **PostgreSQL:** `error_test`, `pagination_test`; `healthrp` package (no dedicated tests).
- **Infrastructure:** cache memory/noop/redis, circuit breaker noop/gobreaker, clock fixed, log noop, messaging noop, task noop/inprocess.
- **HTTP:** Gin pagination, serdser bind/success, healthrs resource, trace-ID middleware.
- **Core:** `cerr`, `model`, `pagination`, `trace`, `healthcheckuc`, `outboxpublisheruc`.
- Redis adapter tests use `miniredis`.

### Documentation

- Rewrote `doc/architecture.md`, `doc/configuration.md`, `doc/operations.md`, `doc/database.md`, `doc/api.md`.
- Updated ADRs 0001, 0003, 0006, 0009, 0013 with amendments; added ADRs 0009–0014.
- Expanded `.env.example` with infrastructure, shutdown, health, and circuit breaker variables.

### Known gaps

- No BNPL business domains, HTTP business handlers, or outbox insert use cases.
- Circuit breaker constructed but not used at domain call sites.
- Kafka consumer has placeholder handler only (no payment-result processing).
- `SHUTDOWN_DRAIN_DELAY` decoded but not applied in bootstrap shutdown timing.
- `memory` cache not exposed as a configuration provider.
- No HTTP metrics or OpenTelemetry tracing export.
- `assets/db/migrations` may be empty; dev/prod seed SQL still empty.
- `scripts/deploy.sh` missing; CD deploy incomplete.
- Destructive `init-dev`/`init-prod` remain unsuitable for production schema evolution.
- **Not production-ready.**

## [0.1.0] - 2026-06-12

### Added

- Cobra-based CLI: default HTTP server startup; `db init-dev`, `db init-prod`, and `db migrate` subcommands.
- YAML configuration loading with `${ENV}` expansion via `configs/config.yaml`.
- PostgreSQL connection pooling and transaction adapters on `pgx` / `pgxpool`.
- Destructive schema initialization: admin-role schema recreation and application-role privilege setup.
- Embedded SQL bootstrap assets (`000_schema.sql`; dev/prod seed files empty).
- Migration use case for sequential filesystem SQL execution with `migrations` table registration.
- Gin HTTP server bootstrap with optional logger and recovery middleware; empty `/api/v1` route group.
- Dockerfile, Docker Compose (PostgreSQL, Kafka), and `scripts/entrypoint.sh`.
- GitHub Actions CI/CD and GitLab CI test/build/docker pipelines.

### Infrastructure

- PostgreSQL repository implementations under `pkg/adapter/db/postgres`.
- Schema repository (`schemarp`) and migration repository (`migrationsrp`).
- `model.Secret` for redacted configuration credentials.

### Known gaps (0.1.0)

- No concrete HTTP API handlers or BNPL business logic.
- `000_dev.sql` and `000_prod.sql` are empty; `init-dev` and `init-prod` yield the same schema.
- Filesystem migration directory not present; YAML migration path not decoded into config struct.
- Kafka, Redis, and structured logging env vars not wired in Go code.
- `scripts/deploy.sh` referenced by CD workflow but not included in repository.
