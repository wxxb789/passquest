# D-0001 — The harness is a Bun + TypeScript program

- **Status:** decided (owner directive, 2026-11-10)
- **Affects:** `harness/`, `scripts/`, `.github/workflows/`, `mise.toml`, `.gitignore`,
  [`harness.md`](../harness.md), [`conventions.md`](../conventions.md)

## Context

The owner develops this repository from Linux, macOS and Windows machines. The harness as
established by goal 0001 is a mix of `bash scripts/gate.sh`, `python3 scripts/check-docs-links.py`
and `bash scripts/check-secrets.sh`. That mix requires Git Bash plus a system Python on Windows,
and every new gate tends to arrive as one more one-off script. The harness is the part of the repo
that decides whether a claim is true, so it is treated as a serious project of its own, not as glue.

## Decision

1. The harness is **one TypeScript program run by Bun**:
   - entry point `bun run gate` (package.json script) which executes `harness/gate.ts`;
   - one module per gate under `harness/gates/<gate>.ts`, each exporting the same small interface
     (`id`, `run(): Promise<GateResult>`), with `pass | fail | skip(reason)` and numeric detail;
   - shared code under `harness/lib/` (process spawning, path resolution, output/reporting,
     repo-relative file walking). A gate that needs new plumbing adds it to `lib/`, it does not
     create a new script.
2. **Cross-platform by construction**: no `bash`, no `sh -c`, no `python3`. Process calls go through
   `Bun.spawn` with argument arrays and resolve Windows executable names (`gofmt.exe`, `go.exe`) from
   the environment rather than assuming a POSIX shell.
3. **Reuse before writing**: a new gate extends the existing runner/report primitives; duplicate logic
   between gates is a defect in the harness, not an accepted cost.
4. **One entry point survives**: CI calls exactly one command, and a local run calls exactly the same
   command. A second gate definition is a broken workflow.
5. **Dependencies stay near zero**: the Go toolchain provides the language gates; Bun/TypeScript
   provides the harness. A new npm/Bun dependency needs the same justification sentence a new Go
   module needs.
6. **Repository hygiene**: `bun.lock` is committed (it is a lockfile, not build output);
   `node_modules/`, `.bun/`, `*.tsbuildinfo`, `coverage/` and build output stay gitignored. Nothing
   generated is committed; nothing needed for a reproducible run is ignored.
7. **Cross-platform is only claimed where proven**: CI runs the gate matrix on ubuntu, macos and
   windows with `oven-sh/setup-bun`. A green Linux job is not evidence for Windows.

## Migration

The bash/Python gates are migrated in the first increment that touches the harness for another
reason (goal 0002 adds import/OCR gates), in the same increment, not as a separate cleanup ticket.
Until that increment lands, `bash scripts/gate.sh` remains the honest entry point and this file is the
binding rule for **any new harness code**: new checks are written directly as TypeScript gate modules.

## Migration outcome (2026-11-10, goal 0002)

The migration landed in the same increment as the import/OCR gates, as required.

- `bun run gate` executes `harness/gate.ts`; 12 gates run in order (typecheck, gofmt, vet, build,
  test, tidy, docs links, secrets/sources, local-only, baseline integrity, import smoke, harness
  self-test).
- `scripts/check-docs-links.py`, `scripts/check-secrets.sh` and `scripts/check-local-only.sh` were
  deleted. `scripts/gate.sh` is a one-line compatibility shim that execs `bun run gate`; no second
  gate definition exists.
- `harness/gates/self-test.ts` feeds each other gate a broken input and fails if the gate does not
  fail (11/11 gates at this increment; the self-test now runs 14 cases over the same 11 gates). CI runs `bun install --frozen-lockfile` then `bun run gate` on
  ubuntu, macos and windows with `oven-sh/setup-bun`; `.gitattributes` pins LF.
- Two dev dependencies (`typescript`, `@types/bun`) exist only for the typecheck gate; `bun.lock` is
  committed. No Bun package is a product dependency.
- Deviation from the original expectation: the harness now needs `bun install` once for `tsc`; a
  contributor without Bun still cannot run the gate (already accepted above). The Go toolchain
  remains the only runtime requirement for the product.

## Consequences and risks

- `bun` becomes a pinned tool alongside Go and Node in `mise.toml`; a contributor without Bun cannot
  run the gate. Accepted: the owner directive names Bun explicitly.
- Goal 0001's evidence records were produced by the bash harness. They stay valid as history; the
  migrated harness must still fail on the failures those gates catch (covered by a harness
  self-test that feeds each gate a deliberately broken input).
- A harness self-test becomes part of the gate chain, so "the gate cannot see the defect" is itself
  a detectable failure.

## Revisit when

Bun is unusable on a target platform, a gate needs a capability Bun lacks, or the matrix shows a
platform-specific harness failure that cannot be fixed without a second runtime.
