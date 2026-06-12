# ADR 0006: Docker Entrypoint Startup Flow

## Status

Accepted

## Context

Container deployments need a deterministic startup sequence: initialize or verify database state, then start the long-running HTTP process. The `Dockerfile` copies the binary, `configs/`, and `scripts/docker-entrypoint.sh`, running as non-root user `bnpluser`.

Without a documented entrypoint contract, operators might assume the container only starts HTTP, or might run destructive bootstrap unintentionally in production.

## Decision

Use `scripts/docker-entrypoint.sh` as `CMD` with a two-step flow:

1. **Database initialization** — `"${APP_EXECUTABLE:-./bnpl}" db "${DB_INIT_COMMAND:-init-dev}"`
2. **Application start** — `exec "${APP_EXECUTABLE:-./bnpl}"` (replaces shell as PID 1)

Environment variables:

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_EXECUTABLE` | `./bnpl` | Path to the compiled binary |
| `DB_INIT_COMMAND` | `init-dev` | Subcommand passed to `bnpl db` (`init-dev` or `init-prod`) |
| `CONFIG_FILE` | `configs/config.yaml` | Set in Dockerfile `ENV` |

The script uses `set -e` so bootstrap failure prevents the HTTP server from starting.

`Makefile` target `run` executes the same entrypoint on the host (after `make build`), not only inside Docker.

## Consequences

**Positive**

- Fresh containers self-bootstrap without a separate init container, suitable for development and greenfield staging.
- `exec` ensures SIGTERM reaches the Go process for graceful shutdown.
- `DB_INIT_COMMAND` allows production-style bootstrap without rebuilding the image.

**Negative**

- **Default `init-dev` is destructive** — every container start drops and recreates the `bnpl` schema unless `DB_INIT_COMMAND` is overridden or entrypoint is replaced.
- Bootstrap requires admin PostgreSQL credentials in config; the entrypoint does not wait for PostgreSQL readiness (depends on orchestrator restart policy).
- Host `make run` also runs destructive bootstrap before the server, which may surprise developers who already initialized the database.

**Operational impact**

- Production deployments must override `DB_INIT_COMMAND` carefully or replace the entrypoint entirely once non-destructive migrations exist.
- Database must be reachable before the container starts step `[1/2]`.
- Logs print config path, init command, working directory, and user at startup.

## Alternatives Considered

**Init container / Kubernetes Job for bootstrap only**

- Preferred for future production; not implemented in this repository. Entrypoint bootstrap is acceptable for early-stage deployments only.

**HTTP server only (no bootstrap in entrypoint)**

- Rejected for developer experience: requires manual `init-dev` before first `docker run`.

**Separate migration sidecar on every deploy**

- Deferred until `db migrate` is fully wired and non-destructive.

## Relationship To Future BNPL Domains

BNPL production rollouts will require:

1. Readiness checks that verify schema version and critical reference data before serving credit decisions.
2. Non-destructive migration application on deploy (`db migrate`), not schema drop.
3. Optional seed/job commands separated from every pod restart.

The entrypoint should evolve to:

- `db migrate` (incremental) on deploy
- `bnpl` HTTP start only on steady state
- Batch jobs (collections, reconciliation) as separate CronJob invocations

Current entrypoint is a **development and greenfield** convenience, not a production schema evolution strategy.
