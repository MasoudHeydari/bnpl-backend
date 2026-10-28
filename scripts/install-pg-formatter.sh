#!/usr/bin/env bash
# Official tagged source + reviewed archive digest; upstream MakeMaker installation
# is isolated to a versioned user-owned prefix. Never invokes sudo or CPAN.
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/dev-tools.sh"
for tool in curl tar gzip sha256sum make mktemp cmp; do
    command -v "$tool" >/dev/null || { echo "Missing $tool. Run: make install-system-deps" >&2; exit 1; }
done
check_perl_dependencies
[[ $PG_FORMATTER_VERSION =~ ^[0-9]+\.[0-9]+(\.[0-9]+)?$ ]] || { echo "Invalid stable pgFormatter version" >&2; exit 1; }
[[ ${PG_FORMATTER_SHA256:-} =~ ^[0-9a-f]{64}$ ]] || { echo "A reviewed PG_FORMATTER_SHA256 is required for this version." >&2; exit 1; }
target="$PG_FORMATTER_INSTALL_DIR/bin/pg_format"
if [[ -e $PG_FORMATTER_INSTALL_DIR || -L $PG_FORMATTER_INSTALL_DIR ]]; then
    if [[ -x $target ]] && [[ $(formatter_version "$target") == "pg_format version $PG_FORMATTER_VERSION" ]]; then
        echo "pgFormatter $PG_FORMATTER_VERSION already installed: $target"
        exit 0
    fi
    echo "Installation directory exists with a different/incomplete version: $PG_FORMATTER_INSTALL_DIR" >&2
    echo "Choose a new user-owned PG_FORMATTER_INSTALL_DIR; existing installations are not overwritten." >&2
    exit 1
fi
case $PG_FORMATTER_INSTALL_DIR in /*) ;; *) echo "PG_FORMATTER_INSTALL_DIR must be absolute." >&2; exit 1;; esac
parent=$(dirname -- "$PG_FORMATTER_INSTALL_DIR")
mkdir -p -- "$parent"
[[ -O $parent && -w $parent ]] || { echo "Installation parent must be user-owned and writable: $parent" >&2; exit 1; }
work=$(mktemp -d "$parent/.pgformatter-install.XXXXXX")
trap 'rm -rf -- "$work"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
url="https://codeload.github.com/darold/pgFormatter/tar.gz/refs/tags/v$PG_FORMATTER_VERSION"
echo "Downloading official pgFormatter $PG_FORMATTER_VERSION (SHA-256 verified)..."
curl --fail --silent --show-error --location --proto '=https' --tlsv1.2 \
    --connect-timeout 20 --max-time 180 --retry 2 "$url" -o "$work/release.tar.gz"
actual=$(sha256sum "$work/release.tar.gz"); actual=${actual%% *}
[[ $actual == "$PG_FORMATTER_SHA256" ]] || { echo "pgFormatter archive SHA-256 mismatch; refusing installation." >&2; exit 1; }
tar -xzf "$work/release.tar.gz" -C "$work"
source_dir="$work/pgFormatter-$PG_FORMATTER_VERSION"
[[ -f $source_dir/Makefile.PL ]] || { echo "Unexpected release layout." >&2; exit 1; }
stage="$work/stage"
# MakeMaker supports PERL_MM_OPT INSTALL_BASE; quote paths for its shellwords
# parser, not a shell evaluation. No source/eval of downloaded environment text.
quoted_stage=${stage//\'/\'\\\'\'}
(
    cd -- "$source_dir"
    env PERL5OPT= PERL5LIB= PERL_MB_OPT= PERL_MM_OPT="INSTALL_BASE='$quoted_stage'" perl Makefile.PL
    env PERL5OPT= PERL5LIB= PERL_MM_OPT= PERL_MB_OPT= make
    env PERL5OPT= PERL5LIB= PERL_MM_OPT= PERL_MB_OPT= make install
) > "$work/build.log" 2>&1 || { echo "pgFormatter build/install failed:" >&2; cat "$work/build.log" >&2; exit 1; }
# Upstream pg_format explicitly resolves RealBin/lib. Keep that supported layout
# so neither interactive PATH nor user PERL5LIB configuration is needed.
ln -s ../lib/perl5 "$stage/bin/lib"
[[ $(formatter_version "$stage/bin/pg_format") == "pg_format version $PG_FORMATTER_VERSION" ]]
# Do not merge with an installation created concurrently.
[[ ! -e $PG_FORMATTER_INSTALL_DIR && ! -L $PG_FORMATTER_INSTALL_DIR ]]
mv -T -- "$stage" "$PG_FORMATTER_INSTALL_DIR"
formatter_version "$target"
echo "Installed: $target"
