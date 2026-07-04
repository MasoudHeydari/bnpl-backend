# ADR 0006: Docker Entrypoint Startup Flow

## Status

Accepted (amended)

## Context

Container deployments need a deterministic way to start the long-running BNPL
HTTP process. The `Dockerfile` copies the binary, `configs/`, and
`scripts/entrypoint.sh`, running as non-root user `bnpluser`.

Database bootstrap and schema evolution are separate operator commands
(`bnpl db init-dev`, `bnpl db init-prod`, `bnpl db migrate`) and must not run
implicitly on every process start.

## Decision

Use `scripts/entrypoint.sh` as `CMD` with a **single** responsibility:

1. **Application start** — `exec "${APP_EXECUTABLE:-./bnpl}"` (replaces shell as PID 1)

Environment variables used by the entrypoint:

| Variable | Default | Purpose |
| --- | --- | --- |
| `APP_EXECUTABLE` | `./bnpl` | Path to the compiled binary |
| `CONFIG_FILE` | `configs/config.yaml` | Set in Dockerfile `ENV` |

The script logs a short startup banner (more detail when
`APP_ENVIRONMENT=development`) and then `exec`s the binary so SIGTERM reaches
the Go process for graceful shutdown.

`Makefile` target `run` executes the same entrypoint on the host (after
`make build`). Schema reset remains explicit via `make init-dev` /
`make init-prod`.

## Consequences

**Positive**

- Entrypoint never drops or mutates schema; accidental data loss on restart is avoided.
- `exec` ensures SIGTERM reaches the Go process for graceful shutdown.
- DB lifecycle stays an explicit CLI/Make step, matching production job patterns.

**Negative**

- First-time or greenfield environments require a separate `make init-dev` (or
  `bnpl db init-prod` / `bnpl db migrate`) before the app can serve traffic.

**Operational impact**

- Production schema evolution uses `bnpl db migrate` (or an init Job), not the
  container entrypoint.
- Logs print environment, config path, listen address, and (in development)
  additional operator context at startup.

## Alternatives Considered

**Init container / Kubernetes Job for bootstrap only**

- Preferred for production; aligns with keeping entrypoint HTTP-only.

**Optional `DB_INIT_COMMAND` in entrypoint (previous design)**

- Rejected: even an opt-in env var made every `make run` / container restart a
  footgun when set, and mixed process start with schema lifecycle.

**Separate migration sidecar on every deploy**

- Still valid for incremental `db migrate`; independent of this entrypoint.

## Relationship To Future BNPL Domains

BNPL production rollouts will require:

1. Readiness checks that verify schema version and critical reference data before serving credit decisions.
2. Non-destructive migration application on deploy (`db migrate`), not schema drop.
3. Optional seed/job commands separated from every pod restart.

The entrypoint stays HTTP-only. Migrations and seed jobs run as separate
commands or CronJobs.

### Amendment history

- **2026-06-27:** Empty `DB_INIT_COMMAND` skipped init; retries via
  `DB_INIT_MAX_ATTEMPTS` / `DB_INIT_RETRY_INTERVAL_SECONDS`.
- **2026-09-24:** Removed all entrypoint DB init. Schema bootstrap is only via
  `make init-dev` / `make init-prod` / `bnpl db migrate`.
