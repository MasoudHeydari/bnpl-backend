#!/usr/bin/env bash
# Preserve the existing official installer, pinned to the selected release tag.
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/dev-tools.sh"
version=${1:?version required}
bin=$(go_tools_bin)
if [[ -x $bin/golangci-lint ]] && [[ $("$bin/golangci-lint" version --short) == "${version#v}" ]]; then
    echo "golangci-lint $version already installed: $bin/golangci-lint"
    exit 0
fi
installer=$(mktemp)
trap 'rm -f -- "$installer"' EXIT
curl -fsSL --proto '=https' "https://raw.githubusercontent.com/golangci/golangci-lint/$version/install.sh" -o "$installer"
sh "$installer" -b "$bin" "$version"
