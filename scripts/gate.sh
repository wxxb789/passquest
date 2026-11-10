#!/usr/bin/env bash
# The one gate entry point for Pass Quest. CI runs exactly this.
# Reading rules for each gate: docs/harness.md
set -uo pipefail
cd "$(dirname "$0")/.."
export PATH="$HOME/.local/share/mise/shims:$PATH"

fail=0
step() { printf '\n== %s\n' "$*"; }
ok()   { printf 'PASS  %s\n' "$*"; }
bad()  { printf 'FAIL  %s\n' "$*"; fail=1; }
skip() { printf 'SKIP  %s\n' "$*"; }

# 1. Format
step "gofmt"
mapfile -t gofiles < <(git ls-files '*.go')
if [ "${#gofiles[@]}" -eq 0 ]; then
  skip "gofmt (no tracked Go files yet)"
else
  out="$(gofmt -l "${gofiles[@]}" 2>&1)"
  if [ -n "$out" ]; then bad "gofmt: unformatted files"; printf '%s\n' "$out"; else ok "gofmt"; fi
fi

# 2-5. Go gates (only once the module exists)
if [ ! -f go.mod ]; then
  skip "go vet/build/test/tidy (no go.mod yet)"
else
  step "go vet"
  if go vet ./... ; then ok "go vet"; else bad "go vet"; fi

  step "go build"
  if go build ./... ; then ok "go build"; else bad "go build"; fi

  step "go test"
  if go test ./... -count=1 ; then ok "go test"; else bad "go test"; fi

  step "go mod tidy -diff"
  if out="$(go mod tidy -diff 2>&1)"; then ok "go mod tidy"; else bad "go mod tidy (unexpected diff)"; printf '%s\n' "$out"; fi
fi

# 6. Documentation links
step "docs links"
if python3 scripts/check-docs-links.py ; then ok "docs links"; else bad "docs links"; fi

# 7. Secrets and private source material
step "secrets/sources"
if bash scripts/check-secrets.sh ; then ok "secrets/sources"; else bad "secrets/sources"; fi

# 8. Local-only runtime (no first-party network imports, no confidential markers)
step "local-only CLI"
bash scripts/check-local-only.sh
local_only_status=$?
case "$local_only_status" in
  0) ok "local-only CLI" ;;
  3) : ;; # the check printed SKIP with its reason
  *) bad "local-only CLI" ;;
esac

printf '\n'
if [ "$fail" -ne 0 ]; then
  printf 'GATE: FAIL\n'
  exit 1
fi
printf 'GATE: PASS\n'
