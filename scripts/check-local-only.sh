#!/usr/bin/env bash
# Fails when Pass Quest's own packages import a network package, when the CLI
# links an HTTP/RPC/SMTP client, or when runtime content carries a confidential
# source marker.
#
# `net` itself is linked transitively by modernc.org/sqlite -> modernc.org/libc
# -> github.com/google/uuid, which no Pass Quest code calls; the evidence record
# pins the runtime proof (strace: 0 network syscalls). First-party imports are
# what this gate can fail honestly.
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH="$HOME/.local/share/mise/shims:$PATH"

if [ ! -f go.mod ] || [ ! -d cmd/passquest ]; then
  echo "SKIP local-only CLI (no CLI surface yet)"
  exit 0
fi

fail=0
bad() { printf 'FAIL  %s\n' "$*"; fail=1; }

if ! package_list="$(go list ./cmd/... ./internal/... 2>&1)"; then
  bad "go list ./cmd/... ./internal/..."
  printf '%s\n' "$package_list"
  exit 1
fi
mapfile -t packages <<< "$package_list"
if [ "${#packages[@]}" -eq 0 ]; then
  bad "no first-party packages found"
  exit 1
fi

if ! imports="$(go list -f '{{.ImportPath}}:{{range .Imports}} {{.}}{{end}}' "${packages[@]}" 2>&1)"; then
  bad "go list imports"
  printf '%s\n' "$imports"
  exit 1
fi
if hits="$(printf '%s\n' "$imports" | grep -E ' (net|net/http|net/rpc|net/smtp)( |$)' || true)" && [ -n "$hits" ]; then
  bad "first-party network import"
  printf '%s\n' "$hits"
fi

if linked="$(go list -deps ./cmd/passquest 2>/dev/null | grep -E '^(net/http|net/rpc|net/smtp)$' || true)" && [ -n "$linked" ]; then
  bad "network client linked into the CLI"
  printf '%s\n' "$linked"
fi

# The internal-only notice must never appear in shipped code, content or tests.
# The absence test builds its marker strings from parts for this reason.
markers='仅限内部使用|严禁外传'
if hits="$(grep -RInE "$markers" --include='*.go' --include='*.json' --include='*.md' --include='*.txt' cmd internal 2>/dev/null || true)" && [ -n "$hits" ]; then
  bad "confidential source marker in runtime files"
  printf '%s\n' "$hits"
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
printf 'local-only: %d first-party packages, 0 network imports, 0 confidential markers\n' "${#packages[@]}"
