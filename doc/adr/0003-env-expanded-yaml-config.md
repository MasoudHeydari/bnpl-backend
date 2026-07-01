# ADR 0003: Environment-Expanded YAML Configuration

## Status

Accepted

## Context

The service must configure PostgreSQL pools, HTTP server settings, and use case parameters across local development, CI, Docker, and future deployed environments. Hardcoding values in Go or maintaining separate config files per environment would diverge quickly.

The current loader in `pkg/adapter/config/config.go`:

1. Reads a YAML file (default `configs/config.yaml`).
2. Expands `${VAR}` placeholders via `os.ExpandEnv`.
3. Decodes into typed structs (`Database`, `Gin`, `Usecases`, `Infrastructure`).

`.env.example` documents the expected environment variables; the Makefile includes `.env` for all targets.

## Decision

Keep YAML as the canonical configuration **shape** and inject environment-specific **values** through `${VAR}` expansion before decoding.

Config path resolution (`cmd/bnpl/bootstrap.FixConfigPath`, called from Cobra `OnInitialize`):

1. `--config` flag
2. `CONFIG_FILE` environment variable
3. Default `configs/config.yaml`

Docker sets `CONFIG_FILE=configs/config.yaml` in the `Dockerfile`.

## Consequences

**Positive**

- `configs/config.yaml` documents structure in one place (database, gin, usecases, infrastructure sections).
- The same binary and config file work locally and in containers; only environment variables change.
- Secrets can be injected at runtime without rebuilding (via `POSTGRES_*_PGPASS` env vars).

**Negative**

- Missing environment variables expand to empty strings silently; there is no post-decode validation layer for required operational fields.
- `config.Load` is not responsible for printing config; `bootstrap.InitDev` / `InitProd` still print via `fmt.Printf("configs: %v\n", c)`.

### Amendment (2026-06-27)

The following gaps from the original ADR are **resolved**:

- `gin.trusted_proxies` — decoded and passed to the Gin adapter.
- `usecases.migration.files-dir` — decoded into `config.Migrations.filesDir` via `yaml:"files-dir"`.
- Kafka, Redis, logging, task, and clock settings — wired under `infrastructure:` (see ADR [0009](0009-provider-based-infrastructure-configuration.md)).

### Amendment (2026-06-30)

The following gaps are **resolved**:

- `versions.config` and `versions.database` — decoded and validated at `config.Load` against binary-supported semver (see `pkg/adapter/config/vers.go`).
- Circuit breaker, shutdown, and health-check settings — wired under `infrastructure:` (see ADR [0009](0009-provider-based-infrastructure-configuration.md) and [0014](0014-health-probes-and-graceful-shutdown.md)).

Remaining gaps: no post-decode validation for empty required env-expanded fields; `SHUTDOWN_DRAIN_DELAY` decoded but not yet used in bootstrap shutdown timing.

**Operational impact**

- Operators must ensure all `${POSTGRES_*}` and `${GIN_*}` variables are set before startup.
- pgpass lines must match host, port, database, and role exactly or `GetRolePassword` returns `no matching password line`.
- Docker deployments need secrets mounted or injected as environment variables alongside `configs/config.yaml`.

## Alternatives Considered

**Environment variables only (12-factor, no YAML file)**

- Rejected: loses structured grouping and makes nested settings (pool tuning, use case sections) harder to document.

**Viper with automatic env binding**

- Rejected for now: current codebase uses a minimal `yaml.v3` + `os.ExpandEnv` approach with no extra dependency; sufficient for bootstrap phase.

**Separate config files per environment (`config.dev.yaml`, `config.prod.yaml`)**

- Rejected: duplicates structure; env expansion achieves the same with one schema.

## Relationship To Future BNPL Domains

BNPL configuration will grow beyond database and HTTP settings:

- Credit policy thresholds, fee rates, delinquency rules
- Kafka topic names, Redis cache TTLs
- Feature flags for merchant-specific products

The YAML + env expansion pattern supports new top-level sections (for example `decision:`, `collections:`) without changing the loading mechanism. A validation layer should be added before production BNPL workflows go live to catch missing or invalid financial parameters.

Planned domains must not rely on silent empty-string expansion for monetary or regulatory settings.
