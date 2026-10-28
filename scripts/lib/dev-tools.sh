#!/usr/bin/env bash
# Shared defaults for Make, staged validation and developer setup. No installation
# or privileged operation happens when this file is sourced.
DEV_TOOLS_ROOT=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/../.." && pwd)
read -r pg_default_version pg_default_sha < "$DEV_TOOLS_ROOT/scripts/pgformatter.lock"
: "${PG_FORMATTER_VERSION:=$pg_default_version}"
if [[ $PG_FORMATTER_VERSION == "$pg_default_version" ]]; then
    : "${PG_FORMATTER_SHA256:=$pg_default_sha}"
fi
: "${PG_FORMATTER_INSTALL_DIR:=${XDG_DATA_HOME:-$HOME/.local/share}/bnpl-tools/pgFormatter/$PG_FORMATTER_VERSION}"
: "${PG_FORMATTER:=$PG_FORMATTER_INSTALL_DIR/bin/pg_format}"

formatter_version() {
    env GATEWAY_INTERFACE= PERL5OPT= PERL5LIB= LC_ALL=C "$1" --version
}

require_formatter() {
    if ! command -v "$PG_FORMATTER" >/dev/null 2>&1; then
        echo "pgFormatter is required. Run: make install-tools or make install-pg-formatter" >&2
        echo "Expected executable: $PG_FORMATTER" >&2
        return 1
    fi
    local actual
    actual=$(formatter_version "$PG_FORMATTER") || return 1
    if [[ $actual != "pg_format version $PG_FORMATTER_VERSION" ]]; then
        echo "pgFormatter version mismatch: expected $PG_FORMATTER_VERSION; got $actual" >&2
        echo "Run make install-pg-formatter or select the pinned executable with PG_FORMATTER." >&2
        return 1
    fi
}

# CLI imports and its pure-Perl build/manpage tooling; CGI/JSON are optional.
check_perl_dependencies() {
    if ! command -v perl >/dev/null 2>&1; then
        echo "Missing required system dependency: perl. Run: make install-system-deps" >&2
        return 1
    fi
    env PERL5OPT= PERL5LIB= perl -Mautodie -MEncode -MText::Wrap \
        -MGetopt::Long -MFile::Basename -MFindBin -MCarp -MExtUtils::MakeMaker \
        -e 1 2>/dev/null || {
        echo "Missing required Perl CLI/build modules (including autodie and ExtUtils::MakeMaker)." >&2
        echo "Run: make install-system-deps (no optional CGI/JSON or CPAN install is needed)." >&2
        return 1
    }
}

# Go chooses its own user bin directory; no system Go/toolchain is installed.
go_tools_bin() {
    command -v go >/dev/null || { echo "Go is required; select the version in go.mod manually." >&2; return 1; }
    local destination workspace
    destination=$(GOTOOLCHAIN=local go env GOBIN) || return 1
    if [[ -z $destination ]]; then
        workspace=$(GOTOOLCHAIN=local go env GOPATH) || return 1
        destination="${workspace%%:*}/bin"
    fi
    printf '%s\n' "$destination"
}
