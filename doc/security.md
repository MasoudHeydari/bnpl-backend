# Security

This document describes how sensitive data is handled in the current codebase, what protections exist, what does **not** exist, and how operators should manage secrets. It does not overstate security guarantees.

## Scope

The repository is in a **bootstrap phase**. Security measures today focus on configuration redaction and PostgreSQL role separation. Authentication, authorization, encryption at rest, and PCI/regulatory controls for BNPL business data are **not implemented**.

## model.Secret

Location: `pkg/core/model/secret.go`

`Secret` wraps sensitive string values loaded from configuration (primarily pgpass-format database credential lines).

### Protections that exist

| Mechanism | Behavior |
| --- | --- |
| Redacted `String()` | Prints `Secret{hash-prefix...}`, never plaintext |
| Redacted JSON/YAML marshal | Outputs `REDACTED[hash...]` |
| SHA-256 hash storage | `Hash()` and `Verify()` compare without exposing value in logs |
| `Equal()` | Compares hashes without revealing either secret |

### Protections that do NOT exist

| Gap | Risk |
| --- | --- |
| Plaintext stored in memory | Process memory contains raw value until `Clear()`; no HSM or sealed storage |
| `Reveal()` unrestricted | Any caller can obtain plaintext; no audit log of reveal events |
| `Clear()` best-effort only | Go strings cannot be reliably zeroed; OS may swap memory |
| No encryption at rest for config | YAML file uses placeholders; secrets live in environment variables in plaintext |
| No rotation workflow | Password changes require manual coordination across DB and env |

### When Reveal() is used

`Database.GetRolePassword` and `ConnectionURL` (`pkg/adapter/config/database.go`) call `Reveal()` to build PostgreSQL URLs. This is required for connectivity and is the intentional escape hatch.

**Operational rule:** Do not add new `Reveal()` call sites without security review. Prefer passing connection URLs from a sealed factory rather than spreading reveal calls.

## Secret Handling Strategy

See ADR [0007-secret-handling-strategy.md](adr/0007-secret-handling-strategy.md).

Summary:

1. Secrets enter via environment variables (`POSTGRES_ADMIN_PGPASS`, `POSTGRES_NORMAL_PGPASS`).
2. YAML references them as `${VAR}` placeholders decoded into `model.Secret`.
3. The application does **not** read `.pgpass` files from disk.

## pgpass Usage

### Application format

Each pgpass entry is a single line:

```
host:port:database:user:password
```

Example from `.env.example`:

```
POSTGRES_ADMIN_PGPASS=127.0.0.1:5432:bnpl_db:admin:tHePaSsWoRd
POSTGRES_NORMAL_PGPASS=127.0.0.1:5432:bnpl_db:bnpl:tHePaSsWoRd
```

`GetRolePassword` builds prefix `{host}:{port}:{dbname}:{role}:` and matches against both admin and normal entries.

### Common failures

- Host or port mismatch (e.g. Docker internal hostname vs `127.0.0.1`)
- Database name mismatch
- Role name mismatch (`admin` vs `bnpl` — see `repo.AdminRole`, `repo.NormalRole`)
- Error: `no matching password line`

### Makefile `mkdir-pgpass`

Creates `dist/.db/.pgpass` with mode `600`. **The Go binary does not read this file.** It exists for optional manual PostgreSQL client use only.

Note: the Makefile historically wrote admin line with port `5452` (typo); corrected to `5432`.

## Configuration Security

| Item | Status |
| --- | --- |
| `.env` gitignored | Expected — verify locally |
| `.env.example` | Placeholder credentials only |
| `.prod.env` | Untracked; **do not commit** production secrets |
| `configs/config.yaml` | No literal secrets; uses `${VAR}` expansion |
| `CONFIG_FILE` in Docker | Default `configs/config.yaml` baked into image |

### Wiring gaps with security impact

- `fmt.Printf("configs: %v\n", c)` in CLI commands prints configuration structure at startup (secrets redacted, but metadata visible).
- `config.Config.String()` uses JSON marshal; secrets redacted via `Secret.MarshalJSON`.
- Missing env vars expand to empty strings — can produce confusing auth failures rather than explicit "secret missing" errors.

## PostgreSQL Role Separation

| Role | Constant | Used for |
| --- | --- | --- |
| `admin` | `repo.AdminRole` | Schema drop/create, role DDL, grants |
| `bnpl` | `repo.NormalRole` | Runtime pool, bootstrap SQL, migrations |

**Intent:** Routine HTTP and migration traffic should use `bnpl`, not `admin`.

**Current gap:** Destructive init commands require admin credentials. Production runtime should not need admin access once schema exists.

## Logging Concerns

| Source | Issue |
| --- | --- |
| Gin logger middleware | Logs HTTP requests when `GIN_LOGGER=true`; no handlers yet, but future APIs must exclude sensitive headers and bodies |
| `log.Printf` on pool close failure | May leak connection errors with host details |
| PostgreSQL `log_statement` | `CreateRoleIfNotExists` embeds password in DDL; may appear in DB logs if statement logging is enabled |
| Structured `LOG_LEVEL` in `.env.example` | **Not wired** in Go code |

### TODO: Logging hardening

- Structured logging with field redaction
- Correlation IDs for audit trails
- Explicit policy for PII in logs (national ID, mobile, account numbers)

## Docker and Deployment Risks

| Risk | Detail |
| --- | --- |
| Destructive `init-dev` / `init-prod` | Explicit CLI/Make only — entrypoint never mutates schema |
| Secrets in env vars | Standard for containers but visible to anyone with `docker inspect` or pod exec access |
| No TLS termination in app | Gin listens plain HTTP; TLS expected at ingress (not documented in repo) |
| Missing `scripts/deploy.sh` | CD workflow references deploy script not in repository |

## Operational Checklist

**Do**

- Keep `.env` and `.prod.env` out of version control
- Use distinct passwords for admin and application roles in production
- Inject secrets via orchestrator secret stores in deployed environments
- Restrict admin credentials to migration/init jobs, not long-running app pods
- Deploy images by immutable SHA tag

**Do not**

- Run `init-dev` against databases containing real customer data
- Run `init-prod` when schema `bnpl` already holds customer data (it refuses, but migrate is the correct path)
- Commit pgpass lines with production passwords
- Assume `model.Secret` encrypts data at rest or in transit
- Rely on Kafka or Redis settings in `.env.example` for security boundaries (unused in Go code)

## Merchant terminal API keys (show-once)

Partner runtime credentials (`X-Terminal-Id` / `X-API-Key`) follow a **show-once** secret model:

- Plaintext `api_key` is returned only from Merchant Admin Panel **Provision** and **Rotate** responses (`Cache-Control: no-store`), and from an idempotent replay of that same issuance request within the configured replay window.
- Persistent storage keeps only an irreversible keyed HMAC verifier (`api_key_hash`). Plaintext is never written for long-term retrieval outside the short-lived encrypted issuance delivery used for idempotent replay.
- `GET /api/v1/merchant/terminal` returns terminal and credential **metadata only** — never the secret.
- Merchants must copy the issued key into their own secure store. Outside the issuance replay window a lost key cannot be recovered from BNPL; use **Rotate** (optionally `grace_period_seconds=0` for immediate retirement of the previous key).
- API keys must not appear in logs, traces, metrics, or error messages.

See [doc/api.md](api.md#partner-terminal-authentication) for the HTTP surface.

## Future BNPL Security Requirements

Not implemented; planned as domains are added:

- Authentication and authorization for merchant and admin APIs
- Audit log for sensitive state changes (credit limits, manual payment marks)
- Field-level encryption for PII where required
- Webhook signature verification for payment callbacks
- Rate limiting and fraud controls on checkout decision endpoints
- Regulatory data retention and right-to-erasure workflows

Track implementation gaps in [doc/architecture.md](architecture.md#open-gaps).
