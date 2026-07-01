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

### 4. Database bootstrap (destructive)

```bash
make init-dev    # development seed path (seed SQL empty today)
# or
make init-prod   # production-style seed path (same effect today)
```

Both drop and recreate schema `bnpl`. See [database.md](database.md).

### 5. Run application

```bash
make run
```

Runs `scripts/docker-entrypoint.sh`, which:

1. Runs `bnpl db ${DB_INIT_COMMAND}` when `DB_INIT_COMMAND` is non-empty (**destructive** with default `init-dev`)
2. Retries failed init up to `DB_INIT_MAX_ATTEMPTS` (default 10) with `DB_INIT_RETRY_INTERVAL_SECONDS` (default 3) between attempts
3. `exec bnpl` — starts HTTP server and background tasks

For repeated local runs without re-init:

```bash
DB_INIT_COMMAND= ./build/bin/bnpl
# or invoke the binary directly after a one-time init:
./build/bin/bnpl
```

## Makefile Targets

### Build / run

| Target | Action |
| --- | --- |
| `build` | Compile to `build/bin/bnpl` |
| `run` | `build` + `up` + `docker-entrypoint.sh` |
| `init-dev` | `bnpl db init-dev` |
| `init-prod` | `bnpl db init-prod` |
| `clean` | Remove binary and `coverage.out` |

### Test / quality

| Target | Action |
| --- | --- |
| `test` | `go test ./...` |
| `test-integration` | Integration tests with `integration` tag |
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
./build/bin/bnpl db init-dev                  # destructive bootstrap
./build/bin/bnpl db init-prod                 # destructive bootstrap
./build/bin/bnpl db migrate                   # filesystem migrations
```

The default HTTP server handles `SIGINT` and `SIGTERM` with graceful shutdown (see below).

## Graceful Shutdown

On `SIGINT` or `SIGTERM`:

1. Readiness fails immediately (`LifecycleState.StartDraining`)
2. Bounded shutdown runs (`SHUTDOWN_TIMEOUT`, default 30s)
3. HTTP server stops accepting new connections
4. Task scheduler and message consumer stop (root context canceled)
5. Short in-flight drain window (100ms hardcoded today)
6. Publisher, subscriber, cache, and database pool close
7. If shutdown exceeds timeout, liveness fails (`MarkShutdownTimedOut`)

`SHUTDOWN_DRAIN_DELAY` is configured (default 5s) but **not yet applied** before HTTP shutdown — readiness still fails on signal via draining state.

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
- `scripts/docker-entrypoint.sh`

Environment:

- `CONFIG_FILE=configs/config.yaml`
- `APP_EXECUTABLE` and `DB_INIT_COMMAND` expected at runtime (see entrypoint)

### Entrypoint behavior

`scripts/docker-entrypoint.sh`:

| Step | Behavior | Notes |
| --- | --- | --- |
| 1 | DB init when `DB_INIT_COMMAND` is set | Skipped when empty |
| 1a | Retry loop | `DB_INIT_MAX_ATTEMPTS` (default 10), `DB_INIT_RETRY_INTERVAL_SECONDS` (default 3) |
| 2 | `exec bnpl` | HTTP server + tasks as PID 1 |

Supported `DB_INIT_COMMAND` values: `init-dev`, `init-prod`. Leave unset or empty to skip initialization:

```bash
docker run -e DB_INIT_COMMAND= ...
# or
docker run -e DB_INIT_COMMAND="" ...
```

Override for production experiments:

```bash
docker run -e DB_INIT_COMMAND=init-prod -e APP_EXECUTABLE=./bnpl ...
```

**Warning:** `init-dev` and `init-prod` are destructive. Steady-state production should use empty `DB_INIT_COMMAND` and external migration jobs.

See ADR [0006](adr/0006-docker-entrypoint-startup-flow.md).

## Provider Requirements by Environment

| Provider setting | External dependency | When required |
| --- | --- | --- |
| `CACHE_PROVIDER=noop` | None | Default local setup |
| `CACHE_PROVIDER=redis` | Redis at `CACHE_REDIS_ADDR` | Startup fails on failed `PING` |
| `MESSAGING_PROVIDER=noop` | None | Default local setup |
| `MESSAGING_PROVIDER=kafka` | Kafka at `KAFKA_BROKER_1` | Lazy connect; publish/consume fails without broker |
| `TASK_PROVIDER=noop` | None | Disables scheduled jobs |
| `TASK_PROVIDER=inprocess` | None | Runs goroutine metrics and outbox publisher when enabled |

## Graceful Shutdown Configuration

| Key | Default | Purpose |
| --- | --- | --- |
| `SHUTDOWN_TIMEOUT` | `30s` | Maximum time for full shutdown sequence |
| `SHUTDOWN_DRAIN_DELAY` | `5s` | Intended delay before HTTP shutdown (not wired in bootstrap yet) |

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

- Default `.env.example` still sets `DB_INIT_COMMAND=init-dev`; container entrypoint destroys schema on every start unless overridden
- No `deploy.sh` in repo despite CD reference
- `assets/db/migrations` directory may be missing until first migration is added
- `SHUTDOWN_DRAIN_DELAY` configured but not applied in bootstrap shutdown timing
- Kafka subscriber runs with placeholder handler only (no domain message processing)
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
- Use `init-dev` for greenfield schema only

### HTTP server starts but no API routes

- Expected — see [api.md](api.md)

### Container wipes database on restart

- Default `DB_INIT_COMMAND=init-dev` in `.env.example`
- Set `DB_INIT_COMMAND=` (empty) for steady-state deployments

### Outbox publisher logs errors with noop messaging

- Expected when rows exist and `MESSAGING_PROVIDER=noop` — messages are accepted locally but not delivered to a broker

## TODO

- [ ] Add `scripts/deploy.sh` or remove CD deploy step
- [ ] Document production entrypoint with empty `DB_INIT_COMMAND`
- [ ] Wire `SHUTDOWN_DRAIN_DELAY` into bootstrap shutdown sequence
- [ ] Document PostgreSQL and backup procedures for production
- [ ] Add Redis to local Compose when cache integration tests are needed

## Related documentation

- [database.md](database.md)
- [configuration.md](configuration.md)
- [security.md](security.md)
