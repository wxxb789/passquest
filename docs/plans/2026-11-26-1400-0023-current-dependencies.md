# 0023 — Every dependency is on its newest served release: increment plan

Contract: [`../goals/0023-current-dependencies.md`](../goals/0023-current-dependencies.md).
Conventions: [`../conventions.md`](../conventions.md). Harness: [`../harness.md`](../harness.md).
Cross-platform rules: [`../cross-platform.md`](../cross-platform.md).
Decisions: [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md) — Q-0021-07 holds the Pi
feed decision this goal continues.
Base: `main` at `943de9d` (the coordinator's `docs/0021-close-0023` tip, squash-merged as #78; the
same tree's pre-squash tip `077d675` is not on `main`; this plan is written on `goal/0023-p0`).
Scouted 2026-11-26; every number below is read from the source named beside it or quoted from the
command shown here.

Local runs on this machine (Windows), 2026-11-26: `mise exec -- go version` =
`go1.27.4 windows/amd64`; `mise exec -- bun --version` = `1.4.5`; `mise exec -- node --version` =
`v26.13.1`; `npm config get registry` = `https://packagefeedproxy.microsoft.io/npm/`;
`mise exec -- go env GOPROXY` = `https://proxy.golang.org,direct`.

## Goal in one sentence

Move every dependency the repository pins — Go modules, the npm packages (the four Pi packages
together, at least to 1.6.0 or item 2 reported blocked by the feed), the Go/Bun/Node toolchains and
the GitHub Actions — to the newest release its source serves, keep the learner-visible behaviour as
before, and keep every gate green on all three OSes.

## Facts this plan rests on (cited or measured, not remembered)

### 1. npm: the corporate feed is the source of truth, and it lags upstream by about a week

- Everything below ran through the configured feed (never the public registry — the goal forbids
  switching, and Q-0021-07 already decided against vendoring).
- Root `package.json` devDependencies are `@types/bun ^1.4.4` and `typescript ^7.0.2`; `bun.lock`
  resolves `@types/bun@1.4.4` and `typescript@7.0.2` (lock `workspaces.""`). The feed's newest:
  `npm view @types/bun dist-tags --json` → `{…,"latest":"1.4.4"}` (76 versions served, last 1.4.4);
  `npm view typescript dist-tags --json` → `{…,"latest":"7.0.2","beta":"7.1.0-beta","rc":"7.0.1-rc"}`;
  the 7.x versions served are 7.0.1-rc, 7.0.2, 7.1.0-beta and 7.1.0-dev nightlies (3890 versions
  total, re-read 2026-11-26; the nightly stream keeps the total moving), so 7.0.2 is the newest
  stable. `npm outdated` at the root printed no rows (exit 0; its `--json` form prints `{}`) —
  nothing to move.
- `agent/package.json` pins the four direct `@earendil-works` packages at `1.5.2` (exact); `bun.lock`
  holds six `@earendil-works` entries, all `1.5.2`: `chord`, `pi-ai`, `pi-codemode`, `pi-durable`,
  `pi-schema`, `pi-telemetry`. The feed's newest for each is 1.5.2 (`npm view <pkg> version` = 1.5.2;
  dist-tags `latest` = 1.5.2; each listing's tail reads `… 1.5.0 1.5.1 1.5.2`). `(cd agent && npm
  outdated)` printed no rows (exit 0; its `--json` form prints `{}`). The feed published pi-ai 1.5.2
  on 2026-11-10T12:20:46Z
  (`npm view @earendil-works/pi-ai time --json`), while upstream 1.6.0 is dated 2026-11-20 and 1.6.1
  2026-11-25 (the goal's own text) — the feed has not picked either up.
- The feed does not serve 1.6.0 at all: `npm view @earendil-works/pi-ai@1.6.0 version` →
  `npm error code E404` / `npm error 404 No match found for version 1.6.0`. Item 2 is therefore
  **blocked at scout time** with exactly this listing as evidence; P4 re-runs the check first.
- Lock hygiene the hard rules pin: `bun.lock` is `"lockfileVersion": 2, "configVersion": 1`, every
  package entry's registry-URL field is `""` (bun uses the configured registry at install time), and
  no package may be moved to an older version than the lock holds.

### 2. Go modules: what the configured proxy serves, and MyGo's newest tag

`mise exec -- go list -m -u all` (GOPROXY `https://proxy.golang.org,direct`; the toolchain comes
from `mise exec`, the cache path from `go env`) printed, for `go.mod`'s direct four and their graph:

- Direct, with no newer release served: `github.com/ebitengine/purego v0.11.3`,
  `github.com/egoist/mygo v0.10.0`, `golang.org/x/sys v0.50.0`.
- Direct, newer served: `modernc.org/sqlite v1.62.0 [v1.63.0]`.
- Indirect (moves through the graph), newer served: `modernc.org/libc v1.82.0 [v1.82.13]`,
  `modernc.org/cc/v4 v4.34.1 [v4.35.0]`, `modernc.org/ccgo/v4 v4.39.2 [v4.39.3]`,
  `modernc.org/gc/v3 v3.2.0 [v3.2.1]`, `golang.org/x/image v0.46.0 [v0.48.0]`,
  `golang.org/x/mod v0.42.0 [v0.43.0]`, `golang.org/x/tools v0.52.0 [v0.53.0]`,
  `github.com/klauspost/compress v1.20.4 [v1.20.5]`, `github.com/stretchr/testify v1.11.1 [v1.12.1]`,
  `github.com/tphakala/simd v1.11.0 [v1.16.0]`, `github.com/tphakala/go-aac v0.7.1 [v0.7.2]`,
  `github.com/jfreymuth/oggvorbis v1.0.5 [v1.0.6]`, and newer pseudo-versions of
  `github.com/google/pprof` and `github.com/mewkiz/pkg` (the output prints both old and new).

Of the 15 rows, three are `go.mod` requirements — `sqlite` (direct) and `modernc.org/libc`,
`github.com/klauspost/compress` (indirect) — and P1 bumps exactly those; `modernc.org/cc/v4` and
`modernc.org/ccgo/v4` are not in `go.mod` but follow `libc`'s new requirement. The other ten rows
(`gc/v3`, `x/image`, `x/mod`, `x/tools`, `testify`, `simd`, `go-aac`, `oggvorbis`, `pprof`,
`mewkiz/pkg`) have no `require` line: their selected versions are their importers' requirements, not
this tree's (`go mod why -m golang.org/x/tools` walks through a dependency's test package; `go mod
why -m github.com/tphakala/simd` reads `(main module does not need module …)` — four of the ten read
that, the other six carry a chain). Measured in a scratch copy
of this tree (2026-11-26): `go get -u all` + `go mod tidy` moved exactly `sqlite` 1.62.0 → 1.63.0,
`libc` 1.82.0 → 1.82.13 and `compress` 1.20.4 → 1.20.5 (`cc/v4` → v4.35.0 and `ccgo/v4` → v4.39.3
following libc), changed only those three `go.mod` lines, and left the ten rows above `[newer]`; the
bare `go get -u ./...` missed the `go.mod`-required `compress`, and an explicit `go get -u
<m>@latest` for all ten graph-only rows ended with tidy keeping five of the added indirect
`require`s (`gc/v3`, `pprof`, `testify`, `x/image`, `x/tools`, at the proxy's newest) and dropping
the other five; the kept rows' new requirements pull seven new bracketed rows into the listing
(`go-cmp`, `objx`, `goldmark`, `go.yaml.in/yaml/v3`, `x/exp`, `x/telemetry`, `demangle`). The recipe
below is therefore `go get -u all`; the graph-only rows are recorded per Q-0023-05, never as
"already newest".

- MyGo's newest release is the one the tree pins: the proxy's version list ends at `v0.10.0`
  (`go list -m -versions github.com/egoist/mygo`), `gh api repos/egoist/mygo/releases/latest` =
  `v0.10.0` (2026-11-17, not a pre-release) and the tag list's top is `v0.10.0`. Its release notes
  name behavior changes (triple-click paragraph selection, popups fit in the window, Windows
  touchscreen scroll, Linux tray identity, `OnDOMReady` on back/forward cache, phone/tablet support)
  — none touches the hidden-title-bar contract, and no move is needed, so the goal's MyGo
  precondition (0022's checks) does not fire.
- `go.mod` holds `go 1.27.4` and a `tool github.com/egoist/mygo/cmd/mygo` line; the three workflows
  that need Go use `go-version-file: go.mod` (gate.yml, frontend-bench.yml ×2, keychain.yml).

### 3. Toolchains: the official listings, and what pins name them

- `mise ls-remote go | tail -4` → `1.27.1 1.27.2 1.27.3 1.27.4` — newest 1.27.4, the pinned one.
- `mise ls-remote bun | tail -4` → `1.4.2 1.4.3 1.4.4 1.4.5` — newest 1.4.5, the pinned one.
- `mise ls-remote node | grep -E '^26\.' | tail -6` → `… 26.12.1 26.13.0 26.13.1` — newest 26 LTS
  patch 26.13.1, the pinned one (Q-0013-03, taken by goal 0020 P4). The installed toolchain matches:
  `go1.27.4`, bun `1.4.5`, node `v26.13.1` (local runs above).
- The pins: `mise.toml` (`go = "1.27.4"`, `node = "26.13.1"`, `bun = "1.4.5"`); `go.mod`'s `go`
  directive plus the workflows' `go-version-file: go.mod`; `bun-version: 1.4.5` at six sites in
  `cask.yml` (×2), `frontend-bench.yml` (×2), `gate.yml`, `keychain.yml`; `node-version: "26.13.1"`
  at `gate.yml:54`; `harness/lib/package.ts:66` `NODE_VERSION = "26.13.1"` with the four archives'
  SHA-256s (`NODE_PINS`, `:135-168`) and the five-source agreement check
  (`NODE_PIN_SOURCES`/`nodePinProblems`, `:181-212`, run at the package gate's start) covering
  `mise.toml`, `gate.yml` and the three pinned sentences (`docs/install.md:7`,
  `docs/install.zh-CN.md:7`, `harness/lib/package.ts`'s header). All of them agree today, so the
  toolchain set needs no move — only the inventory row and its listing as the reason.

### 4. GitHub Actions: the newest major of every action, and whether it still targets Node 20

Every `uses:` in `.github/workflows/*.yml` (`grep -rn "uses:"`), with `releases/latest` (gh api,
2026-11-26) and the `runs.using` read from each tag's `action.yml`:

| Action | Pinned | Newest major | `runs.using` now → newest |
|---|---|---|---|
| `actions/checkout` | `@v4` (10 sites) | `v7.0.1` (2026-07-20) | `node20` → `node24` |
| `actions/setup-go` | `@v5` (4 sites) | `v7.1.0` (2026-11-17) | `node20` → `node24` |
| `actions/setup-node` | `@v4` (1 site) | `v7.1.0` (2026-10-08) | `node20` → `node24` |
| `actions/upload-artifact` | `@v4` (5 sites) | `v7.0.2` (2026-10-07) | `node20` → `node24` |
| `actions/download-artifact` | `@v4` (1 site) | `v8.0.2` (2026-10-07) | `node20` → `node24` |
| `oven-sh/setup-bun` | `@v2` (6 sites) | `v2.2.0` (2026-03-14) | `node24` → `node24` |

That is 21 pinned lines to change in five files (`gate.yml`, `cask.yml`, `frontend-bench.yml`,
`keychain.yml`, `release.yml`); `setup-bun@v2` already floats to the newest release and stays.
Breaking notes read at the newest majors: `checkout` v5+/v6/v7 block fork-PR checkout under
`pull_request_target`/`workflow_run` (this repo's triggers are `pull_request`, `push` on `main` and
tags, `release: published`, `workflow_dispatch`, `workflow_call` — none affected); `upload-artifact`
v6+ and `download-artifact` v7+ need runner ≥ 2.327.1 (GitHub-hosted); `upload-artifact` v7's
`archive: false` and `download-artifact` v8's `skip-decompress` are unused; `download-artifact` v8
defaults a digest mismatch to `error` (release.yml downloads this repo's own gate artifacts), which
is the secure default the repo keeps.

### 5. What the gates can see, and what costs what

- CI (`gate.yml`) runs all four groups — core, desktop, package, self-test — on ubuntu-latest,
  macos-latest and windows-latest for every pull request, so every step below gets three-OS evidence
  from its own PR run. `core` is everything except desktop-smoke, desktop-ux, package-smoke and
  self-test (`harness/gates/index.ts`'s `gateGroups`); agent-smoke is in `core`.
- The package gate builds, installs and drives this run's package per OS; its artifacts
  (`package-<os>`) are where a size pair is read from, never from a build log line.
- The frontend bench (`frontend-bench.yml`) runs on windows-latest and macos-26 when a PR touches
  its path filter — which includes `agent/**`, `go.mod`, `go.sum`, `mise.toml`, `harness/lib/**` and
  the workflow file itself. P1 (go.mod/go.sum), P2 (its own workflow) and P4 (agent/**) are
  therefore bench-triggering steps: seven shots must stay byte-identical and the run must read no
  worse beyond noise. P3 and P5 touch only `harness/gates/agent-smoke.ts` and docs — no bench.
- `agent-smoke` drives the real Pi Durable host against its own loopback SSE stub in four passes —
  source host and staged bundle, each over a chat-completions and a Responses endpoint — and the
  stub already records every provider request body: `received.push({ path: new URL(request.url)
  .pathname, body })` (`harness/gates/agent-smoke.ts:107`), for 11 scenarios per pass. Nothing
  writes those bodies out today; P3 adds exactly that, opt-in.
- No gate reads any of the four version sets (the only pin-reading check is the Node agreement check
  at the package gate's start). The inventory evidence is therefore command output and CI run logs,
  named per row — never a claim.

### 6. What the package build proves about the lock

Staging bundles `durable-host.mjs` from the locked tree and copies the other three hosts
byte-identically (`agent/README.md`, *Boundary*); every package a bundle inlined is read back from
bun's metafile and must be one `bun.lock` pins (`bundlePackages`/`lockProblems`,
`harness/lib/package.ts:458-468`, `:619`), and the licence texts of the inlined packages are
assembled into the staged `agent/THIRD-PARTY-NOTICES.txt` (`notices.ts`). The staged tree lives
under the gitignored `build/package`; nothing about it is committed. So after P4 the staged bundle's
bytes, its inlined-package count (both on the pass line) and the notices file are read from the
gate's own output, and a package that inlined something the lock does not pin fails rather than
ships.

### 7. What must not move, and the rules that bound the steps

- No registry switch, no downgrade, no vendoring (owner, 2026-11-26; Q-0021-07); `bun.lock` keeps
  `""` registry URLs; npm goes only through the feed; Go only through `GOPROXY`.
- No telemetry: `pi-telemetry` stays a transitive entry and is never enabled.
- No change to scoring, consent, the database schema, goldens, host request/response documents or
  the internal-only source rules; `data/private`, `data/local` and credentials never enter a package.
- The goal runs alone with respect to the package build and the lockfile: no other goal may change
  `go.mod`/`go.sum`, `agent/package.json`, `bun.lock` or the package build on this tree while a step
  of this plan is in flight.
- Nothing only the owner can supply is missing at scout time: the one external input is the feed's
  content, and Q-0023-03 discloses what happens while it lags (blocked item, no change, re-runnable).

## Defaults (append as Q-0023-01…05; the coordinator appends the entries before P1's branch is cut)

1. **Q-0023-01 — How a Go module bump lands, and when one is held back.** Each step's implementer
   re-reads `go list -m -u all` through the configured proxy at implementation time and takes every
   newer release it serves **for the modules `go.mod` requires** with `go get -u all` + `go mod tidy`
   (the bare `./...` pattern misses a requirement the main module's packages do not import — on the
   scout tree `klauspost/compress`; `all` reaches it, and the fact-2 probe reads the moved set back),
   quoting old → new per module in the evidence record. Rows with a `[newer]` marker that no
   `require` line names are **graph-only**: the module graph selects their version, `go get -u all`
   leaves them as probed, and a hand-added require is kept or dropped by the next tidy row by row —
   five of the ten each way in fact 2's probe — so they are recorded per Q-0023-05 with their
   reason, never moved by hand and never called "already newest".
   No major module path changes are expected today (the list above is all minor/patch/pseudo), and
   MyGo is already at its newest tag. A module whose new release cannot keep the store's pinned
   facts green (busy timeout, `_txlock=immediate`, one connection per handle, `IsBusy`'s error-code
   mapping) is held at its current version with the failing check named in this entry's outcome line
   — adapt-with-tests comes first, and a hold-back is the disclosed fallback.
   Reverse: revert `go.mod`/`go.sum` to the previous versions.
2. **Q-0023-02 — How the actions are pinned.** The **newest major tag** of every action (the existing
   style: `@v7`, `@v8`), not an exact patch, so a patch is picked up without a repo change; `@v2` for
   `setup-bun` stays and already resolves to the newest release. The bump is proven by the three-OS
   gate run itself: every job green and zero `Node.js 20 is deprecated` lines in its logs (the
   runner's real warning text; the same grep reads 12 on `main`'s gate run before the step).
   Reversal: restore the old major tags.
3. **Q-0023-03 — The Pi packages, and the blocked rule.** Before any file changes, the run checks the
   feed for all six `@earendil-works` entries; only if every one serves ≥ 1.6.0 do the four direct
   pins move together to the newest served version and the lock re-resolve from the feed (a mixed
   set is reported blocked, never shipped). While the feed serves < 1.6.0 for any entry — today's
   state, with the 404 quoted above — P4 changes no package file: its diff is the blocked section of
   `docs/evidence/0023-current-dependencies.md` (the six listing lines and the 404) plus this
   entry's outcome line, *blocked by the feed*; the check stays re-runnable, and the owner action
   named by Q-0021-07 (ask the feed owner to publish 1.6.x) stands. Hand-building or vendoring is
   declined (owner, 2026-11-26).
   Reverse: revert `agent/package.json` and `bun.lock`.
4. **Q-0023-04 — How the system-role change is shown.** The durable host's provider request bodies
   are recorded before and after the bump, for both endpoint formats, by running `agent-smoke` with
   `PASSQUEST_AGENT_SMOKE_DUMP=<dir>` (added by P3, harness-only); the two dumps and their diff go
   into the evidence record. This is a recorded comparison, not an assertion about upstream: if the
   bodies are unchanged the record says so; if they move, the diff is quoted and any adaptation the
   Go side or a host needs lands with the step, with a test that fails when the adaptation is
   reverted. Reverse: remove the env-gated dump; the default gate behaviour was never changed.
5. **Q-0023-05 — What a dependency left in place must carry.** A dependency left where it is
   appears in the inventory with the listing that proves it (the command and its output, dated) and
   its one reason, exactly like a moved one carries old → new and the listing: *already newest*,
   *held back* with the failing check, or *graph-only* — a `[newer]` row no `require` line names,
   carrying its `go list -m -u all` line and its `go mod why -m <m>` output (a chain, or `(main
   module does not need module <m>)` for the four rows that print it), never the words "already
   newest" (Q-0023-01). The *already newest* set today is: the Go toolchain (1.27.4), Bun (1.4.5),
   Node (26.13.1), `@types/bun` (1.4.4), `typescript` (7.0.2), MyGo (v0.10.0), `purego` and `x/sys`
   (the proxy serves nothing newer), and `setup-bun@v2` (v2.2.0, node24). Reverse: none needed —
   this entry only fixes the form of the evidence.

## Steps (one implementer run each; each leaves every gate green; one pull request each)

| # | Step | Proof |
|---|---|---|
| P1 | **Go modules to their newest served releases** — `go.mod`/`go.sum`: `modernc.org/sqlite` 1.62.0 → 1.63.0, `modernc.org/libc` 1.82.0 → 1.82.13, `github.com/klauspost/compress` 1.20.4 → 1.20.5 (every `go.mod` requirement with a newer served release; `cc/v4`/`ccgo/v4` follow the graph); MyGo, `purego`, `x/sys` stay (already newest); the ten graph-only rows are recorded with their reason (Q-0023-05) | `go list -m -u all` before/after with the moved set quoted and the graph-only rows' `go mod why` reasons; `gofmt`/`vet`/`build` clean; `go test ./internal/... -count=1`; `go-tidy` green; `local-only`, `golden-contract` green; three-OS core/desktop/package run; per-OS size pair; bench seven shots byte-identical |
| P2 | **GitHub Actions to their newest majors** — 21 `uses:` lines in five workflows: `checkout@v7`, `setup-go@v7`, `setup-node@v7`, `upload-artifact@v7`, `download-artifact@v8`; `setup-bun@v2` unchanged | the PR's three-OS run: every job green and `gh run view <id> --log \| grep -c "Node.js 20 is deprecated"` = 0 (before: 12 on `main` run 45784987207); no old-major `uses:` left (the file grep reads 0); `runs.using: node24` read back from each pinned tag's `action.yml`; bench seven shots byte-identical |
| P3 | **The request-body recorder** — `PASSQUEST_AGENT_SMOKE_DUMP=<dir>` in `harness/gates/agent-smoke.ts` writes each pass's recorded bodies as JSON; unset, the gate is exactly as today; set, a dump that cannot be written fails the gate, and one must-fail self-test case proves it | local Windows run with the variable set: both formats' files exist and parse (the chat pass carries a `role: "system"` message); a run without it leaves no file and the pass line unchanged; the self-test case reddens with the dump failure named; three-OS core run |
| P4 | **The Pi packages (feed-gated, last)** — check the feed for all six entries first; if ≥ 1.6.0 for all: four `agent/package.json` pins together + `bun.lock` re-resolved from the feed; if not: no package file changes — the record's blocked section and Q-0023-03's outcome line are the diff | blocked path: the record's six `npm view` lines and the 404; bump path: the `bun.lock` diff (all six entries, `""` URLs, no downgrade), before/after request-body dumps and their per-format diff, `agent-smoke` and the package smoke on three OSes, bundle bytes/inlined-package count from the pass line, per-OS size pair, bench seven shots |
| P5 | **Evidence record and decisions** — `docs/evidence/0023-current-dependencies.md` (the four-set inventory table, every row's source; extends P4's blocked section where the feed lagged), Q-0023 outcome lines, `docs/harness.md` rows, `Q-0006-02`/`Q-0021-07` dated updates | `docs-links`, `install-guide`, `secrets`, `typecheck` green; every number cross-checked against the named run id or command output |

Each step is one pull request cut from `main` after the previous step merged. Each step updates
**only** its own entries' outcome lines (P1 -01, P2 -02, P4 -03/-04, P5 -05), so no ID is ever
written twice. P4 is the last dependency-moving step (P5 closes the record) and the only step whose
content is external (the feed); its blocked path still carries a diff — the record's blocked section
and Q-0023-03's outcome line.

### P1 — Go modules to their newest served releases (fact 2; Done-when 3)

1. Re-run `mise exec -- go list -m -u all` and record the before listing; run
   `mise exec -- go get -u all` then `mise exec -- go mod tidy`; record the after listing. The
   modules that move — on the scout tree `modernc.org/sqlite` 1.62.0 → 1.63.0, `modernc.org/libc`
   1.82.0 → 1.82.13 and `github.com/klauspost/compress` 1.20.4 → 1.20.5, with `modernc.org/cc/v4`
   and `modernc.org/ccgo/v4` following libc's requirement — are quoted old → new in the evidence
   record with the proxy as their source. The rows still `[newer]` afterwards are graph-only: no
   `require` line names them (fact 2), so they carry their `go mod why -m <m>` output in the record —
   a chain, or `(main module does not need module <m>)` for `simd`, `go-aac`, `oggvorbis` and
   `mewkiz/pkg` — and are not hand-added to `go.mod` (Q-0023-05).
2. Watch the driver: `modernc.org/sqlite` 1.62.0 → 1.63.0 and `modernc.org/libc` 1.82.0 → 1.82.13
   rebuild the pure-Go SQLite path under `internal/store`. The store's pinned facts must read
   unchanged after the bump — `busy_timeout(5000)`, `_txlock=immediate`, `SetMaxOpenConns(1)`, no
   `journal_mode` write, and `IsBusy`'s primary-code 5/6 mapping (goal 0020 P2) — and goal 0020 P1's
   per-user default, notice and CLI tests must stay green. If a new release cannot, per Q-0023-01 the
   module is held back with the failing check named, never worked around.
3. Re-run `go vet ./...`, `go build ./...`, `go test ./internal/... -count=1`,
   `gofmt -l internal cmd` (empty), and the single gates `go-tidy`, `local-only` (the bumped
   `libc`/`sqlite` must add no network import to any first-party package) and `golden-contract` (no
   golden changes).
4. The three-OS run is the authority for the smokes and the installed package: this step changes the
   binary, so the `package` group's install-and-drive passes count on each OS. Read the per-OS
   `package-<os>` artifact sizes before (the last `main` gate run's artifacts, run id named) and
   after (this PR's run) and record the pair — the number is recorded, not judged.
5. The bench is triggered by the `go.mod`/`go.sum` path filter; its seven shots must be
   byte-identical on windows-latest and macos-26 and read no worse beyond noise.

*Fails when removed:* the moved set's versions are the observable — `mise exec -- go list -m -f
'{{.Path}} {{.Version}}' modernc.org/sqlite modernc.org/libc github.com/klauspost/compress` prints
1.62.0/1.82.0/1.20.4 again with the bump reverted, and `go list -m -u all` reprints their `[newer]`
markers (the ten graph-only rows keep theirs either way — which is why the record names them).

### P2 — GitHub Actions to their newest majors (fact 4; Done-when 3)

1. Before editing, re-read for each action's newest major its `action.yml` (`runs.using`) and its
   release notes' breaking section, and quote them in the PR: the fork-PR block (`checkout` v5+), the
   runner floor 2.327.1 (`upload-artifact` v6+, `download-artifact` v7+), and `download-artifact`
   v8's digest-mismatch default to `error`. None of the repo's triggers or inputs is affected;
   `release.yml`'s `pattern`/`path`/`merge-multiple` inputs are confirmed against the v8 `action.yml`
   before the edit, because that workflow only runs on a tag push.
2. Replace the 21 pinned lines: `actions/checkout@v4` → `@v7` (10), `actions/setup-go@v5` → `@v7`
   (4), `actions/setup-node@v4` → `@v7` (1), `actions/upload-artifact@v4` → `@v7` (5),
   `actions/download-artifact@v4` → `@v8` (1). `oven-sh/setup-bun@v2` is untouched.
3. Proof on the PR: the three-OS gate run — every group green on ubuntu-latest, macos-latest and
   windows-latest — and `gh run view <id> --log | grep -c "Node.js 20 is deprecated"` = 0. That
   string is the runner's own text, read not assumed: `main`'s gate run 45784987207
   (2026-11-26) reads 12, its line naming `actions/checkout@v4`, `actions/setup-go@v5` and
   `actions/setup-node@v4`, while a log whose jobs run the new majors reads 0 (x-motemen/ghq run
   45145321152, 2026-11-23 — `grep -c "Node.js 20"` = 0 across its jobs). The workflow files carry
   the same check mechanically: `grep -rnE "uses: +actions/[a-z-]+@v[4-6]" .github/workflows` reads
   the 21 old lines before the edit and must read 0 after (final refs: `checkout@v7`, `setup-go@v7`,
   `setup-node@v7`, `upload-artifact@v7`, `download-artifact@v8`). The workflow job also uploads the
   package artifacts as usual; no package bytes change, so no size pair is required, and the
   `package` group still proves the package builds and installs under the new actions.
4. The bench is triggered because `frontend-bench.yml` is one of the changed files; seven shots
   byte-identical on windows-latest and macos-26.

*Fails when removed:* the file grep and the deprecation grep — a run with any old major still pinned
reports the warning naming that `uses:` line, and the file grep (`uses: +actions/[a-z-]+@v[4-6]`)
reads the 21 old lines again; the log line is the evidence, read from the run, not from a local
claim.

### P3 — The durable host's request bodies become recordable (fact 5; the instrument for Done-when 2)

1. `harness/gates/agent-smoke.ts`: when `PASSQUEST_AGENT_SMOKE_DUMP` names a directory, the gate
   checks it once before the passes run and fails, naming the variable and the path, when it is
   missing or unwritable; each pass then writes `<dir>/<label>.json` = `{ label, format, requests }`
   from the stub's own `received` array, and a write that still fails at pass time is a named
   failure too. Unset, nothing is written and every assertion count, failure message and the pass
   line stay byte-identical to today.
2. No product file changes; `docs/harness.md` row 17 gains one sentence naming the opt-in variable.
   One must-fail case is added to `agent-smoke.cases.ts` for `brokenCases()`: its `prepare` sets the
   variable to a path that cannot exist (under an existing regular file in the fixture) and the gate
   must fail with the dump failure named. The restore is code this step adds: `runCase`'s `finally`
   today only removes its fixture root (`rmSync(root, { recursive: true, force: true })`,
   `self-test.ts:403-405`) and the file's one env save/restore is `processTreeProblem`'s own TZ
   block (`self-test.ts:111-123`) — so P3 saves `PASSQUEST_AGENT_SMOKE_DUMP` at `runCase`'s start
   and restores it in that `finally` (deleting it when it was unset), leaving no dump configured
   for the cases after it; `harness/gates/self-test.ts` is therefore a changed file of this step.
   Row 27's case count moves by one, so P5 re-quotes that row.
3. Proofs, local Windows first: `agent-smoke` once with the variable pointed at
   `Q:/scratch/omp/g23/P3/` (created first) — both formats' files exist, each parses, the chat pass
   contains a `role: "system"` message and the responses pass's `input` items — and once without it —
   no file appears and the pass line is the same as before the step. The must-fail case is proven by
   the self-test gate's own run (once locally, then its three-OS job); the three-OS core run proves
   the default path unchanged.

*Fails when removed:* with the variable set, the missing dump is a failure — reverted code writes
no file and the gate's own check reddens; without the variable nothing changes either way, which is
the point of the opt-in.

### P4 — The Pi packages (feed-gated; last) (facts 1, 5, 6; Done-when 2)

1. **The feed check first.** `npm view <pkg> version` through the feed for `chord`, `pi-ai`,
   `pi-durable`, `pi-schema`, `pi-telemetry`, `pi-codemode`; require ≥ 1.6.0 for all six. If any is
   below, or the six disagree: touch no package file, and put the blocked evidence where the record
   can quote it — a dated *blocked* section in a new `docs/evidence/0023-current-dependencies.md`
   carrying the six listing lines and the `@earendil-works/pi-ai@1.6.0` 404, plus Q-0023-03's
   outcome line (*blocked by the feed*) and the owner action (ask the feed owner to publish 1.6.x).
   That docs diff is the branch's pull request; the step stays re-runnable by re-cutting its branch
   when the feed catches up. Do not hand-build or vendor.
2. **The bump.** With all six served at ≥ 1.6.0: set all four direct pins in `agent/package.json` to
   the same newest served version and run `mise exec -- bun install` at the root (the configured
   feed; never another registry). Read `bun.lock` back: the six `@earendil-works` entries at the new
   version, registry URLs still `""`, and no other locked package moved down — the old → new diff for
   the six is quoted in the evidence record, and if the re-resolve adds or moves `undici` or
   `undici-types` rows, their diff too (today's lock has no `undici` entry — only `undici-types` and
   an unresolved optional peer on `openai@7.19.0`). Read the
   served packages' own `engines` field from the installed `node_modules` (the artifact, not a
   guess): it must accept the bundled Node 26.13.1.
3. **The request-body comparison.** Run `agent-smoke` with `PASSQUEST_AGENT_SMOKE_DUMP` to a
   `before/` directory on the pre-bump tree (P3 landed) and to an `after/` directory on the bumped
   tree; diff the two per format and quote the system-role entries of the chat pass (the format
   1.6.0 changes for non-OpenAI providers) and the responses pass in the evidence record. If the
   diff shows the Go side or a host must adapt (the goal's rule: a released change is adapted or
   recorded), adapt in this step with a test that fails when the adaptation is reverted.
4. **The package proof.** The staged bundle is rebuilt from the new lock; the pass line gives the
   bundle's bytes and inlined-package count, `lockProblems` proves each inlined package is one the
   lock pins, and the staged `agent/THIRD-PARTY-NOTICES.txt` texts — including any `undici` or
   `undici-types` the new lock resolves — are read from the staged tree. `agent-smoke` (11 scenarios
   × 4 passes) and the `package`
   group run on all three OSes; the per-OS size pair is read from the `package-<os>` artifacts
   before (last `main` run) and after (this run); the bench (agent/** path filter) reads no
   regression with byte-identical shots.
5. **Decisions.** Q-0006-02 gains a dated *Updated* line with the new pins; Q-0021-07 gains its
   dated *Revisited* line only when its own "Revisit when" (the feed serving ≥ 1.6.0 for all six)
   fires.

*Fails when removed:* on the bump path, the lock and the staged bundle — `bun.lock`'s six entries
read back at 1.5.2 with the bump reverted, and the after-dump diff loses its "after" side; on the
blocked path, the record's six listings and 404, absent without P4.

### P5 — Evidence record, decisions, and the close (Done-when 1, 4, 5 and the record of 2, 3)

1. `docs/evidence/0023-current-dependencies.md`: extends P4's blocked section where the feed lagged
   (creates the file otherwise) with one inventory table whose set column distinguishes the four sets
   (Go modules / npm packages / toolchains / GitHub Actions), each row old version, new version,
   where the new version was read from (the feed listing, the proxy's listing, the toolchain's
   `mise ls-remote`, the action's releases), and the upstream notes that matter to this app; rows
   left in place carry their listing, date and reason (Q-0023-05 — "already newest", "graph-only" or
   "held back"). Plus: the action-log greps, the two request-body dumps and their diff (on the
   blocked path the record states their absence), the bundle bytes/inlined count, the per-OS size
   pairs, the bench records, and the known limits (below).
2. Q-0023-05's outcome with the audit's final form; `docs/harness.md` rows 17 (P3's sentence) and any
   row whose counts a step's pass line moved — each re-quoted from its own run, never predicted.
3. Coordinator's close: `docs/goals/README.md`'s state column and the archive move, in the close's
   own commit (conventions, *Working rules*). No product file changes here.

*Fails when removed:* `docs-links` and `install-guide` name a broken or drifted sentence; the
record's numbers are cross-checked against the named runs by the coordinator.

## Each change and the check that fails when it is removed

| Change | The check that reddens without it |
|---|---|
| the Go module versions | `go list -m -f '{{.Path}} {{.Version}}'` over the moved set prints the old versions; `go list -m -u all` reprints the moved set's `[newer]` markers (the graph-only rows keep theirs either way) (P1) |
| the action majors | `grep -rnE "uses: +actions/[a-z-]+@v[4-6]" .github/workflows` reads the 21 old lines again, and the run's log grep reports `Node.js 20 is deprecated`, naming the old `uses:` line (P2) |
| the opt-in dump | the dump-enabled `agent-smoke` run leaves no file and the gate's own dump check fails, and the must-fail case reports that failure (P3) |
| the Pi pins and the lock | `bun.lock`'s six entries read 1.5.2 again and the package gate's `lockProblems`/bundle count follows the old bundle (P4; the blocked path's check is the record's six listings) |
| the recorded comparison | the evidence record's diff no longer pairs a before and an after dump, which is Done-when 2's artifact (P4/P5) |

## Done-when mapping

| Contract item | Steps | Evidence |
|---|---|---|
| 1 the inventory says what moved | P5 (rows fed by P1, P2, P4) | the one table with per-row source and notes; the listings quoted in this plan and re-read in the run |
| 2 the Pi packages are current | P4 (blocked rule) | the six feed listings and the 404 (or the lock diff at ≥1.6.0); on the bump path only, the before/after request-body dumps for both formats and the item-2 agent/package smokes on three OSes |
| 3 Go modules, npm, toolchains, actions current; warning gone | P1, P2, P4, P5 | `go list -m -u all` before/after (moved set and graph-only reasons); the feed's `npm outdated` (no rows); the toolchain listings; the action table; a workflow log with zero `Node.js 20 is deprecated` lines; the full gate on three OSes |
| 4 behaviour unchanged | P1, P2, P4, P5 | unchanged goldens, host protocol, schema and CLI output (the three-OS gate); the bench's seven shots; the per-OS size pairs; the request-body diff as the one listed host change |
| 5 the pins agree | P3 (row edit), P5 | the package gate's start-of-run agreement check over `mise.toml`, `gate.yml` and the three pinned sentences — green on every run; no new pin source appears (the actions keep the existing version-in-workflow style) |

## Risks (each with how it is detected)

- **The SQLite driver bump changes a behaviour the store relies on** (busy timeout, `_txlock`,
  error codes, a new default pragma) → `internal/store`'s DSN and interleaving tests, the goal-0020
  held-lock case and the import/tactic/adaptive smokes red on the step's own run; detection is those
  failing tests named in the output, and per Q-0023-01 the fallback is a held-back module with the
  failing check recorded, never a workaround.
- **An action major breaks a job for a runner-specific reason** (a runner floor, an input rename, an
  ESM/`action.yml` change) → the step's own three-OS run fails in the named job/step, not locally;
  the mitigation is reading the newest major's `action.yml` and breaking notes before editing, and
  `release.yml`'s inputs are read from the v8 files rather than assumed.
- **The deprecation warning survives because a `uses:` site was missed** → the inventory is
  `grep -rn "uses:" .github/workflows`, re-run in the step, and the file grep
  (`uses: +actions/[a-z-]+@v[4-6]`) must read 0; a missed site keeps a line in the file grep, and
  the run's log grep (`Node.js 20 is deprecated`) counts > 0 and names the surviving old major.
- **The feed serves a mixed Pi set** (some 1.6.x, some 1.5.2) → the six `npm view` lines disagree;
  per Q-0023-03 nothing lands and the item is reported blocked, never a mixed lock.
- **A Pi bump changes request bodies the Go validator or a host rejects** → agent-smoke's per-format
  assertions and tutor-smoke red on three OSes; the before/after dump diff names the field that
  moved; the fix is an adaptation with a test that fails when reverted.
- **A Pi 1.6.x release wants a newer runtime than 26.13.1** → read from the installed package's
  `engines` after `bun install` (the artifact); if it did, the pin bump would need its own step and
  an owner-visible statement — stop and report rather than guess.
- **The lock re-resolve rewrites registry URLs or downgrades a package** → the `bun.lock` diff is
  read back entry by entry; a `""` URL replaced by a host, or a lower version anywhere, stops the
  step and the install is re-run from the feed (never edited by hand).
- **The bench or a window gate is noisy on this machine (the brief's 100 % CPU fact)** → the bench's
  same-build re-measure and CI's run are the authority; local window runs are advisory, and the
  report says which evidence came from CI.
- **Another goal edits the package build or the lockfile on this tree concurrently** → duplicate or
  conflicting diffs in `go.mod`/`go.sum`/`bun.lock`; the goal's own rule keeps the plan alone, and
  the coordinator orders merges.

## Evidence plan

Local (Windows): the version listings quoted above, re-read at each step's start;
`mise exec -- go list -m -u all` before/after (P1); `gofmt`/`go vet`/`go build`/`go test
./internal/... -count=1` (P1); the single gates `go-tidy`, `local-only`, `golden-contract` (P1),
`typecheck`, `docs-links`, `secrets` (P3, P5) through
`mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts <gate-file> <exportName>`; `agent-smoke` with
and without `PASSQUEST_AGENT_SMOKE_DUMP` (P3, and P4's before/after dumps on the bump path). The
`package` group runs
once per package-changing step (P1, P4), locally only if the step's row demands it — the brief keeps
the local package run optional and CI is the authority.

CI (the authority): each step's `gate.yml` matrix (core/desktop/package/self-test on ubuntu-latest,
macos-latest, windows-latest) and the `frontend-bench.yml` pair (windows-latest, macos-26) for the
bench-triggering steps. The numbers in `docs/evidence/0023-current-dependencies.md` are read from the
`package-<os>` artifacts, the run logs and the run ids, never from a claim. Runner-by-runner: this
machine proves the Go build/test path, the listings, and the dump runs; macOS and Linux window,
package and bench evidence is CI-only, and the record says so. Known limits: `release.yml` runs only
on a tag push, so its action inputs are proven by reading the pinned `action.yml` now and by the next
release's run.

## Facts each step hands to the next

- **P1 → P4, P5:** the moved module set and the proxy listing (so the inventory rows are read, not
  remembered); whether the driver bump needed any adaptation; the size pair's run ids.
- **P2 → P5:** the run id with a zero deprecation count and the previous `main` run's non-zero one.
- **P3 → P4, P5:** the dump directory layout and one sample entry per format (the before side of
  Done-when 2).
- **P4 → P5:** the feed state at its re-check (served versions, or the blocked listing and its
  record section), the lock diff, the after dump, the bundle's bytes/inlined count, the notices
  texts, the size pair.
- **P5 → the close:** every artifact path and run id, the inventory's final rows, the known limits.

## Concurrency and files other goals must not touch at the same time

The goal's own constraint: it does not run beside another goal that changes the package build or the
lockfile on one tree. Its steps own `go.mod`, `go.sum`, `agent/package.json`, `bun.lock`,
`harness/gates/agent-smoke.ts`, `harness/gates/self-test.ts` and the self-test case files, the five
workflow files and the documents above; a parallel goal touching any of them waits for the merge.

## Coordinator actions needed

- **Append Q-0023-01…05 to `docs/decisions/OPEN-QUESTIONS.md` before P1's branch is cut** (the
  goal's disclosure rule, the order goals 0020 P1 and 0022 used), and nothing else about them: each
  step updates only its own entries' outcome lines (P1 -01, P2 -02, P4 -03/-04, P5 -05).
- **Open one draft pull request per step** (branches `goal/0023-p1`…`goal/0023-p5`, each cut from
  `main` after the previous merge) so the three-OS gate — and, where the path filter fires, the
  bench — runs on every increment; the run ids it produces are what the evidence record names.
- **Nothing else**: no secrets, no release, no remote write, no tap change. The Pi step's blocked
  path needs no external action — its listing and 404 land in the evidence record (P4's own diff),
  and the owner action it names (ask the feed owner to publish 1.6.x) is the only outside
  dependency; the feed is configured on this machine only (user-level npm config; the repo carries
  no `.npmrc`/`bunfig.toml` and the workflows set no registry) and CI installs from `bun.lock`
  (`bun install --frozen-lockfile` at each install site), so CI needs no feed configuration.
