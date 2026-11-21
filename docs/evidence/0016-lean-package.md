# 0016 — A lean package that ships only what the app runs: evidence

Contract: [`../goals/0016-lean-package.md`](../goals/0016-lean-package.md).
Plan: [`../plans/2026-11-20-2100-0016-lean-package.md`](../plans/2026-11-20-2100-0016-lean-package.md).
Guide: [`../install.md`](../install.md). Decisions: Q-0016-01…12 in
[`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

## Environment

Windows 11 Enterprise (10.0.26300), 16 logical CPUs, Go 1.27.4, Bun 1.4.5, mise Node 22.23.4 for the
harness, and the package's own pinned Node 22.23.5 (`NODE_PINS`, Q-0013-03) everywhere a package
runtime is named. macOS arm64 and Linux x86_64 were not run locally: CI's `macos-latest` and
`ubuntu-latest` `package` jobs are their only evidence. This machine was at **100% CPU** (other
sessions) for every local number below.

Two trees hold the before/after numbers:

| | Tree | Commit | Role |
|---|---|---|---|
| before | detached worktree `Q:/repos/passquest-base` | `4633b07` (the goal base) | the last tree that staged the hosts' production `node_modules`; CI's `main` before the goal |
| after | this checkout (`goal/0016-lean-package`, PR #39) | `2fb97f2` (P4) | the branch CI ran; P5 (this step) changes these documents and one comment in a self-test case file, so the package is byte-identical to what this tree stages |

## Size: staged tree, installed tree, package

### Cited from named CI runs

CI run **44735711698** (`main`, `135c8508`, green) is the "before"; CI run **44854554521** (this
branch at `2fb97f2`, green, PR #39) is the "after". Both figures per OS come from the same reading —
the package job's own pass line (`staged N files`, then `contents scan: N files, M MB`).

| OS | run | staged files | installed tree, uncompressed (contents scan) | package (gate's reading) | artifact bytes (`gh api …/artifacts`) |
|---|---|---|---|---|---|
| Windows x64 | 44735711698 | 12,774 | 12,776 files / 196 MB | 36.5 MB | 36,503,189 |
| Windows x64 | 44854554521 | **7** | **9 files / 108 MB** | **28.8 MB** | 28,826,835 |
| macOS arm64 | 44735711698 | 12,774 | 12,778 files / 225 MB | 45.0 MB | 44,957,966 |
| macOS arm64 | 44854554521 | **7** | **11 files / 133 MB** | **31.9 MB** | 31,939,416 |
| Linux x86_64 | 44735711698 | 12,774 | 12,776 files / 233 MB | 63.5 MB | 62,908,027 |
| Linux x86_64 | 44854554521 | **7** | **9 files / 146 MB** | **48.6 MB** | 48,049,371 |

Change, on the uniquely comparable readings: staged files 12,774 → 7; installed tree 196 → 108 MB
(Windows), 225 → 133 MB (macOS), 233 → 146 MB (Linux); package 36.5 → 28.8 MB, 45.0 → 31.9 MB,
63.5 → 48.6 MB. One percentage per package, computed from the artifact bytes: −21.0% / −29.0% /
−23.6% (1 − after/before of 28,826,835/36,503,189, 31,939,416/44,957,966 and 48,049,371/62,908,027);
the artifacts are the workflow's zip of the package, a second, differently-sized reading of the same
file.

The v0.15.0 release assets are the published release's own bytes — `pass-quest-setup-0.15.0-amd64.exe`
36,501,195 B, `pass-quest-0.15.0-arm64.dmg` 44,963,840 B and `Pass_Quest-0.15.0-x86_64.AppImage`
63,490,552 B (tag `v0.15.0`, commit `c9a83a0`) — and they agree with the before row's rounded
36.5 / 45.0 / 63.5 MB readings, not with its artifact bytes, which are run 44735711698's own zips of
a `main` `135c8508` build: the two readings differ by 1,994 B / 5,874 B / 582,525 B. No installer size
is promised (the plan's own risk note: a tree a compressor already handled well shrinks less than it
looks).

The staged tree is the same seven files on all three OSes, but not the same bytes: each OS packs its
own pinned Node runtime — `node.exe` 87,077,704 B (Windows x64), 113,024,160 B (macOS arm64) and
124,909,296 B (Linux x64), read from the archives `NODE_PINS` names and each verified against its
pin's SHA-256 here — so the same payload with that OS's runtime in place of `node.exe` is
114,648,517 B (109.3 MiB) on macOS arm64 and 126,533,653 B (120.7 MiB) on Linux x64, against
Windows' 88,702,061 B (84.6 MiB), each within 100 B (the per-OS bundles: CI printed 1,557,686 /
1,557,708 / 1,557,656 B, this machine 1,557,628 B). **84.6 MiB is therefore a Windows figure**; macOS
and Linux have no printed staged-byte total at all, and their own size is what their installed-tree
scan reads.

The installed-tree scan counts every readable payload file except the container itself, so it
includes that OS's bundled Node runtime (87,077,704 B on Windows) and the app's own scaffolding: on
Linux the AppImage's extract-and-run tree, which is larger than the 48.6 MB image. Of the 88 packages
staging shipped before, 7 are reachable from the bundled hosts (plan's scout: `openai`,
`@earendil-works/pi-durable`, `pi-schema`, `pi-ai`, `chord`, `pi-telemetry`, `partial-json`); the
other 81 — provider SDKs, `photon`, `quickjs-wasi` and their trees — are gone.

### Re-derived on this machine, both trees (`bun run package`, Windows x64)

| Tree | Command | Staged tree | `agent/` | `node/` | icon |
|---|---|---|---|---|---|
| before `4633b07` | `mise exec -- bun run package` (after `bun install --frozen-lockfile`) | **12,774 files / 176,191,950 B (168.0 MiB)** | 12,772 files / 89,102,167 B, of which `node_modules` 12,768 files / 89,040,427 B | 1 file / 87,077,704 B | 12,079 B |
| after `2fb97f2` | `mise exec -- bun run package` | **7 files / 88,702,061 B (84.6 MiB)** | 5 files / 1,612,278 B (bundle 1,557,628 + three copies 8,913 / 8,967 / 11,697 + notices 25,073) | 1 file / 87,077,704 B | 12,079 B |

The before row reproduces the plan's scouted `measure-staged.mjs` figures byte for byte (12,774 files
/ 176,191,950 B; `node_modules` 12,768 files / 89,040,427 B). The after row matches CI's `staged 7
files` on all three OSes. The three copied hosts are byte-identical to their sources (sha256
`f0fdb2f7…`, `ba701c5c…`, `43a3a60a…` for `anki-host.mjs`, `tutor-host.mjs`, `search-host.mjs`).

The Windows installer re-derived here is **28,835,896 B (28.8 MB)** on the final tree against
**36,502,438 B (36.5 MB)** on the base tree — the same 28.8 MB CI read on the branch. The staging log
lines are in `Q:/scratch/omp/g16/P5/package-final.log`: `stage: agent: 4 host scripts (1 bundled, 3
copied byte-identical)`, `stage: check: agent is lean: 5 files (host scripts and
THIRD-PARTY-NOTICES.txt only), no node_modules, no symlinks, no package specifier, no machine path`,
`stage: check: node --version: v22.23.5` and `stage: check: durable-host.mjs answered status on the
staged runtime in 401 ms`.

## Cold start of the durable host

Process start → first `status` answer, n=15 per tree, measured alternately (the order swapped every
round) so a drifting machine load cannot land on one side; both on the package's own staged Node
22.23.5 (the same `node.exe` on both sides: 87,077,704 B, sha256 `247e9693…`). The request is
`status-req.json` beside the script — `{"protocol_version":1,…,"store_path":"…/status-probe.sqlite",
"mode":"status"}` with an existing empty store file (without one the host answers `state=failed`) —
and every sample below answered `state=status`:

| Host | min | median | max |
|---|---|---|---|
| before: staged unbundled `agent/durable-host.mjs` (32,163 B) beside `agent/node_modules` | 951.6 ms | 1,234.4 ms | 2,443.8 ms |
| after: staged bundle `agent/durable-host.mjs` (1,557,628 B), no `node_modules` | **636.7 ms** | **851.2 ms** | **1,207.1 ms** |

All 30 samples, in the order they were taken (ms):

| Tree | samples 1–15 |
|---|---|
| before | 1191.1, 1129.5, 1363.1, 1057.5, 951.6, 1123.5, 1321.8, 1234.4, 1531.0, 2443.8, 1285.3, 1302.6, 1301.7, 1142.4, 1147.9 |
| after | 805.4, 768.7, 784.3, 636.7, 745.0, 851.2, 1168.0, 929.4, 876.1, 672.7, 976.5, 1207.1, 902.7, 1102.6, 833.8 |

Reproducible: `Q:/scratch/omp/g16/P5/coldstart-ab.mjs`, run from the repository root as
`mise exec -- node Q:/scratch/omp/g16/P5/coldstart-ab.mjs 15 <before runtime> <before host> <after
runtime> <after host> <request file>` (driver Node v22.23.4; before = the base worktree's staged
package, after = this tree's). Its output is published whole in
`Q:/scratch/omp/g16/P5/coldstart-final.log` (run 2026-11-21T10:59:39+08:00), which carries the raw
lines, the command and both tree paths. CPU load 100%
(`(Get-CimInstance Win32_Processor).LoadPercentage`) before the run and after it. The medians moved
1,234.4 → 851.2 ms (−31%, from the samples above): the absolute numbers are the machine's load
talking as much as the bundle — P0's probe, as the plan's facts section records it, measured the same
pair at a lighter load as 439.4 / 503.7 / 569.3 ms (unbundled) and 247.5 / 268.1 / 299.3 ms (bundle),
and under 100% CPU both sides slow down while the bundle keeps its lead. Reported, not gated
(Q-0016-06): a single `status` answer at staging is the only assertion, and it is a load proof that a
plain bundle passes too — behaviour is `agent smoke`'s and `brief`'s job.

Only the durable host is measured: the plan's wording was `per host`, and the durable host is the one
host whose staging changed (Q-0016-06 records that narrowing). The other three are staged as
byte-identical copies of their sources (sha256 above), so their start path is the same bytes it was
before the goal.

## Third-party notices (the produced file)

`agent/THIRD-PARTY-NOTICES.txt` inside every package: **25,073 bytes**, sha256
`5b9d32a7c78e8277ce39d81246a839f2f0f70902f760ce9020c07e8db63acc83`, **7 packages / 11 sections** (22
rule lines). Staging wrote it from the packages the one bundle inlines — log line `stage:
agent/THIRD-PARTY-NOTICES.txt: 7 inlined package(s), 11 text(s), 25073 bytes` — and the package
smoke's contents check verified `11 notice text(s) against the staged bundles' set` in the Windows,
macOS and Linux package jobs of run 44854554521.

| Package | Licence | Source in the package | Copyright line |
|---|---|---|---|
| `@earendil-works/chord@1.5.2` | MIT | SPDX fallback — the tarball ships no licence file | Earendil Works |
| `@earendil-works/pi-ai@1.5.2` | MIT | SPDX fallback | Mario Zechner |
| `@earendil-works/pi-durable@1.5.2` | MIT | SPDX fallback | Earendil Works |
| `@earendil-works/pi-schema@1.5.2` | MIT | SPDX fallback (its own terms; its only licence file is TypeBox's, below) | Earendil Works |
| `@earendil-works/pi-schema@1.5.2` | MIT | `THIRD_PARTY_NOTICES/typebox-LICENSE` (TypeBox, inlined into `pi-schema`'s dist) | 2017-2026 Haydn Paterson |
| `@earendil-works/pi-telemetry@1.5.2` | MIT | SPDX fallback | Mario Zechner |
| `openai@7.19.0` | Apache-2.0 | `LICENSE` | 2026 OpenAI |
| `openai@7.19.0` | MIT | `src/_vendor/partial-json-parser/LICENSE` | 2023 Promplate Dev Team |
| `openai@7.19.0` | ISC | `src/_vendor/zod-to-json-schema/LICENSE` | 2020 Stefan Terdell |
| `openai@7.19.0` | BSD-3-Clause | `src/internal/qs/LICENSE.md` | 2014 Nathan LaFreniere and contributors |
| `partial-json@0.1.7` | MIT | `LICENSE` | 2023 Promplate Dev Team |

The preamble and the SPDX fallback texts are the harness's own wording (`harness/lib/notices.ts`);
every other byte is copied from a package file, and the file is held to the machine-path rule — only
the checkout root is refused in it, in both spellings (`docs/cross-platform.md`'s packaging bullet;
taken as the machine-path clause of Q-0016-01).

## Gate runs

| Run | Result |
|---|---|
| CI 44854554521 (`2fb97f2`, PR #39) | 12/12 jobs green. `package smoke` 532 assertions (Windows) / 531 (macOS, Linux) over `contents, layout, study, agents, brief, anki, upgrade, credential, broken`; `agent smoke` 238 assertions (Windows) / 242 (macOS, Linux), 11 scenarios × 2 hosts (source host, staged bundle), 50 provider requests, bundle 1,557,686 / 1,557,708 / 1,557,656 B with 7 inlined packages; `harness self-test` 83 cases (9 must pass), 24/24 gates |
| CI 44735711698 (`main`, `135c8508`) | 12/12 green; `package smoke` 487 / 486 / 486 assertions over the eight old cases, the before numbers above |
| `mise exec -- bun run package` (this tree, 2026-11-21) | staged 7 files, 88.7 MB; `pass-quest-setup-0.15.0-amd64.exe` 28,835,896 B; staging checks and the staged-runtime `status` proof printed |
| `mise exec -- bun run package` (base tree, 2026-11-21) | staged 12,774 files, 176 MB; installer 36,502,438 B |
| cold start, both trees (this tree, 2026-11-21) | the table and the 30 raw samples above, all `state=status`; `Q:/scratch/omp/g16/P5/coldstart-final.log` |
| `docs-links`, `secrets` and `typecheck` gates (this step's final tree) | PASS — `294 checked, 0 broken`; `755 files, 0 findings`; `0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `gofmt -l internal cmd`, `go build ./...` (this step, final tree) | `gofmt` prints nothing and the build succeeds; no Go file changed in the whole goal (row below) |
| `harness self-test` gate (this step, final tree) | PASS — `83 cases (9 must pass), 24/24 gates` in 1491 s, load 100% before and after (`Q:/scratch/omp/g16/P5/selftest-final.log`); this step's only change under `harness/` is a comment in `package-smoke.cases.ts` |
| `git diff 4633b07..HEAD -- agent internal cmd '*.go' mygo.json bun.lock agent/package.json` | empty: no host script, Go file, lock file or module manifest changed in the whole goal |

The self-test's case count is read two ways that agree: the three CI jobs print `83 cases (9 must
pass), 24/24 gates`, and counting a case's own `label` field — `grep -h '^      label: "'
harness/gates/self-test/*.cases.ts | wc -l`, six spaces deep (a nested `label` field elsewhere in
those files is not a case) — gives 83 over the 24 case files without running the gate.

## Done when → evidence

| # | Contract item | Evidence |
|---|---|---|
| 1 | Only code that runs is shipped | Staging and the package smoke's contents check both run `assertLeanAgentTree` (`harness/lib/package.ts`): the installed `agent/` holds the four hosts and `THIRD-PARTY-NOTICES.txt` only — no `node_modules`, no symlink, no other file type, no staged file naming a package specifier, no file embedding the machine path of the builder. Self-test must-fail fixtures: `installed agent tree carrying node_modules`, `installed agent host naming a package specifier`, `installed agent file embedding a machine path`; the staging-side lean check and the run-time-resolution refusal are driven through staging by the `stagingRefusalsProblem` unit. The tree itself: the staged `agent/` went 12,772 files / 89.1 MB → 5 files / 1.6 MB and the whole staged tree 12,774 files → 7 on all three OSes, with the package bytes following (table above) |
| 2 | The smaller package is measured, not asserted | The two tables above give, per OS's package job, the staged file count, the installed tree (the uncompressed reading), the package size and the uploaded artifact bytes, each citing its CI run (the v0.15.0 release assets stand beside the before row), plus the local re-derivation of both trees; no number is promised that a run did not print |
| 3 | The study brief works in the installed app | The `brief` case (`harness/gates/package-smoke/brief.ts`) in the three package jobs of run 44854554521: the installed binary's own CLI pauses a run (exit 4) against a held stub answer, the window resumes it from Today, the stub answers step 1, step 2 and the final text, `brief:changed` reports completion, and `agent status` / `report` through the installed binary show two committed steps and an untouched attempt report after the window's resume made exactly three provider requests on the one path (checked as `resumed.length === 3`; the CLI half's own pause made one) |
| 4 | Nothing else changes for the learner | The 11-section durable matrix runs against the source host *and* the staged bundle in the three `core` jobs (run 44854554521); `gofmt`/`vet`/`build`/`test` and every other gate stay green on three OSes; the golden-contract gate is green with no `internal/**/testdata/` change; `agent/*.mjs`, `internal/`, `cmd/`, `mygo.json`, `bun.lock` and `agent/package.json` have an empty diff across the goal; three of the four hosts ship byte-identical (sha256 above), and the bundled one answers the same protocol documents (matrix pass) |
| 5 | Third-party notices travel with the code | The contents check verifies the installed file against the set the staged bundles required (`11 notice text(s) verified…`, three OSes), self-test fixtures fail when a text is missing (`installed notices file that lost a licence text`, `…lost one of two identical licence texts`) and the four notice units pin the assembly; the listing above is read from the produced file |
| 6 | Host start-up is reported | The cold-start table and its 30 published samples: 15 per tree, min/median/max, both on the package's own runtime, driven by the named script and command with the CPU load stated (the goal asks for the durable host, and Q-0016-06 records that only it is measured) |

## Known limits

- **The licence-name rule misses some licence names.** `harness/lib/notices.ts` carries a file whose
  *name* holds a licence word, or that sits under a directory whose name does (at any depth), and
  excludes code extensions; a singular `license/` directory, a `COPYRIGHT` file or a name in another
  language would be missed or carried only if an ancestor directory matches. Over-inclusion is the
  accepted side; omission is the risk, and it is bounded by the inlined set being 7 packages today.
- **A package root that is a symlink fails staging**, and that path is unproven on POSIX: the licence
  source refuses a symlink by name (Q-0016-10), and Windows refused to create the fixture link in
  the self-test. If a future dependency resolves through a symlinked package root on macOS or Linux,
  staging would fail rather than assemble a partial notice set — a loud failure, but not one CI has
  exercised.
- **`emptyInlinedSetProblem`'s call site is untested**: the guard has a unit
  (`inlinedSetProblem`), but no test drives `bundleHost` with a metafile that names a package and
  inlines none, so the guard's wiring to staging is proven by reading, not by a run.
- **The installed binary as a CLI resolves no resources directory** (only the window does), so the
  `brief` case's CLI half pins `PASSQUEST_NODE` and `PASSQUEST_AGENT_HOST` at the installed copy's
  own files; the window half pins nothing, and that is what proves the app's own resolution of the
  bundled runtime and hosts through `internal/hostpaths`. That is a deviation from plan P3, which
  asked for a CLI half with no pins; it is taken because the CLI composition root resolves no
  resources directory — the rule the package smoke's own row states for the
  `upgrade` case's CLI half (`docs/harness.md` row 24) — so an unpinned CLI half would fail for a
  reason that is not the package's.
- **Local window gates are unhealthy under load.** This machine ran at 100% CPU during this step (and
  during P1–P4), where the desktop and window parts of the package smoke fail with
  `child process(es) outlived the app: …msedgewebview2.exe` or timeouts; CI on three OSes is the
  authority for those cases, and no local run of them is claimed here.
- **The SPDX-fallback notices are our wording**, not upstream text: five `@earendil-works` sections
  are the SPDX MIT text with the package's `author` line (chord, pi-ai, pi-durable,
  `pi-schema`'s own terms, pi-telemetry — the file's other six sections are copied texts), because
  those tarballs carry no licence file. If a package's real terms differ from what its `package.json`
  declares, the notice would state MIT wrongly — the fix is upstream shipping a licence file.
- **macOS and Linux are proven only by CI** for everything OS-specific: the `.dmg` and AppImage
  packaging, the staged tree's POSIX shapes, and the `.app`'s install tree scan. The bundle's
  metafile inside a POSIX checkout is also reasoned, not run: if bun spelled an inlined package's
  input as a symlinked path, `bundlePackages`/`noticeSource` would refuse it by name (a loud failure).
- **Cold start is reported, not gated**, and its absolute numbers move with this machine's load
  (P0's lighter-load probe, as the plan records it, versus the 100%-CPU table above); only the
  relative lead is stable.
- **The package cannot shrink much further without replacing the Node runtime**: on Windows, 87.1 MB
  of the 88.7 MB staged tree is `node/node.exe`, and the runtime is a larger share on macOS and Linux
  (113.0 MB of ≈114.6 MB, 124.9 MB of ≈126.5 MB; Q-0013-03, out of scope here). The 108 MB installed
  tree (Windows) is that payload plus the app; macOS and Linux read 133 / 146 MB for the same reason.
- **The before numbers rest on artifacts that expire**: the `.exe`/`.dmg`/AppImage bytes quoted from
  the v0.15.0 release are permanent, CI artifacts are kept 7 days, and run logs are the judge of
  record for the rest.
