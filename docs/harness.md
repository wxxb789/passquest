# Harness

The harness is the part of this repository that makes a claim checkable by a machine. It is expected
to grow with the product: a new capability that nothing can verify is unfinished work.

Binding project conventions (stack, truth ownership, layout, uncertainty protocol):
[`conventions.md`](conventions.md).

## One entry point

```bash
bash scripts/gate.sh
```

### Harness platform (binding, 2026-11-10)

The harness is a **Bun + TypeScript program** — one module per gate under `harness/gates/`, shared
code under `harness/lib/` — because the owner runs this repository from Linux, macOS and Windows and
a bash/Python gate chain needs Git Bash plus a system Python on Windows. Decision and migration plan:
[`decisions/D-0001-bun-typescript-harness.md`](decisions/D-0001-bun-typescript-harness.md).

Until the migration lands (it ships with the increment that adds import/OCR gates), `scripts/gate.sh`
stays the honest entry point. **New checks are written directly as TypeScript gate modules** — never
as another `scripts/*.sh` or `scripts/*.py` one-off. `bun run gate` becomes the single entry point at
that point, and CI then runs the same command on ubuntu, macos and windows (`oven-sh/setup-bun`).

CI (`.github/workflows/ci.yml`) runs exactly this on every pull request and on `main`. There is no
second gate definition — if CI and a local run disagree, the script is the authority and the workflow
is broken.

## Gates, in order

| # | Gate | Command | Reading rule |
|---|------|---------|--------------|
| 1 | Format | `gofmt -l` over tracked `*.go` | Any listed file is a failure; fix formatting, do not add an ignore list. |
| 2 | Vet | `go vet ./...` | A vet diagnostic is a defect, not noise. |
| 3 | Build | `go build ./...` | Must succeed with an empty module cache too (CI proves it). |
| 4 | Test | `go test ./... -count=1` | No skipped tests. A test that only asserts a count is not verification. |
| 5 | Tidy | `go mod tidy -diff` | Empty diff; a dirty module file means a dependency was added or dropped silently. |
| 6 | Docs links | `python3 scripts/check-docs-links.py` | Relative links in markdown must resolve to a real path. |
| 7 | Secrets / sources | `bash scripts/check-secrets.sh` | Credentials, private keys, `.env` files and confidential source markers outside the allowlist fail the run. |
| 8 | Local-only CLI | `bash scripts/check-local-only.sh` | Pass Quest's own packages must not import `net`, `net/http`, `net/rpc` or `net/smtp`; runtime files must not contain the internal-only source marker. `net` is linked transitively by the pure-Go SQLite chain (`modernc.org/sqlite` → `libc` → `google/uuid`) and is never called; runtime proof belongs in the evidence record. |

Skipped gates print `SKIP` with a reason. A skip is only legitimate for a surface that does not exist
yet (for example: no `go.mod` before the first increment); a skip that hides a real surface is a
harness defect.

## What CI proves and what it does not

- CI runs on Linux only. Anything platform-specific (packaging, WebView, Anki) needs its own
  evidence and must not be claimed as verified from a green CI.
- CI is a floor. A merged pull request additionally needs the machine review marker and a local gate
  re-run on the exact head being merged.
- Do not run the full gate chain twice on one commit and call it two proofs. The local run on the
  final tree is the evidence; CI confirms it reproduces from a clean checkout.

## Growth policy

Add a gate when, and only when, a real failure mode exists that the current set cannot see:

| New surface | Gate it should bring |
|---|---|
| JSON/CLI protocol or persisted schema | golden-file or schema test, and a migration test from the previous version |
| Local practice CLI | first-party import scan for network packages and confidential markers (`check-local-only.sh`) |
| Import / OCR | fixture-based test with a deliberately damaged input, plus an explicit "unverified" state assertion |
| Provider call | recorded-fixture test with the network disabled; a failure path that cannot corrupt attempts |
| Anki integration | fake AnkiConnect server test (idempotency, conflict, offline queue) — never a real deck |
| Desktop shell | a smoke launch that fails on a blank window; screenshots alone are not evidence |

Rules for the harness itself:

- A check that always passes is worse than no check: it converts an unknown into a false green.
  Delete it or make it fail on the real defect.
- A check that depends on a network call belongs in CI only if the call is deterministic; otherwise
  record the fixture.
- Keep gate output short and numeric: counts, exit codes, paths.

## Environment

- Toolchain is pinned by `mise.toml` (Go, Node). A contributor without mise must still be able to run
  `scripts/gate.sh` with a matching Go on `PATH`.
- Private material lives under `data/private/`, `data/local/`, `local-data/`, or `.passquest/`
  (gitignored). Nothing there may be copied into `testdata/`.
