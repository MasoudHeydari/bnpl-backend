# Runtime and DB Admin build/deployment

Two documents are shipped: `configs/runtime.yaml` and `configs/dbadmin.yaml`.
Environment values are expanded through the existing mechanism; Development and
Production use these same schemas. Runtime keeps all supported business providers,
policies and defaults. DB Admin validates only database/version/environment/logging
and migration settings, resolving credentials only for the selected command.
Production builds cannot perform Development reset. A production init requires
`APP_ENVIRONMENT=production` and refuses an existing schema, without dropping it.

## Local artifacts and commands

- `make build-development`: `build/bin/bnpl-development`; copies the compatibility
  executable `build/bin/bnpl` used by `make run` and existing local scripts.
- `make build-production`: `build/bin/bnpl-production` (does not overwrite the
  Development compatibility artifact).
- `make build-dbadmin-development`: `build/bin/bnpl-dbadmin-development`.
- `make build-dbadmin-production`: `build/bin/bnpl-dbadmin-production`.

Each compiles the corresponding `./cmd/bnpl` or `./cmd/bnpl-dbadmin` entrypoint with
`go_json nomsgpack development|production`, CGO disabled, trimpath and existing
linker settings. Runtime has no db/init/migrate commands. DB Admin has no serve,
policy-import, payment or business worker capability. Both default help/unknown
commands are safe; DB Admin's Docker default is `--help`, never initialization.
The business `policy-assignment import-trusted` command remains with Runtime and
verifies the normal connection's safety before executing.

Lifecycle targets: `db-init-development`, `db-init-production`,
`db-migrate-development`, `db-migrate-production`. `init-dev`/`init-prod` are
compatibility aliases to DB Admin. Development initialization is destructive only
in the Development build and matching environment; Production initialization is
non-destructive. Use migration for an existing schema.

Discoverability: `db-invariants-list`, `db-privileges-list` are offline;
`db-invariants-verify-development|production` and
`db-privileges-verify-development|production` use the same read-only verifier.
`db-invariants-verify` aliases Development verification. `CONFIG` overrides the
DB Admin template path; `RUNTIME_CONFIG` selects the template for `make run` and the business import target.
Explicit CLI `--config` wins over CONFIG_FILE. Existing .env loading remains;
use `make -e` or command-line assignments when overriding .env with an isolated
test environment. `make env` now prints names with redacted values.

`make db-invariants-test` requires real disposable PostgreSQL, with
`COLLECTIONS_TEST_DATABASE_URL` and `DATABASE_SECURITY_TEST_URL`. Security tests
create/drop uniquely named test databases and provision the four roles; use a
**dedicated cluster**, never production or a shared project DB. The restricted
payment integration additionally uses `COLLECTIONS_RUNTIME_TEST_URL` for the
actual test bnpl login. Dependency absence is failure, never a successful skip.

## Four local and published image combinations

With default IMAGE_NAME=bnpl-backend and IMAGE_VERSION=v0.1.0:

- `build-image-development`: `bnpl-backend:v0.1.0-development`.
- `build-image-production`: `bnpl-backend:v0.1.0-production`.
- `build-dbadmin-image-development`: `bnpl-backend-dbadmin:v0.1.0-development`.
- `build-dbadmin-image-production`: `bnpl-backend-dbadmin:v0.1.0-production`.

`publish-image-development|production` and
`publish-dbadmin-image-development|production` explicitly build/tag/push the
matching image to the existing
`registry.hamdocker.ir/parstis-technology/<name>:<version>-<environment>` convention.
IMAGE_VERSION, IMAGE_NAME, DBADMIN_IMAGE_NAME, registry and namespace are overridable.
Build targets do not push. Legacy `build-image`/`publish-image-to-hamravesh` keep
the Production Runtime `<name>:<version>` compatibility tag only.

One multi-stage Dockerfile has `runtime` and `dbadmin` finals and BUILD_ENV as its
compile-time environment dimension. Both run as unprivileged bnpluser with CA
certificates and tzdata. Runtime contains only bnpl, runtime.yaml and the existing
startup script. It contains no admin binary/config or migration/audit assets.
DB Admin contains only bnpl-dbadmin, dbadmin.yaml, migrations and read-only audits;
it has no Runtime executable/config or business entrypoint. The baseline is
embedded only in the admin binary. Credentials are injected, never copied into
an image. .dockerignore excludes .env, pgpass, private keys and local credentials.
Optional business import CSV files and TLS secrets must be mounted explicitly.

## Production Job sequence

For a fresh environment, provision the database plus admin credential using the
platform, then run a Production DB Admin Kubernetes Job with its isolated secrets:
`bnpl-dbadmin --config configs/dbadmin.yaml db init-prod`. This provisions roles,
creates owner-owned schema, initializes the frozen baseline and applies the
existing incremental chain using migrator/owner. Run a separate verification Job:
`... db invariants verify`. Only after success deploy Runtime with the normal
PGPass secret and business configuration, then check readiness.

For an existing environment first follow the non-destructive ownership plan/apply
procedure in [database-roles.md](database-roles.md). Audit financial invariants and
resolve historical findings explicitly before migration. Drain old writers before
ownership/ACL tightening. An old/new rolling overlap is not compatible with the
old permission DELETE path or the new strict startup verifier. Keep a maintenance
window for catalog/table DDL locks; restore only compatible writers after Job
verification. No production target is an implicit destructive default.

Runtime Deployments must not mount admin/migrator/provisioning secrets. Admin Jobs
must not mount Kafka/Redis/Financial Core secrets or run persistent workers.
Restrict network and RBAC to the selected Job's needs. Failed Job errors close
pools; the process exits. No startup grants, migration auto-run or repair flag is
provided. A rollback to old code must be evaluated against the tightened ACL and
permission-history schema, not achieved by disabling invariants.

Runtime may create session-local temporary tables. This does not grant permission
to create or alter objects in the managed `bnpl` schema or create schemas in
the database. Keep Runtime database CREATE and schema CREATE privileges denied;
no database-wide `REVOKE TEMPORARY FROM PUBLIC` is required.
