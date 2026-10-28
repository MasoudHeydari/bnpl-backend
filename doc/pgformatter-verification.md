# pgFormatter and development setup verification

Starting HEAD: `67cbfb92604749fa9b4de2caee1390eaa32d158c` on
`feat-add-decision-domain`. No PR or application deployment is part of this task.

The user explicitly changed the initial scope: all current pre-production SQL,
including baseline and existing migrations, was formatted. No grandfather list
or legacy exemption remains. Recorded hashes in Go were deliberately **not**
changed, per the user's manual-hash handoff request.

## Delivered tooling

- Targets: install-system-deps, install-pg-formatter, setup-dev, sql-fmt,
  sql-fmt-check and test-dev-tooling; existing install-tools now includes the
  formatter and still activates install-hooks. All previous targets are retained.
- pgFormatter 5.11, official tagged source, pinned SHA-256 from
  scripts/pgformatter.lock, upstream MakeMaker build and user-prefix installation.
  Runtime CLI/build Perl modules verified; optional CGI/JSON and CPAN omitted.
- Existing moving Go tool installs now have configurable fixed versions; no
  application go.mod dependency was added and no Go toolchain is installed.
- Shared formatting script/config for Make, exact staged blobs and CI, with safe
  filenames, no automatic hook writes, temp cleanup, missing/version diagnostics,
  retained comments/literals and bounded convergence for upstream multi-pass cases.
- Explicit system package installation is separate from unprivileged setup.
  Setup delegates to existing targets, verifies tools/hooks, safely prepares local
  config and never starts containers, initializes/migrates/resets a DB or uses sudo.
- Existing CI jobs and CD are retained. New SQL job installs the same pinned
  formatter, runs check and isolated tooling tests without executing SQL.

See [developer instructions and troubleshooting](development-setup.md).

## Executed verification

- Real clean formatter installation into a user prefix containing spaces: PASS.
  Reinstallation skipped download/write; version and SHA validation passed.
- Actual `make setup-dev` twice: PASS. Go tools, formatter and hook installed/
  verified. On repeat .env, .git/config, go.mod and go.sum hashes were unchanged.
  No privileged package installation, container or database operation occurred.
- `make test-dev-tooling`: PASS, 20 scenarios, real formatter/isolated Git commits
  and exact index contents. Go/lint and privileged commands are strict test doubles
  in fixtures; production tooling is not silently replaced by mocks.
- Cases include no-SQL commits without formatter, good/bad/new/copied/renamed/
  deleted SQL, both directions of partial staging, spaces/special characters/
  newline names, staged config versus unstaged config, ignored/generated/symlink
  files, empty file sets, missing/wrong versions, no index/worktree writes on
  checks, comments/PG syntax/long literal preservation, idempotent formatting,
  corrupt archive rejection, temp cleanup, missing Perl dependencies, unsupported
  OS and apt failure diagnostics, no redundant privileged calls, safe setup repeat.
- `make sql-fmt`: all 10 canonical SQL files processed, 9 changed; already formatted
  000_prod.sql was retained without rewriting. Symlink aliases are not independent
  sources and were not replaced or followed into duplicate formatting.
- `make sql-fmt-check`: PASS for all 10 files, no exemptions, no writes.
- Independent token/comment comparison of original Git blobs against all 10 final
  files: PASS for token order, quoted values and comment text/order. PL/pgSQL body
  code was compared recursively; no SQL execution or database connection was used.
- `make build-development build-production build-dbadmin-development build-dbadmin-production`:
  PASS. Application Dockerfiles and runtime dependencies remain unchanged.
- `go test -tags development ./...`: all packages except postgres/invariants pass.
  Its two checksum guard tests fail as expected because manual hash updates were
  explicitly deferred: TestRegistryMatchesVersionedMigration and
  TestPrivilegeRegistryMatchesReviewedMigration. They were not disabled/skipped.
  The frozen baseline guard will likewise reject initialization until its recorded
  digest is updated. No database integration/migration was run against stale hashes.
- Registry source-body extraction in its test now tolerates formatter whitespace/
  keyword casing, while still comparing the exact body hash. No runtime verifier,
  recorded hash or financial/domain code was weakened or changed.
- Bash syntax, Git whitespace checks and CI YAML parsing: PASS. Local equivalents
  of the new SQL CI job were executed; hosted Actions was not run here.

The explicit OS package installer was tested with command doubles, real
prerequisite validation, and an actual idempotent run with every package already
installed (apt/sudo skipped). It was not used to perform privileged apt operations.
Only user-owned developer tools/configuration were prepared. Existing comments,
SQL literals and runtime business behavior were retained.

## Manual digest handoff

Do not interpret formatting success as checksum compatibility. The following are
current raw-file SHA-256 values; Go migration constants include `sha256:` where
already specified. Files 000001/000002 have no corresponding managed Go constant,
but a local database that already applied them may retain their old checksums.
No stored database history has been changed. Review/reset only an explicitly
chosen pre-production environment separately; setup-dev never does that.

- `assets/db/audits/installment_integrity.sql`
  - Previous: `3e06a08e86172db3a8e25ec8ff710bfec307a58959e6750026a93007be5c13ad`
  - Current: `32594dfb4aff18a4486dec8aa1863795c17b121823a8fbd8950cd12f4aad3c9d`
- `assets/db/audits/unresolved_financial_outcomes.sql`
  - Previous: `d6cf4a49b7aad1e25c81c29c6217192eed0134bc0d2e486bf34fc50c42af7644`
  - Current: `e04833a1ccfaa3d1031a6da8af7f640fa8b8cbf876e798e6f585cd400f802849`
- `assets/db/migrations/000001_collections_default_notification.sql`
  - Previous: `ed26d65520f1ca851b5eae3fff5292960b35ec8554ecc3662719091e2f4f26fa`
  - Current: `a185b76b452cd402ba8d6fcb43ab77c6f40c742c920c8e277fb4872d2b61af36`
- `assets/db/migrations/000002_independent_credit_restrictions.sql`
  - Previous: `8e5400781c195ef19d7a27a2af7ffcc63fb34d75a587dfc85c483414874b1f1c`
  - Current: `183b4bcfc5f77460c1e67078b7444fb249e9d54877ec40b0f169f2050585c869`
- `assets/db/migrations/000003_installment_payment_invariants.sql`
  - Previous: `6267ddaade88869646b835af4f2a8dfc2c4ed4c1c6ce91ff065fa5cf39be18a4`
  - Current: `6bc8d59281b16dacd812620864ed4e014d29258a62b7fe139d3cb782e8b54a50`
- `assets/db/migrations/000004_terminal_permission_history.sql`
  - Previous: `b7e10ed853d28057a901f118745b43418b65e3106ba32520b01394151ab918b6`
  - Current: `0442b7e4fd60f3be20c75f7fc4ba7c61d2b839ff06a2e488ae5db4441f221b69`
- `assets/db/migrations/000005_runtime_privileges.sql`
  - Previous: `79faf162f74a9e36936396fcb9d6ab78ce6cfe0318a5e9b574414545adbfeba3`
  - Current: `7b6e4c7e8f1fc97802ed532b5c4b62b7c77e34764f4af0d330b6ceedfbc6f07c`
- `pkg/adapter/db/postgres/migrationsrp/000_dev.sql`
  - Previous: `29abe1255d49e5ccc778d93c3da8f8ef2edd0861729244e40dce5e9ecdc35f28`
  - Current: `df9c75aecd8999d050a253fcd724b96947534349ca71f8bc239eccd0e6d749a8`
- `pkg/adapter/db/postgres/migrationsrp/000_prod.sql`
  - Previous: `38375c780b248c15a01f16a8d6756ee139b5226dc2117d24944db5b577cd12bd`
  - Current: `38375c780b248c15a01f16a8d6756ee139b5226dc2117d24944db5b577cd12bd`
- `pkg/adapter/db/postgres/migrationsrp/000_schema.sql`
  - Previous: `4d8805ab961058ae6052a296aa2c51f76cc618c16559066c673226671cbed223`
  - Current: `3d6a6bea1a70b181771376cb676633b7f693619a233c8c8e86d0b29a1ff7043f`

Recorded Go metadata to update manually after review:

- `pkg/adapter/db/postgres/migrationsrp/settler.go`: baselineSchemaSHA256.
- `pkg/adapter/db/postgres/invariants/registry.go`: MigrationSHA256 and the exact
  FunctionSHA256 entries below (a function can be referenced by multiple triggers).
- `pkg/adapter/db/postgres/invariants/privilege_registry.go`:
  PrivilegeMigrationSHA256 and PermissionHistoryMigrationSHA256.

Exact new `prosrc` body fingerprints (no `sha256:` prefix in FunctionSHA256):

- `guard_installment_charge`: `1148ef6380d0fa13d6585b9c7569ab4a76beb0253e99a9d4c97d6f5e7a45a104`
- `guard_installment_settlement`: `e604f4e21902ce9b6bf683e4fbeef630125cbe553c45b5702749f0ab523f1ae6`
- `guard_payment_attempt_freeze`: `4e3d220dbb84ce8bf2eb8c9fda5a76ad8155f135d961b24acde458b2db36783e`
- `guard_payment_items`: `84bb4b3aa94057f16745ebbfa759bd181bec78cf53e78b46d7611c321683fa11`
- `verify_payment_snapshot_commit`: `5f3f6a2f9c101d09d07523a967cb2f97f3129d32c1c3917a174fb3dd0cf771d2`

Future production rule: freeze the baseline before the first production release;
format new migrations before publication/application and do not rewrite already
published SQL/checksums afterward.

## Known limits

Upstream hard wrap-limit can change string literal values, so the project uses
structural formatting and a soft 120-column guide rather than a hard cap.
Upstream 5.11 needs multiple passes for some layouts; every mode uses the same
bounded fixed point and refuses nonconverging output without rewriting that file.
Archive SHA pinning protects repeat downloads, but is not an independently signed
upstream release assertion. SQL formatting is not a substitute for SQL syntax,
privilege, migration, application or database integration verification.

The native hook's SQL, lint-config and module-tidiness steps pass. Its final
full-repository lint step reports 896 issues in the existing application; no rule was disabled to force a green hook. The linter configuration now
selects the repository's required Development profile so existing checks can
actually typecheck the application instead of failing solely on a missing tag.
This tooling task does not refactor unrelated business code to resolve that debt.


## Changed-file summary

Developer tooling/config: Makefile, .githooks/pre-commit, .golangci.yml,
.github/workflows/ci.yml, .pg_format and shared installer/check/setup/test scripts.
README and the development/verification guides document commands and limitations.
Nine current canonical SQL files changed formatting; the tenth was already in
policy. Registry test extraction was adapted to the formatter's legal layout.
No application Dockerfile, CD workflow, go.mod/go.sum, recorded Go checksum or
business-domain implementation changed.

Exact paths (relative to repository root):

- `.githooks/pre-commit`
- `.github/workflows/ci.yml`
- `.golangci.yml`
- `.pg_format`
- `Makefile`
- `README.md`
- `assets/db/audits/installment_integrity.sql`
- `assets/db/audits/unresolved_financial_outcomes.sql`
- `assets/db/migrations/000001_collections_default_notification.sql`
- `assets/db/migrations/000002_independent_credit_restrictions.sql`
- `assets/db/migrations/000003_installment_payment_invariants.sql`
- `assets/db/migrations/000004_terminal_permission_history.sql`
- `assets/db/migrations/000005_runtime_privileges.sql`
- `doc/development-setup.md`
- `doc/pgformatter-verification.md`
- `pkg/adapter/db/postgres/invariants/registry_test.go`
- `pkg/adapter/db/postgres/migrationsrp/000_dev.sql`
- `pkg/adapter/db/postgres/migrationsrp/000_schema.sql`
- `scripts/install-golangci-lint.sh`
- `scripts/install-pg-formatter.sh`
- `scripts/lib/dev-tools.sh`
- `scripts/pgformatter.lock`
- `scripts/setup-dev.sh`
- `scripts/sql-format.sh`
- `scripts/system-deps.sh`
- `scripts/tests/dev-tooling.sh`
