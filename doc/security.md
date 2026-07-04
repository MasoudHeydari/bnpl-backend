# Security

This document describes how sensitive data is handled in the current codebase, what protections exist, what does **not** exist, and how operators should manage secrets. It does not overstate security guarantees.

## Scope and trust boundaries

This codebase implements three distinct authentication surfaces:

1. **Trusted Gateway**: ordinary `/api/v1` customer/merchant/admin routes
   use `AuthContext.Attach` and permission checks. BNPL reads
   `X-Auth-Subject-Id`, `X-Auth-Tenant-Id`, `X-Auth-Context-Id` and
   `X-Auth-Permissions`; it does **not** independently parse or validate JWTs.
2. **Partner checkout**: `/api/v1/partner` authenticates
   `X-Terminal-Id`/`X-API-Key`, independently of Gateway identity headers.
3. **Hosted checkout**: `/api/v1/checkout-sessions/:token` uses its
   public session token and OTP as appropriate. This surface is intentionally
   outside the Gateway-authenticated route group.

### Required production deployment boundary

**Do not release** unless all the following have been independently verified
with the actual deployed Gateway/network configuration:

- Only the trusted Envoy/Gateway can access the BNPL business listener;
  untrusted clients, sibling workloads and public networks cannot bypass it.
- Gateway validates inbound JWTs, **removes client-supplied `X-Auth-*` headers**,
  and injects authenticated subject/tenant/context/permissions values.
- Traffic from Gateway to BNPL cannot be impersonated by another workload;
  enforce network restrictions and an appropriate workload identity mechanism
  as required by the deployment topology.
- Test negative requests directly against the application endpoint from an
  untrusted source and confirm denial at the network trust boundary.
- Do not mistake valid UUID header syntax for authenticated identity.

Kubernetes NetworkPolicy/Cilium, firewall/service exposure and Envoy filter
configuration are **external deployment controls**; their correct installation
cannot be proved from the Go source alone. TLS to Gateway clients and safe
Gateway-to-BNPL transport must be verified separately.

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
| `configs/runtime.yaml` / `configs/dbadmin.yaml` | Separate Runtime and DB Admin config files with `${VAR}` expansion |
| Runtime and DB Admin images | Separate least-privilege binaries and config artifacts |

### Security configuration

Runtime config validation rejects unsafe production provider choices
(including insecure external gRPC transports and development-only HTTP debug
logging). Never expose admin/DB provisioning secrets to Runtime. Environment
expansion is not a secret store; use controlled injection and access policies.

## PostgreSQL Role Separation

Production uses separate `bnpl_owner`, `bnpl_migrator`, and restricted
Runtime roles. Schema and privileged DDL belong to `cmd/bnpl-dbadmin`
and its separately deployed administration job; business Runtime must not
have schema modification privileges or DB administration credentials.
Startup database invariant and privilege verification are part of the
first-release checklist. See
[database roles](architecture/database-roles.md) and
[database privileges](architecture/database-privileges.md).

## Logging and Sensitive Data

- The development-only HTTP debug logger is unredacted and must never be
  enabled against real customer data or production services.
- Production response logging uses an allowlist; raw response bodies and
  authentication secrets must not be included.
- Do not log terminal API keys, OTP values, tokenized checkout URLs,
  database passwords or Financial Core authentication credentials.
- Audit records use transactional outbox messages and stable trace /
  correlation identities. Logging is not an authoritative payment ledger.

## Docker and Deployment Risks

| Risk | Detail |
| --- | --- |
| Destructive `init-dev` | Separate DB Admin development build only; production init refuses existing schema |
| Secrets in env vars | Standard for containers but visible to anyone with `docker inspect` or pod exec access |
| Trusted gateway and transport | Gin relies on ingress TLS and trusted Gateway controls; verify actual network exposure and inter-service protection before release |
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
- Assume application-layer UUID header validation authenticates the Gateway or prevents direct-to-app spoofing

## Merchant terminal API keys (show-once)

Partner runtime credentials (`X-Terminal-Id` / `X-API-Key`) follow a **show-once** secret model:

- Plaintext `api_key` is returned only from Merchant Admin Panel **Provision** and **Rotate** responses (`Cache-Control: no-store`), and from an idempotent replay of that same issuance request within the configured replay window.
- Persistent storage keeps only an irreversible keyed HMAC verifier (`api_key_hash`). Plaintext is never written for long-term retrieval outside the short-lived encrypted issuance delivery used for idempotent replay.
- `GET /api/v1/merchant/terminal` returns terminal and credential **metadata only** — never the secret.
- Merchants must copy the issued key into their own secure store. Outside the issuance replay window a lost key cannot be recovered from BNPL; use **Rotate** (optionally `grace_period_seconds=0` for immediate retirement of the previous key).
- API keys must not appear in logs, traces, metrics, or error messages.

See [doc/api.md](api.md#partner-terminal-authentication) for the HTTP surface.

## Future Security Requirements

Additional compliance controls such as field-level encryption, regulatory
retention, abuse analytics and incident-response workflows depend on the
deployment and regulatory scope. They are not substitutes for the required
Gateway trust boundary, permissions, terminal auth and PostgreSQL financial
invariants already implemented.

See [architecture](architecture.md) and [operations](operations.md) for release validation.
