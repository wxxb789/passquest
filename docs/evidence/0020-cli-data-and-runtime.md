# 0020 — The command line keeps the learner's data where the window does, and the package runs on Node 26: evidence

Contract: [`../goals/archive/0020-cli-data-and-runtime.md`](../goals/archive/0020-cli-data-and-runtime.md).
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
recovered by the installer's compression. The per-OS CI artifact pair for all three platforms is in
the P5 section below (the `package-<os>` artifacts of runs 45589197958 and 45610985768); the sizes
are recorded, not judged, and a bigger package is the disclosed outcome here.

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

## P5 (2026-11-25) — the completed record (Done-when 1–6; branch `goal/0020-p5`, base `3aa4403`)

This step changes documents only — both guides, this record, `docs/harness.md`, the Q-0020-04/-07
outcome lines and the dated note in the goal-0016 record — and runs the four document gates locally.
Every number below is read from the run or artifact named beside it. The runs it names are the pull
requests that carried the goal: P1 #59 (gate run `45571233470`, final attempt 2), P2 #60 (gate run
`45577642306`), P3 #63 (gate run `45589197958`), P4 #64 (gate run `45610985768`, the final tree),
plus this step's own local runs on this machine. Machine paths in the quoted lines are elided
(`…`), as in the notes above.

### Done-when 1 — one data directory

**Implementation (P1, merged `96a2a1f`).** `internal/userdata` owns `AppName`, `DataEnv`,
`DatabaseEnv`, `DatabaseFile`, `LegacyDatabasePath`, `Dir` and `DatabasePath`;
`internal/study.DatabasePath`/`DefaultDatabasePath` delegate to it; the CLI resolves through one
`Options` helper (`ports` and `openDatabase` share it) and renders an unresolvable config directory
as `config_dir_unavailable` (exit 2, one message naming `--db`/`PASSQUEST_DB`); the window's default
seam calls `userdata.Dir(os.UserConfigDir, getenv)` and `resolveStorage`'s install refusal and 0700
tightening are unchanged. Tests: `TestDir`/`TestDatabasePath` (userdata), `TestDatabasePath`
(study), `TestDatabasePath`/`TestDatabasePathUnresolvableIsRefusal` (cli),
`TestNextCommandOmitsDefaultDatabase`/`TestNextCommandRepeatsOverrideSelectedDatabase`,
`TestDefaultDatabaseDirectoryIsTightened`, `TestUserdataParity`/`TestDefaultDataSeam` (desktop) and
`TestWorkspaceTodayDatabaseArgumentFollowsTheSelectedStore`. The suite is green on CI in run
`45571233470` (`go test (38 packages, 1004 tests, 0 skipped)` on macOS and Ubuntu, `1003` on
Windows) and in run `45577642306` (`1008`/`1009`).

**The both-directions case, per OS** (P3's `default-data` desktop-smoke scenario; run
`45610985768`, first CI appearance in `45589197958`). With no `PASSQUEST_DESKTOP_DATA` and no
`PASSQUEST_DB`, and the scenario's own temp HOME/`APPDATA`/`XDG_CONFIG_HOME`, the window records the
walkthrough attempt and stays alive while a CLI process (no `--db`) reads that attempt back through
`report --json` and records a second one; the pass line records the path and the attempt ids, and
the scenario requires the file to be the only `passquest.sqlite` under its tree:

| OS | `default-data` reading (run `45610985768`) |
|---|---|
| Windows | `…\default-data\home\AppData\Pass Quest\passquest.sqlite written by the window and the CLI, read back by the window's Today (attempts 1+2)` |
| macOS | `…/default-data/home/Library/Application Support/Pass Quest/passquest.sqlite written by the window and the CLI, read back by the window's Today (attempts 1+2)` |
| Linux | `…/default-data/home/.config/Pass Quest/passquest.sqlite written by the window and the CLI, read back by the window's Today (attempts 1+2)` |

The elided prefixes are the run's own temp roots (`…\AppData\Local\Temp\…`,
`/var/folders/…` and `/tmp/passquest-desktop-smoke-…` in that run). The checks behind the detail
line: the CLI read the window's 1 attempt beside the live process, its `tactic practice` exited 0
and left 2, the ids were `1,2`, exactly one database existed under the tree, and a second launch's
Today listed the same ids with 2 assisted attempts for the skill.

### Done-when 2 — the old default is never moved silently

**The notice (P1).** On a run that resolves the default (no `--db`, no `PASSQUEST_DB`), with a file
at `<cwd>/.passquest/passquest.sqlite` and a different resolved database, the CLI writes exactly one
stderr line before the command's own work:
`passquest: a database from the old default location exists at <old>; the command line now uses
<new>. Pass Quest will not move it; move or merge it yourself` — plus, only when the per-user
database does not exist yet and the shell family is detectable, a move command quoted for the
detected family (`mv`/`move`/`Move-Item`) with the family, its source and the override hint beside
it. With both files the line ends `(the per-user database already exists, so no move command is
printed; merge instead of overwriting it).`; an unknown `PASSQUEST_SHELL` drops the command and the
run continues; a `cmd` family whose paths hold `%` drops the command for copy-safety (cmd.exe
expands `%NAME%` inside quotes); a resolved database equal to the old file is suppressed. It only
reads the filesystem, so `--json` stdout is byte-identical.

**Tests:** `TestOldDefaultNoticeBeforeWork`, `TestOldDefaultNoticeOncePerRunAndNothingMoved` (the
old file's bytes and mode and its directory are compared before and after),
`TestOldDefaultNoticeCommandPerShell`, `TestOldDefaultNoticeWithBothDatabasesPrintsNoMoveCommand`,
`TestOldDefaultNoticeDropsCmdCommandOnPercentPath`, `TestOldDefaultNoticeAbsent` — all green in run
`45571233470`'s `go test` gate (the pass line names packages and counts, not test names). The
learner-facing description of the warning is this step's (both guides' data sections, Done-when 6
below).

### Done-when 3 — the window and the command line are safe together

**The guarantee (P2, merged `61427d6`), unchanged facts:** one connection per handle
(`SetMaxOpenConns(1)`), `busy_timeout(5000)`, `_txlock=immediate`, no WAL; the window opens the
store per call and the CLI is one process per command; `store.IsBusy` matches the driver's typed
result code (`errors.As` to `*sqlite.Error`, primary code 5/6) and `internalError` maps a busy error
to exit 2, code `database_busy`, message `database is busy: the Pass Quest window (or another
command) has it open; close the window or retry`.

**Measured; this step's local run (Windows, `go test … -v -count=1`).**
`TestInterleavedWritersBothPersist` logged `ids read: first handle [1 2 3], second handle [1 2 3]
(writes interleaved as [1 2 3])`; `TestWriteWaitsForAHolderUnderTheTimeout` logged `write waited
360.5216ms for a lock held 300ms`; `TestWritePastTimeoutRefusesAndLeavesNoRow` logged `busy refusal
after 311.3319ms with busy_timeout(300)`; `TestBusyDatabaseRefusal` logged `busy refusal after
5.0407919s (the write wait Open configures is 5000 ms)` and its command's error document carried
the code and message above. CI's own reading: run `45610985768`'s `go test` line names
`TestBusyDatabaseRefusal 5.1s` (macOS) and `5.0s` (Ubuntu) among the five slowest tests.

**Two live processes, per OS:** the same `default-data` scenario asserts `held.killed` — the CLI's
read and its write happen inside the first window launch's `killWhen` while that window is alive —
and the window's second launch reads both attempts back (Done-when 1's table).

### Done-when 4 — the installed binary runs the AI commands from its own package

**Implementation (P3, merged `94dd55c`).** `internal/desktop` exports `ResourceDir()` (the old
`resourceDir()` body: `""` in a development build, else MyGo's `PathResources`);
`cmd/passquest-desktop/main.go` passes it as `cli.Options.Resources` on the argument path only, and
`cli.Options.withDefaults` threads it into the four default host transports; `internal/hostpaths`
is unchanged. Go tests: `cmd/passquest-desktop/main_test.go` (a development build passes `""` and
the argument path still dispatches) and `internal/cli/resources_test.go` (the four default
transports carry `Options.Resources`, and the absent search host is reported with the tree's own
candidate `<tree>/agent/search-host.mjs`).

**The `cli-runtime` package-smoke case (P3), per OS.** Its child runs the installed binary with
arguments on the package smoke's PATH, which holds no `go`, `bun` or `node` (asserted), on the
`study` case's temp per-user root, with no `PASSQUEST_NODE` and no `PASSQUEST_*_HOST` (the case
fails if the environment pins any), one loopback provider stub in the registry and its stub
credential. The notice line must name the database the app's own root holds, `model test
stub/stub-model --json` must read `verified` after exactly one stub request, and the tree must hold
one `passquest.sqlite` — the per-user one. The pass line (run `45610985768`; the same shape in
`45589197958`):

| OS | `cli-runtime` reading |
|---|---|
| Windows | `cli-runtime: …\home-study\AppData\Pass Quest\passquest.sqlite opened by the installed binary with no node on PATH; stub/stub-model verified` |
| macOS | `cli-runtime: …/home-study/Library/Application Support/Pass Quest/passquest.sqlite opened by the installed binary with no node on PATH; stub/stub-model verified` |
| Linux | `cli-runtime: …/home-study/.config/Pass Quest/passquest.sqlite opened by the installed binary with no node on PATH; stub/stub-model verified` |

A host-backed success with no `node` on `PATH` and no override in the environment is the packaged
runtime's and hosts'; on macOS the same run proves `Contents/Resources` is reachable. The CLI's own
notice names the resolved path, so the case also ties the CLI's resolution to the window's.

**The pins removed (P3):** the `upgrade` case's revision run and the `brief` case's CLI half now
pass no `PASSQUEST_NODE`/`PASSQUEST_*_HOST`; the goal-0016 record's deviation block carries the
dated note. A development build still passes `""`, so a source checkout's `passquest` and
`passquest-desktop` keep the checkout's own lookups (pinned by the `main_test.go` case).

### Done-when 5 — Node 26 LTS everywhere

**The pins (P4, above):** `NODE_VERSION`/`NODE_PINS` = **26.13.1** with nodejs.org's four SHA-256s,
`mise.toml` and `gate.yml` at the same patch; `nodePinProblems` covers the five sources and staging
re-checks each downloaded archive; `mise exec -- node --version` = `v26.13.1`; the package gate's
pass line ends `node pin: 5 sources agree on Node 26.13.1` on all three OSes of run `45610985768`.

**The seven smokes on the three OSes (run `45610985768`):**

| Gate | Windows | macOS | Ubuntu |
|---|---|---|---|
| agent smoke | 238 assertions | 242 assertions | 242 assertions |
| tutor smoke | 359 assertions | 359 assertions | 359 assertions |
| note smoke | 1367 assertions | 1367 assertions | 1367 assertions |
| find smoke | 113 assertions | 113 assertions | 113 assertions |
| assess smoke | 125 assertions | 125 assertions | 125 assertions |
| provider smoke | 115 assertions | 115 assertions | 115 assertions |
| package smoke | 577 assertions over the ten cases; `pass-quest-setup-0.15.1-amd64.exe` 32.8 MB | 576; `pass-quest-0.15.1-arm64.dmg` 37.2 MB | 576; `Pass_Quest-0.15.1-x86_64.AppImage` 53.6 MB |

The agent lines name the bundle the second half of each scenario ran against
(`bundle 1557686 bytes` on Windows, `1557708` on macOS, `1557656` on Ubuntu, 7 inlined packages
each; `permissions skipped on Windows`); the package lines carry `contents, layout, study,
cli-runtime, agents, brief, anki, upgrade, credential, broken`, the contents-scan counts, the
upgrade comparison (`32 tables, 160 rows, 1546 fields compared, 0 difference(s); schema 17 -> 17,
unchanged (a reopen, no migration step)`), `note 101 revised in place, queued note flushed through
the window`, `schema 18 refused, 1 file(s) unchanged` and the pin line.

**The installed artifact's size, before and after, per OS** — read with `stat` from the
`package-windows-latest`, `package-macos-latest` and `package-ubuntu-latest` artifacts of each run
(`gh run download`), the same files the pass lines print as `28.8`/`48.6`/`32.0 MB` (before) and
`32.8`/`53.6`/`37.2 MB` (after):

| Platform | Before: run `45589197958` (#63, Node 22.23.5) | After: run `45610985768` (#64, Node 26.13.1) | Change |
|---|---|---|---|
| Windows x64, `pass-quest-setup-0.15.1-amd64.exe` | 28,842,481 B | 32,764,070 B | +3,921,589 B (+13.6 %) |
| macOS arm64, `pass-quest-0.15.1-arm64.dmg` | 31,958,528 B | 37,230,592 B | +5,272,064 B (+16.5 %) |
| Linux x86_64, `Pass_Quest-0.15.1-x86_64.AppImage` | 48,642,552 B | 53,639,672 B | +4,997,120 B (+10.3 %) |

P4's local Windows pair (above) reads 28,842,773 → 32,756,317 bytes on the same two trees built in
this worktree; the CI artifact pair above is the record's own. The sizes are recorded, not judged.

### Done-when 6 — the learner is told the truth

**The guides (this step).** Both guides' "Where your data lives, and how to remove it" sections now
carry the same facts, inside the existing section (no new `##` heading): the command line resolves
the same per-user directory, `--db`/`PASSQUEST_DB` still override the file,
`PASSQUEST_DESKTOP_DATA` moves both entry points together, the installed app's own executable is
the command line when started with arguments (Windows
`"%LOCALAPPDATA%\Programs\Pass Quest\Pass Quest.exe"`, macOS
`"/Applications/Pass Quest.app/Contents/MacOS/Pass Quest"`, Linux the AppImage file itself), it
runs the package's own Node runtime and `agent/` hosts, `PASSQUEST_NODE`/`PASSQUEST_*_HOST` still
win, and what happens with an old `./.passquest/passquest.sqlite` (one warning naming both paths,
nothing moved/copied/merged; the move command's conditions, including the `cmd.exe`-with-`%` case
that drops the command), and the Windows shell note that PowerShell's `>` redirection of the
windowed executable leaves empty files and no exit code, that a PowerShell pipe waits but does not
carry the program's own bytes (it decodes stdout with `[Console]::OutputEncoding` — IBM437 under this
machine's `powershell` 5.1.26100.9711 and `pwsh` 7.7.0 — and rewrites the line endings), and that
the guides' `cmd /c` form, with the redirect inside its arguments, writes the program's own bytes
and leaves the exit code in `$LASTEXITCODE`. Measured on this machine with a `-H=windowsgui` probe
that prints one UTF-8 line, sleeps 3 s and exits 7: the PowerShell `>` form returned early (0.4 s
under `powershell`, 1.2 s under `pwsh`) with an empty file and no `$LASTEXITCODE`; the pipe returned
after the sleep (3.3 s and 3.0 s) with `$LASTEXITCODE` 7 and re-encoded bytes (29 B and 42 B against
the probe's own 28 B); the `cmd /c` form and `Start-Process -Wait -RedirectStandardOutput` each
carried the line byte-identically (28 B) with an exit code of 7. The non-ASCII check, with the staged
installed binary (`build/windows-amd64/Pass Quest.exe`, reached through a junction standing in for
`%LOCALAPPDATA%\Programs\Pass Quest`) and an imported source titled `中文标题 practice`: the pipe
wrote 487 B under `powershell` and 500 B under `pwsh` against the 470 B the binary itself wrote, the
title field's 21 bytes `e4b8ade69687e6a087e9a298207072616374696365` (`中文标题 practice`) coming back
as `532ba1b5fbe7b5e1e754f3ff207072616374696365` under `powershell` and
`cea3e29595c2a1c2b5c3bbc3a7c2b5c3a1c3a7ce98c3b3c3bf207072616374696365` under `pwsh`, while the
`cmd /c` form wrote `sources --json` and `report --json` byte-identically to a shell redirect
(470 B and 3031 B) in both shells. Both package-smoke case lists in both guides gained `cli-runtime`
in the gate's own order.

**The CLI's usage line (P1):** `Default database: $PASSQUEST_DB or passquest.sqlite in the per-user
data directory (docs/install.md)`. **`docs/agent.md`:** P3's resolution sentence (installed binary,
`PASSQUEST_*` precedence, the checkout's own lookups) beside the packaging bullet and P4's runtime
paragraph.

**Gates.** Run `45610985768`'s three `core` jobs read `install guide (7 sections, …)`,
`docs links (371 checked, 0 broken)`, `secrets/sources (814 files, 0 findings)` and `typecheck (0
type errors …)`; this step's local run over the edited tree read `STATUS pass 7 sections, 4 code
blocks and 7 screenshot places agree; 6 links and 5 URLs agree; …` (install-guide), `STATUS pass
371 checked, 0 broken` (docs-links), `STATUS pass 814 files, 0 findings` (secrets) and `STATUS pass
0 type errors in tsconfig.json, desktop/web/tsconfig.json` (typecheck). The owner's review of the
guides is the coordinator's close.

### Known limits carried to the close

- macOS and Linux window halves are CI's (runs `45589197958` and `45610985768`); this machine's
  window runs are advisory (it was at 97–100 % CPU).
- `default-data` on Linux runs under Xvfb with no window manager, so its titlebar maximize reading
  is recorded, not asserted (Q-0022-04); FUSE mounts and Wayland are not covered.
- A CI-built `.dmg` carries no quarantine attribute, so Gatekeeper is not exercised.
- The darwin and linux runtime archives were verified here by SHA-256 and member path only (P4's
  note); the win-x64 staged runtime executed here and on CI's `package-windows-latest` job, the
  darwin-arm64 and linux-x64 ones only on CI's `package-macos-latest` and `package-ubuntu-latest`
  jobs (each job stages and runs only its host target), and nothing in the repository's runs builds
  or runs the darwin-x64 pin (no workflow or script names that target; macOS is Apple-silicon only).
- `cli-runtime` proves one host-backed command per OS; the other host-backed commands in an
  installed copy are covered by the `agents`, `brief`, `anki` and `upgrade` cases.
- The `package-<os>` artifacts expire after 7 days; the run ids above are the durable reference.
- P1's first Windows `core` attempt in run `45571233470` failed the assess smoke on its delayed-stdin
  timeout (`forced timeout was not counted / timed-out item is not listed`, 51.4 s, artifact
  `gate-output-windows-latest-core`) and that job's retry passed (8.5 s, 125 assertions); the same
  gate passed on macOS and Ubuntu in that run and on all three OSes of runs `45577642306`,
  `45589197958` and `45610985768`.
