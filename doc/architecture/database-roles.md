# Database roles and independent applications

Runtime is `cmd/bnpl`; database administration is `cmd/bnpl-dbadmin`. Both retain
`development` and `production` build profiles. There is no dbadmin capability tag.
Runtime has HTTP, business administration (`policy-assignment import-trusted`),
workers and integrations. It does not import schema provisioning or migration
orchestration. DB Admin imports only its own small config, PostgreSQL, logging,
Clock and the existing migration/initialization use cases; it cannot serve traffic.
See [privileges](database-privileges.md) and [deployment](database-deployment.md).

## Four application roles

- `admin`: LOGIN, externally provisioned database administrator. Used only for
  first initialization or the explicit ownership upgrade. CREATEROLE and the
  ability to create the managed schema, grant owner membership and transfer the
  existing objects are required. This is not an ordinary query login.
- `bnpl_owner`: NOLOGIN, no superuser/CREATEROLE/CREATEDB/REPLICATION/BYPASSRLS,
  no memberships. Owns bnpl schema, tables, indexes, attached sequences, row types,
  functions and migration metadata. It has no password/PGPass/connection URL.
- `bnpl_migrator`: LOGIN, NOINHERIT, no privileged attributes, member only of
  `bnpl_owner`, without ADMIN OPTION. On PostgreSQL 16+ the membership explicitly
  has INHERIT FALSE and SET TRUE. Every physical pool connection executes
  `SET ROLE bnpl_owner` before any migration work. The existing advisory-lock,
  checksum, transactional/nontransactional and dirty migration engine is reused.
  The effective owner persists through the transaction and connection lifecycle.
- `bnpl`: LOGIN, NOINHERIT, no privileged attributes, no direct memberships at all
  (therefore no transitive administrative role path). Never owns managed objects.
  Only the reviewed DML allowlist is granted. No DELETE/TRUNCATE/DDL/delegation.

PostgreSQL built-in/platform roles are not counted as application roles. Tests
use a real admin login and real bnpl/migrator connections, not SET ROLE simulation
of a restricted Runtime. Production does not require that admin always be a
superuser: an authorized platform DBA can arrange the necessary ownership and
role-administration rights. Transferring objects owned by another role requires
ownership authority. The preflight reports missing authority rather than silently
using REASSIGN OWNED or granting access across unrelated schemas. A legacy bnpl
superuser, a database owned by bnpl, or CREATE via public schema/database is an
operator blocker; establish a separate admin and remove that capability explicitly.
Only a platform superuser can remediate another superuser or certain cluster-wide
attributes. No command automatically grants itself superuser privileges.

## Credentials and provisioning

`configs/runtime.yaml` and its Runtime config type contain **only** the normal
PGPass entry. Requests for another connection role are rejected. It has no
administrative factories. `configs/dbadmin.yaml` has admin/migrator PGPass entries
and an explicit `provisioning.normal_password` secret for first-time creation;
there is no normal-role connection constructor. It requires no business secrets.
Only the selected operation resolves its required credential, so migration or
verification does not need the admin or normal provisioning secret. Owner has none.
Each application checks the `application` discriminator and its supported versions.
Path precedence is `--config/-c`, then `CONFIG_FILE`, then the application's default.
Pass an explicit config when an inherited CONFIG_FILE points at the other app.

Provisioning is idempotent and retains existing passwords. It validates attributes
and memberships, rather than silently downgrading a privileged/shared role. Password
rotation is deliberately a separate audited operator action using the platform's
secret-management process, not an incidental effect of running a migration.
Administrative pools always disable application SQL tracing. Role identifiers are
quoted with pgx; DDL password literals escape quotes and backslashes independently
of server string settings. Credential-operation errors exclude the SQL/password.
Configuration parser/connection errors do not echo expanded secrets. Configure
PostgreSQL/proxy statement and parameter logging so provisioning passwords are
redacted or excluded; application tracing controls cannot redact an independent
server audit sink. Restrict admin Job logs and secret access accordingly.

## Existing database ownership upgrade

1. Back up and drain old Runtime writers, consumers and scheduled jobs. Inventory
   existing owners, table/column ACLs and role memberships with:
   `bnpl-dbadmin --config configs/dbadmin.yaml db ownership plan`.
   It uses a read-only snapshot, prints no passwords, and reports unsupported or
   extension-owned managed objects and missing ownership authority as blockers.
2. Resolve platform blockers explicitly. The admin must have ownership authority
   over the old managed objects and the ability to grant/create the new roles.
   If temporary legacy-owner membership is needed, have the platform DBA arrange
   and later remove it; the tool does not grant itself access to unrelated schemas.
   Do not rename/drop existing database business rows to bypass a blocker.
3. Run `... db ownership apply`. Under the existing migration advisory lock and
   a transaction it rechecks the plan, provisions/validates roles and transfers
   only bnpl objects. Attached/identity sequences follow their owning tables;
   they are not individually reassigned. Standalone sequences, functions,
   domains/types and migration metadata are covered. Unsupported object kinds
   fail preflight. No blanket REASSIGN OWNED or cross-schema operation is used.
4. Run `... db migrate` with migrator credentials. Incremental 000004 adds
   permission revocation history; 000005 installs explicit privileges/default deny.
   The frozen baseline and migrations 000001–000003 remain unchanged.
5. Run `... db invariants verify`, then deploy the matching Runtime with only
   normal credentials. Do not resume old binaries that DELETE terminal permissions
   or use table-wide mutations forbidden by the new policy.

Plan/apply is repeatable. Failure rolls back the ownership transaction; migration
failure retains the existing engine's transactional/dirty semantics. Neither
operation deletes or repairs Decisions, Payments or financial/Collections history.
Runtime startup verifies effective safety and fails closed; it never repairs DDL
or permissions. Live target ownership/grants must be inspected during deployment;
passing disposable tests is not a production audit.

Platform prerequisite: PostgreSQL grants database TEMPORARY to PUBLIC by default.
Because revoking that privilege affects other logins in the target database, the
application does not silently change it. Have the database owner/platform DBA run
`REVOKE TEMPORARY ON DATABASE <target> FROM PUBLIC` and explicitly regrant it only
to unrelated authorized maintenance roles if needed. Also remove any database or
public-schema CREATE path to Runtime. Bootstrap/preflight and Runtime verification
fail until the effective boundary is safe. This requires database ownership or
proper grant authority, not necessarily superuser. It does not change another
database or rewrite managed business data.
