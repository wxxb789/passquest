#!/usr/bin/env bash
# Fails on committed credentials, private keys, local state, and source material that must stay
# private. Tracked files only: an untracked .env is a local state, not a leak.
set -uo pipefail
cd "$(dirname "$0")/.."

fail=0
bad() { printf 'FAIL  %s\n' "$*"; fail=1; }

tracked="$(git ls-files)"

# 1. Credential-shaped strings
patterns='sk-[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|gh[pousr]_[A-Za-z0-9]{30,}|xox[baprs]-[A-Za-z0-9-]{10,}|AIza[0-9A-Za-z_-]{30,}|-----BEGIN [A-Z ]*PRIVATE KEY-----'
if hits="$(git grep -InE "$patterns" -- . 2>/dev/null)"; then
  bad "credential-shaped string in tracked files"
  printf '%s\n' "$hits" | head -20
fi

# 2. .env files other than the example
while IFS= read -r f; do
  [ -z "$f" ] && continue
  case "$f" in
    *.env.example|*/.env.example) continue ;;
  esac
  bad "tracked environment file: $f"
done <<< "$(printf '%s\n' "$tracked" | grep -E '(^|/)\.env($|\.)' || true)"

# 3. Private source / local state that must never ship
while IFS= read -r f; do
  [ -z "$f" ] && continue
  case "$f" in
    *.pdf|*.sqlite|*.sqlite-wal|*.sqlite-shm|*.db|data/private/*|data/local/*|local-data/*|.passquest/*)
      bad "private or binary state tracked: $f" ;;
  esac
done <<< "$tracked"

# 4. Generic key assignment with a literal value (long enough to be real).
# Both YAML/TOML style (key: "value") and JSON style ("key": "value") must match.
if hits="$(git grep -InE '"?(api[_-]?key|secret|password|token|client[_-]?secret)"?[[:space:]]*[:=][[:space:]]*["'"'"'][A-Za-z0-9_/+-]{24,}["'"'"']' -- '*.go' '*.ts' '*.js' '*.json' '*.yaml' '*.yml' '*.toml' 2>/dev/null)"; then
  bad "literal secret assignment in tracked source"
  printf '%s\n' "$hits" | head -20
fi

if [ "$fail" -ne 0 ]; then
  exit 1
fi
echo "secrets/sources: clean ($(printf '%s\n' "$tracked" | wc -l) tracked files)"
