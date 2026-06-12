# ADR 0005: Single Binary With Cobra Commands

## Status

Accepted

## Supersedes

ADR title previously recorded as "Unified CLI for HTTP and Database Operations" (`0005-cli-http-and-db-entrypoints.md`).

## Context

Operators need multiple runtime modes from one deployable artifact:

| Mode | Command | Current behavior |
| --- | --- | --- |
| HTTP server | `bnpl` (default) | Loads config, opens normal-role pool, starts Gin |
| Dev bootstrap | `bnpl db init-dev` | Destructive schema reset + embedded dev SQL |
| Prod bootstrap | `bnpl db init-prod` | Destructive schema reset + embedded prod SQL |
| Migrations | `bnpl db migrate` | Applies filesystem SQL files sequentially |

All modes share configuration loading (`config.Load`), PostgreSQL adapter wiring, and the same binary produced by `cmd/bnpl/main.go` → `command.Execute()`.

## Decision

Use one Cobra command tree rooted at `bnpl`:

```
bnpl                    # default: startWebServer
bnpl db init-dev        # InitDBUseCase.InitDev
bnpl db init-prod       # InitDBUseCase.InitProd
bnpl db migrate         # migrationsuc.UseCase.MigrateAll
```

Persistent flag `--config` / `-c` and `CONFIG_FILE` env var select the YAML file for all subcommands.

The default command starts the HTTP server without requiring an explicit `serve` subcommand. This matches Docker entrypoint behavior (`exec bnpl` after bootstrap).

## Consequences

**Positive**

- One image, one binary, one config format for all operational tasks.
- Dependency wiring stays in `pkg/adapter/config` rather than duplicated across separate binaries.
- Kubernetes or systemd can use the same container with different args (`bnpl db migrate`) if entrypoint is overridden.

**Negative**

- Invoking `bnpl` with no arguments starts the web server, which can surprise operators expecting a help-only root command.
- Default command opens a DB pool even though no HTTP handlers use it yet (`routes.Register` ignores the pool parameter).
- CLI commands print full config to stdout (`fmt.Printf("configs: %v\n", c)`), which is noisy and may expose configuration structure in logs.

**Operational impact**

- `Makefile` targets (`run`, `init-dev`, `init-prod`) wrap CLI subcommands.
- Docker `CMD` runs `docker-entrypoint.sh`, which calls `bnpl db init-*` then `exec bnpl`.
- Exit code 1 on any command failure via `command.Execute()` → `os.Exit(1)`.

## Alternatives Considered

**Separate binaries (`bnpl-server`, `bnpl-migrate`)**

- Rejected: duplicates config loading and increases image size and release coordination.

**Explicit `bnpl serve` subcommand as default**

- Rejected: extra operator step; Docker and Makefile would need updating; current default matches container entrypoint.

**Database tools as shell scripts only**

- Rejected: would bypass typed config and transaction boundaries implemented in Go use cases.

## Relationship To Future BNPL Domains

Additional subcommands will likely appear as operational needs grow:

- `bnpl db migrate-status` — inspect applied migrations
- `bnpl jobs run-collections` — batch delinquency processing
- `bnpl admin recompute-limits` — maintenance tasks

Business HTTP APIs remain separate from CLI; Cobra commands are for bootstrap, migrations, and batch operations, not merchant or borrower-facing traffic.

Future BNPL domains should expose use cases to **both** HTTP handlers and CLI/job entrypoints through shared constructors in `pkg/adapter/config`, preserving one wiring path.
