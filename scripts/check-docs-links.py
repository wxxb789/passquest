#!/usr/bin/env python3
"""Fail when a relative markdown link or image points at a path that does not exist."""

from __future__ import annotations

import re
import subprocess
import sys
import urllib.parse
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
LINK = re.compile(r"!?\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")
SKIP_DIRS = {".git", "node_modules", "dist", "build", ".obsidian"}


def tracked_markdown() -> list[Path]:
    try:
        out = subprocess.run(
            ["git", "ls-files", "*.md"], cwd=ROOT, capture_output=True, text=True, check=True
        ).stdout
        paths = [ROOT / line for line in out.splitlines() if line.strip()]
    except (subprocess.CalledProcessError, FileNotFoundError):
        paths = [p for p in ROOT.rglob("*.md") if not SKIP_DIRS & set(p.parts)]
    return [p for p in paths if p.is_file() and not SKIP_DIRS & set(p.relative_to(ROOT).parts)]


def main() -> int:
    problems: list[str] = []
    checked = 0
    for md in tracked_markdown():
        text = md.read_text(encoding="utf-8", errors="replace")
        for raw in LINK.findall(text):
            target = raw.split("#", 1)[0]
            if not target or "://" in target or target.startswith(("mailto:", "tel:", "data:")):
                continue
            if target.startswith("/"):
                resolved = ROOT / target.lstrip("/")
            else:
                resolved = (md.parent / urllib.parse.unquote(target)).resolve()
            checked += 1
            if not resolved.exists():
                rel = md.relative_to(ROOT)
                problems.append(f"{rel}: broken link -> {raw}")

    if problems:
        print("\n".join(problems))
        print(f"docs links: {len(problems)} broken of {checked} checked")
        return 1
    print(f"docs links: {checked} checked, 0 broken")
    return 0


if __name__ == "__main__":
    sys.exit(main())
