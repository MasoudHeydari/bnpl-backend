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

Order (`cmd/bnpl/command/fixConfigPath`):

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
| `APP_EXECUTABLE` | `./bnpl` (`.env.example`: `./bnpl`; container: `./bnpl`) | `docker-entrypoint.sh`, `make run` |
| `DB_INIT_COMMAND` | `init-dev` | `docker-entrypoint.sh` — destructive bootstrap subcommand |

Container env vars are typically injected by orchestrator; they are not defined in `configs/config.yaml`.

## YAML Structure (active sections)

```yaml
database:
  host: ${POSTGRES_HOST}
  port: ${POSTGRES_PORT}
  name: ${POSTGRES_DB}
  max_connections: ${POSTGRES_MAX_CONNECTIONS}
  min_connections: ${POSTGRES_MIN_CONNECTIONS}
  max_connection_life_time: ${POSTGRES_MAX_CONNECTION_LIFE_TIME}
  max_connection_idle_time: ${POSTGRES_MAX_CONNECTION_IDLE_TIME}
  health_check_period: ${POSTGRES_HEALTH_CHECK_PERIOD}
  connection_timeout: ${POSTGRES_CONNECTION_TIMEOUT}
  pgpass_entries:
    admin: ${POSTGRES_ADMIN_PGPASS}
    normal: ${POSTGRES_NORMAL_PGPASS}

gin:
  host: ${GIN_HOST}
  port: ${GIN_PORT}
  trusted_proxies:
    - ${GIN_TRUSTED_PROXY}
  mode: ${GIN_MODE}
  logger: ${GIN_LOGGER}
  recovery: ${GIN_RECOVERY}

usecases:
  migration:
    files-dir: ${MIGRATIONS_FILES_DIR}
```

### Unused YAML sections

| Section | In YAML | Read by Go |
| --- | --- | --- |
| `versions.database` | Yes | **No** |
| `versions.config` | Yes | **No** |
| `gin.trusted_proxies` | Yes | **No** (Gin hardcodes `127.0.0.1`) |
| `usecases.migration.files-dir` | Yes | **No** (struct field not tagged) |

## Configuration Reference

| Key | Source | Required for startup | Default (.env.example) | Wired | Notes |
| --- | --- | --- | --- | --- | --- |
| `CONFIG_FILE` | env, CLI | No | `configs/config.yaml` | Yes | Overrides default path |
| `POSTGRES_HOST` | env | Yes | `127.0.0.1` | Yes | pgpass prefix match |
| `POSTGRES_PORT` | env | Yes | `5432` | Yes | pgpass prefix match |
| `POSTGRES_DB` | env | Yes | `bnpl_db` | Yes | pgpass prefix match |
| `POSTGRES_MAX_CONNECTIONS` | env | Yes | `10` | Yes | `MaxConns` |
| `POSTGRES_MIN_CONNECTIONS` | env | Yes | `1` | Yes | `MinConns` |
| `POSTGRES_MAX_CONNECTION_LIFE_TIME` | env | Yes | `3600s` | Yes | |
| `POSTGRES_MAX_CONNECTION_IDLE_TIME` | env | Yes | `1800s` | Yes | |
| `POSTGRES_HEALTH_CHECK_PERIOD` | env | Yes | `60s` | Yes | |
| `POSTGRES_CONNECTION_TIMEOUT` | env | Yes | `30s` | Yes | |
| `POSTGRES_ADMIN_PGPASS` | env | Yes (init) | pgpass line | Yes | Admin role bootstrap |
| `POSTGRES_NORMAL_PGPASS` | env | Yes | pgpass line | Yes | Runtime role |
| `GIN_HOST` | env | Yes | `127.0.0.1` | Yes | |
| `GIN_PORT` | env | Yes | `8000` | Yes | |
| `GIN_MODE` | env | Yes | `debug` | Yes | |
| `GIN_LOGGER` | env | Yes | `true` | Yes | |
| `GIN_RECOVERY` | env | Yes | `true` | Yes | |
| `GIN_TRUSTED_PROXY` | env | No | `127.0.0.1` | **No** | Present in YAML only |
| `MIGRATIONS_FILES_DIR` | env | For migrate | `assets/db/migrations` | **No** | Not decoded to struct |
| `POSTGRES_USER` | env | Compose only | `bnpl` | No (Go) | Docker Postgres bootstrap |
| `POSTGRES_PASSWORD` | env | Compose only | `change-me` | No (Go) | Distinct from pgpass |
| `APP_EXECUTABLE` | env | No | `./bnpl` | Entrypoint only | Not read by Go config |
| `DB_INIT_COMMAND` | env | No | `init-dev` | Entrypoint only | Not read by Go config |
| `KAFKA_ENABLED` | .env only | No | `true` | **No** | Not in config.yaml |
| `KAFKA_BROKERS` | .env only | No | `127.0.0.1:9092` | **No** | |
| `KAFKA_CLIENT_ID` | .env only | No | `bnpl` | **No** | |
| `LOG_LEVEL` | .env only | No | `debug` | **No** | |

## Makefile Integration

```make
-include .env
export
```

All Make targets inherit `.env` variables. Use `make env` to print exportable lines.

## model.Secret in Configuration

`database.pgpass_entries.admin` and `.normal` decode into `model.Secret`. Serialization redacts values; `Reveal()` used only for connection URL construction.

See [security.md](security.md).

## Loading Process

1. `fixConfigPath()` sets `cfgPath`
2. `config.Load(cfgPath)` reads and expands YAML
3. Validates single root YAML mapping node
4. Decodes into `Config`
5. Callers invoke `ConnectionPool`, `Gin.NewEngine`, `NewMigrationsUseCase`, etc.

CLI commands currently print loaded config: `fmt.Printf("configs: %v\n", c)`.

## Defaults

Go code defaults only the config **file path**. All other defaults come from `.env` / operator-provided environment, not from Go constants.

## TODO

- [ ] Add `yaml:"files-dir"` to `Migrations` struct (or equivalent wiring)
- [ ] Decode `gin.trusted_proxies` and pass to Gin adapter
- [ ] Decode or remove `versions` section
- [ ] Post-load validation with clear errors for missing required fields
- [ ] Remove debug config printing from CLI commands
- [ ] Wire or remove Kafka and `LOG_LEVEL` from `.env.example`

## Related documentation

- [security.md](security.md) — pgpass and secrets
- [operations.md](operations.md) — local setup
- ADR [0003](adr/0003-env-expanded-yaml-config.md)
