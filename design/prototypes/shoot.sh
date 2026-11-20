#!/usr/bin/env bash
# Screenshot every route of one prototype in light/dark and zh/en with headless Edge.
# Usage: bash design/prototypes/shoot.sh <prototype-file.html> <out-subdir> [width] [height]
# Example: bash design/prototypes/shoot.sh a-folio.html a
# Output: design/prototypes/_shots/<out-subdir>/<route>-<theme>-<lang>.png
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
FILE="$1"; OUT="$2"; W="${3:-1440}"; H="${4:-900}"
EDGE="/c/Program Files (x86)/Microsoft/Edge/Application/msedge.exe"
WIN_HERE="$(cygpath -m "$HERE")"
mkdir -p "$HERE/_shots/$OUT"
PROFILE="$(cygpath -w "${TMPDIR:-/tmp}")\\edge-shot-$OUT-$$"
ROUTES="${ROUTES:-today practice feedback assess report anki settings}"
COMBOS="${COMBOS:-light:zh dark:zh light:en}"
for r in $ROUTES; do
  for combo in $COMBOS; do
    theme="${combo%%:*}"; lang="${combo##*:}"
    png="$WIN_HERE/_shots/$OUT/$r-$theme-$lang.png"
    "$EDGE" --headless=new --disable-gpu --hide-scrollbars --no-first-run \
      --user-data-dir="$PROFILE" --window-size="$W,$H" --virtual-time-budget=4000 \
      --screenshot="$png" \
      "file:///$WIN_HERE/$FILE?theme=$theme&lang=$lang&shot=1#/$r" >/dev/null 2>&1 || echo "FAILED $r $theme $lang"
    echo "$png"
  done
done
