#!/usr/bin/env bash
# One read-only formatter invocation/policy for Make, Git index validation and CI.
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/dev-tools.sh"
mode=${1:-}; shift || true
case $mode in fmt|check|staged) ;; *) echo "Usage: $0 fmt|check|staged [-- SQL-files...]" >&2; exit 1;; esac
root=$(git rev-parse --show-toplevel)
cd -- "$root"
work=$(mktemp -d)
replacement=
cleanup() {
    if [[ -n $replacement ]]; then rm -f -- "$replacement"; fi
    rm -rf -- "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
applicable() {
    case $1 in vendor/*|*/vendor/*|gen/*|*/gen/*|third_party/*|*/third_party/*|node_modules/*|*/node_modules/*|build/*|dist/*|tmp/*|.git/*) return 1;; esac
    [[ $1 =~ \.[sS][qQ][lL]$ ]]
}
if [[ $mode == staged ]]; then
    [[ $# == 0 ]] || { echo "staged checks always inspect the Git index" >&2; exit 1; }
    git diff --cached --name-only --diff-filter=ACMR -z -M -C > "$work/candidates"
elif [[ $# == 0 ]]; then
    git ls-files --cached --others --exclude-standard -z > "$work/candidates"
else
    [[ $1 != -- ]] || shift
    printf '%s\0' "$@" > "$work/candidates"
fi
files=()
while IFS= read -r -d '' path; do
    applicable "$path" || continue
    if [[ $mode == staged ]]; then
        object_mode=$(git ls-files --format='%(objectmode)' -- "$path")
        [[ $object_mode == 100644 || $object_mode == 100755 ]] || continue
    else
        [[ -f $path && ! -L $path ]] || continue
    fi
    files+=("$path")
done < "$work/candidates"
if ((${#files[@]} == 0)); then
    echo "No applicable SQL files; SQL formatting check skipped."
    exit 0
fi
config="$root/.pg_format"
if [[ $mode == staged ]]; then
    # Stage policy together with SQL. Unstaged config edits must not make an
    # invalid staged blob pass a different policy than CI will see.
    git cat-file blob ':0:.pg_format' > "$work/config" || { echo "Stage the project .pg_format policy before staging SQL." >&2; exit 1; }
    config="$work/config"
    if git cat-file -e ':0:scripts/pgformatter.lock' 2>/dev/null; then
        read -r staged_version staged_sha < <(git cat-file blob ':0:scripts/pgformatter.lock')
        [[ $PG_FORMATTER_VERSION == "$staged_version" ]] || { echo "Formatter must match staged pgformatter.lock ($staged_version)." >&2; exit 1; }
    fi
fi
[[ -f $config ]] || { echo "Missing project .pg_format policy" >&2; exit 1; }
require_formatter
failures=0; formatted=0
for path in "${files[@]}"; do
    if [[ $mode == staged ]]; then
        git cat-file blob ":0:$path" > "$work/input"
    else
        cp -- "$path" "$work/input"
    fi
    # No in-place or output-file option may leak through config. SQL is read
    # only from stdin; checks compare temporary output and never execute SQL.
    cp -- "$work/input" "$work/previous"
    stable=0
    # Some upstream layouts (notably a long CREATE TABLE) need a second pass.
    # Use the same bounded fixed point in fmt/check/hook, never different rules.
    for attempt in 1 2 3 4 5 6; do
        env GATEWAY_INTERFACE= PERL5OPT= PERL5LIB= LC_ALL=C "$PG_FORMATTER" \
            --no-rcfile --config "$config" --no-inplace --output - - \
            < "$work/previous" > "$work/output"
        if cmp -s -- "$work/previous" "$work/output"; then stable=1; break; fi
        cp -- "$work/output" "$work/previous"
    done
    if ((stable == 0)); then
        printf 'pgFormatter did not converge for %q; that file was not changed.\n' "$path" >&2
        exit 1
    fi
    if cmp -s -- "$work/input" "$work/output"; then continue; fi
    if [[ $mode == fmt ]]; then
        cmp -s -- "$path" "$work/input" || { printf 'File changed during formatting: %q\n' "$path" >&2; exit 1; }
        # Preserve permissions and replace atomically; already formatted files
        # never get rewritten. Temporary output lives outside the Git index.
        replacement=$(mktemp "$(dirname -- "$path")/.sql-format.XXXXXX")
        if ! { cat "$work/output" > "$replacement" && chmod --reference="$path" "$replacement" && mv -fT -- "$replacement" "$path"; }; then
            rm -f -- "$replacement"
            exit 1
        fi
        replacement=
        printf 'Formatted: %q\n' "$path"
        formatted=$((formatted+1))
    else
        printf 'SQL needs formatting: %q\n' "$path" >&2
        failures=$((failures+1))
    fi
done
if ((failures)); then
    echo "SQL validation failed. Run make sql-fmt (or bash scripts/sql-format.sh fmt -- <paths>), then restage the intended SQL with git add -p or git add -- <paths>." >&2
    echo "The hook/check did not modify the working tree or index." >&2
    exit 1
fi
printf 'SQL %s passed: %s files; %s formatted.\n' "$mode" "${#files[@]}" "$formatted"
