# Harness

The harness is the part of this repository that makes a claim checkable by a machine. It is expected
to grow with the product: a new capability that nothing can verify is unfinished work.

Binding project conventions (stack, truth ownership, layout, uncertainty protocol):
[`conventions.md`](conventions.md).

## One entry point

```bash
bun run gate
```

`bash scripts/gate.sh` remains a compatibility shim that execs the same command. There is no second
gate definition: CI and a local run both call `harness/gate.ts` through the root `gate` script.

The harness is a **Bun + TypeScript program** — one module per gate under `harness/gates/`, shared
code under `harness/lib/`. Decision and rationale:
[`decisions/D-0001-bun-typescript-harness.md`](decisions/D-0001-bun-typescript-harness.md). The
migration landed with goal 0002; no `bash` or `python3` gate scripts remain.

CI (`.github/workflows/gate.yml`) runs `bun install --frozen-lockfile` and then `bun run gate` on
ubuntu, macos and windows. A green Linux job is not evidence for Windows; the matrix is the evidence.

## Gates, in order

| # | Gate | Module | Reading rule |
|---|------|--------|--------------|
| 1 | Typecheck | `harness/gates/typecheck.ts` | `tsc --noEmit` over `harness/**`; a diagnostic is a defect. Requires `bun install`; `bun.lock` is committed. |
| 2 | Format | `harness/gates/gofmt.ts` | Any listed tracked `*.go` file is a failure; fix formatting, do not add an ignore list. |
| 3 | Vet | `harness/gates/go-vet.ts` | A vet diagnostic is a defect, not noise. |
| 4 | Build | `harness/gates/go-build.ts` | Must succeed with an empty module cache too (CI proves it). |
| 5 | Test | `harness/gates/go-test.ts` | No skipped tests: `go test -json` test-level `skip` actions fail the gate and are named. A test that only asserts a count is not verification. |
| 6 | Tidy | `harness/gates/go-tidy.ts` | Empty diff; a dirty module file means a dependency was added or dropped silently. |
| 7 | Docs links | `harness/gates/docs-links.ts` | Relative links in tracked markdown must resolve to a real path. |
| 8 | Secrets / sources | `harness/gates/secrets.ts` | Credentials, private keys, `.env` files and confidential source files outside the allowlist fail the run. Finding locations are printed, never the matched secret. Every tracked text file is scanned; there is no size cap. |
| 9 | Local-only CLI | `harness/gates/local-only.ts` | Pass Quest's own packages must not import `net`, `net/http`, `net/rpc` or `net/smtp`; files under `cmd/` and `internal/` — runtime **and** test files (`*.go`, `*.json`, `*.md`, `*.txt`) — must not contain the internal-only source marker. `net` is linked transitively by the pure-Go SQLite chain (`modernc.org/sqlite` → `libc` → `google/uuid`) and is never called; runtime proof belongs in the evidence record. |
| 10 | Baseline integrity | `harness/gates/baseline.ts` | The committed page-linked baseline must keep 165 pages / 22 native / 143 OCR, the manifest page set and the page files must be exactly 1..165, every manifest byte and non-space character count must match its file (Unicode `White_Space` plus form feed, matching `source.CountNonSpace`), each page must be valid UTF-8 (strict decode; a literal U+FFFD is allowed), the README must state `unreviewed`, and no Go test or `testdata/` fixture may reference `data/baselines`. The baseline is working context, never an answer key or fixture. |
| 11 | Import smoke | `harness/gates/import-smoke.ts` | Builds the real CLI and drives a deliberately damaged redistributable fixture: unreviewed pages and unmapped answers must be visible, practice must be refused until page and answer are reviewed, the pre-answer JSON must hide the key, an attempt must survive into a second process's report, and the database must be `0600`. 35 assertions on POSIX; on Windows the database-mode assertion is reported as skipped (34 assertions, 1 skipped) because Windows has no POSIX mode. |
| 12 | Tactic smoke | `harness/gates/tactic-smoke.ts` | Drives the real CLI through `tactic list`, `show`, the guided awaiting/answer pair, the independent awaiting/answer pair, and `report --json`. The lesson must carry why/when/steps/boundary; the guided awaiting document must be `assisted`, carry guidance and hide the key, and the guided answered document must carry the same guidance; the independent awaiting and answered documents must be `independent` and carry no guidance, with a different question and a key-hiding pre-answer document; the report must show one assisted and one independent attempt, both correct, with `correct`, `assisted_correct` and `independent_correct` visible; an exhausted independent request must exit 2. On a second database, a guided pre-answer document must record exposure without an attempt, the exposed question must be refused as independent (`invalid_input`), and the next guided request must resume that question. 70 assertions. |
| 13 | Harness self-test | `harness/gates/self-test.ts` | Feeds each other gate a deliberately broken input (19 cases over 12/12 gates). A gate that passes its broken input is itself a failure: it converts an unknown into a false green. |

Skipped gates print `SKIP` with a reason. A skip is only legitimate for a surface that does not exist
yet (for example: no `go.mod` before the first increment, no baseline before it is committed); a skip
that hides a real surface is a harness defect.

## What CI proves and what it does not

- CI runs the same command on Linux, macOS and Windows. Platform-specific packaging, WebView or Anki
  behaviour still needs its own runtime evidence; the matrix is not a desktop-app test.
- CI is a floor. A merged pull request additionally needs the machine review marker and a local gate
  re-run on the exact head being merged.
- Do not run the full gate chain twice on one commit and call it two proofs. The local run on the
  final tree is the evidence; CI confirms it reproduces from a clean checkout.
- Do not run the import smoke or a benchmark concurrently with the full Go test suite on a
  memory-constrained host; they are cheap, but the Go toolchain parallelises packages by default.

## Growth policy

Add a gate when, and only when, a real failure mode exists that the current set cannot see:

| New surface | Gate it should bring |
|---|---|
| JSON/CLI protocol or persisted schema | golden-file or schema test, and a migration test from the previous version |
| Local practice CLI | first-party import scan for network packages and confidential markers (`local-only.ts`) |
| Import / OCR | fixture-based test with a deliberately damaged input, plus an explicit "unverified" state assertion (`import-smoke.ts` and Go tests) |
| Scoring tactic / assisted evidence | guided/independent smoke with key hiding, stage gating and an assisted-vs-independent report split (`tactic-smoke.ts`) |
| Private source baseline | provenance/manifest integrity and test/fixture isolation (`baseline.ts`) |
| Provider call | recorded-fixture test with the network disabled; a failure path that cannot corrupt attempts |
| Anki integration | fake AnkiConnect server test (idempotency, conflict, offline queue) — never a real deck |
| Desktop shell | a smoke launch that fails on a blank window; screenshots alone are not evidence |

Rules for the harness itself:

- A check that always passes is worse than no check: it converts an unknown into a false green.
  Delete it, or add it to `harness/gates/self-test.ts` with an input that must fail.
- A check that depends on a network call belongs in CI only if the call is deterministic; otherwise
  record the fixture.
- Keep gate output short and numeric: counts, exit codes, paths.
- New checks are TypeScript gate modules sharing `harness/lib/`; never another `scripts/*.sh` or
  `scripts/*.py` one-off. Duplicate logic between gates is a harness defect.

## Environment

- Toolchain is pinned by `mise.toml` (Go, Node, Bun). A contributor without mise needs matching Go
  and Bun on `PATH`; `python3` and a POSIX shell are not required.
- `bun install` installs the two dev dependencies (TypeScript and Bun types) used by the typecheck
  gate; `bun.lock` is committed so the install is reproducible.
- Private material lives under `data/private/`, `data/local/`, `local-data/`, or `.passquest/`
  (gitignored). Nothing there may be copied into `testdata/`.
