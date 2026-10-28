#!/usr/bin/env bash
# Only the explicit install mode may invoke apt/sudo. --check is unprivileged.
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/dev-tools.sh"
if [[ ${1:-} == --check ]]; then
    missing=()
    for tool in bash curl git make tar gzip sha256sum cmp mktemp find protoc gcc; do
        command -v "$tool" >/dev/null || missing+=("$tool")
    done
    if ((${#missing[@]})); then
        printf 'Missing required system dependencies: %s\nRun: make install-system-deps\n' "${missing[*]}" >&2
        exit 1
    fi
    check_perl_dependencies
    exit 0
fi
[[ $# == 0 ]] || { echo "Usage: $0 [--check]" >&2; exit 1; }
[[ $(uname -s) == Linux && -r /etc/os-release ]] || { echo "Automatic installation supports Ubuntu/Debian Linux only. Install the documented prerequisites manually, then run make setup-dev." >&2; exit 1; }
source /etc/os-release
case ${ID:-} in ubuntu|debian) ;; *) echo "Unsupported OS ${ID:-unknown}. Install the documented prerequisites manually, then run make setup-dev." >&2; exit 1;; esac
command -v apt-get >/dev/null && command -v dpkg-query >/dev/null || { echo "Ubuntu/Debian apt-get and dpkg-query are required." >&2; exit 1; }
# Full perl provides the core CLI/MakeMaker modules. build-essential supports
# existing Go CGO/race checks; protobuf-compiler supports the existing proto target.
packages=(ca-certificates curl git make tar gzip coreutils findutils perl build-essential protobuf-compiler)
missing=()
for package in "${packages[@]}"; do
    [[ $(dpkg-query -W -f='${Status}' "$package" 2>/dev/null || true) == 'install ok installed' ]] || missing+=("$package")
done
if ((${#missing[@]} == 0)); then
    echo "All supported system prerequisite packages are already installed; apt/sudo skipped."
    check_perl_dependencies
    exit 0
fi
printf 'System packages to install (no Go toolchain changes): %s\n' "${missing[*]}"
privilege=()
if ((EUID != 0)); then
    command -v sudo >/dev/null || { echo "sudo is unavailable. Ask your administrator to install the listed packages with apt-get." >&2; exit 1; }
    echo "This explicit system installation uses sudo apt-get; sudo may request your password."
    privilege=(sudo)
fi
"${privilege[@]}" apt-get update || { echo "apt-get update failed; check network/package sources." >&2; exit 1; }
"${privilege[@]}" apt-get install --no-install-recommends -y "${missing[@]}" || { echo "System package installation failed; rerun make install-system-deps after resolving apt errors." >&2; exit 1; }
check_perl_dependencies
