# Configuration

## Overview

Configuration is file-based with environment variable expansion:

1. Resolve config path (see below)
2. Read YAML file
3. Expand `${VAR}` via `os.ExpandEnv`
4. Decode into `pkg/adapter/config.Config`

YAML defines structure; environment variables supply values.

Implementation: `config.Load` in `pkg/adapter/config/config.go`.

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
- Missing variables → **empty string** (no error at load time)
- Durations (`3600s`) and integers decoded after expansion

**Risk:** Silent misconfiguration if env vars are unset.

## Docker-Related Configuration

| Variable | Default | Used by |
| --- | --- | --- |
| `CONFIG_FILE` | `configs/config.yaml` | Go app, Docker image |
| `APP_EXECUTABLE` | `./bnpl` | `docker-entrypoint.sh`, `make run` |
| `DB_INIT_COMMAND` | `init-dev` in `.env.example`; empty skips init in entrypoint | `docker-entrypoint.sh` only |
| `DB_INIT_MAX_ATTEMPTS` | `10` | Entrypoint DB init retries |
| `DB_INIT_RETRY_INTERVAL_SECONDS` | `3` | Entrypoint retry sleep |

Container env vars are typically injected by orchestrator; entrypoint-only variables are not defined in `configs/config.yaml`.

## YAML Structure

Active sections in `configs/config.yaml`:

```yaml
database:       # PostgreSQL pool and pgpass entries
gin:            # HTTP server
usecases:
  migration:    # Filesystem migration directory
  outbox_publisher:  # Outbox worker settings
infrastructure:
  trace:        # Placeholder (ULID provider, no options)
  clock:        # System + business timezones
  logger:       # noop | slog
  cache:        # noop | redis
  messaging:    # noop | kafka
  task:         # noop | inprocess + scheduled tasks
versions:         # Present in YAML; not read by Go
```

### Unused YAML sections

| Section | In YAML | Read by Go |
| --- | --- | --- |
| `versions.database` | Yes | **No** |
| `versions.config` | Yes | **No** |
| `infrastructure.trace` | Yes (empty) | Yes (always ULID; no sub-options) |

## Infrastructure Provider Defaults

Defaults below match `.env.example`. Override per environment as needed.

| Setting | Env var(s) | Provider values | Default | Wired |
| --- | --- | --- | --- | --- |
| Logger | `LOG_PROVIDER`, `LOG_LEVEL`, `LOG_FORMAT` | `noop`, `slog` | `slog` / `info` / `json` | **Yes** |
| Cache | `CACHE_PROVIDER`, `CACHE_REDIS_*` | `noop`, `redis` | `noop` | **Yes** |
| Messaging | `MESSAGING_PROVIDER`, `KAFKA_*` | `noop`, `kafka` | `noop` | **Yes** (publisher + subscriber constructed) |
| Task | `TASK_PROVIDER`, `TASK_*` | `noop`, `inprocess` | `inprocess` | **Yes** |
| Clock | `SYSTEM_CLOCK_LOCATION`, `BUSINESS_CLOCK_LOCATION` | N/A (system clock) | `UTC`, `Asia/Tehran` | **Yes** |
| Trace | — | ULID only | — | **Yes** |
| Circuit breaker | — | — | — | **No** (not implemented) |

When `CACHE_PROVIDER=redis`, startup performs a Redis `PING` and fails if unreachable.

When `MESSAGING_PROVIDER=kafka`, Sarama producer/consumer clients are created; broker connectivity is validated lazily on first use.

## Configuration Reference

### Application and Docker

| Key | Source | Required for startup | Default (.env.example) | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `CONFIG_FILE` | env, CLI | No | `configs/config.yaml` | Yes | Overrides default path |
| `APP_EXECUTABLE` | env | No | `./bnpl` | Entrypoint only | Not read by Go config |
| `DB_INIT_COMMAND` | env | No | `init-dev` | Entrypoint only | Empty value skips DB init |
| `DB_INIT_MAX_ATTEMPTS` | env | No | `10` | Entrypoint only | |
| `DB_INIT_RETRY_INTERVAL_SECONDS` | env | No | `3` | Entrypoint only | |

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
| `GIN_LOGGER` | env | Yes | `true` | Yes | |
| `GIN_RECOVERY` | env | Yes | `true` | Yes | |
| `GIN_TRUSTED_PROXY` | env | No | `127.0.0.1` | **Yes** | Decoded from `gin.trusted_proxies` |

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
| `KAFKA_TOPIC_PAYMENT_RESULTS` | env | When kafka | `financial-core.payment-results` | Yes | Logical topic name |
| `KAFKA_PRODUCER_REQUIRED_ACKS` | env | When kafka | `-1` | Yes | |
| `KAFKA_PRODUCER_MAX_RETRIES` | env | When kafka | `5` | Yes | |
| `KAFKA_PRODUCER_RETURN_SUCCESSES` | env | When kafka | `true` | Yes | |
| `KAFKA_CONSUMER_TOPIC_PAYMENT_RESULTS` | env | When kafka | same as producer topic | Yes | Consumer subscription list |

Kafka topic env vars are wired into config but **no consumer is started** in bootstrap yet. Outbox publisher uses the `destination` column as the publish topic.

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
5. Bootstrap invokes `ConnectionPool`, `NewLoggerInfrastructure`, `NewGinEngine`, etc.

`db init-dev` / `init-prod` still print loaded config: `fmt.Printf("configs: %v\n", c)`.

## Defaults

Go code defaults only the config **file path**. Provider defaults and operational values come from `.env` / operator-provided environment, not from Go constants.

## TODO

- [ ] Post-load validation with clear errors for missing required fields
- [ ] Remove debug config printing from CLI init commands
- [ ] Circuit breaker configuration section
- [ ] Wire Kafka subscriber startup when consumer handlers exist
- [ ] Multi-broker Kafka YAML (`KAFKA_BROKER_2`, …)

## Related documentation

- [security.md](security.md) — pgpass and secrets
- [operations.md](operations.md) — local setup
- ADR [0003](adr/0003-env-expanded-yaml-config.md)
- ADR [0009](adr/0009-provider-based-infrastructure-configuration.md)
