# Database security and entrypoint separation — implementation report

> Historical report from the initial refactor. The subsequent cleanup allows
> session-local TEMPORARY tables, simplifies developer tooling and updates
> SQL fingerprints after formatting. A later pre-release consolidation moved
> the five development-only migrations into the initial schema. This file is a
> historical implementation report, not the current installation procedure.
> Refer to the latest source and README.

Actual starting HEAD: `bdf5bd01dd5678e71a3465adc0b7d95b6482d3b1`.
Branch: `feat-add-decision-domain`. The remote was rechecked before publication;
there was no intervening branch advancement. No PR or merge into main is involved.
The resulting commit SHA is supplied with the delivery message.

## Architecture and configuration

There are independent `cmd/bnpl/main.go` and `cmd/bnpl-dbadmin/main.go` executables.
Runtime retains HTTP, business use cases, integrations, workers, readiness and the
business policy-assignment importer. Database CLI commands and privileged factories
have been removed from its command tree, configuration root, repository wiring and
use-case wiring. Its default is `configs/runtime.yaml` and its only PGPass entry is
normal. Requests for another database role fail before credential resolution.

DB Admin has its own bootstrap, commands and separately validated `dbadminconfig`
root using `configs/dbadmin.yaml`. It constructs only logger, selected DB pool,
Clock and the existing initialization/migration engine, then closes pools and exits.
It has no serve/business command or Runtime bootstrap import. Its safe default is
help. Config marker/version/environment validation prevents accepting the other
application's document. `--config/-c` and CONFIG_FILE overrides remain; each binary
has its own default. Administrative operations require no business provider secrets.
Only the selected credential is resolved. No owner credential exists.

Credential-free PostgreSQL transport settings are shared through the existing
adapter layer's focused `postgres/connection` package. Runtime query logging retains
its behavior. Admin tracing is always disabled and enabling it in admin config is
rejected. Expanded YAML, invalid connection strings, DDL passwords and PGPass values
are excluded from operational errors. Role identifiers and password literals are
safely quoted. Existing role passwords are retained; rotation is explicit.

## Roles, ownership and privileges

[Database roles](architecture/database-roles.md) records exact attributes and the
upgrade procedure. The four typed constants remain in `pkg/core/repo/role.go`:
admin LOGIN for provisioning, NOLOGIN bnpl_owner for object ownership,
bnpl_migrator LOGIN/NOINHERIT with only controlled owner membership, and bnpl LOGIN
with no memberships or privileged attributes. PostgreSQL 16+ membership INHERIT/SET
options are configured explicitly; role-level NOINHERIT alone is insufficient.
Every physical migrator connection assumes owner before any transaction or DDL.

Fresh initialization uses the existing frozen initializer and incremental engine
under migrator/owner, never normal. Existing schemas use explicit `db ownership
plan` and `apply`: read-only inventory/preflight, scoped transactional transfer,
the existing advisory lock, no REASSIGN OWNED, no deletion. Indexes and attached
identity sequences follow their table; standalone sequences, functions, domains,
enums and independent composite types are handled. Unrelated schema/data are not
transferred. Unsupported or extension-owned objects, unsafe role attributes,
missing transfer authority and platform DDL privileges are actionable blockers.
A repeat application is safe. Non-superuser admin initialization/migration was
also exercised on a separate cluster with CREATEROLE/CREATEDB, without SUPERUSER.

Runtime has no managed ownership, schema/database CREATE, DELETE,
TRUNCATE, REFERENCES, TRIGGER or grant-option privileges. Its effective rights are
checked across all 39 registered tables and every column. The complete exact
matrix and exceptions are in [database-privileges](architecture/database-privileges.md)
and the typed `invariants/privilege_registry.go` metadata.

Decisions deliberately retain table UPDATE for Checkout locking; they are not
claimed database-immutable. Other mutable tables have explicit update-column
allowlists. Narrow key-column locking exceptions are documented for allocation,
merchant, terminal and reminder queries. Terminal permission replacement also
uses a key-preserving parent write to serialize old repeatable-read snapshots.
Financial identity is not arbitrarily updateable, and existing financial triggers
provide independent cross-table protection.

Migration `000004_terminal_permission_history.sql` adds revoked_at plus active-only
uniqueness. Authorization reads only active assignments. Replacement validates all
input, serializes on the terminal parent, revokes removed grants and inserts
missing grants in the caller's transaction. Regrant preserves old revoked rows;
concurrent replacements cannot combine stale authorization sets. No DELETE occurs.

Migration `000005_runtime_privileges.sql` installs explicit grants, removes old
PUBLIC/normal table and column grants, and establishes owner-created future-object
default deny. The registry never installs privileges. Changing policy requires a
new reviewed migration and updated verification contract. Unknown tables fail
verification and receive no automatic business access.

## Verification and lifecycle

The existing trigger/function hashes and migration 000003 are unchanged. Runtime
runs `VerifyRuntime` on the normal connection before external client/use-case
composition, listeners or workers. It checks identity, original triggers, role and
ownership safety, actual effective table/column/sequence/function privileges,
PUBLIC and membership paths, default ACLs, permission-history schema and required
migration checksums. Function overloads and column-level drift are detected.
No DDL or automatic repair runs at startup. A real NewApp test verifies clean
startup, fail-closed behavior on DELETE/trigger drift and zero leaked normal-role
connections after failure. DB Admin uses the same read-only checks and adds safe
role/ACL/ownership inventory. Public catalogs suffice; no extra Runtime admin
permissions were introduced. PostgreSQL enforces permissions per statement;
verification detects drift rather than intercepting SQL.

The migration engine's advisory locking, checksums, dirty detection,
transactional/nontransactional support and production safety remain intact.
Real migrator tests cover complete fresh chain, legacy upgrade, repeated runs,
valid-format checksum mismatch and dirty history refusal. Production init refuses
an existing schema without mutation. Development reset remains build- and
environment-gated.

## Builds, Docker and Make

[Build/deployment guide](architecture/database-deployment.md) lists every target,
artifact and image name. The four binary artifacts are bnpl-development,
bnpl-production, bnpl-dbadmin-development and bnpl-dbadmin-production under
build/bin. Development also maintains the bnpl compatibility executable.
All use `go_json nomsgpack` plus the matching environment, CGO disabled, trimpath
and existing linker flags. There is no new capability build tag.

One Dockerfile has runtime/dbadmin final targets, BUILD_ENV selection, CA/tzdata
and an unprivileged OS user. Runtime contains only its executable/template and
existing entrypoint. DB Admin contains only its executable/template plus migration
assets. The previously bundled standalone audit SQL files were subsequently
removed. No .env, pgpass or credential ENV is baked in. Default DB Admin
CMD is help. Both compiled dependencies and actual image binaries/build tags were
inspected; separation is not merely command-name filtering.

Preserved/extended targets include build-development/production, run, init-dev/
init-prod, policy-assignment-import-trusted, test/codegen targets, build-image and
publish-image-to-hamravesh. Added DB Admin build/image/publishing variants, four DB
lifecycle targets, offline invariant/privilege list, environment-specific verify,
verify-entrypoint-boundaries and verify-docker-boundaries. The old invariant test
target remains real-PG/race and fails for missing dependencies. Building never
pushes. Four explicit publication targets retain the existing Hamravesh registry,
namespace and IMAGE_VERSION convention, with distinct executable/environment tags.

## Final verification results

Executed results are recorded below. Logs are retained outside the repository in
the task workspace. All database writes used dedicated disposable clusters and
unique databases/schemas. Shared/production project data was not migrated.


- `go test -tags development ./...`: PASS, all packages.
- `go test -tags production ./cmd/... ./pkg/adapter/dbadminconfig ./pkg/core/usecase/migrationsuc`: PASS. Existing bootstrap fixtures now use a permitted in-process scheduler in production and assert Swagger remains absent there; production guards were not relaxed.
- `go test -tags production ./cmd/... -run '^$'`: PASS, both entrypoint trees compile.
- `go test -count=1 -race -tags 'development integration' ./pkg/adapter/db/postgres/invariants/... ./pkg/core/usecase/collectionsuc/... ./pkg/core/usecase/contractuc/... ./cmd/bnpl/bootstrap/...`: PASS (88.537s, 49.362s, 3.645s, 12.272s respectively). The last bootstrap ordering adjustment was additionally retested against real PG/race: PASS, 11.016s.
- Real security tests include every registered table's allowed/forbidden operations and writable columns; actual LOGIN roles; DELETE/TRUNCATE/managed-DDL/SET ROLE denials; missing/extra effective grants; PUBLIC, grant options and inherited-role paths; future default drift; function overload drift; unknown tables; trigger drift; legacy ownership/data preservation; permission revoke/regrant/concurrent replacement/RR conflicts; successful effective owner across migration transactions; checksum/dirty refusal.
- Selected fresh init/migration, default deny and terminal permission tests on a separate PostgreSQL cluster whose admin has CREATEROLE/CREATEDB and **no SUPERUSER**: PASS on final code (2.109s).
- Real repayment command → aggregate FC fake → authoritative result → atomic full settlement → principal-only exposure release/contract completion/Collections recovery runs with the actual restricted bnpl login: PASS. Existing reconciliation, duplicate-event, rollback, checkout activation and financial concurrency tests also pass.
- `make verify-entrypoint-boundaries`: PASS; runs all four required binary build targets and checks both environment profiles' compiled dependencies/CLI capabilities.
- All four Docker build targets: PASS. Local test tags: `bnpl-backend:security-check-development`, `bnpl-backend:security-check-production`, `bnpl-backend-dbadmin:security-check-development`, `bnpl-backend-dbadmin:security-check-production`. A temporary registry metadata network failure was retried successfully; no Docker build remains unverified.
- `make -e verify-docker-boundaries IMAGE_VERSION=security-check`: PASS for all four final images, including actual binary Go build tags, nonroot user, filesystem/command separation, no credential ENV and safe default DB Admin help.
- `make db-invariants-list` and `make db-privileges-list`: PASS offline. Combined list shows existing trigger registrations and the 39-table privilege policy.
- `make -e db-invariants-test` with both dedicated test DSNs: PASS, real PG/race, 44.837s, no dependency skips.
- Development/Production Make invariant verification targets against disposable CLI databases: PASS. Privilege verification uses the same implementation.
- Actual `db-init-development`/`db-init-production` flows: PASS on disposable databases. Repeated Production initialization correctly returns a conflict without recreation; repeated Production migration skips all five applied migrations; Production verification passes.
- Read-only ownership plan and repeated explicit ownership apply on an already upgraded disposable database: PASS. Legacy migration tests verify retained row counts and unrelated schema contents, as well as enum/domain/composite/identity ownership.
- `git diff --check`: PASS. Frozen baseline and migrations 000001–000003 have no diff; trigger hashes remain the previously reviewed values. No Decisions/Payments or financial/Collections business history was silently repaired or deleted. Deliberate Development reset remains the explicit exception for disposable Development data.

No public API/protobuf/port signature changed, so Swagger/mocks required no
regeneration. Test configuration paths were updated to runtime.yaml for existing
E2E harness compatibility; live external FC/Kafka/SMS/provider E2E was not run.
Container publishing targets were implemented but **not pushed to Hamravesh**.
No production deployment, live database role/ownership transfer or real-target
privilege audit was performed. All test writes were confined to task-owned
`bnpl-codex-security` and `bnpl-codex-security-nonsuper` clusters, ports 55440/55441,
using unique databases/schemas. The shared bnpl-db container was not modified.

## Deployment prerequisites and remaining operational work

The implementation has no known unresolved code defect from this verification.
Production rollout still requires an actual owner/ACL inventory, backups, drained
old writers, authorized ownership transfer, all incremental migrations, full
verification and deployment of compatible Runtime only. The old binary's DELETE
permission replacement and Runtime-owned schema model are not compatible with
these ACLs; do not overlap old writers with tightening. Password rotation and
platform server/proxy audit redaction remain explicit operator responsibilities.

Runtime may create session-local temporary tables. This is independent from
managed-schema and database CREATE privileges, which remain restricted.
No database-wide TEMPORARY revocation is required for BNPL bootstrap.
A legacy Runtime superuser/database owner must first be replaced by a separately
authorized administrator. Insufficient existing-object transfer authority and
extension/unsupported managed objects also require explicit platform review.
No general or cross-schema ownership reassignment is provided.

Existing generic FC accounting/noncash boundaries and all prior financial rules
remain unchanged. This security task does not claim to add authoritative noncash
settlement or solve any previously documented FC economic-contract dependency.

## Changed-file manifest

Paths below are relative to repository root. A/M/D indicate added, modified or
removed paths; Git may display matching old/new files as renames.

- A `.dockerignore`
- M `.env.example`
- M `Dockerfile`
- M `Makefile`
- M `README.md`
- A `assets/db/migrations/000004_terminal_permission_history.sql`
- A `assets/db/migrations/000005_runtime_privileges.sql`
- A `cmd/bnpl-dbadmin/bootstrap/boot.go`
- A `cmd/bnpl-dbadmin/bootstrap/init_development.go`
- A `cmd/bnpl-dbadmin/bootstrap/init_production.go`
- A `cmd/bnpl-dbadmin/bootstrap/ownership.go`
- A `cmd/bnpl-dbadmin/command/boundary_test.go`
- A `cmd/bnpl-dbadmin/command/command.go`
- A `cmd/bnpl-dbadmin/command/db.go`
- A `cmd/bnpl-dbadmin/command/db_development.go`
- A `cmd/bnpl-dbadmin/command/db_production.go`
- A `cmd/bnpl-dbadmin/command/init_dev.go`
- A `cmd/bnpl-dbadmin/command/init_prod.go`
- A `cmd/bnpl-dbadmin/command/invariants.go`
- A `cmd/bnpl-dbadmin/command/migrate.go`
- A `cmd/bnpl-dbadmin/command/ownership.go`
- A `cmd/bnpl-dbadmin/command/privileges.go`
- A `cmd/bnpl-dbadmin/command/root.go`
- A `cmd/bnpl-dbadmin/main.go`
- M `cmd/bnpl/bootstrap/app.go`
- M `cmd/bnpl/bootstrap/boot.go`
- D `cmd/bnpl/bootstrap/boot_init_dev.go`
- D `cmd/bnpl/bootstrap/boot_init_prod.go`
- M `cmd/bnpl/bootstrap/bootstrap.go`
- A `cmd/bnpl/bootstrap/database_security_integration_test.go`
- M `cmd/bnpl/bootstrap/infrastructure_test.go`
- D `cmd/bnpl/bootstrap/invariants.go`
- M `cmd/bnpl/bootstrap/policy_assignment.go`
- M `cmd/bnpl/bootstrap/repo.go`
- M `cmd/bnpl/bootstrap/repo_test.go`
- M `cmd/bnpl/bootstrap/route_test.go`
- M `cmd/bnpl/bootstrap/usecase.go`
- M `cmd/bnpl/bootstrap/usecase_test.go`
- A `cmd/bnpl/command/boundary_test.go`
- M `cmd/bnpl/command/command.go`
- D `cmd/bnpl/command/db.go`
- D `cmd/bnpl/command/db_development.go`
- D `cmd/bnpl/command/db_production.go`
- D `cmd/bnpl/command/init_dev.go`
- D `cmd/bnpl/command/init_prod.go`
- D `cmd/bnpl/command/invariants.go`
- D `cmd/bnpl/command/migrate.go`
- M `cmd/bnpl/command/root.go`
- D `configs/config.yaml`
- A `configs/dbadmin.yaml`
- A `configs/runtime.yaml`
- A `doc/architecture/database-deployment.md`
- M `doc/architecture/database-invariants.md`
- A `doc/architecture/database-privileges.md`
- A `doc/architecture/database-roles.md`
- A `doc/database-security-verification.md`
- M `doc/database.md`
- M `pkg/adapter/config/database.go`
- M `pkg/adapter/config/database_test.go`
- M `pkg/adapter/config/load.go`
- M `pkg/adapter/config/load_test.go`
- M `pkg/adapter/config/repayment_shortlink_test.go`
- A `pkg/adapter/config/runtime_security_test.go`
- M `pkg/adapter/config/usecase.go`
- M `pkg/adapter/config/usecase_test.go`
- M `pkg/adapter/config/validate.go`
- M `pkg/adapter/config/validate_test.go`
- A `pkg/adapter/db/postgres/connection/settings.go`
- A `pkg/adapter/db/postgres/invariants/privilege_integration_test.go`
- A `pkg/adapter/db/postgres/invariants/privilege_registry.go`
- A `pkg/adapter/db/postgres/invariants/privilege_registry_test.go`
- A `pkg/adapter/db/postgres/invariants/privilege_verify.go`
- M `pkg/adapter/db/postgres/merchantterminalrp/query.go`
- A `pkg/adapter/db/postgres/schemarp/credential_test.go`
- A `pkg/adapter/db/postgres/schemarp/ownership.go`
- M `pkg/adapter/db/postgres/schemarp/query.go`
- M `pkg/adapter/db/postgres/schemarp/query_development.go`
- A `pkg/adapter/dbadminconfig/config.go`
- A `pkg/adapter/dbadminconfig/config_test.go`
- A `pkg/adapter/dbadminconfig/environment_development.go`
- A `pkg/adapter/dbadminconfig/environment_production.go`
- M `pkg/core/repo/role.go`
- M `pkg/core/repo/schema_settings.go`
- M `pkg/core/repo/schemarp.go`
- M `pkg/core/usecase/collectionsuc/payment_components_integration_test.go`
- M `pkg/core/usecase/migrationsuc/error_production.go`
- M `pkg/core/usecase/migrationsuc/initdb.go`
- M `pkg/core/usecase/migrationsuc/initdb_development_test.go`
- M `pkg/core/usecase/migrationsuc/initdb_production_test.go`
- M `pkg/core/usecase/migrationsuc/initdb_test.go`
- A `scripts/verify-docker-boundaries.sh`
- A `scripts/verify-entrypoint-boundaries.sh`
- M `test/e2e/confirm_saga_harness_test.go`
- M `test/e2e/fc_reconciliation_pending_session_test.go`
- M `test/e2e/installment_repayment_reconciliation_test.go`
