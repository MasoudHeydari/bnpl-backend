#!/usr/bin/env bash
# Safe repeated setup: no apt/sudo, containers, database actions or overwrites.
set -euo pipefail
source "$(dirname -- "${BASH_SOURCE[0]}")/lib/dev-tools.sh"
cd -- "$DEV_TOOLS_ROOT"
bash scripts/system-deps.sh --check
command -v go >/dev/null || { echo "Install the Go version declared in go.mod yourself; setup never replaces Go." >&2; exit 1; }
required=$(awk '/^go / {print $2}' go.mod)
actual=$(GOTOOLCHAIN=local go env GOVERSION)
[[ ${actual#go} == "$required" ]] || { echo "Expected Go $required from go.mod; found $actual. Select that toolchain manually." >&2; exit 1; }
tools_bin=$(go_tools_bin)
export PATH="$tools_bin:$PATH"
# install-tools already activates install-hooks; do not install the hook twice.
make install-tools
for tool in golangci-lint staticcheck revive goimports mockery protoc-gen-go protoc-gen-go-grpc swag gotestsum; do
    command -v "$tool" >/dev/null || { echo "Installed tool not found: $tool (expected in $tools_bin)" >&2; exit 1; }
done
require_formatter
[[ $(git config --local --get core.hooksPath) == .githooks && -x .githooks/pre-commit ]]
make .env http-client-env
echo "Development setup complete: system prerequisites, Go tools, pgFormatter and existing Git hooks verified."
printf 'For interactive Git hooks, add this directory to PATH if needed: %s\n' "$tools_bin"
echo "Review local .env values; install/configure Docker/Compose separately if needed. No containers or database commands were started."
