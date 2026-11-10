#!/usr/bin/env bash
# Compatibility shim. The single gate definition is harness/gate.ts, run by Bun;
# CI calls `bun run gate`. This wrapper keeps the documented `bash scripts/gate.sh`
# entry point working during the transition.
set -euo pipefail
cd "$(dirname "$0")/.."

if ! command -v bun >/dev/null 2>&1; then
  printf 'FAIL  bun is required to run the harness; install Bun 1.4.4 or newer\n' >&2
  exit 1
fi
exec bun run gate
