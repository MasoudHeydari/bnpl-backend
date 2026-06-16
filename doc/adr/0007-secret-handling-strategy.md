# ADR 0007: Secret Handling Strategy

## Status

Accepted

## Context

PostgreSQL access requires admin credentials for schema bootstrap and a restricted application role (`bnpl`) for runtime queries. Credentials must not appear in logs, config dumps, or version control.

The codebase introduces `model.Secret` (`pkg/core/model/secret.go`) for pgpass-format lines in configuration. Operators also maintain Docker Compose credentials (`POSTGRES_USER`, `POSTGRES_PASSWORD`) separate from application pgpass entries.

## Decision

**Configuration-time secrets via pgpass-format lines**

Store credentials in YAML as `model.Secret` values expanded from environment variables:

```yaml
pgpass_entries:
  admin: ${POSTGRES_ADMIN_PGPASS}
  normal: ${POSTGRES_NORMAL_PGPASS}
```

Format: `host:port:database:user:password`

`Database.GetRolePassword` (`pkg/adapter/config/database.go`) matches prefix `host:port:db:role:` against admin and normal entries and extracts the password suffix.

**Redaction at serialization boundaries**

`Secret` implements safe `String`, `MarshalJSON`, `MarshalYAML`, and `Format` — outputs show `REDACTED[hash...]` or `Secret{hash...}`, never the raw value.

**Explicit reveal for connection only**

`Secret.Reveal()` returns the plaintext password. Used only when building connection URLs (`ConnectionURL`). Documented as security-sensitive.

**No runtime `.pgpass` file**

The Go application does not read `~/.pgpass` or `dist/.db/.pgpass`. The Makefile `mkdir-pgpass` target creates a file for optional manual tooling but is not consumed by the binary.

## Consequences

**Positive**

- Accidental logging of config structs redacts secret values via `String()` / JSON marshaling.
- Password verification without storing plaintext comparisons (`Secret.Verify` uses SHA-256 hash).
- Environment injection works with Docker and Kubernetes secrets as env vars.

**Negative**

- **No encryption at rest** in config files; YAML on disk contains `${VAR}` placeholders only, but expanded env vars live in process memory in plaintext.
- **No secret rotation workflow** implemented; changing passwords requires coordinated updates to PostgreSQL, env vars, and pgpass lines.
- `Reveal()` returns plaintext with no audit trail; any code calling it can expose credentials.
- `Clear()` zeroes memory in Go but does not guarantee OS-level secure wiping.
- CLI startup prints config via `fmt.Printf("%v", c)` — secrets redacted, but pgpass line structure and host/db names are visible.
- DDL in `schemarp.CreateRoleIfNotExists` interpolates password into SQL string; visible in PostgreSQL logs if statement logging is enabled.
- Docker Compose `POSTGRES_PASSWORD` is separate from pgpass entries; misconfiguration can cause drift between bootstrap user and application roles.

**Operational impact**

- Operators must keep pgpass host, port, database, and role aligned with `database.host`, `database.port`, `database.name`, and role constants (`admin`, `bnpl`).
- `.env` must not be committed (gitignored); `.env.example` uses placeholder passwords.
- Production should inject secrets via orchestrator secret stores, not baked into images.

## Alternatives Considered

**Read standard PostgreSQL `.pgpass` file**

- Rejected: file permissions and mount complexity in containers; env-based lines work uniformly across local and deployed environments.

**Hashicorp Vault / cloud secret manager integration**

- Deferred: appropriate for production BNPL but not required for current bootstrap-only scope.

**Store only password env vars (no pgpass line format)**

- Rejected: pgpass format encodes matching context (host, port, db, user) reducing wrong-password selection across environments.

## Relationship To Future BNPL Domains

Financial platforms require broader secret categories:

- Payment gateway API keys
- Merchant webhook signing secrets
- PII encryption keys
- JWT signing keys for admin and merchant APIs

`model.Secret` provides a pattern for config-loaded credentials. Future work:

- Extend `Secret` usage to all sensitive config fields
- Add audit logging when `Reveal()` is called (or eliminate direct reveal via sealed connection factories)
- Integrate with a secret manager for rotation
- Never log request/response bodies containing PAN, national ID, or account numbers

Regulatory audit domains will require proving who accessed credentials and when; current implementation does not provide that.

See [doc/security.md](../security.md) for operational guidance.
