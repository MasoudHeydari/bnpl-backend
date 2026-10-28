#!/usr/bin/env bash
# Real formatter + isolated Git repos. Go/lint/system installers are narrow test
# doubles, so these tests never change a developer's tools, index, packages or DB.
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/../lib/dev-tools.sh"
require_formatter
formatter="$PG_FORMATTER"
real_make=$(command -v make)
work=$(mktemp -d)
trap 'rm -rf -- "$work"' EXIT
export MOCK_LOG="$work/mock.log"
mkdir -p "$work/bin"
cat > "$work/bin/go" <<'MOCK'
#!/usr/bin/env bash
set -eu
printf 'go %s\n' "$*" >> "$MOCK_LOG"
case "$*" in
 'mod tidy -diff') exit "${MOCK_TIDY_EXIT:-0}";;
 'env GOVERSION') echo go1.26.2;;
 'env GOBIN') echo "$MOCK_BIN";;
 *) echo "Unexpected Go invocation: $*" >&2; exit 91;;
esac
MOCK
cat > "$work/bin/golangci-lint" <<'MOCK'
#!/usr/bin/env bash
set -eu
printf 'golangci-lint %s\n' "$*" >> "$MOCK_LOG"
case "$*" in 'config verify'|'run ./...') exit "${MOCK_LINT_EXIT:-0}";; *) exit 92;; esac
MOCK
chmod +x "$work/bin/go" "$work/bin/golangci-lint"
export PATH="$work/bin:$PATH" MOCK_BIN="$work/bin"
log="$work/result.log"
passes=0
pass() { passes=$((passes+1)); printf 'PASS %s\n' "$1"; }
die() { echo "FAIL: $*" >&2; cat "$log" >&2; exit 1; }
expect_failure() { if "$@" > "$log" 2>&1; then die "Expected failure: $*"; fi; }
contains() { grep -F -- "$1" "$log" >/dev/null || die "Missing diagnostic: $1"; }
new_case() {
    case_dir="$work/$1"
    mkdir -p "$case_dir/scripts" "$case_dir/.githooks" "$case_dir/queries" "$case_dir/proto"
    cp -a "$DEV_TOOLS_ROOT/scripts/lib" "$case_dir/scripts/"
    cp "$DEV_TOOLS_ROOT/scripts/"{sql-format.sh,pgformatter.lock,setup-dev.sh,system-deps.sh,install-pg-formatter.sh} "$case_dir/scripts/"
    cp "$DEV_TOOLS_ROOT/.pg_format" "$case_dir/"
    cp "$DEV_TOOLS_ROOT/.githooks/pre-commit" "$case_dir/.githooks/"
    chmod +x "$case_dir/.githooks/pre-commit"
    printf 'version: "2"\n' > "$case_dir/.golangci.yml"
    printf 'module example.test/tooling\n\ngo 1.26.2\n' > "$case_dir/go.mod"
    printf 'ignored.sql\n.env\n' > "$case_dir/.gitignore"
    cd -- "$case_dir"
    git init -q
    git config user.name 'Tooling Test'
    git config user.email 'tooling-test@example.invalid'
    git add .
    git commit -qm fixture
    git config core.hooksPath .githooks
}
fmt() { PG_FORMATTER="$formatter" bash scripts/sql-format.sh fmt -- "$@" > "$log" 2>&1; }
hook() { bash .githooks/pre-commit > "$log" 2>&1; }
commit() { git commit -qm "$1" > "$log" 2>&1; }
blob() { git rev-parse ":0:$1"; }

new_case no-sql
printf 'notes\n' > note.txt; git add note.txt
PG_FORMATTER="$work/missing" commit no-sql
for command in 'golangci-lint config verify' 'go mod tidy -diff' 'golangci-lint run ./...'; do grep -F "$command" "$MOCK_LOG" >/dev/null || die "Existing hook check missing"; done
pass 'actual commit without SQL does not require pgFormatter; all old hook checks run'

new_case good
printf 'select 1;\n' > queries/good.sql
fmt queries/good.sql; git add queries/good.sql
PG_FORMATTER="$formatter" commit good
cp queries/good.sql queries/copied.sql; git add queries/copied.sql
PG_FORMATTER="$formatter" hook
contains '1 files'
PG_FORMATTER="$formatter" commit copied
# Rename detection must inspect the destination; then deletion skips dependency.
git mv queries/good.sql 'queries/renamed file.sql'
PG_FORMATTER="$formatter" hook
contains '1 files'
PG_FORMATTER="$formatter" commit renamed
git rm -q 'queries/renamed file.sql'
PG_FORMATTER="$work/missing" commit deleted
pass 'added/formatted/copied/renamed SQL commits pass; deleted SQL skips formatter'

new_case bad
printf 'select 1;\n' > queries/bad.sql; git add queries/bad.sql
before=$(sha256sum queries/bad.sql); index=$(sha256sum .git/index); staged=$(blob queries/bad.sql)
PG_FORMATTER="$formatter" expect_failure bash .githooks/pre-commit
contains 'SQL needs formatting'; contains 'queries/bad.sql'
[[ $(sha256sum queries/bad.sql) == "$before" && $(sha256sum .git/index) == "$index" && $(blob queries/bad.sql) == "$staged" ]] || die 'hook changed SQL/index'
PG_FORMATTER="$formatter" expect_failure git commit -qm bad
[[ $(git rev-list --count HEAD) == 1 ]] || die 'bad SQL committed'
pass 'bad staged SQL blocks actual commit without working-tree/index writes'

new_case partial-bad-index
printf 'select 1;\n' > queries/partial.sql; git add queries/partial.sql
staged=$(blob queries/partial.sql)
fmt queries/partial.sql
before=$(sha256sum queries/partial.sql)
PG_FORMATTER="$formatter" expect_failure bash .githooks/pre-commit
[[ $(sha256sum queries/partial.sql) == "$before" && $(blob queries/partial.sql) == "$staged" ]] || die 'partial stage changed'
pass 'bad index + formatted unstaged SQL still fails'

new_case partial-good-index
printf 'select 1;\n' > queries/partial.sql; fmt queries/partial.sql; git add queries/partial.sql
staged=$(blob queries/partial.sql)
printf 'select 2;\n' > queries/partial.sql
before=$(sha256sum queries/partial.sql)
PG_FORMATTER="$formatter" commit partial
[[ $(git rev-parse HEAD:queries/partial.sql) == "$staged" && $(sha256sum queries/partial.sql) == "$before" ]] || die 'staged content substituted'
pass 'good index + bad unstaged SQL commits exact staged blob'

new_case special-name
name='queries/a b [x] $(touch MARKER):test.sql'
printf 'select 1;\n' > "$name"; git add -- "$name"
PG_FORMATTER="$formatter" expect_failure bash .githooks/pre-commit
contains 'SQL needs formatting'; [[ ! -e MARKER ]] || die 'filename evaluated'
fmt "$name"; git add -- "$name"
PG_FORMATTER="$formatter" commit special
pass 'whitespace, brackets, dollar syntax and colon in filenames are safe'
new_case newline-name
name=$'queries/new\nline.sql'
printf 'select 1;\n' > "$name"; git add -- "$name"
PG_FORMATTER="$formatter" expect_failure bash .githooks/pre-commit
contains 'SQL needs formatting'
fmt "$name"; git add -- "$name"
PG_FORMATTER="$formatter" commit newline
pass 'NUL-delimited index traversal supports newline filenames'


new_case missing-formatter
printf 'select 1;\n' > queries/file.sql; fmt queries/file.sql; git add queries/file.sql
PG_FORMATTER="$work/missing" expect_failure bash .githooks/pre-commit
contains 'make install-tools or make install-pg-formatter'
pass 'missing formatter blocks SQL commits with installation guidance'
mkdir -p "$work/wrong-version/bin"
printf '#!/bin/sh\necho "pg_format version 4.0"\n' > "$work/wrong-version/bin/pg_format"
chmod +x "$work/wrong-version/bin/pg_format"
PG_FORMATTER="$work/wrong-version/bin/pg_format" expect_failure bash .githooks/pre-commit
contains 'version mismatch'
PG_FORMATTER_INSTALL_DIR="$work/wrong-version" expect_failure bash "$DEV_TOOLS_ROOT/scripts/install-pg-formatter.sh"
contains 'different/incomplete version'
pass 'wrong executable version rejected; existing installation never overwritten'


new_case staged-policy
printf 'select 1;\n' > queries/file.sql; fmt queries/file.sql; git add queries/file.sql
sed -i 's/keyword-case=2/keyword-case=1/' .pg_format
PG_FORMATTER="$formatter" hook
pass 'staged SQL uses staged config despite different working-tree config'

new_case postgres
cat > queries/postgres.sql <<'SQL'
-- Keep the explanatory comment.
create table example(id bigint generated always as identity, amount numeric(20,2), data jsonb);
create function example_trigger() returns trigger language plpgsql as $body$
begin
  raise exception using errcode='BN003', message='keep me';
end
$body$;
select data->>'text' from example where id = any(array[1,2]);
SQL
# A long literal must remain byte-for-byte, never be hard wrapped/anonymized.
literal=$(printf 'x%.0s' {1..180})
printf "select '%s';\n" "$literal" >> queries/postgres.sql
fmt queries/postgres.sql
before=$(sha256sum queries/postgres.sql)
PG_FORMATTER="$formatter" bash scripts/sql-format.sh check > "$log" 2>&1
[[ $(sha256sum queries/postgres.sql) == "$before" ]] || die 'check modified SQL'
grep -F -- "'$literal'" queries/postgres.sql >/dev/null || die 'long literal changed'
grep -F -- '-- Keep the explanatory comment.' queries/postgres.sql >/dev/null || die 'comment removed'
fmt queries/postgres.sql
[[ $(sha256sum queries/postgres.sql) == "$before" ]] || die 'format not idempotent'
pass 'PostgreSQL syntax/comments/literals preserved; check read-only; formatting idempotent'
mkdir -p "$work/hostile-home"
printf 'anonymize=1\nnocomment=1\nkeyword-case=1\n' > "$work/hostile-home/.pg_format"
HOME="$work/hostile-home" XDG_CONFIG_HOME="$work/hostile-home" PG_FORMATTER="$formatter" bash scripts/sql-format.sh check > "$log" 2>&1
[[ $(sha256sum queries/postgres.sql) == "$before" ]] || die 'home configuration affected project policy'
pass 'home/XDG formatter settings cannot override project configuration'


new_case excluded
mkdir -p vendor gen build
printf 'select 1;\n' > vendor/third.sql
printf 'select 1;\n' > gen/generated.sql
printf 'select 1;\n' > build/temp.sql
printf 'select 1;\n' > ignored.sql
ln -s missing.sql alias.sql
git add vendor gen build alias.sql
PG_FORMATTER="$work/missing" bash scripts/sql-format.sh staged > "$log" 2>&1
PG_FORMATTER="$work/missing" bash scripts/sql-format.sh check > "$log" 2>&1
pass 'generated/vendor/build/ignored files and SQL symlinks excluded; empty set succeeds'

new_case existing-go-checks
printf 'notes\n' > note.txt; git add note.txt
MOCK_TIDY_EXIT=1 PG_FORMATTER="$work/missing" expect_failure bash .githooks/pre-commit
contains 'go.mod or go.sum is not tidy'
MOCK_LINT_EXIT=1 PG_FORMATTER="$work/missing" expect_failure bash .githooks/pre-commit
contains 'golangci-lint configuration is invalid'
pass 'existing tidy/config error handling preserved'

# Install into a temporary user prefix with spaces, verify version/cleanup and
# skip a second download. This is the sole network action in the test suite.
cd -- "$DEV_TOOLS_ROOT"
prefix="$work/user tools/pgFormatter"
PG_FORMATTER_INSTALL_DIR="$prefix" bash scripts/install-pg-formatter.sh > "$log" 2>&1
[[ $(formatter_version "$prefix/bin/pg_format") == "pg_format version $PG_FORMATTER_VERSION" ]]
before=$(stat -c '%Y:%s' "$prefix/bin/pg_format")
PG_FORMATTER_INSTALL_DIR="$prefix" bash scripts/install-pg-formatter.sh > "$log" 2>&1
contains 'already installed'
[[ $(stat -c '%Y:%s' "$prefix/bin/pg_format") == "$before" ]]
[[ -z $(find "$work/user tools" -maxdepth 1 -name '.pgformatter-install.*' -print) ]]
pass 'clean pinned installation, custom path with spaces, version, idempotency and cleanup'

mkdir -p "$work/no-perl"
printf '#!/bin/sh\nexit 1\n' > "$work/no-perl/perl"; chmod +x "$work/no-perl/perl"
PATH="$work/no-perl:$PATH" PG_FORMATTER_INSTALL_DIR="$work/deps-missing" expect_failure bash scripts/install-pg-formatter.sh
contains 'Missing required Perl CLI/build modules'; [[ ! -e $work/deps-missing ]]
pass 'missing Perl modules fail before installation with actionable guidance'

mkdir -p "$work/bad-download"
cat > "$work/bad-download/curl" <<'MOCK'
#!/usr/bin/env bash
while (($#)); do if [[ $1 == -o ]]; then printf corrupt > "$2"; exit 0; fi; shift; done
exit 93
MOCK
chmod +x "$work/bad-download/curl"
PATH="$work/bad-download:$PATH" PG_FORMATTER_INSTALL_DIR="$work/corrupt/pg" expect_failure bash scripts/install-pg-formatter.sh
contains 'SHA-256 mismatch'; [[ ! -e $work/corrupt/pg ]]
[[ -z $(find "$work/corrupt" -name '.pgformatter-install.*' -print) ]]
pass 'corrupt release rejected without installing or leaking temporary files'

# Explicit OS installation is tested with command doubles, never sudo/apt itself.
mkdir -p "$work/system"
cat > "$work/system/dpkg-query" <<'MOCK'
#!/bin/sh
printf 'install ok installed'
MOCK
cat > "$work/system/sudo" <<'MOCK'
#!/bin/sh
echo "sudo $*" >> "$MOCK_LOG"
exit 94
MOCK
cat > "$work/system/apt-get" <<'MOCK'
#!/bin/sh
echo "apt-get $*" >> "$MOCK_LOG"
exit 94
MOCK
chmod +x "$work/system/"*
: > "$MOCK_LOG"
PATH="$work/system:$PATH" bash scripts/system-deps.sh > "$log" 2>&1
[[ ! -s $MOCK_LOG ]] || die 'unnecessary privileged operation'
cat > "$work/system/uname" <<'MOCK'
#!/bin/sh
echo Darwin
MOCK
chmod +x "$work/system/uname"
PATH="$work/system:$PATH" expect_failure bash scripts/system-deps.sh
contains 'Ubuntu/Debian Linux only'
pass 'installed system packages avoid sudo/apt; unsupported OS rejected safely'
rm -- "$work/system/uname"
cat > "$work/system/dpkg-query" <<'MOCK'
#!/bin/sh
case "$*" in *perl) exit 1;; *) printf 'install ok installed';; esac
MOCK
PATH="$work/system:$PATH" expect_failure bash scripts/system-deps.sh
contains 'System packages to install (no Go toolchain changes): perl'
contains 'apt-get update failed'
pass 'only missing packages selected; apt failure is actionable (privileged commands mocked)'


# Exercise setup with the real Make safe-config/hook targets, while all developer
# tool installations are replaced by a strict command double.
new_case setup
cp "$DEV_TOOLS_ROOT/Makefile" .
printf 'APP_ENVIRONMENT=development\n' > .env.example
mkdir -p doc/collections
printf '{}\n' > doc/collections/http-client.env.json.example
for tool in staticcheck revive goimports mockery protoc-gen-go protoc-gen-go-grpc swag gotestsum protoc; do
    printf '#!/bin/sh\nexit 0\n' > "$work/bin/$tool"; chmod +x "$work/bin/$tool"
done
export REAL_MAKE="$real_make"
cat > "$work/bin/make" <<'MOCK'
#!/usr/bin/env bash
set -eu
printf 'make %s\n' "$*" >> "$MOCK_LOG"
case "$*" in
 install-tools) "$REAL_MAKE" install-hooks;;
 '.env http-client-env') "$REAL_MAKE" .env http-client-env;;
 *) echo "Unexpected setup operation: $*" >&2; exit 95;;
esac
MOCK
chmod +x "$work/bin/make"
PG_FORMATTER="$formatter" bash scripts/setup-dev.sh > "$log" 2>&1
printf 'KEEP_LOCAL_VALUE=yes\n' > .env
before=$(sha256sum .env); hooks=$(sha256sum .git/config)
PG_FORMATTER="$formatter" bash scripts/setup-dev.sh > "$log" 2>&1
[[ $(sha256sum .env) == "$before" && $(sha256sum .git/config) == "$hooks" ]] || die 'setup overwrote local configuration or unnecessarily reset hooks'
[[ $(git config core.hooksPath) == .githooks ]]
pass 'setup delegates install-tools, installs existing hooks once and preserves .env on repeat'

printf 'PASS: %s developer-tooling scenarios.\n' "$passes"
