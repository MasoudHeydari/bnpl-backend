# Operations

## Prerequisites

| Tool | Version / notes |
| --- | --- |
| Go | 1.26.2 (`go.mod`) |
| Docker + Compose | Local PostgreSQL and Kafka |
| PostgreSQL | Reachable at configured host/port for app and CLI |

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

### 2. Infrastructure

```bash
make up
```

Starts via `docker-compose.local.yml`:

- PostgreSQL — `127.0.0.1:5432`
- Kafka — `127.0.0.1:9092` (not used by Go code today)

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

1. Executes `bnpl db ${DB_INIT_COMMAND:-init-dev}` (**destructive**)
2. `exec bnpl` — starts HTTP server

For repeated local runs without re-init, invoke the binary directly:

```bash
./build/bin/bnpl
```

## Makefile Targets

### Build / run

| Target | Action |
| --- | --- |
| `build` | Compile to `build/bin/bnpl` |
| `run` | `build` + `docker-entrypoint.sh` |
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

**Note:** No `*_test.go` files exist in the repository at present.

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
./build/bin/bnpl [--config path]              # HTTP server
./build/bin/bnpl db init-dev                  # destructive bootstrap
./build/bin/bnpl db init-prod                 # destructive bootstrap
./build/bin/bnpl db migrate                   # filesystem migrations (partially wired)
```

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

| Step | Command | Notes |
| --- | --- | --- |
| 1 | `bnpl db $DB_INIT_COMMAND` | Default `init-dev`; fails fast on error (`set -e`) |
| 2 | `exec bnpl` | HTTP server as PID 1 |

Override for production experiments:

```bash
docker run -e DB_INIT_COMMAND=init-prod -e APP_EXECUTABLE=./bnpl ...
```

**Warning:** Both init commands are destructive. Production should eventually replace step 1 with `db migrate` or an external migration job.

See ADR [0006](adr/0006-docker-entrypoint-startup-flow.md).

## Docker Compose

File: `docker-compose.local.yml`

- App runs on **host** (`make run`), not in Compose
- DB credentials from `.env` (`POSTGRES_USER`, `POSTGRES_PASSWORD`)
- Application connects via `POSTGRES_HOST=127.0.0.1` and pgpass lines

Stale references to `database.pass-dir` have been removed from Compose comments.

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

- Default container entrypoint destroys schema on every start
- No `deploy.sh` in repo despite CD reference
- `db migrate` not operational until config wiring and migration files exist
- No health endpoint for load balancer probes
- Kafka started locally but unused by application
- No automated rollback documented

## Troubleshooting

### `config.Load(...): reading config file`

- Verify `CONFIG_FILE` or pass `--config`
- Ensure `configs/config.yaml` exists (copied in Docker image)

### `no matching password line`

- Align pgpass lines with `POSTGRES_HOST`, `POSTGRES_PORT`, `POSTGRES_DB`, role names

### Pool creation fails

- Run `make up`; check `docker compose ps`
- Verify PostgreSQL on `127.0.0.1:5432`

### `db migrate` fails

- Expected until `MIGRATIONS_FILES_DIR` is wired and directory exists
- Use `init-dev` for greenfield schema only

### HTTP server starts but no API routes

- Expected — see [api.md](api.md)

### Container wipes database on restart

- Default `DB_INIT_COMMAND=init-dev` in entrypoint
- Override entrypoint or env for steady-state deployments

## TODO

- [ ] Add `scripts/deploy.sh` or remove CD deploy step
- [ ] Document production entrypoint without destructive init
- [ ] Add health check endpoint for orchestrators
- [ ] Document PostgreSQL and backup procedures for production

## Related documentation

- [database.md](database.md)
- [configuration.md](configuration.md)
- [security.md](security.md)
