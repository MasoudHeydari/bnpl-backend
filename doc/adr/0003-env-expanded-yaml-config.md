# ADR 0003: Environment-Expanded YAML Configuration

## Status

Accepted

## Context

The service must configure PostgreSQL pools, HTTP server settings, and use case parameters across local development, CI, Docker, and future deployed environments. Hardcoding values in Go or maintaining separate config files per environment would diverge quickly.

The current loader in `pkg/adapter/config/config.go`:

1. Reads a YAML file (default `configs/config.yaml`).
2. Expands `${VAR}` placeholders via `os.ExpandEnv`.
3. Decodes into typed structs (`Database`, `Gin`, `Usecases`).

`.env.example` documents the expected environment variables; the Makefile includes `.env` for all targets.

## Decision

Keep YAML as the canonical configuration **shape** and inject environment-specific **values** through `${VAR}` expansion before decoding.

Config path resolution (`cmd/bnpl/command/root.go`):

1. `--config` flag
2. `CONFIG_FILE` environment variable
3. Default `configs/config.yaml`

Docker sets `CONFIG_FILE=configs/config.yaml` in the `Dockerfile`.

## Consequences

**Positive**

- `configs/config.yaml` documents structure in one place (database, gin, usecases sections).
- The same binary and config file work locally and in containers; only environment variables change.
- Secrets can be injected at runtime without rebuilding (via `POSTGRES_*_PGPASS` env vars).

**Negative**

- Missing environment variables expand to empty strings silently; there is no post-decode validation layer.
- Some YAML keys are not wired to Go structs today:
  - `gin.trusted_proxies` — ignored; Gin adapter hardcodes `127.0.0.1` (`pkg/adapter/restful/gin/gin.go`).
  - `usecases.migration.files-dir` — `config.Migrations.migDirPath` is unexported without a `yaml` tag (`usecase.go`), so `db migrate` receives an empty directory path.
  - `versions` section — present in YAML but not decoded.
- `config.Load` prints the full config in CLI commands via `fmt.Printf("configs: %v\n", c)`, which exposes struct layout (secrets are redacted via `model.Secret.String()` but operational detail still leaks).

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
