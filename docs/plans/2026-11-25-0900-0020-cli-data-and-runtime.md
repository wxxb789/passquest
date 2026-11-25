# 0020 — The command line keeps the learner's data where the window does, and the package runs on Node 26: increment plan

Contract: [`../goals/archive/0020-cli-data-and-runtime.md`](../goals/archive/0020-cli-data-and-runtime.md).
Conventions: [`../conventions.md`](../conventions.md). Harness: [`../harness.md`](../harness.md).
Cross-platform rules: [`../cross-platform.md`](../cross-platform.md).
Install guides: [`../install.md`](../install.md), [`../install.zh-CN.md`](../install.zh-CN.md).
Base: `main` at `b338c67` (0022 P1 merged, 2026-11-25). Scouted 2026-11-25; every number below is
read from the source named beside it or from a run named in the evidence plan. Local runs on this
machine: `mise exec -- gofmt -l internal cmd` printed nothing (`exit 0`), `mise exec -- go vet ./...`
printed nothing (`exit 0`), `mise exec -- node --version` = `v22.23.4`.

## Goal in one sentence

Make the command line and the installed binary use the window's own per-user database and the
package's own Node runtime and hosts, and move the bundled runtime, `mise.toml` and CI to the latest
Node 26 LTS — with the old default told about, never moved, and every existing gate green.

## Facts this plan rests on (cited or measured, not remembered)

1. **Where each entry point resolves the database today.**
   - **The window.** `openStudy` (`internal/desktop/desktop.go:223`) calls `resolveStorage`
     (`internal/desktop/paths.go:72`) with `userData = mygo.App.Path(mygo.PathUserData)`
     (`desktop.go:225-228`), and `resolveStorage` returns `DataDir` plus `Database` = `EnvDB`
     (`PASSQUEST_DB`, `paths.go:27`) or `DataDir/passquest.sqlite` (`DatabaseFile`, `paths.go:38`).
     MyGo's own `PathUserData` is `os.UserConfigDir()` joined with the app name and created 0755
     (`mygo@v0.10.0/paths.go:90-92`, `:53-66`); the app name is set by `mygo.App.SetName` before the
     window exists (`internal/desktop/desktop.go:87`), and it is `AppName` = `"Pass Quest"`
     (`internal/desktop/paths.go:18`) unless `PASSQUEST_DESKTOP_DATA` hashes a separate instance into
     it (`paths.go:161-170`).
   - **The command line.** `study.DatabasePath` (`internal/study/study.go:59-67`) resolves
     `--db` → `PASSQUEST_DB` → `DefaultDatabasePath`, a **relative** `filepath.Join(".passquest",
     "passquest.sqlite")` (`study.go:52`). Every CLI command reaches it: either through
     `Options.ports` (`internal/cli/cli.go:110-128`) into `study.Open`, or through
     `openDatabase` (`cli.go:281-287`, `OpenPrivate` for the default). The usage text names the old
     default (`cli.go:263`, `Default database: $PASSQUEST_DB or ./.passquest/passquest.sqlite`).
   - **The installed binary's CLI mode.** `cmd/passquest-desktop/main.go:17-22` runs
     `cli.Run(args, cli.Options{})` whenever the app is started with arguments — the path the durable
     host's `agent record-step` callback uses (`docs/cross-platform.md`, *Packaging*: the
     GUI-subsystem binary "is the CLI too when given arguments"). It resolves the same relative
     default, so from an installed copy started anywhere it writes a database into the working
     directory.
   - **Who compares against the default.** `internal/cli/next.go:53` and
     `internal/workspace/workspace_today.go:115` pass `study.DefaultDatabasePath` to
     `cmdline.NextDatabaseArgument` (`internal/cmdline/commands.go:24-29`), which omits `--db` from a
     generated command only when the selected path equals the default; that is why a window's own
     command is printed with an absolute `--db` today (the window's `w.dbPath` never equals the
     relative legacy default).
   - **Tests that open the default store.** `mustRunNoDB` (`internal/cli/tactic_test.go:15-37`) runs
     commands with `Getenv` returning `""` and no `--db`; `TestNextCommandOmitsDefaultDatabase`
     (`internal/cli/next_test.go:360-375`) and `TestDefaultDatabaseDirectoryIsTightened`
     (`internal/cli/source_test.go:626-655`) depend on the default landing in the process working
     directory, and the latter pins its 0700 mode. `TestDatabasePath` (`internal/cli/cli_test.go:120-138`)
     pins the relative default string. These are the tests P1 rewrites, not deletes.
2. **The per-user directory's three spellings, and the window's promise about it.**
   `%APPDATA%\Pass Quest` (Windows), `~/Library/Application Support/Pass Quest` (macOS),
   `$XDG_CONFIG_HOME/Pass Quest`, default `~/.config/Pass Quest` (Linux) — `docs/install.md:169-180`,
   its table `:175-177`, mirrored by the Chinese guide, and recorded in Q-0013-04
   (`docs/decisions/OPEN-QUESTIONS.md:1546-1560`). The harness computes the same directory for tests
   as `dataRootFor` (`harness/gates/package-smoke.ts:154-158`) with the root variables set by
   `homeEnv` (`:168-172`). `resolveStorage` refuses a default that
   lands inside the app's own files (`paths.go:87-89`, `insideInstall` `:110-119`) and tightens the
   app-owned directory to 0700 (`paths.go:89-98`); `PASSQUEST_DESKTOP_DATA` remains the explicit
   override.
3. **How the AI runtime is found today.**
   - `internal/hostpaths` resolves from one input set: `PASSQUEST_<HOST>` (`Host.Env`) wins, else the
     caller's `Resources` directory (`<resources>/agent/<script>`, `hostpaths.go:54-96`), else the
     working directory and the executable's directory and its parent (`:70-96`, `appendFallback`
     `:109-119`); the Node runtime is `PASSQUEST_NODE` (authoritative), else
     `<resources>/node/bin/node` (or `node/node.exe`), else `"node"` from `PATH` (`:121-143`).
   - The window passes `Resources: resourceDir()` (`internal/desktop/desktop.go:237`), where
     `resourceDir` (`:251-258`) returns `""` for a development build (`mygo.IsDev()`) and else
     `mygo.App.Path(mygo.PathResources)` — `Contents/Resources` inside a macOS bundle, the
     executable's directory elsewhere (`mygo@v0.10.0/paths.go:104-113`, `resourcesDir` `:126`).
   - The CLI's composition root passes nothing: `cli.Options.withDefaults` builds
     `tutor.HostTransport{Getenv, Environ}` and the three siblings (`internal/cli/cli.go:80-91`) with
     an empty `Resources`. So an installed binary in CLI mode can only find `node` on `PATH` and the
     hosts through the cwd/executable fallbacks — on macOS the hosts live at
     `Contents/Resources/agent` (`docs/cross-platform.md`, *Packaging*), which none of the fallbacks
     reaches. The harness already works around this and says so:
     `harness/gates/desktop-smoke/scenario.ts:57` ("the CLI composition root resolves no resources
     directory (only the window does)"), and the upgrade case pins `PASSQUEST_NODE` and
     `PASSQUEST_ANKI_HOST` for the installed binary's revision run
     (`harness/gates/package-smoke/upgrade.ts:767-790`).
4. **What keeps two writers over one database safe today** (Done-when 3, no product change planned
   unless a test says otherwise).
   - Every connection gets `busy_timeout(5000)` and `_txlock=immediate`
     (`internal/store/store.go:199-212`, `:209`), one connection per handle
     (`SetMaxOpenConns(1)`, `store.go:174-176`), and no WAL: `journal_mode` is never set
     (`rg journal_mode internal` finds nothing).
   - The window does **not** hold the store between calls: `workspace.Open` opens per call
     (`internal/workspace/workspace.go:66-76`, `:252`) "so a second process (the durable host
     recording a step) can write between calls. Short local writes are serialised". The CLI is one
     process per command.
   - The driver's error type carries SQLite's own result code: `*sqlite.Error` with
     `Code() int` (`modernc.org/sqlite@v1.62.0/error.go:12-21`), constructed from the raw result code
     (`conn.go:1033-1048`), so `busy`/`locked` are detectable as `code&0xff == 5 || code&0xff == 6`.
   - Where a CLI error is rendered: `internalError` (`internal/cli/errors.go:28-38`) and `fail`
     (`:44-50`), which every command's store error passes through.
5. **The Node pin today, and the target.**
   - `harness/lib/package.ts:66` `NODE_VERSION = "22.23.5"` with four archives and SHA-256s
     (`:135-168`), each "nodejs.org's `SHASUMS256.txt` value … checked again on download"
     (Q-0013-03, `docs/decisions/OPEN-QUESTIONS.md:1523-1534`); staging fetches
     `nodejs.org/dist/v<NODE_VERSION>/<archive>` (`package.ts:989-992`) and, for the host target,
     runs `node --version` and refuses a mismatch (`:1090-1091`).
   - `mise.toml:3` pins `node = "22"`; `.github/workflows/gate.yml:52-54` pins
     `actions/setup-node@v4` to `node-version: "22"`; no other workflow sets a Node version
     (`rg node .github/workflows` finds only that). The hosts' own engine requirement is
     `"node": ">=22.19.0"` in each `agent/node_modules/@earendil-works/*/package.json`
     (read 2026-11-25), and `docs/agent.md:94` states "Agent commands need Node ≥ 22.19".
   - **Measured 2026-11-25** (`https://nodejs.org/dist/index.json`): the newest 26 LTS is
     **v26.13.1** (2026-11-10, `"lts": "Lithium"`, `"security": true`); 26.13.0 was the first
     `Lithium` tag (2026-10-28), and v27 is still alpha. `https://nodejs.org/dist/v26.13.1/SHASUMS256.txt`
     (fetched 2026-11-25) carries
     `e2edda75bfb089654705cf65a13b58933bfd5f57148e8ebda916f58a2474f1f7  node-v26.13.1-win-x64.zip`,
     `aec2514ea1ceb2ba52455e9ec65986f6749a06dd759e5db2a72e6889668c9735  node-v26.13.1-darwin-arm64.tar.gz`,
     `82bb96078a24ca50b9c9a38f2379e6be6d87171fcd400fc14c8934b3bce96e2f  node-v26.13.1-darwin-x64.tar.gz`,
     `5c063e46bab0fbc1c17cc445d7ff129cbdcd0471be6ed83b6ab864f8143bc864  node-v26.13.1-linux-x64.tar.xz`.
     All four targets the pins cover exist for 26.13.1 (the same index lists `win-x64-zip`,
     `osx-arm64-tar`, `osx-x64-tar`, `linux-x64` for it).
6. **What the gates can see, and what they cost.**
   - `desktop-smoke` drives the real window through the smoke hook; each scenario gets its own
     `PASSQUEST_DESKTOP_DATA` directory unless the scenario sets `dataOverride: false`
     (`harness/gates/desktop-smoke/scenario.ts:36-47`, `:116`), and `launchEnv` is merged under the
     hook's own variables (`:110-125`). Launches are hidden by default
     (`harness/lib/desktop-page.ts:277-300`), inherit the process environment with `PASSQUEST_*`
     stripped (`:133-139`), and the gate builds the dev binary with `go build`
     (`harness/lib/desktop.ts:39-56`, so `mygo.IsDev()` is true and `resourceDir()` returns `""`).
     `SCENARIOS` holds fourteen entries today (`harness/gates/desktop-smoke.ts:38`).
   - `package-smoke` installs this run's package and launches the app with a PATH stripped of
     `go`, `bun` and `node` (`harness/gates/package-smoke.ts:175`, `:183-199`), a cwd outside the
     checkout, and `HOME`/`APPDATA`/`XDG_CONFIG_HOME` pointing at a temp root (`homeEnv`, `:168-172`)
     so the app's *default* data path runs; the case list is nine entries
     (`:117`), and the installed binary is already driven as its own CLI in the `upgrade` case.
   - A new per-OS check must therefore be able to observe: the file the other entry point wrote
     (both gates can walk a temp tree), a host-backed command's success with no Node on `PATH`
     (package smoke's stripped PATH makes any success the packaged runtime's), and the absence of
     `PASSQUEST_*` overrides in the environment it passes.
   - Window gates are timing-sensitive on this machine (the brief's 100 % CPU fact); CI's three-OS
     matrix is the authority for them, and this plan adds no window check whose only evidence is a
     local run.
7. **The documents and gates that pin this area, and where a change may land.**
   - `install-guide` (`harness/gates/install-guide.ts`) reads the two guides as one document: the
     English guide's `##` sections are exactly the seven in `GUIDE_SECTIONS` (`:65-73`), the Chinese
     guide mirrors them, and every pinned fact must sit in its own section (`:174-179` pins the macOS
     data directory there). Its `APP_SOURCES` (`:54-59`) includes `internal/desktop/paths.go` and
     reads the app name with `/AppName\s*=\s*"([^"]+)"/` from `APP_NAME_DECLARATION` (`:62`): the
     literal `AppName = "Pass Quest"` must keep its home in that file. New prose belongs inside the
     existing "Where your data lives, and how to remove it" section on both sides — a new `##`
     heading fails the gate. The gate pins no Node fact and no package-smoke case list: the Node
     sentence sits outside `GUIDE_SECTIONS` (the FACTS check reads only section bodies,
     `install-guide.ts:456-458`), so P4's pin-agreement check reads it instead, and the guides' case
     list is the reviewer's to hold (P5).
   - `docs/harness.md` rows 8, 24, 25, 26 and 27 carry the pinned facts and the per-gate counts; the
     numbers a step writes into them are read from that step's own gate pass lines (the P9 re-quote
     rule the rows themselves state).
   - `golden-contract` (row 23) refuses a `*golden*` change unless the same diff also touches a
     record under `docs/evidence/`, a goal under `docs/goals/` or a decisions record: the steps below
     change no golden (they change the *default* resolution, and every golden test passes `--db`),
     and the Q-0020 entries are on `main` before P1 is cut, so nothing here depends on the rule — a
     step that ever did need a golden change would have to carry its own record in its own diff.
   - The package gate's own build is the only place the pins are exercised end to end: staging
     downloads, checks each SHA-256, runs `node --version` on this host's target and drives the
     staged durable host on that runtime (`harness/lib/package.ts:1090-1091`, `docs/harness.md`
     row 26, "stage: check: …" lines).

## Defaults (append as Q-0020-01…08; the last entry is Q-0022-09)

1. **Q-0020-01 — One resolution, shared by both entry points.** A new package `internal/userdata`
   owns the per-user data directory and the database path: `AppName = "Pass Quest"`,
   `DataEnv = "PASSQUEST_DESKTOP_DATA"`, `DatabaseEnv = "PASSQUEST_DB"`,
   `DatabaseFile = "passquest.sqlite"`, `LegacyDatabasePath = .passquest/passquest.sqlite`,
   `Dir(configDir func() (string, error), getenv func(string) string) (string, error)` (the
   `DataEnv` override — whitespace-trimmed and taken only when non-empty, the rule the window
   applies at `paths.go:73` — else `configDir()` joined with `AppName`), and
   `DatabasePath(flag string, configDir, getenv) (string, error)` (flag as given when it is not
   whitespace-only → the trimmed `DatabaseEnv`, exactly `study.DatabasePath`'s rule today
   (`study.go:59-67`) → `DatabaseFile` inside `Dir`). Both reads trim whitespace and the flag is
   returned untrimmed, so a whitespace-only `PASSQUEST_DESKTOP_DATA` still resolves the app-owned
   per-user default — never a directory named from a space, as the window ignores the blank value
   today. `internal/study.DatabasePath` and the window's default seam both
   call it; `internal/desktop` keeps its own literals for the four names the package shares
   (`AppName`, `EnvData`, `EnvDB`, `DatabaseFile` — the install-guide gate's `APP_NAME_DECLARATION`
   reads the first from `paths.go`) with a parity test pinning all four against `internal/userdata`'s
   values, so a drift in any of them fails before a run. `configDir` is a seam, not a second
   implementation: both roots pass `os.UserConfigDir`, so the directory stays `os.UserConfigDir()`'s
   own answer and the window's location cannot move. The CLI honors `PASSQUEST_DESKTOP_DATA` for the
   same reason the window does — it *is* the window's data-directory override, and the smoke gates
   isolate runs with it — with `--db` and `PASSQUEST_DB` still deciding first, exactly as today. A
   `PASSQUEST_DESKTOP_DATA` directory is the learner's explicit choice, taken as given exactly as the
   window takes it (`paths.go:70-98`: only the app-owned default is tightened to 0700), so the CLI
   leaves it alone too; only the app-owned per-user default keeps `store.OpenPrivate`'s 0700
   tightening. When no config directory can be resolved and no override is set, the command fails
   with one clear message naming `PASSQUEST_DB`/`--db` rather than falling back anywhere. Reverse:
   point the two roots at another package.
2. **Q-0020-02 — The notice about the old default.** On a run that resolves the default location
   (no `--db`, no `PASSQUEST_DB`), when a database file exists at
   `filepath.Join(workdir, ".passquest", "passquest.sqlite")`, the CLI writes **one** line to stderr,
   before the command's own work, naming the old path, the new path and that nothing is moved
   (`Pass Quest will not move it; move or merge it yourself`), whose move command names the resolved
   **absolute** paths (the working directory's old file and the per-user database — the same pair the
   prose prints, never a `%APPDATA%`/`$env:` spelling) with each argument rendered through
   `cmdline`'s shared `CommandQuoter.Quote` (`internal/cmdline/shell.go:104`) for the family
   `cmdline.DetectShell` reports — the family, its source and the override hint printed beside it,
   exactly as `next` does, through one helper both callers share (`docs/cross-platform.md`, *Shell
   output*). When detection fails (an unknown
   `PASSQUEST_SHELL`) the notice prints the paths and the prose without a command and the run
   continues: an unrelated command must not fail because of the notice.
   Nothing is copied, moved, merged or deleted — the notice is a read of the filesystem plus a line
   on stderr, so `--json` stdout stays byte-identical. It shows whether or not the per-user database
   exists: with both files the learner must still be told which one the command reads, and the
   wording names both paths either way. Reversal: condition it on the per-user database being absent
   (the contract's literal case) and drop the other branch.
3. **Q-0020-03 — Failing clearly when the wait runs out.** No locking change: the window opens per
   call, the CLI is one process per command, and SQLite's write lock plus `busy_timeout(5000)` is the
   guarantee (fact 4). `internal/store` gains `IsBusy(err) bool` (the driver's `Error.Code()` with
   `code&0xff` 5 or 6), and `internal/cli`'s `internalError` maps a busy error to a usage refusal
   (exit 2, code `database_busy`) whose message says the window has the database open and gives one
   action (close the window, or retry). One mapping point, so a busy error from the open *or* from a
   later write reads the same. No WAL, no lock file, no retry loop, no process-level lock: each would
   be a piece of state this goal does not need. Reverse: drop `IsBusy` and let the driver's text
   through again.
4. **Q-0020-04 — How the package is recognised when a binary runs as the CLI.** The CLI path takes
   its resources directory from the **same** computation the window uses — `mygo.App.Path
   (mygo.PathResources)`, already in `resourceDir()` — exposed to `cmd/passquest-desktop` as
   `desktop.ResourceDir()` and carried into `cli.Options` as `Resources`, which `withDefaults` puts
   into every default host transport. A development build (`mygo.IsDev()`) passes `""`, so a source
   checkout's CLI keeps today's lookups; an installed binary passes the packaged directory on all
   three platforms, so the resolution's own rules (explicit `PASSQUEST_*` first, packaged directory
   second) decide, and macOS's `Contents/Resources` is reachable. `internal/hostpaths` is not
   changed: no new heuristic, no install sniffing. The standalone `cmd/passquest` binary keeps its
   source-checkout behaviour (it never had resources). Reverse: pass `""` again and restore the
   harness's `PASSQUEST_NODE`/`PASSQUEST_*_HOST` pins in the package smoke.
5. **Q-0020-05 — The Node 26 pin. 26.13.1**, the newest 26 LTS at scout time, with the four
   SHA-256s above, in `harness/lib/package.ts` `NODE_VERSION`/`NODE_PINS`; `mise.toml` and
   `.github/workflows/gate.yml` pin the *same exact version*, so the development toolchain, the
   source-host runs in CI and the shipped runtime agree and a patch difference cannot hide a defect.
   The step that lands the pin re-reads `https://nodejs.org/dist/index.json` and that version's
   `SHASUMS256.txt` at implementation time: if 26 has a newer LTS release by then, it takes that
   release and records the version and the reason in the evidence record instead of copying the four
   values above. Reversal: any later step may bump every version-carrying place together — the pins,
   `mise.toml`, `gate.yml` and the three pinned sentences the agreement check reads; the archive and
   member names, the download URL's `v<NODE_VERSION>` prefix and the staged `node --version`
   comparison all carry the version too, so a half-done bump fails the download, the agreement check
   or a `node --version` comparison rather than shipping.
6. **Q-0020-06 — What the learner reads.** The CLI's usage line names the rule, not a literal path:
   `Default database: $PASSQUEST_DB or passquest.sqlite in the per-user data directory
   (docs/install.md)`, because the literal path differs per platform and the guides already name it.
   `docs/install.md` and `docs/install.zh-CN.md` gain, inside their existing data section, what the
   command line and the installed binary do: where the command line keeps its database by default,
   that `PASSQUEST_DESKTOP_DATA` moves both entry points together, that the installed app's own
   binary is the command line when given arguments (with the per-platform way to reach it), and what
   to do with an old `./.passquest`. `docs/agent.md` states the same resolution for the host
   lookups. Disclosed consequence of one shared resolution: the window's Today command now omits
   `--db` when the generated command resolves the same store (it always printed one before), because
   the window's own path *is* the default now — a copied command still opens the same database, and
   nothing below the header changes.
   Reverse: restore the literal usage line and the guides' old sentences; the command text follows
   the resolution again if the comparison is dropped.
7. **Q-0020-07 — What the machine owns now, CI the rest.** The two new per-OS checks are a
   `desktop-smoke` scenario (`default-data`) and a `package-smoke` case (`cli-runtime`), each with a
   self-test case proving it fails on a deliberately broken fixture; both run on all three OSes in
   CI. Locally this machine proves the Go tests and the two gates' cheap paths only: window cases are
   advisory here (the brief's 100 % CPU fact) and CI's run is the authority. The package gate runs
   once, at the end of the step that changes the package.
8. **Q-0020-08 — What this goal does not do.** No migration of existing data, no WAL change, no
   lock file, no change to where the window keeps data, no change to `PASSQUEST_DESKTOP_DATA`'s
   meaning for the window, no change to the database schema, the bound window API, the host
   protocols, scoring, consent or any golden. `PASSQUEST_NODE` and the four `PASSQUEST_*_HOST`
   variables keep winning over everything.
   Reverse: none needed — this entry records what was left alone; a later goal that wants any of
   these starts its own decision.

## Steps (one implementer run each; each leaves every gate green)

| # | Step | Proof |
|---|---|---|
| P1 | One data directory: `internal/userdata`, `study.DatabasePath`, the CLI's default and the notice, the window's default seam, the rewritten default-path tests, the five documents the old default lives in, Q-0020-01/02/06/08's outcome lines | `go test ./internal/... -count=1` with the new userdata, cli, desktop and study tests; `go build`/`vet`/`gofmt`; `import-smoke`, `tactic-smoke`, `adaptive-smoke`, `docs-links`, `secrets` green; no test or gate touches a real per-user directory |
| P2 | Safe together: `store.IsBusy`, the `internalError` mapping, the interleaving tests, the mid-command mapping test | `go test ./internal/store/... ./internal/cli/... -count=1` with the two-handle and the held-lock cases, and a case that fails on the driver's raw text (Q-0020-03) |
| P3 | The installed binary's CLI: `desktop.ResourceDir`, `cli.Options.Resources`, the package smoke's `cli-runtime` case and the `default-data` desktop scenario, the upgrade case's pins removed, `docs/agent.md`'s resolution sentence, both self-test cases | the package smoke's `cli-runtime` case locally (once, at the end); `desktop-smoke`'s `default-data` scenario locally; both self-test cases fail on their broken fixture; the three-OS CI run as the authority |
| P4 | Node 26.13.1: `NODE_PINS`, `mise.toml`, `gate.yml`, the pin-agreement check and its self-test case, the guides' Node sentence, `docs/agent.md`'s runtime sentence, Q-0013-03 revisited | `mise exec -- node --version` = `v26.13.1`; the agreement case failing on a mismatched pair and passing on the matching one; the package gate's staging lines (`node 26.13.1`, `node --version: v26.13.1`, the durable host's status probe on that runtime); agent, tutor, note, find, assess, provider and package smokes green on the three OSes; the package sizes before and after |
| P5 | Evidence, decisions, the guides' facts and the counts | `docs/evidence/0020-cli-data-and-runtime.md` with every Done-when item's artifact and run id; `docs-links`, `install-guide`, `secrets`, `typecheck` green; the coordinator's review and the goal's close |

Each step is one pull request cut from `main` after the previous step merged; the coordinator appends
the plan's defaults Q-0020-01…08 to `docs/decisions/OPEN-QUESTIONS.md` **before P1's branch is cut**
(the goal's own rule is that a default is disclosed when it is taken — the same order goal 0022
used), and each step updates **only** its own entries' outcome lines where the implementation
differed from this plan: P1 -01/-02/-06/-08, P2 -03, P3 -04, P4 -05 plus the Q-0013-03 revisit note,
P5 -07. No step writes an entry another step owns, so no ID is ever appended twice.

**Each change and the check that fails when it is removed** (the rule that a check must be able to
observe what it claims):

| Change | The check that reddens without it |
|---|---|
| the CLI's default is the per-user directory | `internal/userdata`'s table test and the CLI/study/desktop tests on the resolved path (P1); the `default-data` scenario's file walk and the CLI's read-back on each OS (P3) |
| the notice about the old file | the notice test asserts the exact line, presence and absence per input, and the old file's bytes and mode afterwards (P1) |
| the busy mapping in `errors.go` | the held-lock test asserts the `database_busy` code and message; against the base tree it reads the driver's raw text (P2) |
| the CLI's resources directory | the `cli-runtime` case fails with the resolution error naming the paths it tried; the `upgrade` and `brief` cases fail their pin-free CLI halves (P3) |
| the Node 26 pins | the pin-agreement check over the two pin lines and the three pinned sentences (P4; its self-test case reddens on a mismatch); staging's own `node --version` and SHA-256 checks (`package.ts:1090-1091`, the download's pin check) |

### P1 — One data directory (facts 1, 2; Done-when 1, 2)

1. `internal/userdata/userdata.go` (new) with the names and the two functions of Q-0020-01, a table
   test for `Dir` and `DatabasePath` (override wins; a whitespace-only override is ignored; config
   directory joined with the name; the config-directory failure is returned, never swallowed;
   `DatabaseFile` inside the directory), and
   the package doc saying what it owns. `LegacyDatabasePath` is a constant here — the old default is
   *named* only by the notice.
2. `internal/study/study.go`: `DatabasePath` becomes `(selected string, configDir, getenv) (string,
   error)` delegating to `internal/userdata`; `DefaultDatabasePath` becomes
   `DefaultDatabasePath(configDir, getenv) (string, error)`, so the two comparison call sites resolve
   the default the same way the session did. `internal/cmdline`'s `NextDatabaseArgument` is
   unchanged; on a resolution error the call sites pass `""` as the default, which prints the
   selected path — the safe direction.
3. `internal/cli`: `Options` gains `ConfigDir func() (string, error)` (default `os.UserConfigDir`) and
   the notice's once-per-run state; `ports`/`openDatabase` resolve through one helper. `privateDefault`
   keeps its meaning for the app-owned default — tightened to 0700, the directory now being the
   per-user one — and turns false when `PASSQUEST_DESKTOP_DATA` selected the directory, which is the
   learner's choice and is not tightened (the tests pin both sides). The notice is written to
   `opt.Stderr` from that helper (Q-0020-02, quoted for the detected shell family); the usage line
   (`cli.go:263`) takes Q-0020-06's wording. An unresolvable per-user directory becomes one refusal
   naming `PASSQUEST_DB`/`--db`, surfaced exactly where the store open fails today.
4. `internal/desktop`: `openStudy`'s `userData` seam becomes
   `func() (string, error) { return userdata.Dir(os.UserConfigDir, getenv) }`; `resolveStorage`, the
   install refusal, the 0700 tightening, `EnvData`, `EnvDB` and `AppName`'s literal stay as they are
   (fact 2, the install-guide gate's regex). `internal/workspace`'s empty-`DBPath` fallback
   (`workspace.go:132-134`) gets the same seam through `Options`.
5. Tests, all isolated from any real per-user directory (a temp config root through the new seam, or
   `--db`). `runNoDB` (`tactic_test.go:15-28`, fourteen call sites — twelve through `mustRunNoDB`,
   two direct) builds its `Options` with the temp config root, because `next` is the one caller that
   opens a store (`next.go:37`) — without the seam
   `TestNextCommandOmitsDefaultDatabase` creates a real per-user database the moment the default
   moves. `TestDatabasePath` (`cli_test.go:120-138`) states the per-user default and both overrides;
   `TestDefaultDatabaseDirectoryIsTightened` (`source_test.go:626-655`) asserts the *per-user*
   directory is created 0700 and that no `.passquest` appears in the working directory;
   `TestNextCommandOmitsDefaultDatabase` (`next_test.go:360-375`) asserts no `--db` under the temp
   config root, and the three `next` goldens (`:503-529`) keep passing because they pass
   `--db attempts.sqlite`, which is not the default, so `NextDatabaseArgument` still prints `--db`
   and no golden moves; new tests: the notice once per run with the old file's bytes and mode
   unchanged afterwards, no notice with `--db`/`PASSQUEST_DB`, no notice without the old file, the
   both-files branch naming both paths, and the move command's two quoted absolute paths per family;
   `internal/desktop`: the four-value parity test and the
   default seam resolving under a temp config root; `internal/study`: the delegation table.
6. `docs/decisions/OPEN-QUESTIONS.md`: Q-0020-01, -02, -06 and -08's outcome lines (the entries are on
   `main` before this branch; an implementation fact that differed from the plan is recorded here,
   never as a second entry).
7. The documents P1 makes false, in the same diff: `docs/find.md` (`:15`), `docs/notes.md` (`:38`),
   `docs/tutor.md` (`:35`) and `docs/source-import.md` (`:8-9`, and its Known-limits restatement at
   `:150-151`) each name the CLI's old default or the 0700 rule's reach; `docs/desktop.md` (`:52`)
   says "The CLI default is unchanged."; `docs/conventions.md`'s layout block gains
   `internal/userdata/`. No gate reads these sentences (`docs-links` checks links only), so the diff
   is the check. `Q-0011-04` gains a dated *Revisited* line and `Q-0002-02`'s platform-limit
   paragraph (`OPEN-QUESTIONS.md:148-152`) extends to the per-user default, naming
   `PASSQUEST_DESKTOP_DATA` as the third exempt choice beside `--db`/`PASSQUEST_DB` — the same
   exemption `source-import.md:150-151` states; the historical records under `docs/evidence/**` stay
   as history.

*Audits it must keep green:* `gofmt`, `go vet`, `go build`, the whole `go test ./internal/...` suite,
`typecheck`, `docs-links`, `secrets`, `local-only` (the new package needs no `BOUNDARIES` entry —
the table matches prefixes — and `docs/conventions.md` declares it in the same diff), `import-smoke`,
`tactic-smoke`, `adaptive-smoke`, and `workspace`/`desktop` Go tests. *Measurement row:* none (no
page, no window, no package). *Shots it may change:* none.

*Proof:* `mise exec -- gofmt -l internal cmd` (empty), `mise exec -- go vet ./...`,
`mise exec -- go build ./...`, `mise exec -- go test ./internal/... -count=1` — the new cases named in
the run's own output; `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts typecheck.ts typecheck`;
`… docs-links.ts docsLinks`; `… secrets.ts secrets`; and a grep-backed statement in the report that no
test reaches a real per-user directory — every `Run` in a test selects a database or injects a temp
config root, the `mustRunNoDB` callers are checked one by one, and `next`'s is the only store opener.

### P2 — The window and the command line are safe together (fact 4; Done-when 3)

- `internal/store`: `IsBusy(err error) bool` (`errors.As` to the driver's `*sqlite.Error`, primary
  code 5/6) with a unit test that feeds a real busy error and a lookalike text and distinguishes them
  (a test on the string alone would pass with the mapping removed).
- `internal/cli/errors.go`: `internalError` maps `store.IsBusy` to a usage refusal (exit 2, code
  `database_busy`, one action), so a busy error from any command — at open or later — reads the same;
  a test runs a command against a database whose write lock is held past the timeout and asserts the
  message and the exit code. It **fails** against the base tree's raw driver string.
- `internal/store` interleaving tests (serial, they hold locks): two handles on one file, interleaved
  writes, both rows present; one handle holding `BEGIN IMMEDIATE` for less than the timeout while the
  other writes — the writer waits and succeeds; the lock held past the timeout — the other handle
  fails with a busy error, its own transaction leaves no partial row. The timeout cases may open
  their handles with a shorter `busy_timeout` DSN to keep the suite fast; the test names the value it
  used.
- The three facts the plan does **not** change are asserted as they are: `busy_timeout(5000)`,
  `_txlock=immediate`, one connection per handle (`internal/store/store_test.go`'s existing DSN
  cases cover the first two).

*Audits it must keep green:* `go test ./internal/...`, `gofmt`, `go vet`, `go build`, `typecheck`,
`secrets`. *Measurement row:* none. *Shots it may change:* none.

*Proof:* `go test ./internal/store/... -count=1` and `go test ./internal/cli/... -count=1` with the
new case names in the output; the held-lock test's measured wait; and a statement in the report of
the interleaving the test actually produced (the two writes' order read from the rows).

### P3 — The installed binary runs the AI commands from its own package (facts 3, 6; Done-when 1, 3, 4)

- `internal/desktop`: export `ResourceDir()` (the body of `resourceDir`, unchanged semantics: `""`
  in development); `cmd/passquest-desktop/main.go`'s `run` passes it into `cli.Options{Resources: …}`
  for the argument path only; a `cmd/passquest-desktop` test pins that a development build passes
  `""` and that the built CLI path still dispatches (the existing `main_test.go` shape).
- `internal/cli`: `Options.Resources` reaches the four default transports (`withDefaults`), and a Go
  test proves the threading end to end without a package: a temp resources tree holds
  `agent/find-host.mjs` and `node/bin/node`, and a command whose host resolution fails names the
  temp path in its error (the same assertion shape `hostpaths`' own tests use).
- The package smoke's new case `cli-runtime` (case list `harness/gates/package-smoke.ts:117`), run
  after `study` so the app's data root exists: the installed binary started with arguments, on the
  same PATH stripped of `node`/`go`/`bun`, the same temp per-user root, and **no** `PASSQUEST_NODE`
  or `PASSQUEST_*_HOST`: one host-backed command against the loopback provider stub (`model test
  stub/stub-model`, the case the desktop smoke already seeds for the window), asserting exit 0 and a
  `verified` provider state. The stub registry variable and `PASSQUEST_STUB_API_KEY` it needs (what
  the other cases already pass) are not resolution overrides; the case asserts that neither
  `PASSQUEST_NODE` nor any `PASSQUEST_*_HOST` was in the child's environment. A successful
  host-backed command with no Node on `PATH` is the proof that the package's own runtime and hosts
  ran it; the case additionally reads the resolved paths back from the CLI's own output where it
  names them.
- The `upgrade` case's revision run and the `brief` case's CLI half drop their `PASSQUEST_NODE` and
  `PASSQUEST_*_HOST` pins (`harness/gates/package-smoke/upgrade.ts:777-792`, `brief.ts`'s
  `installedEnv`): the installed binary resolves the packaged runtime and hosts itself now, and a
  case that still pinned them would hide the regression. The scenario's `runtime`/`durableHost`
  fields stay for the pre-launch existence check (`brief.ts:75-81`: a tree that lost either fails
  **by name** instead of as "node not found"), with their docs and the four stale comments updated
  in the same diff (`harness/gates/desktop-smoke/scenario.ts:52-63`,
  `harness/gates/package-smoke/brief.ts:13-18` and the pre-launch rationale at `brief.ts:65-74`,
  `harness/gates/package-smoke/upgrade.ts:45-49`).
- `docs/agent.md`: the resolution sentence Q-0020-06 promises — the installed binary started with
  arguments resolves the package's own `agent/` hosts and `node/` runtime as the window does, with
  the `PASSQUEST_*` overrides still winning — lands beside the packaging bullet
  (`docs/agent.md:69-75`), above the runtime sentence P4 rewrites (`:94`).
- The `default-data` desktop-smoke scenario (`harness/gates/desktop-smoke/` new file, registered in
  `SCENARIOS`, `desktop-smoke.ts:38`): `dataOverride` and `launchEnv` are fields of the gate-wide
  `Shared`, built once (`scenario.ts:14-63`, `desktop-smoke.ts:76`), so the scenario builds its own
  run object — a `home` under its `dir`, `dataRoot = dataRootFor(home, "Pass Quest")`, and
  `{...s, dataDir: dataRoot, database: path.join(dataRoot, "passquest.sqlite"), shared:
  {...s.shared, dataOverride: false, launchEnv: {...s.shared.launchEnv, ...homeEnv(home)}}}` — and
  every launch and CLI run it makes carries that `homeEnv(home)` block (`HOME`/`XDG_CONFIG_HOME` on
  POSIX, `APPDATA`/`LOCALAPPDATA` on Windows): `cleanEnv` (`desktop-page.ts:133`) strips only
  `PASSQUEST_*`, so a run without the block would read and write the developer's real per-user tree.
  `homeEnv` and `dataRootFor` move from `harness/gates/package-smoke.ts:154-172` into
  `harness/lib/desktop-page.ts` beside `cleanEnv`, one implementation both gates import (the package
  smoke's `siteOf`, `package-smoke.ts:352-380`, is the per-case shape this mirrors). The scenario
  cannot use the shared `cli()` or `cliJson` helpers — both append `--db` (`scenario.ts:131`,
  `desktop-page.ts:230`) — so its CLI half runs the built binary through `run`/`parse` directly with
  no `--db`, the registry variable and the same `homeEnv(home)`. It launches with `dataOverride:
  false` and no `PASSQUEST_DB`: the window records an attempt (the existing practice walkthrough,
  which needs no seeding — the sample is embedded, `internal/study/content.go:14`); the gate asserts
  the database exists at the per-user path and nowhere else under the temp root; the first launch is
  held with `killWhen` (the `resume` pattern, `resume.ts:44`) and, while the window is still
  running, the CLI reads the attempt back through `report --json` and records a second attempt — the
  two processes are live at once, Done-when 3's literal case beside P2's held-lock tests; a second
  window launch shows both. That is Done-when 1's per-OS both-directions case and Done-when 3's real
  two-process case in one scenario; the scenario's detail line records the paths it read so the
  artifact carries them.
- Self-test cases (`harness/gates/self-test/`): a `package-smoke` fixture app whose data root is a
  different directory than the CLI's default must fail `cli-runtime`/the both-directions assertion
  naming the two paths; a `desktop-smoke` fixture main (`cmd/passquest-desktop/main.go`, the shape
  the desktop-smoke cases already use — it answers `PASSQUEST_DESKTOP_SCRIPT` and writes
  `PASSQUEST_DESKTOP_RESULT`) that keeps its database outside the per-user root must fail
  `default-data`; and the `self-test` gate's `brokenCases()`/`passCases()` lists
  (`harness/gates/self-test.ts:313`, `:345`) and counts follow (the P9 re-quote rule for
  `docs/harness.md` row 27).

*Audits it must keep green:* the whole `desktop` group on the three OSes (the new scenario included),
the `package` group on the three OSes (the new case, the stripped-pin `upgrade` and `brief` CLI
halves, the content scan and the layout case unchanged), `typecheck`, `secrets`, `docs-links`,
`gofmt`/`vet`/`build`/`go test`, and the self-test gate with its new cases. *Measurement row:* the
branch's `frontend-bench.yml`
run is not required by this step (no page and no window option changes); if the coordinator opens a
draft pull request, the bench's seven shots must stay byte-identical on both OSes. *Shots it may
change:* none.

*Proof:* the package gate's `cli-runtime` case once, at the end, on this machine (`mise exec -- bun
Q:/scratch/pq-harness-run/run-0020.ts package-smoke.ts packageSmoke`), with its pass line and the
case's own recorded paths quoted in the report; `desktop-smoke` with the new scenario
(`… desktop-smoke.ts desktopSmoke`) if the machine's load permits, otherwise the CI run named by the
coordinator; both self-test cases failing on their broken fixture with exactly their case names.
Runner by runner: this machine (Windows) proves the Go tests, the self-test cases and — load
permitting — the two gates; the `default-data` scenario's macOS and Linux halves and the macOS
`Contents/Resources` and Linux AppImage half of `cli-runtime` are proven **only** by CI's `desktop`
and `package` jobs on `macos-latest` and `ubuntu-latest`, and the report says so instead of claiming
three-OS evidence from one machine.

### P4 — Node 26.13.1 everywhere (fact 5; Done-when 5)

- `harness/lib/package.ts`: `NODE_VERSION = "26.13.1"` and the four `NODE_PINS` (archive, member,
  path, SHA-256) re-read at implementation time per Q-0020-05; nothing else in staging changes.
- A pin-agreement check: `harness/lib/package.ts` gains a pure function that compares `NODE_VERSION`
  against every source that carries the pin — the `node = "…"` line of `mise.toml` (equality), the
  `node-version:` line of `.github/workflows/gate.yml` (equality), and the pinned-runtime version
  token of `docs/install.md`, `docs/install.zh-CN.md` and `harness/lib/package.ts`'s own header
  sentence — returning one named failure per source that disagrees (the source, the value it read,
  `NODE_VERSION`). The package gate runs it at the start of every run, before any case, over the
  repository's own files resolved from the harness's own location (the way
  `harness/gates/self-test/fixture.ts:57` resolves the repository root), **never** `ctx.root`: the
  self-test runs the gate with `ctx.root` at a fixture tree whose `FIXTURE_FILES`
  (`package-smoke.cases.ts:100-107`) carries no `mise.toml` and no `.github/`, so a `ctx.root` read
  would red every existing fixture case. No gate reads any of these version lines today
  (`harness/gate.ts` reads `gate.yml` only for the gate groups), so without this a `mise.toml`,
  `gate.yml` or guide left behind is invisible. `PackageSmokeOptions` gains an optional
  `nodePinSources` record (label → text) so `harness/gates/self-test/package-smoke.cases.ts` can
  hand the gate a mismatched set failing by name plus an accepted matching set; every other run, the
  fixture cases included, keeps the default read — the repository's own (matching) sources.
- `mise.toml:3` → `node = "26.13.1"`; `.github/workflows/gate.yml:52-54` → `node-version: "26.13.1"`
  (the only workflow that pins Node; `frontend-bench.yml`, `cask.yml` and `keychain.yml` run Bun
  only).
- `docs/install.md:7` and `docs/install.zh-CN.md:7` (the "pinned Node 22 runtime" sentence — both
  sides carry the same fact) and `harness/lib/package.ts:3`'s header comment name 26.13.1. The
  sentence sits in the guides' intro, before the first `##`, and `install-guide` checks each FACT
  only inside its `GUIDE_SECTIONS` section (`install-guide.ts:456-458`), so the pin-agreement check
  reads the three sentences instead; the sentence lands in the same step as the pin, so no merged
  tree says "Node 22" beside a 26 runtime.
- `docs/agent.md:94` (the runtime sentence below the command list) names the runtimes that hold
  after this step: the packages' own requirement (`>=22.19.0`), what a package bundles (26.13.1),
  and that the development toolchain and CI run the same patch as the bundle.
- `docs/decisions/OPEN-QUESTIONS.md`: Q-0013-03 gains its dated revisit with the new pin and the four
  SHA-256s (`nodejs.org` named as their source), and Q-0020-05's outcome line carries the version
  actually taken. The recorded reason is the owner's decision in this goal's contract (2026-11-22),
  not a fired clause: the entry's own "Revisit when" terms (Node 22 leaving support, a new target, a
  system Node) do not hold — Node 22 is still in maintenance until 2027-04-30.
- The evidence record gains the package-size pair: the installed artifact's bytes per OS before
  (from the current release/package artifacts) and after (from this step's `package-<os>` CI
  artifact), each with the artifact name and run id — read from the artifact, never from a build log
  line alone.

*Audits it must keep green:* the whole 27-gate matrix on the three OSes — the agent, tutor, note,
find, assess and provider smokes included, because they spawn the source hosts on the runner's Node,
and the package smoke because it runs the staged durable host (its `node:sqlite` storage) on the
pinned runtime — the self-test gate with the agreement case, and the package gate's start-of-run
agreement check; `go test` unchanged; `typecheck` unchanged. *Measurement row:* none required; the
`frontend-bench.yml` path filter watches `mise.toml` (`:16`), so a run may be started by the push —
it must read not worse beyond noise with all seven shots byte-identical on both OSes. *Shots it may
change:* none.

*Proof:* locally `mise exec -- node --version` = `v26.13.1` and `mise exec -- go test
./internal/... -count=1` (the hosts are spawned by gates, not by unit tests); the agreement case
failing on its mismatched fixture and passing on the matching pair; the package gate's staging lines
(`node 26.13.1: build/package/node/…`, `check: node --version: v26.13.1`, `check: …
durable-host.mjs answered status on …`); the three-OS CI run's seven smoke gates and the package
gate; the size pair from the two artifacts.

### P5 — Evidence, decisions, the guides and the counts (Done-when 6, and the record of 1–5)

- `docs/install.md` and `docs/install.zh-CN.md`: inside "Where your data lives, and how to remove it"
  (and its Chinese twin), Q-0020-06's sentences — the command line's default, that
  `PASSQUEST_DESKTOP_DATA` moves both entry points, the installed binary as a command line with the
  per-platform way to reach it (Windows: the app executable in the install directory; macOS:
  `…/Pass Quest.app/Contents/MacOS/…`; Linux: the AppImage file itself) and what to do with an old
  `./.passquest`. The package-smoke case list the same guides print (`docs/install.md:243`, `:289`;
  `docs/install.zh-CN.md:159`, `:188`) gains `cli-runtime` in the order the gate's pass line prints
  it. Both sides carry the same facts; no new `##` heading (fact 7).
- `docs/evidence/0020-cli-data-and-runtime.md`: per Done-when item, the artifact and the run id —
  the per-OS both-directions readings of `default-data` (the paths and the attempt ids), the
  `cli-runtime` case's environment and result, the concurrency tests' measured wait and refusal
  message, the four pins, the gate pass lines of the seven smokes on the three OSes, the package-size
  pair, and the known limits (below).
- `docs/harness.md`: row 8's install-guide facts if the guides' pinned facts changed, row 24's
  scenario and assertion counts, row 26's case list, counts and its resolution sentences (the row's
  "the CLI composition root resolves no resources directory, only the window does" and the
  brief-case sentence about the pinned runtime and durable host, `:79`), row 27's case counts — each
  read from the same step's own pass lines (the rows' own rule).
- `docs/evidence/0016-lean-package.md`: a dated note on the disclosed limit at `:207-213` recording
  that goal 0020 P3 removed the deviation — the `brief` case's CLI half now resolves the installed
  copy's packaged runtime and host with no pin — naming P3's three-OS package run as its evidence,
  and correcting the block's row reference (`:213` says row 24; the package smoke is row 26).
- `docs/goals/README.md`'s state column and the goal file's archive move are the coordinator's close
  (conventions, *Working rules*), in the same commit as the close's diff.

*Audits it must keep green:* `docs-links`, `install-guide`, `secrets`, `typecheck` and the whole
matrix on the final tree — this step changes no product file, so every earlier row still reads.
*Measurement row:* none. *Shots it may change:* none.

*Proof:* `run-0020.ts docs-links.ts docsLinks`, `… install-guide.ts installGuide`, `… secrets.ts
secrets`, `… typecheck.ts typecheck`, all green; the evidence record's numbers cross-checked against
the named runs by the coordinator; the guides reviewed side by side for the same facts.

## Done-when mapping

| Contract item | Steps | Evidence |
|---|---|---|
| 1 one data directory, one computation, overrides unchanged | P1, P3 | `internal/userdata`'s table test and the CLI/desktop/study tests for the default and both overrides (P1); the `default-data` scenario on the three OSes: the window writes, the CLI with no `--db` reads it back and writes, the window reads both (P3), with the paths recorded |
| 2 existing data never moved silently | P1 | the notice tests: once per run, both paths named, the old file's bytes and mode unchanged, nothing created beside it; the guides' "move it yourself" sentence (P5) |
| 3 safe together | P2, P3 | the interleaving tests (waits and succeeds inside the timeout; a clean `database_busy` refusal past it) and the per-OS `default-data` case, whose CLI write runs inside the first launch's `killWhen` while the window process is alive; the disclosed facts: per-call store opens, one connection per handle, `busy_timeout(5000)`, `_txlock=immediate`, no WAL |
| 4 the installed binary runs the AI commands from its own package | P3 | the `cli-runtime` case per OS (PATH without Node, no overrides, a host-backed command verified) and the `upgrade` and `brief` cases with their pins removed; `desktop.ResourceDir()`'s Go test pins the development build's `""` |
| 5 Node 26 LTS everywhere | P4 | the four pins, the pin-agreement check and the revisited Q-0013-03; `mise exec -- node --version`; the three-OS runs' agent/tutor/note/find/assess/provider/package smokes; the installed size before and after |
| 6 the learner is told the truth | P1, P3, P4, P5 | the two guides' data sections with the same facts; the CLI's usage line; `docs/agent.md`'s resolution sentence (P3) and runtime sentence (P4); `docs-links` and `install-guide` green; the owner's review |
| constraints: no schema, consent, bound-API or host-protocol change; nothing copied or deleted; the window's location unchanged | P1–P5 | the diff holds no schema or `agent/*.mjs` change; `internal/desktop`'s `resolveStorage` and `AppName` keep their behaviour (tests unchanged where behaviour is unchanged); the window's default still comes from `os.UserConfigDir` joined with the same name; the notice only reads |

## Risks (each with how it is detected)

- **The CLI's default diverges from the window's directory** (e.g. a third spelling of the name, or
  `os.UserConfigDir` replaced by a hand-rolled rule) → the `default-data` scenario fails on the
  missing database at the per-user path, and P1's four-value parity test reddens on a drifted name or
  variable before a run. Detection is the scenario's own assertion, not a claim.
- **A test or gate writes into a real per-user directory** (the default moved under every caller) →
  the tests that resolve the default are the four P1 rewrites plus the `mustRunNoDB` callers, and the
  report's grep-backed audit names each one and the seam it injects (`runNoDB` is the chokepoint,
  `next` the one store opener); the notice's own side effect makes a real `%APPDATA%`/`$HOME` run
  visible as a created directory too. The `default-data` scenario and the package smoke keep their
  temp roots by construction (`homeEnv`).
- **A hidden window on macOS or Linux cannot resolve the per-user root under a temp `HOME`** (MyGo's
  `PathUserData` on macOS goes through `os.UserConfigDir` → `$HOME/Library/Application Support`, and
  the login keychain is detached) → the scenario sets the smoke hook's own secret-service variable
  (it already does) and reads the file, not the credential store; if a platform still cannot, the
  scenario's failure line names the launch and the path it looked for, and the case becomes a
  recorded coverage limit rather than a green claim.
- **The Ubuntu runner has no window manager** (`.github/workflows/gate.yml:61-66` sets up WebKitGTK
  and Xvfb, not a WM; goal 0022's Q-0022-04 records the same fact) → the new scenario uses no window
  state: hidden launches, the page's own study flow (the `practice` scenario already runs there) and
  the filesystem; anything that would need WM behaviour is not asserted on Linux.
- **A command that resolves no config directory falls back somewhere** → the refusal test
  (`ConfigDir` returning an error with no override) asserts the exit code, the code and the message;
  a fallback would silently succeed and fail that test.
- **The notice appears when it should not** (`--db`, `PASSQUEST_DB`, no old file, or twice in one
  run) → the notice tests; and the JSON goldens/smokes would show a stderr difference only where a
  test asserts stderr, so the tests state it explicitly.
- **The notice's command is wrong for the learner's shell** (a paste that fails, or an unknown
  `PASSQUEST_SHELL`) → the notice renders through `cmdline`'s detection and per-family quoting, and
  its move command names the resolved absolute pair, so no shell has to expand an environment
  variable (a `%APPDATA%` spelling fails in PowerShell); the family/source/hint line prints beside
  the command, as `next` does; on a detection error it prints no command and still succeeds, and the
  notice test covers both paths.
- **A busy error still surfaces as driver text** (a code shape other than 5/6, or an unwrapped error
  from a path that bypasses `internalError`) → `IsBusy`'s test feeds both a real lock and a lookalike
  string; the mapping test runs the real command; the report lists the commands whose error paths
  reach `internalError`/`fail` and any that do not.
- **The concurrency tests flap on a loaded machine** (the 5 s timeout measured while the box is at
  100 % CPU) → the interleaving tests use their own short timeouts where possible and assert
  monotonic facts (a row exists, a wait happened, a refusal was typed), never wall-clock bands; the
  held-lock case is serial and named as such.
- **The installed binary resolves the wrong resources** (e.g. a dev build's `""` reaching a package,
  or `Contents/Resources` missed) → the `cli-runtime` case fails with the resolution error naming the
  paths it tried (`hostpaths`' own error text lists the candidates), and the `upgrade` case's
  pin-free revision run fails the same way; on macOS and Linux only CI can see it.
- **Node 26's `node:sqlite` behaves differently** (a removed experimental surface, a new warning on
  stderr, a changed path) → the package smoke's staged-host status probe runs the durable host on the
  pinned runtime, and `agent-smoke` runs all eleven scenarios against source and bundle; a warning
  where a gate asserts stderr content is read from the failing line, not guessed.
- **A newer 26.x LTS ships before the pin lands** → Q-0020-05's re-read at implementation time; the
  evidence record names the version it took and why. A `mise.toml`, `gate.yml` or pinned sentence
  left behind is caught by the pin-agreement check (P4), which runs at the gate's start and names
  the source and the value it read.
- **The package grows differently than the pin suggests** (a larger runtime archive) → the size pair
  is read from the two artifacts; the size is recorded, not judged; a *bigger* package is disclosed
  with the numbers.
- **`install-guide` fails on the new guide prose** (a fact moved, a heading added, a Chinese side
  missing a sentence) → the gate names the section and the fact; the guides are edited inside the
  existing section on both sides (fact 7), and the gate runs before the PR closes.
- **This machine is at 100 % CPU** → the new window run is advisory here; the report says which
  evidence came from CI, and the self-test cases (pure fixtures, no window) still fail as designed
  locally.
- **Another branch's parallel work in the harness** (goal 0022's remaining steps) → the overlap list
  below; the coordinator orders the merges, and any conflicting file is rebased, never force-resolved
  by deleting the other goal's checks.

## Evidence plan

Local (Windows): `mise exec -- gofmt -l internal cmd`, `go vet ./...`, `go build ./...`, `go test
./internal/... -count=1` after P1, P2 and P4; the gates `typecheck`, `docs-links`, `secrets`,
`local-only`, `import-smoke`, `tactic-smoke`, `adaptive-smoke` where the step's surface demands them;
and, once each at the point its step says so, `desktop-smoke` (`default-data`) and `package-smoke`
(`cli-runtime`) — the package gate once, at the end of P3. The two self-test cases run through the
self-test gate with `--only`-style selection where its shape allows, else through the whole gate —
P3's two and P4's agreement case alike.

CI (the authority): the branch's `gate.yml` matrix — `core`, `desktop`, `package`, `self-test` on
ubuntu-latest, macos-latest and windows-latest — once per step on its pull request, with the
`package-<os>` artifacts and, on a failure, the `gate-output-<os>-<group>` logs. The numbers in
`docs/evidence/0020-cli-data-and-runtime.md` are read from those artifacts and named by run id.
Disclosed limits: macOS and Linux window behaviour is CI-only from this machine; the `default-data`
scenario's WebView profile and the credential store are per-run temp state (the smoke hook's own
variables), so it never touches the learner's; the package smoke's macOS `.dmg` carries no quarantine
attribute; the Linux AppImage runs with `APPIMAGE_EXTRACT_AND_RUN=1`.

## Facts each step hands to the next

- **P1 → all:** the resolved path each platform produces under a temp config root, the notice's exact
  line and its once-per-run mechanism, the `ConfigDir` seam's shape in `Options`/`workspace.Options`,
  the list of tests whose expectation moved (so P3's scenario asserts the same paths) and the
  documents whose sentences moved.
- **P2 → P3, P5:** the measured wait inside `busy_timeout`, the exact refusal line, and whether any
  command needed the mapping beyond `internalError`.
- **P3 → P4, P5:** whether the package's `node/` path resolution and the macOS `Contents/Resources`
  case were proven on CI, the `cli-runtime` case's environment and result, and the case counts the
  self-test and `docs/harness.md` rows must carry.
- **P4 → P5:** the runtime version actually pinned, each pin's provenance line, the pin-agreement
  case's name and the self-test count it adds, the staged-host check lines, and the size pair.
- **P5 → the close:** every artifact path and run id, the guides' final wording, and the known limits
  list.

## Files this plan must touch that goal 0022 also touches (for the coordinator's merge order)

Goal 0022 runs in `Q:/repos/passquest` in parallel (P1 already merged here as `b338c67`; its P2–P5 are
still in flight). Shared files:

- `internal/desktop/desktop.go` — 0022 P2 adds the window options (`TitleBarStyle`, `TitleBarHeight`)
  and P3/P4 extend them per OS; this plan exports `ResourceDir()` and changes `openStudy`'s
  `userData` seam in the same file. Both changes are small and in different functions; the later
  merge rebases on the earlier one.
- `harness/gates/desktop-smoke.ts` and `harness/gates/desktop-smoke/*` — 0022 adds `titlebar.ts` and
  boot assertions; this plan adds a `default-data` scenario and registers it in the same `SCENARIOS`
  list. New files do not conflict; the list line and `docs/harness.md` row 24's counts do.
- `harness/lib/desktop-page.ts` — 0022 P1 already changed it; this plan moves `homeEnv` and
  `dataRootFor` here from `harness/gates/package-smoke.ts:154-172`, beside `cleanEnv`, and both
  gates import them (one implementation; the package smoke keeps no copy), which no 0022 step edits
  again.
- `harness/gates/self-test/*` and `harness/gates/self-test.ts` — both goals add cases and both change
  the count lines; count lines are re-quoted from the run, so the later merge takes the sum.
- `docs/harness.md` — 0022 P5 writes rows 24/25/27's final counts; this plan writes rows 8/24/26/27.
- `docs/decisions/OPEN-QUESTIONS.md` — both append; the entries are disjoint, the last merge keeps
  both.
- 0022's remaining steps do **not** touch: `mise.toml`, `.github/workflows/gate.yml`, `docs/install.md`,
  `docs/install.zh-CN.md`, `harness/lib/package.ts`, `harness/lib/package-build.ts`,
  `harness/gates/package-smoke*`, `internal/cli/**`, `internal/study/**`, `internal/store/**`,
  `internal/hostpaths/**`, `internal/workspace/**`, `internal/cmdline/**` and
  `cmd/passquest-desktop/main.go` — so this plan's changes there merge without waiting on 0022.
  `desktop/web/**` is 0022's surface (its P2–P4 edit the stylesheets, the top bar and the generated
  client); this plan changes no frontend file, so it neither conflicts with nor waits on 0022's page
  work.

Recommended order: P1, P2, P4 (no overlap in the first two; P4 now adds a
`harness/gates/self-test/package-smoke.cases.ts` case whose count lines the later merge takes as the
sum, the same rule as P3's cases), then P3 and P5 after 0022's last harness-touching step has merged,
so the count lines and the desktop-smoke list are written once against one base.

## Coordinator actions needed

- **Append Q-0020-01…08 to `docs/decisions/OPEN-QUESTIONS.md` before P1's branch is cut** (the
  goal's disclosure rule, the order goal 0022 used) and **nothing else about them**: each step
  updates only its own entries' outcome lines (P1 -01/-02/-06/-08, P2 -03, P3 -04, P4 -05 and the
  Q-0013-03 revisit, P5 -07), so no entry is written twice.
- **Open one draft pull request per step** (branches `goal/0020-p1`…`goal/0020-p5`, each cut from
  `main` after the previous merge) so the three-OS gate runs on every increment; the `package` group
  is the slowest (~8 minutes per OS) and is where P3 and P4's real proof lands.
- **Nothing else**: no secrets, no release, no remote write, no tap change. macOS and Linux window
  evidence and the four Node archives' fetches happen on CI's runners; the owner's input is not
  needed beyond the decisions already recorded (Q-0000-07's platforms, the goal's Node 26 intent).
  Nothing here needs a credential, a budget or a package from the corporate npm feed: the Node
  archives come from the public `nodejs.org` with published SHA-256s, every provider call in the new
  checks goes to the harness's loopback stub, and the `@earendil-works` packages are goal 0021's
  scope and are not touched.
