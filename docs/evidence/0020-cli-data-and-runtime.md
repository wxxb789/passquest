# 0020 — The command line keeps the learner's data where the window does, and the package runs on Node 26: evidence

Contract: [`../goals/0020-cli-data-and-runtime.md`](../goals/0020-cli-data-and-runtime.md).
Plan: [`../plans/2026-11-25-0900-0020-cli-data-and-runtime.md`](../plans/2026-11-25-0900-0020-cli-data-and-runtime.md).
Decisions: Q-0020-01…08 in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

Later steps append their dated notes; P5 completes the record. Every number below is read from the
artifact or the run its note names.

## Environment

Windows 11 Enterprise (10.0.26300), Go, Node and Bun through `mise` (the pins in `mise.toml`; the
Node pin is this step's own change). macOS and Linux were not available locally; CI's three-OS
matrix is the authority for their halves and the report says so per item. The machine is shared and
was at 97 % CPU load from other sessions during the run (the brief's known environment fact).

## P4 (2026-11-25) — Node 26.13.1 everywhere (branch `goal/0020-p4`, base `910a1b5`, PR stacked on P3)

**What changed.**

- `harness/lib/package.ts`: `NODE_VERSION` = `26.13.1` and the four `NODE_PINS` (archive, member,
  path, SHA-256); its header sentence names the same patch. New `NODE_PIN_SOURCES`,
  `readNodePinSources(root)` and `nodePinProblems(texts)`: one named failure per source that
  disagrees with `NODE_VERSION` or carries no version token.
- `harness/gates/package-smoke.ts`: `PackageSmokeOptions.nodePinSources` (label → text) for the
  self-test, and the gate runs the pin check at its start, before any case, over the repository's
  own files resolved from the gate module's own location (never `ctx.root`, which a self-test
  fixture points at a tree without `mise.toml` or `.github/`). The pass line ends with
  `node pin: 5 sources agree on Node <version>`.
- `mise.toml` (`node = "26.13.1"`), `.github/workflows/gate.yml` (`node-version: "26.13.1"`),
  `docs/install.md:7` and `docs/install.zh-CN.md:7` (the pinned-runtime sentence, both sides) and
  `docs/agent.md` (the runtime paragraph) name the same patch; `docs/decisions/OPEN-QUESTIONS.md`
  carries Q-0013-03's dated revisit and Q-0020-05's outcome.
- `harness/gates/self-test/package-smoke.cases.ts`: one broken case (a handed set whose
  `mise.toml` and `docs/install.md` disagree) and one accepted case (all five sources carry
  `NODE_VERSION`).

**The pin's provenance, measured at implementation time.** `https://nodejs.org/dist/index.json`
(fetched 2026-11-25) lists **v26.13.1** (2026-11-10, `"lts": "Lithium"`, `"security": true`) as the
newest 26.x release; v27 is still alpha. That release's `SHASUMS256.txt` states
`e2edda75bfb089654705cf65a13b58933bfd5f57148e8ebda916f58a2474f1f7  node-v26.13.1-win-x64.zip`,
`aec2514ea1ceb2ba52455e9ec65986f6749a06dd759e5db2a72e6889668c9735  node-v26.13.1-darwin-arm64.tar.gz`,
`82bb96078a24ca50b9c9a38f2379e6be6d87171fcd400fc14c8934b3bce96e2f  node-v26.13.1-darwin-x64.tar.gz`,
`5c063e46bab0fbc1c17cc445d7ff129cbdcd0471be6ed83b6ab864f8143bc864  node-v26.13.1-linux-x64.tar.xz`,
and all four archives were downloaded and their SHA-256 recomputed equal to those values before the
pins were written. The members the pins name were read from the archives' own listings:
`node-v26.13.1-win-x64/node.exe`, `node-v26.13.1-darwin-arm64/bin/node`,
`node-v26.13.1-darwin-x64/bin/node`, `node-v26.13.1-linux-x64/bin/node`. No newer 26 LTS existed, so
Q-0020-05's planned version was taken.

**The agreement check, and the lines when it refuses.** `mise exec -- node --version` printed
`v26.13.1` (the mise pin resolves). The two self-test cases were driven through the self-test's own
case objects and its `runCase`/`runPassCase` shape (the gate has no case selector): over a handed
set whose `mise.toml` and `docs/install.md` disagree, the gate's start refuses with exactly the two
named lines `mise.toml: pinned Node reads 22, want 26.13.1` and
`docs/install.md: pinned Node reads 22.23.5, want 26.13.1`; the accepted twin passes and the pass
line carries `node pin: 5 sources agree on Node 26.13.1`. With the repository's own files under
check, temporarily reverting `mise.toml` to `node = "22"` made the gate fail at its start with
`mise.toml: pinned Node reads 22, want 26.13.1`; the file was restored and the same check then read
"all sources agree". A source that is missing or unreadable, or whose sentence no longer carries a
version token, is a named failure too (`<label>: no pinned Node version found`), so deleting a
pinned sentence cannot silence the check.

**The self-test gate (local, whole gate, one run).** `harness-self-test` ran 1719s and reported
exactly one problem, in a goal-0022 P1 desktop-ux fixture case — "a faked title-bar room whose
clearance grew the util capsule with padding-right":
`desktop-ux: the faked title area broke an earlier stage: desktop-ux motion: script failed: mygo:
javascript error: Cannot read properties of null (reading 'click')`. A second, independent run of
that one case (the same fixture, the same runner shape) failed at a different stage with
`desktop-ux prepare: script failed: mygo: javascript error: timed out waiting for a practice item
[route #/today]` in 86s — the second signature the brief's known environment facts name for window
gates on this machine under load (it was at 97–100 % CPU during both runs). That case copies
product sources only (`DESKTOP_UX_SOURCES`) and drives a real hidden window; P4's diff changes none
of those files, no desktop-ux code and no page file, so the failure is not attributable to this
step, and CI's desktop group is the authority for it. The gate pushes one problem line per failed
case and this run pushed one, so no other case failed — P4's two pin cases included (their
case-level lines are above). `docs/harness.md` row 27's counts are read from CI's green self-test
pass line.

**Staging on the pinned runtime (local, Windows-x64).** A local `stageResources` call over this tree
— the same call the package gate's build makes, with the gate's own `stage:` prefix — printed
`stage: node 26.13.1: build/package/node/node.exe`,
`stage: check: node --version: v26.13.1`,
`stage: check: durable-host.mjs answered status on the staged runtime in 223 ms` and a staged tree
of 7 files, 101,713,517 bytes. The win-x64 archive came from the local download cache, and staging
verified that file's bytes against the new pin's SHA-256 on that run (the check runs on every use,
cached or downloaded).

**Smokes on Node 26 (local, this machine, one run each).**

| Gate | Pass line |
|---|---|
| agent smoke | `238 assertions, 11 scenarios x 2 hosts (source host, staged bundle), 49 provider requests, permissions skipped on Windows, bundle 1557656 bytes with 7 inlined packages` |
| tutor smoke | `156 assertions` |
| note smoke | `1367 assertions` |
| find smoke | `113 assertions` |
| assess smoke | `125 assertions` |
| provider smoke | `92 assertions` |

**Checks from P4's proof that are CI's, not this machine's.** The darwin-arm64, darwin-x64 and
linux-x64 runtimes were verified here by SHA-256 and member path only; they do not execute on
Windows. The three-OS smoke matrix and the `package-<os>` artifact sizes are read from the branch's
CI run; the Windows pair is recorded here because both halves were built locally from this tree.

**The installed artifact's size, before and after (Windows-x64, local pair).** Both halves are
`pass-quest-setup-0.15.1-amd64.exe` built from this worktree, read with `stat` (the gate's pass line
prints the same file as `32.8 MB`): the before is P3's own package run over this branch's base
(Node 22.23.5, 2026-11-25 17:09) at **28,842,773 bytes**; the after is this step's run
(Node 26.13.1, 2026-11-25 18:05) at **32,756,317 bytes** — +3,913,544 bytes (+13.6 %), which is the
larger win-x64 runtime (`node.exe` 87,077,704 B → 100,089,160 B in the staged trees) partly
recovered by the installer's compression. The per-OS pairs for macOS and Linux are read from the
branch CI run's `package-<os>` artifacts with their own run id (P5, or the coordinator's CI check);
the sizes are recorded, not judged, and a bigger package is the disclosed outcome here.

**The package smoke on the final tree (local, Windows-x64, one run).**

```
STATUS pass 572 assertions over contents, layout, study, cli-runtime, agents, brief, anki, upgrade,
credential, broken; pass-quest-setup-0.15.1-amd64.exe 32.8 MB (staged 7 files); contents scan: 9
files, 121 MB, … 7.0s; cli-runtime: …\home-study\AppData\Pass Quest\passquest.sqlite opened by the
installed binary with no node on PATH; stub/stub-model verified; upgrade: 32 tables, 160 rows, 1546
fields compared, 0 difference(s); schema 17 -> 17, unchanged (a reopen, no migration step); note 101
revised in place, queued note flushed through the window; schema 18 refused, 1 file(s) unchanged;
node pin: 5 sources agree on Node 26.13.1
```

`ELAPSED 433s` (7.2 minutes). The staged runtime the durable host and the brief case ran on is the
pin's own (the staging check above), and `cli-runtime` opened the window's database from the
installed binary with no `node` on `PATH` — P3's case, unchanged, on the new runtime. The raw line
carries the run's own temp path; it is elided above, as every machine path in this record is.
