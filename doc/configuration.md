# Configuration

## Overview

Configuration is file-based with environment variable expansion:

1. Resolve config path (see below)
2. Read YAML file
3. Expand `${VAR}` via `os.ExpandEnv`
4. Decode into `pkg/adapter/config.Config`
5. Validate via `Config.Validate()` (hierarchical checks; fails fast before wiring infrastructure)

YAML defines structure; environment variables supply values.

Implementation: `config.Load` in `pkg/adapter/config/load.go`.

## Config File Resolution

Order (`cmd/bnpl/bootstrap.FixConfigPath`, invoked from Cobra `OnInitialize`):

1. `--config` / `-c` CLI flag
2. `CONFIG_FILE` environment variable
3. Default: `configs/config.yaml`

Docker `Dockerfile` sets `ENV CONFIG_FILE=configs/config.yaml`.

## Files

| File | Role |
| --- | --- |
| `configs/config.yaml` | Canonical YAML schema |
| `.env.example` | Documented env vars for new environments |
| `.env` | Local overrides (gitignored); loaded by Makefile |
| `.prod.env` | Production template (untracked — do not commit secrets) |

## Variable Expansion

Uses `os.ExpandEnv`:

- `${POSTGRES_HOST}` → value from environment
- Missing variables → **empty string** (no error at expansion time)
- Durations (`3600s`) and integers decoded after expansion
- `Config.Validate()` rejects empty required values, invalid URLs, and unsafe production provider combinations before database/Kafka/Redis connections are opened

**Risk:** Silent misconfiguration if env vars are unset during expansion; validation catches most required fields at startup.

## Docker-Related Configuration

| Variable | Default | Used by                     |
| --- | --- |-----------------------------|
| `CONFIG_FILE` | `configs/config.yaml` | Go app, Docker image        |
| `APP_EXECUTABLE` | `./bnpl` | `entrypoint.sh`, `make run` |

Container env vars are typically injected by orchestrator; entrypoint-only variables are not defined in `configs/config.yaml`.

## YAML Structure

Active sections in `configs/config.yaml`:

```yaml
runtime:        # Deployment environment
domain:         # Deployment-enabled domain capabilities (supported_jurisdictions ⊆ code-supported set)
database:       # PostgreSQL pool, pgpass entries, optional development query logging
gin:            # HTTP server
usecases:
  health:       # Enable /healthz/live and /healthz/ready
  migration:    # Filesystem migration directory
  outbox_publisher:  # Outbox worker settings
infrastructure:
  shutdown:     # Graceful shutdown timeout and drain delay
  health:       # Readiness dependency checks (database, cache, messaging)
  trace:        # Placeholder (ULID provider, no options)
  clock:        # System + business timezones
  logger:       # noop | slog
  metrics:      # noop | prometheus
  http_client:  # noop | standard; shared bounded outbound HTTP/HTTPS client
  cache:        # noop | redis
  messaging:    # noop | kafka
  circuit_breaker:  # noop | gobreaker
  task:         # noop | inprocess + scheduled tasks
  versions:         # Config and database schema semver (validated at load)
external:
  credit_assessment:  # noop | creditbureau
  webhook_dispatcher: # noop | http; HTTP limits under http:
  financial_core:     # noop | grpc; payment_return_url (HTTPS, configured server-side)
```

### Version validation

| Section | In YAML | Read by Go | Validated |
| --- | --- | --- | --- |
| `versions.config` | Yes | **Yes** | Must match binary-supported major; minor ≤ supported |
| `versions.database` | Yes | **Yes** | Same rules as config version |
| `infrastructure.trace` | Yes (empty) | Yes (always ULID; no sub-options) | — |

Current supported versions in binary: **config 0.3.0** / **database 0.2.0**.

### Domain capabilities

`domain.supported_jurisdictions` enables a deployment-specific subset of jurisdictions
this BNPL build can support in code (`model.SupportedJurisdictions()`, currently `IR` only).
It means jurisdictions in which this deployment may **originate new BNPL business**
(checkout create, direct decision create). It is not phone-parser configuration and
does not disable servicing of already-created obligations.

```yaml
domain:
  supported_jurisdictions:
    - IR
```

- Must be non-empty, unique, and every value must be code-supported.
- Config never invents jurisdiction capability; it only enables a subset for origination.
- At startup the list constructs an immutable Core `domaincapability.Jurisdictions`.
- Phone Registry consumes that capability (does not re-read the YAML list independently)
  and still requires a compiled adapter for every origination-enabled jurisdiction.
- MerchantProfile.Jurisdiction selects phone validation rules for that merchant.

### PostgreSQL query logging

`database.query_logging` is an opt-in diagnostic feature:

```yaml
database:
  query_logging:
    enabled: true
    include_arguments: true
```

- `enabled`: installs a pgx query tracer and writes SQL through the configured application logger.
- `include_arguments`: renders `$1`, `$2`, ... into a logging-only SQL representation; pgx still executes the original parameterized query.
- Query logging is **forbidden in `production`**. `Config.Validate()` rejects startup when `database.query_logging.enabled=true` in production.
- Enabling query logging also requires a non-`noop` logger provider.

Environment variables: `POSTGRES_QUERY_LOGGING_ENABLED` and `POSTGRES_QUERY_LOGGING_INCLUDE_ARGUMENTS`.

### Contract and payment processing

Under `usecases.contract`:

| Key | Env var | Purpose |
| --- | --- | --- |
| `payment_result_processing_grace_period` | `CONTRACT_PAYMENT_RESULT_PROCESSING_GRACE_PERIOD` | Operational grace (0–60 minutes) after effective payment deadline before expiring `payment_pending` contracts while still accepting on-time Financial Core results |
| `expiry_batch_size` | `CONTRACT_EXPIRY_BATCH_SIZE` | Maximum `payment_pending` contracts expired per job run |
| `cancellation_saga.retry_base_delay` | `CONTRACT_CANCELLATION_SAGA_RETRY_BASE_DELAY` | Initial delay before the next Financial Core cancellation Saga retry |
| `cancellation_saga.retry_max_delay` | `CONTRACT_CANCELLATION_SAGA_RETRY_MAX_DELAY` | Maximum backoff delay between Financial Core cancellation Saga retries |

Unset cancellation-saga retry values default to `1s` / `1m`.

Contract cancellation is driven by the durable Saga executor (`usecases.saga_executor` + `infrastructure.task.scheduled.saga_executor`). `Config.Validate()` requires the Saga job to be enabled and forbids a noop task provider while cancellation Saga is in use.

The same grace period is injected into `financialcorepaymentresultuc` for checkout payment-result validation.

Under `usecases.installment` (formerly `usecases.checkout_installment_offer`):

| Key | Env var | Purpose |
| --- | --- | --- |
| `mark_due_batch_size` | `INSTALLMENT_MARK_DUE_BATCH_SIZE` | Batch size for scheduled → due lifecycle job |
| `mark_overdue_batch_size` | `INSTALLMENT_MARK_OVERDUE_BATCH_SIZE` | Batch size for due → overdue lifecycle job |
| `installment_payment_requests_destination` | `KAFKA_TOPIC_INSTALLMENT_PAYMENT_REQUESTS` | Outbox destination for installment payment requests |

YAML path rename only; environment variable names are unchanged. Checkout installment offer schedule/policy defaults are sourced from the checkout policy inside `decisionuc` at Decision creation — not configured in YAML.

Financial Core installment payment initiation uses the external adapter (`external.financial_core.installment_payment`; **noop by default**). No real gRPC installment-payment adapter is wired until the official Financial Core proto is available.

Financial Core payment-result consumption is owned by `financialcorepaymentresultuc`. Kafka topic names are infrastructure configuration (`infrastructure.messaging.kafka.topics.payment_results` for inbound results; `payment_application_results` for outbound acknowledgements). Payment-application destination is required when constructing the payment-result use case.

The Kafka payment-result adapter applies documented envelope compatibility defaults (`event_type`, `event_version`, `source_service`) in one mapper when missing from the message; defaulted fields are logged. The use case requires all four trusted envelope fields after mapping.

Physical topic (`financial_core.payment_application_results`) is separate from logical event type (`bnpl.payment_application_result.v1`). Outbox rows use `financial_payment_intent_id` as `message_key` for partition ordering.

Installment contractual `grace_period_days` is persisted on installment rows at offer time — not configured in YAML.

## Infrastructure Provider Defaults

Defaults below match `.env.example`. Override per environment as needed.

| Setting | Env var(s) | Provider values | Default | Wired |
| --- | --- | --- | --- | --- |
| Logger | `LOG_PROVIDER`, `LOG_LEVEL`, `LOG_FORMAT` | `noop`, `slog` | `slog` / `info` / `json` | **Yes** |
| Outbound HTTP client | `HTTP_CLIENT_*` | `noop`, `standard` | `standard` | **Yes** (shared default profile; noop forbidden in production) |
| Cache | `CACHE_PROVIDER`, `CACHE_REDIS_*` | `noop`, `redis` | `noop` | **Yes** |
| Messaging | `MESSAGING_PROVIDER`, `KAFKA_*` | `noop`, `kafka` | `noop` | **Yes** (publisher + subscriber constructed) |
| Task | `TASK_PROVIDER`, `TASK_*` | `noop`, `inprocess` | `inprocess` | **Yes** |
| Clock | `SYSTEM_CLOCK_LOCATION`, `BUSINESS_CLOCK_LOCATION` | N/A (system clock) | `UTC`, `Asia/Tehran` | **Yes** |
| Trace | — | ULID only | — | **Yes** |
| Circuit breaker | `CIRCUIT_BREAKER_PROVIDER`, `CIRCUIT_BREAKER_GOBREAKER_*` | `noop`, `gobreaker` | `gobreaker` | **Yes** (wraps Financial Core gRPC and credit assessment clients) |
| Health probes | `HEALTH_ENABLED` | on/off | `true` | **Yes** |
| Shutdown | `SHUTDOWN_TIMEOUT`, `SHUTDOWN_DRAIN_DELAY` | durations | `30s`, `5s` | **Yes** |

When `CACHE_PROVIDER=redis`, startup performs a Redis `PING` and fails if unreachable.

When `MESSAGING_PROVIDER=kafka`, Sarama producer/consumer clients are created; broker connectivity is validated lazily on first use.

## Configuration Reference

### Application and Docker

| Key | Source | Required for startup | Default (.env.example) | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `CONFIG_FILE` | env, CLI | No | `configs/config.yaml` | Yes | Overrides default path |
| `APP_EXECUTABLE` | env | No | `./bnpl` | Entrypoint only | Not read by Go config |

### PostgreSQL

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `POSTGRES_HOST` | env | Yes | `127.0.0.1` | Yes | pgpass prefix match |
| `POSTGRES_PORT` | env | Yes | `5432` | Yes | |
| `POSTGRES_DB` | env | Yes | `bnpl_db` | Yes | |
| `POSTGRES_SSL_MODE` | env | Yes | `disable` | Yes | |
| `POSTGRES_MAX_CONNECTIONS` | env | Yes | `10` | Yes | |
| `POSTGRES_MIN_CONNECTIONS` | env | Yes | `1` | Yes | |
| `POSTGRES_MAX_CONNECTION_LIFE_TIME` | env | Yes | `3600s` | Yes | |
| `POSTGRES_MAX_CONNECTION_IDLE_TIME` | env | Yes | `1800s` | Yes | |
| `POSTGRES_HEALTH_CHECK_PERIOD` | env | Yes | `60s` | Yes | |
| `POSTGRES_CONNECTION_TIMEOUT` | env | Yes | `30s` | Yes | |
| `POSTGRES_ADMIN_PGPASS` | env | Yes (init) | pgpass line | Yes | Admin bootstrap |
| `POSTGRES_NORMAL_PGPASS` | env | Yes | pgpass line | Yes | Runtime role |
| `POSTGRES_USER` | env | Compose only | `admin` | No (Go) | Docker Postgres image |
| `POSTGRES_PASSWORD` | env | Compose only | `change-me` | No (Go) | Distinct from pgpass |

### HTTP (Gin)

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `GIN_HOST` | env | Yes | `127.0.0.1` | Yes | |
| `GIN_PORT` | env | Yes | `8000` | Yes | |
| `GIN_MODE` | env | Yes | `debug` | Yes | |
| `GIN_LOGGER` | env | Yes | `true` | Yes | Enables AccessLogger |
| `GIN_TRUSTED_PROXY` | env | No | `127.0.0.1` | **Yes** | Decoded from `gin.trusted_proxies` |
| `GIN_HTTP_DEBUG_LOGGER_ENABLED` | env | No | `false` | Yes | Development builds only; rejected in Production |
| `GIN_HTTP_DEBUG_LOGGER_MAX_REQUEST_BODY_BYTES` | env | No | `65536` | Yes | Log capture limit only |
| `GIN_HTTP_DEBUG_LOGGER_MAX_RESPONSE_BODY_BYTES` | env | No | `65536` | Yes | Log capture limit only |
| `GIN_RESPONSE_LOGGING_ENABLED` | env | No | `false` | Yes | Allowlisted AccessLogger response projection |
| `GIN_RESPONSE_LOGGING_MAX_BODY_BYTES` | env | No | `16384` | Yes | Requires `GIN_LOGGER=true` |

HTTP panic recovery is mandatory and always enabled; there is intentionally no runtime toggle for disabling it.

#### HTTPDebugLogger (Development)

Disabled by default. Requires a **development** build (`make build-development`). Captures unredacted request/response headers, query strings, URLs, and bounded body snapshots for temporary diagnostics.

**WARNING:** HTTPDebugLogger records potentially sensitive request and response data without redaction. Enable it only for temporary debugging in controlled development environments using test data. Do not use it with production credentials, real customer information, or centralized/shared log destinations. Using the `development` build profile does not automatically make captured data safe — the operator is responsible for isolated environments and appropriate test credentials.

Capture size limits truncate only the log representation; client-visible HTTP bytes are unchanged. Truncation is reported explicitly with limit and captured-size metadata.

Production binaries do not compile the unredacted implementation. Runtime `enabled=true` is rejected during configuration validation in Production.

#### Safe response logging (Production)

Disabled by default. When `GIN_RESPONSE_LOGGING_ENABLED=true`, AccessLogger may enrich the existing access record with a **strict allowlist** projection of JSON response fields for known routes (`c.FullPath()`). Unknown routes, unknown fields, unsupported content types, compressed bodies, invalid JSON, and oversized bodies fail closed with an explicit `http.response_body.reason` such as `no_allowlist_policy`, `unsupported_content_type`, `body_too_large`, or `invalid_json`.

Raw response bodies are never logged as a fallback. Sensitive response headers (`Set-Cookie`, `Location`, `Authorization`) are never dumped. Adding a new loggable field or route requires a source-code review of the allowlist policy.

### Graceful shutdown

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `SHUTDOWN_TIMEOUT` | env | Yes | `30s` | Yes | `infrastructure.shutdown.timeout` |
| `SHUTDOWN_DRAIN_DELAY` | env | Yes | `5s` | Yes | Applied after draining starts, before HTTP shutdown |

### Health use case

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `HEALTH_ENABLED` | env | Yes | `true` | Yes | `usecases.health.enabled` |

Readiness dependency checks are configured as a YAML list at `infrastructure.health.checks` (not env-expanded). Default in `configs/config.yaml`: `database`, `cache`, `messaging`.

### Migrations use case

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `MIGRATIONS_FILES_DIR` | env | For migrate | `assets/db/migrations` | **Yes** | `usecases.migration.files-dir` |

### Outbox publisher use case

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `OUTBOX_PUBLISHER_WORKER_ID` | env | Yes (when task enabled) | `local` | Yes | |
| `OUTBOX_PUBLISHER_BATCH_SIZE` | env | Yes | `100` | Yes | |
| `OUTBOX_PUBLISHER_RETRY_BASE_DELAY` | env | Yes | `1s` | Yes | |
| `OUTBOX_PUBLISHER_RETRY_MAX_DELAY` | env | Yes | `1m` | Yes | |

### Clock

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `SYSTEM_CLOCK_LOCATION` | env | Yes | `UTC` | Yes | Infrastructure timestamps |
| `BUSINESS_CLOCK_LOCATION` | env | Yes | `Asia/Tehran` | Yes | Future business date rules |

### Logging

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `LOG_PROVIDER` | env | Yes | `slog` | Yes | `noop` discards logs |
| `LOG_LEVEL` | env | When slog | `info` | Yes | `debug`, `info`, `warn`, `error` |
| `LOG_FORMAT` | env | When slog | `json` | Yes | `json` or `text` |

### Outbound HTTP/HTTPS client

The first phase uses one shared default profile for all third-party HTTP/HTTPS adapters. There are no per-client overrides yet. `max_in_flight` is the worker-pool-equivalent execution limit; it is implemented with a semaphore rather than dedicated worker goroutines so a single reusable `net/http.Client` and connection pool can be shared safely.

The bounded queue provides backpressure. When both `max_in_flight` and `max_queue_size` are exhausted, the client returns a saturation error immediately. The noop provider performs no network I/O and returns a disabled error; production validation rejects `HTTP_CLIENT_PROVIDER=noop`.

Operational metrics deliberately use only bounded labels such as HTTP method and outcome class. Full URLs, paths, query strings, entity IDs, and trace IDs are never metric labels because they would create unbounded Prometheus cardinality.

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `HTTP_CLIENT_PROVIDER` | env | Yes | `standard` | Yes | `noop`, `standard`; noop forbidden in production |
| `HTTP_CLIENT_MAX_IN_FLIGHT` | env | When standard | `50` | Yes | Global concurrent execution limit |
| `HTTP_CLIENT_MAX_QUEUE_SIZE` | env | When standard | `100` | Yes | Bounded waiting queue; `0` = fail fast |
| `HTTP_CLIENT_QUEUE_WAIT_TIMEOUT` | env | When standard | `500ms` | Yes | Max queue wait before rejection |
| `HTTP_CLIENT_TIMEOUT` | env | When standard | `5s` | Yes | End-to-end `net/http.Client` timeout |
| `HTTP_CLIENT_DIAL_TIMEOUT` | env | When standard | `2s` | Yes | TCP dial timeout |
| `HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT` | env | When standard | `2s` | Yes | HTTPS handshake timeout |
| `HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT` | env | When standard | `3s` | Yes | Wait for response headers |
| `HTTP_CLIENT_IDLE_CONN_TIMEOUT` | env | When standard | `90s` | Yes | Idle keep-alive lifetime |
| `HTTP_CLIENT_MAX_IDLE_CONNS` | env | When standard | `100` | Yes | Total idle pooled connections |
| `HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST` | env | When standard | `20` | Yes | Idle pooled connections per host |
| `HTTP_CLIENT_MAX_CONNS_PER_HOST` | env | When standard | `50` | Yes | Active+dialing+idle connections per host |

### Cache

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `CACHE_PROVIDER` | env | Yes | `noop` | Yes | |
| `CACHE_REDIS_ADDR` | env | When redis | `127.0.0.1:6379` | Yes | Required non-empty for redis |
| `CACHE_REDIS_USERNAME` | env | No | empty | Yes | |
| `CACHE_REDIS_PASSWORD` | env | No | empty | Yes | |
| `CACHE_REDIS_DB` | env | When redis | `0` | Yes | |
| `CACHE_REDIS_DIAL_TIMEOUT` | env | When redis | `5s` | Yes | |
| `CACHE_REDIS_READ_TIMEOUT` | env | When redis | `3s` | Yes | |
| `CACHE_REDIS_WRITE_TIMEOUT` | env | When redis | `3s` | Yes | |
| `CACHE_REDIS_POOL_SIZE` | env | When redis | `10` | Yes | |
| `CACHE_REDIS_MIN_IDLE_CONNS` | env | When redis | `1` | Yes | |

### Messaging (Kafka)

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `MESSAGING_PROVIDER` | env | Yes | `noop` | Yes | |
| `KAFKA_BROKER_1` | env | When kafka | `127.0.0.1:9092` | Yes | Single broker in YAML list |
| `KAFKA_CLIENT_ID` | env | When kafka | `bnpl-backend` | Yes | |
| `KAFKA_GROUP_ID` | env | When kafka | `bnpl-consumer-group` | Yes | Subscriber only |
| `KAFKA_TOPIC_PAYMENT_RESULTS` | env | When kafka | `financial-core.payment-results` | Yes | Consumer topic for Financial Core payment results |
| `KAFKA_TOPIC_PAYMENT_APPLICATION_RESULTS` | env | When kafka | `financial_core.payment_application_results` | Yes | Outbox destination for payment-application acknowledgements (event type remains `bnpl.payment_application_result.v1`) |
| `KAFKA_PRODUCER_REQUIRED_ACKS` | env | When kafka | `-1` | Yes | |
| `KAFKA_PRODUCER_MAX_RETRIES` | env | When kafka | `5` | Yes | |
| `KAFKA_PRODUCER_RETURN_SUCCESSES` | env | When kafka | `true` | Yes | |
| `KAFKA_CONSUMER_TOPIC_PAYMENT_RESULTS` | env | When kafka | same as producer topic | Yes | Consumer subscription list |

Kafka topic env vars are wired into config. Outbox publisher uses the per-row `destination` column as the publish topic. The message subscriber starts with bootstrap and processes Financial Core payment results through `financialcorepaymentresultuc`.

### Circuit breaker

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `CIRCUIT_BREAKER_PROVIDER` | env | Yes | `gobreaker` | Yes | `noop` disables protection |
| `CIRCUIT_BREAKER_GOBREAKER_MAX_REQUESTS` | env | When gobreaker | `3` | Yes | Half-open request cap |
| `CIRCUIT_BREAKER_GOBREAKER_INTERVAL` | env | When gobreaker | `60s` | Yes | Closed-state counter reset |
| `CIRCUIT_BREAKER_GOBREAKER_TIMEOUT` | env | When gobreaker | `30s` | Yes | Open-state duration |
| `CIRCUIT_BREAKER_GOBREAKER_FAILURE_THRESHOLD` | env | When gobreaker | `5` | Yes | Consecutive failures to open |

### Task runtime

| Key | Source | Required | Default | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `TASK_PROVIDER` | env | Yes | `inprocess` | Yes | `noop` disables scheduling |
| `TASK_INPROCESS_DEFAULT_TIMEZONE` | env | When inprocess | `Asia/Tehran` | Yes | Cron default TZ |
| `TASK_RUNTIME_GOROUTINE_METRICS_ENABLED` | env | No | `true` | Yes | Scheduled task |
| `TASK_RUNTIME_GOROUTINE_METRICS_HANDLER` | env | No | `runtime_goroutine_metrics` | Yes | |
| `TASK_RUNTIME_GOROUTINE_METRICS_TRIGGER_TYPE` | env | No | `interval` | Yes | `manual`, `cron`, `interval`, `event` |
| `TASK_RUNTIME_GOROUTINE_METRICS_CRON` | env | No | empty | Yes | When type=cron |
| `TASK_RUNTIME_GOROUTINE_METRICS_TIMEZONE` | env | No | `Asia/Tehran` | Yes | |
| `TASK_RUNTIME_GOROUTINE_METRICS_INTERVAL` | env | No | `15m` | Yes | When type=interval |
| `TASK_RUNTIME_GOROUTINE_METRICS_RUN_ON_START` | env | No | `true` | Yes | |
| `TASK_OUTBOX_PUBLISHER_ENABLED` | env | No | `true` | Yes | |
| `TASK_OUTBOX_PUBLISHER_HANDLER` | env | No | `outbox_publisher` | Yes | |
| `TASK_OUTBOX_PUBLISHER_TRIGGER_TYPE` | env | No | `interval` | Yes | `interval` or `cron` |
| `TASK_OUTBOX_PUBLISHER_CRON` | env | No | empty | Yes | |
| `TASK_OUTBOX_PUBLISHER_TIMEZONE` | env | No | `UTC` | Yes | |
| `TASK_OUTBOX_PUBLISHER_INTERVAL` | env | No | `5s` | Yes | |
| `TASK_OUTBOX_PUBLISHER_RUN_ON_START` | env | No | `true` | Yes | |

## Local Development With Noop Providers

For minimal dependencies (PostgreSQL only):

```bash
HTTP_CLIENT_PROVIDER=noop
CACHE_PROVIDER=noop
MESSAGING_PROVIDER=noop
TASK_PROVIDER=inprocess   # or noop to disable background jobs
LOG_PROVIDER=slog
```

Redis and Kafka in Compose can remain stopped when noop providers are selected. The HTTP server still requires PostgreSQL for pool creation and outbox task (if enabled).

## Makefile Integration

```make
-include .env
export
```

All Make targets inherit `.env` variables. Use `make env` to print exportable lines.

## model.Secret in Configuration

`database.pgpass_entries.admin` and `.normal` decode into `model.Secret`. `Config.String()` JSON-marshals the struct with redacted secrets.

See [security.md](security.md).

## Loading Process

1. Cobra parses flags; `FixConfigPath` sets bootstrap config path
2. `config.Load(cfgPath)` reads and expands YAML
3. Validates single root YAML mapping node
4. Decodes into `Config`
5. Validates `versions.config` and `versions.database`
6. Runs `Config.Validate()` for hierarchical startup validation
7. Bootstrap invokes `ConnectionPool`, `NewLoggerInfrastructure`, `NewGinEngine`, etc.

`db init-dev` / `init-prod` still print loaded config: `fmt.Printf("configs: %v\n", c)`.

## Defaults

Go code defaults only the config **file path**. Provider defaults and operational values come from `.env` / operator-provided environment, not from Go constants.

### External Webhook Dispatcher

| Key | Source | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `EXTERNAL_WEBHOOK_DISPATCHER_PROVIDER` | env | Yes | `http` | `http` or `noop`; `noop` forbidden in production |
| `EXTERNAL_WEBHOOK_DISPATCHER_HTTP_TIMEOUT` | env | When http | `10s` | Zero uses deliverer default; must not be negative |
| `EXTERNAL_WEBHOOK_DISPATCHER_HTTP_DIAL_TIMEOUT` | env | When http | `5s` | Zero uses deliverer default; must not be negative |
| `EXTERNAL_WEBHOOK_DISPATCHER_HTTP_MAX_RESPONSE_BYTES` | env | When http | `65536` | Zero uses deliverer default; must not be negative |

Provider-specific settings live under `external.webhook_dispatcher.http` / `noop`. Bootstrap wires the selected adapter in `NewExternal`; OutboxPublisher only receives the ready `messaging.Publisher`.

### External Financial Core

| Key | Source | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `EXTERNAL_FINANCIAL_CORE_PROVIDER` | env | Yes | `noop` | `grpc` in production; `noop` forbidden when `APP_ENVIRONMENT=production` |
| `FINANCIAL_CORE_PAYMENT_RETURN_URL` | env | Yes | — | Backend-owned HTTPS redirect URL sent to Financial Core; not accepted from API clients |
| `EXTERNAL_FINANCIAL_CORE_GRPC_ADDRESS` | env | When grpc | — | Required when provider is `grpc` |
| `EXTERNAL_FINANCIAL_CORE_GRPC_TIMEOUT` | env | When grpc | `5s` | Must be positive |
| `EXTERNAL_FINANCIAL_CORE_GRPC_INSECURE` | env | When grpc | `true` | Local development only |

### External Customer Reference Service

| Key | Source | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `EXTERNAL_CUSTOMER_PROVIDER` | env | Yes | `noop` | `grpc` in production; `noop` forbidden when `APP_ENVIRONMENT=production` |
| `EXTERNAL_CUSTOMER_GRPC_ADDRESS` | env | When grpc | — | Required when provider is `grpc` |
| `EXTERNAL_CUSTOMER_GRPC_TIMEOUT` | env | When grpc | `5s` | Must be positive |
| `EXTERNAL_CUSTOMER_GRPC_INSECURE` | env | When grpc | `true` | Local development only; must be `false` in production |

Identity owns `ResolveSubjectByMobile` only (merchant-asserted phone → subject). It does not submit notifications or SMS.

### External Notification Service

| Key | Source | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `EXTERNAL_NOTIFICATION_PROVIDER` | env | Yes | `noop` | `grpc` in production; `noop` forbidden when `APP_ENVIRONMENT=production` |
| `EXTERNAL_NOTIFICATION_GRPC_ADDRESS` | env | When grpc | — | Required when provider is `grpc` |
| `EXTERNAL_NOTIFICATION_GRPC_TIMEOUT` | env | When grpc | `5s` | Must be positive |
| `EXTERNAL_NOTIFICATION_GRPC_INSECURE` | env | When grpc | `true` | Local development only; must be `false` in production |

Notification owns durable `SubmitNotification` acceptance for customer token checkout OTP. BNPL still generates and verifies OTP codes.

### Checkout session OTP

Code length, lifetime (2m), wrong-code attempt budget (5), send cooldown (60s), and sends per checkout (3) are immutable security policy (`pkg/core/policy/otp`, `V1`). They are deliberately **not** configurable; the retired `CHECKOUT_SESSION_OTP_TTL` and `CHECKOUT_SESSION_OTP_MAX_ATTEMPTS` keys are ignored if still set in the environment.

| Key | Source | Required | Default | Notes |
| --- | --- | --- | --- | --- |
| `CHECKOUT_SESSION_OTP_TEMPLATE_ID` | env | Yes | `bnpl.external_checkout_otp.v1` | Notification Service logical template id (`code`, `merchant_name`) |
| `CHECKOUT_SESSION_OTP_ACTIVE_PEPPER_KEY_ID` | env | When more than one key | only key | Must exist in the pepper ring |
| `CHECKOUT_SESSION_OTP_PEPPER_KEYS` | env | Yes | — | `id:secret,...`; each secret at least 32 bytes, no commas |

Cross-checkout OTP send budgets (5 / 10m per phone fingerprint, provisional 30,000 / 10m per merchant as a V1 runaway-abuse ceiling) live in `otp.V1` with the per-checkout terms. Partner Create and customer token checkout do not use gateway JWT tenant/context headers. Checkout payment submission uses Financial Core v2 (`payment.v1.CheckoutPaymentService`) without FinancialScope/tenant/context on the new payment RPCs. BNPL purchase and installment-repayment parties are `payer=customer` and `payee=platform` with an empty platform `party_id` (FC contract; not configurable and never MerchantID). Platform party id is never a runtime config knob.

Checkout and installment repayment flows set `success_return_url`, `failure_return_url`, and `cancel_return_url` from this single configured URL when creating a Financial Core payment session. Installment repayment requested session expiry uses the domain constant `DefaultPaymentSessionTTL` (15m from durable `PaymentAttempt.CreatedAt`), not installment contractual due dates.

Fingerprint algorithm selection is **not** configurable. Bootstrap wires the SHA-256 adapter explicitly; changing the algorithm requires a versioned data-contract migration, not YAML or environment toggles.

## TODO

- [ ] Remove debug config printing from CLI init commands
- [ ] Wire `SHUTDOWN_DRAIN_DELAY` into bootstrap shutdown sequence
- [ ] Multi-broker Kafka YAML (`KAFKA_BROKER_2`, …)

## Related documentation

- [security.md](security.md) — pgpass and secrets
- [operations.md](operations.md) — local setup
- ADR [0003](adr/0003-env-expanded-yaml-config.md)
- ADR [0009](adr/0009-provider-based-infrastructure-configuration.md)
