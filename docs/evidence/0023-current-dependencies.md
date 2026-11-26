# 0023 — Every dependency is on its newest served release: evidence

## P1 (2026-11-26) — Go modules to their newest served releases (branch `goal/0023-p1`)

Contract: [`../goals/0023-current-dependencies.md`](../goals/0023-current-dependencies.md).
Plan: [`../plans/2026-11-26-1400-0023-current-dependencies.md`](../plans/2026-11-26-1400-0023-current-dependencies.md).
Defaults: Q-0023-01 and Q-0023-05 in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).
Base: this branch is stacked on `goal/0023-p0` (plan commit `01c333c`); `main` is `943de9d`.
Every number below is read from the command quoted beside it, run on this machine on 2026-11-26.

**What changed.** `go.mod`, exactly three `require` lines: `modernc.org/sqlite v1.62.0 → v1.63.0`
(direct, the pure-Go SQLite driver), `modernc.org/libc v1.82.0 → v1.82.13` and
`github.com/klauspost/compress v1.20.4 → v1.20.5` (both `// indirect`). `go.sum` follows for those
three plus `modernc.org/cc/v4 v4.34.1 → v4.35.0` and `modernc.org/ccgo/v4 v4.39.2 → v4.39.3`, which
no `require` line names and which `libc`'s own new requirement selects. Nothing else in the module
files moved: the `go 1.27.4` directive, the `tool github.com/egoist/mygo/cmd/mygo` line and the
other 15 `require` lines are byte-identical (the full `git diff` is those three lines of `go.mod`
and five module pairs of `go.sum`).

**The selection, from the configured proxy** (`GOPROXY=https://proxy.golang.org,direct`).
`mise exec -- go list -m -u all` at `01c333c` printed `modernc.org/sqlite v1.62.0 [v1.63.0]`,
`modernc.org/libc v1.82.0 [v1.82.13]`, `github.com/klauspost/compress v1.20.4 [v1.20.5]`,
`modernc.org/cc/v4 v4.34.1 [v4.35.0]`, `modernc.org/ccgo/v4 v4.39.2 [v4.39.3]` and the ten
graph-only rows below. Then, on this branch:

```
$ mise exec -- go get -u all
go: upgraded github.com/klauspost/compress v1.20.4 => v1.20.5
go: upgraded modernc.org/libc v1.82.0 => v1.82.13
go: upgraded modernc.org/sqlite v1.62.0 => v1.63.0
$ mise exec -- go mod tidy        # no output, exit 0
$ mise exec -- go list -m -u all  # the moved set now reads:
modernc.org/sqlite v1.63.0
modernc.org/libc v1.82.13
github.com/klauspost/compress v1.20.5
modernc.org/cc/v4 v4.35.0
modernc.org/ccgo/v4 v4.39.3
```

`mise exec -- go list -m -f '{{.Path}} {{.Version}}' modernc.org/sqlite modernc.org/libc
github.com/klauspost/compress` on the bumped tree prints `modernc.org/sqlite v1.63.0`,
`modernc.org/libc v1.82.13`, `github.com/klauspost/compress v1.20.5`.

**Already newest, no marker before or after** (`go list -m -u all` prints version only):
`github.com/egoist/mygo v0.10.0` (MyGo's newest tag, its own release notes name no hidden-title-bar
change — the goal's MyGo precondition does not fire), `github.com/ebitengine/purego v0.11.3`,
`golang.org/x/sys v0.50.0`.

**The ten graph-only rows (Q-0023-05).** No `require` line names any of them; `go get -u all`
leaves their graph-selected versions where they were and the plan forbids hand-adding a require.
They keep their `[newer]` markers after the bump; each row's `mise exec -- go mod why -m <m>`
output is its reason:

| Module (after listing) | `go mod why -m` (chains shown as ` → `) |
|---|---|
| `github.com/google/pprof v0.0.0-20261016150608-1facf8f91b50 [v0.0.0-20261118010117-084b33a85f63]` | `…/internal/store → modernc.org/sqlite → modernc.org/sqlite.test → github.com/google/pprof/profile` |
| `github.com/jfreymuth/oggvorbis v1.0.5 [v1.0.6]` | `(main module does not need module github.com/jfreymuth/oggvorbis)` |
| `github.com/mewkiz/pkg v0.0.0-20250417130911-3f050ff8c56d [v0.0.0-20261125230257-7bcffafe131e]` | `(main module does not need module github.com/mewkiz/pkg)` |
| `github.com/stretchr/testify v1.11.1 [v1.12.1]` | `…/github.com/egoist/mygo/cmd/mygo → go.digitalxero.dev/rpm → go.digitalxero.dev/rpm.test → github.com/stretchr/testify/assert` |
| `github.com/tphakala/go-aac v0.7.1 [v0.7.2]` | `(main module does not need module github.com/tphakala/go-aac)` |
| `github.com/tphakala/simd v1.11.0 [v1.16.0]` | `(main module does not need module github.com/tphakala/simd)` |
| `golang.org/x/image v0.46.0 [v0.48.0]` | `…/internal/desktop → github.com/egoist/mygo → github.com/egoist/mygo.test → github.com/egoist/mygo/ui → golang.org/x/image/bmp` |
| `golang.org/x/mod v0.42.0 [v0.43.0]` | `…/internal/store → modernc.org/sqlite → modernc.org/libc → modernc.org/libc.test → modernc.org/ccgo/v4/lib → golang.org/x/mod/semver` |
| `golang.org/x/tools v0.52.0 [v0.53.0]` | `…/internal/store → modernc.org/sqlite → modernc.org/libc → modernc.org/libc.test → golang.org/x/tools/go/packages` |
| `modernc.org/gc/v3 v3.2.0 [v3.2.1]` | `…/internal/store → modernc.org/sqlite → modernc.org/libc → modernc.org/libc.test → modernc.org/ccgo/v4/lib → modernc.org/ccgo/v4/lib/internal/secret_sauce → modernc.org/gc/v3` |

Six chains run through a dependency's own test package (as the plan's fact 2 measured); the four
`(main module does not need module …)` rows are exactly the four the plan names. The other 37 rows
of `go list -m -u all` carry no `[newer]` marker.

**The driver watch (Done-when 4).** The store's pinned facts read unchanged on the bumped driver:
`internal/store/store.go:177` `db.SetMaxOpenConns(1)`, `:210` `q.Add("_pragma",
"busy_timeout(5000)")`, `:213` `q.Add("_txlock", "immediate")`, `IsBusy` (`:265`) still mapping the
driver's typed result code (`errors.As` to `*sqlite.Error`, primary code 5/6), and no
`journal_mode` write anywhere in the tree (`grep -rn "journal_mode" internal/` exits 1, no match).
The tests that prove the behaviour under those facts are green: `internal/store`
(`TestSQLiteURIFormatsPathsForEveryOS` pins `busy_timeout(5000)` and `_txlock=immediate` in the DSN,
`TestIsBusyDistinguishesTypedCodeFromLookalikeText`, `TestInterleavedWritersBothPersist`,
`TestWriteWaitsForAHolderUnderTheTimeout`, `TestWritePastTimeoutRefusesAndLeavesNoRow`) and goal
0020 P1's per-user default/notice tests plus the CLI mapping (`internal/cli`
`TestOldDefaultNoticeBeforeWork`, `TestOldDefaultNoticeOncePerRunAndNothingMoved`,
`TestOldDefaultNoticeCommandPerShell`, `TestOldDefaultNoticeWithBothDatabasesPrintsNoMoveCommand`,
`TestOldDefaultNoticeDropsCmdCommandOnPercentPath`, `TestOldDefaultNoticeAbsent`,
`TestBusyDatabaseRefusal`) — all inside the `go test ./internal/...` run below. No adaptation was
needed; Q-0023-01's hold-back clause did not fire.

**Local checks, on the bumped tree (all run at `goal/0023-p1`, 2026-11-26).**

| Command | Result |
|---|---|
| `mise exec -- gofmt -l internal cmd` | no output (exit 0) |
| `mise exec -- go vet ./...` | exit 0, no diagnostics |
| `mise exec -- go build ./...` | exit 0 |
| `mise exec -- go test ./internal/store/ -count=1` | `ok  github.com/wxxb789/passquest/internal/store  7.900s` |
| `mise exec -- go test ./internal/... -count=1` | exit 0; 28 packages `ok`, the only non-`ok` lines are 7 `[no test files]` markers |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts go-test.ts goTest` | `STATUS pass 39 packages, 1040 tests, 0 skipped; slowest packages: github.com/wxxb789/passquest/internal/cli 62.5s, github.com/wxxb789/passquest/internal/store 16.0s, github.com/wxxb789/passquest/internal/workspace 13.7s` `[68s]` — the whole `./...` tree, `cmd/` included, under the gate's no-skip rule |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts go-tidy.ts goTidy` | `STATUS pass` `[0s]` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts local-only.ts localOnly` | `STATUS pass  39 first-party packages, 1 allowed network package: internal/update -> api.github.com, 0 confidential markers, 0 boundary violations` `[2s]` — the bumped `libc`/`sqlite` add no network import to a first-party package |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts golden-contract.ts goldenContract` | `STATUS pass no golden changed against 943de9da9032` `[0s]` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts typecheck.ts typecheck` | `STATUS pass 0 type errors in tsconfig.json, desktop/web/tsconfig.json` `[2s]` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts secrets.ts secrets` | `STATUS pass 818 files, 0 findings` `[0s]` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run-0020.ts docs-links.ts docsLinks` | `STATUS pass 394 checked, 0 broken` `[0s]` |

**Fails when removed.** With the pre-bump `go.mod`/`go.sum` restored in a scratch copy of the module
(no edit to the repository), the same commands read the old set back and the `[newer]` markers
return:

```
$ mise exec -- go list -m -f '{{.Path}} {{.Version}}' modernc.org/sqlite modernc.org/libc github.com/klauspost/compress
modernc.org/sqlite v1.62.0
modernc.org/libc v1.82.0
github.com/klauspost/compress v1.20.4
$ mise exec -- go list -m -u all | grep -E '^(modernc.org/sqlite|modernc.org/libc|github.com/klauspost/compress|modernc.org/cc)'
github.com/klauspost/compress v1.20.4 [v1.20.5]
modernc.org/cc/v4 v4.34.1 [v4.35.0]
modernc.org/ccgo/v4 v4.39.2 [v4.39.3]
modernc.org/libc v1.82.0 [v1.82.13]
modernc.org/sqlite v1.62.0 [v1.63.0]
```

(Five rows, not three: the scratch copy is exactly the pre-bump module pair, so `cc/v4` and
`ccgo/v4` are back at libc 1.82.0's requirement as well — the graph, not a hand edit, is what moved
them.)

**The package size pair (recorded, not judged).** Before: run `45784987207` (`main` at `943de9d`,
conclusion `success`), its `package-<os>` artifacts downloaded and the inner installers read with
`stat` — Windows x64 `pass-quest-setup-0.15.2-amd64.exe` **32,767,774 B**, macOS arm64
`pass-quest-0.15.2-arm64.dmg` **37,250,560 B**, Linux x86_64 `Pass_Quest-0.15.2-x86_64.AppImage`
**53,647,864 B**. After: this pull request's own gate run — its `package-<os>` artifacts, to be read
the same way once CI has run (this machine cannot produce the macOS or Linux artifact). Deferred to
that run: the three-OS `core`/`desktop`/`package` matrix (the authority for the smokes and the
installed package; this step changes the binary), and the `frontend-bench` pair the `go.mod`/`go.sum`
path filter triggers (its seven shots must read byte-identical on `windows-latest` and `macos-26`).

**Q-0023-01 outcome (for the coordinator; `docs/decisions/OPEN-QUESTIONS.md` on this branch still
has no Q-0023 entry to append to).** *Outcome (2026-11-26, P1): held on this tree.*
`go get -u all` + `go mod tidy` moved exactly the three `go.mod` requirements with a newer served
release (`sqlite` 1.62.0 → 1.63.0, `libc` 1.82.0 → 1.82.13, `compress` 1.20.4 → 1.20.5; `cc/v4` and
`ccgo/v4` followed through `libc`'s requirement), the store's pinned facts and goal 0020 P1's tests
stayed green without adaptation, and the ten `[newer]` rows with no require line are recorded as
graph-only above. MyGo, `purego` and `x/sys` are already newest.

## P2 — GitHub Actions to their newest majors

Branch `goal/0023-p2`, stacked on the plan branch's tip `01c333c`. Plan:
[`../plans/2026-11-26-1400-0023-current-dependencies.md`](../plans/2026-11-26-1400-0023-current-dependencies.md)
(row P2); decision Q-0023-02. Every command below ran on this machine (Windows 11, 2026-11-26) and
its output is quoted from the run.

### What changed

21 `uses:` lines in five workflows (`git diff --stat`: 5 files, 21 insertions, 21 deletions;
`git diff --check` clean):

| File | Sites | Old → new |
|---|---|---|
| `.github/workflows/cask.yml` | 77, 145 | `actions/checkout@v4` → `@v7` |
| `.github/workflows/frontend-bench.yml` | 80, 86, 235 | `actions/checkout@v4` → `@v7` |
| | 92, 239 | `actions/setup-go@v5` → `@v7` |
| | 218, 278 | `actions/upload-artifact@v4` → `@v7` |
| `.github/workflows/gate.yml` | 38 | `actions/checkout@v4` → `@v7` |
| | 42 | `actions/setup-go@v5` → `@v7` |
| | 52 | `actions/setup-node@v4` → `@v7` |
| | 111, 126 | `actions/upload-artifact@v4` → `@v7` |
| `.github/workflows/keychain.yml` | 72, 78 | `actions/checkout@v4` → `@v7` |
| | 84 | `actions/setup-go@v5` → `@v7` |
| | 215 | `actions/upload-artifact@v4` → `@v7` |
| `.github/workflows/release.yml` | 26, 47 | `actions/checkout@v4` → `@v7` |
| | 48 | `actions/download-artifact@v4` → `@v8` |

`oven-sh/setup-bun@v2` is untouched (6 sites; read 2026-11-26:
`gh api repos/oven-sh/setup-bun/releases/latest` = v2.2.0 (2026-03-14), and
`action.yml?ref=v2` reads `using: "node24"` — already the newest major and runtime); the
local reuse `uses: ./.github/workflows/gate.yml` (`release.yml:38`) is untouched. After the edit the
whole inventory reads: `checkout` 10, `setup-go` 4, `setup-node` 1, `upload-artifact` 5,
`download-artifact` 1, `setup-bun` 6, local reuse 1.

### Proof 1 — no old major left, and the check fails when the change is removed

Before the edit (base `01c333c`'s workflow files; `git diff 943de9d 01c333c -- .github/workflows`
is empty, so they are the files `main`'s gate run 45784987207 — 12 deprecation lines, quoted under
CI items — ran), the mechanical check read the 21 old lines:

```
$ grep -rnE "uses: +actions/[a-z-]+@v[4-6]" .github/workflows/ | wc -l
21
$ for f in cask frontend-bench gate keychain release; do n=$(git show 01c333c:.github/workflows/$f.yml | grep -cE "uses: +actions/[a-z-]+@v[4-6]"); echo "$f.yml $n"; done
cask.yml 2
frontend-bench.yml 7
gate.yml 5
keychain.yml 4
release.yml 3
```

After the edit:

```
$ grep -rnE "uses: +actions/[a-z-]+@v[4-6]" .github/workflows/ | wc -l
0
$ grep -rnE "uses: +actions/[a-z-]+@v(7|8)" .github/workflows/ | wc -l
21
```

Re-reading base `01c333c`'s own files restores exactly those 21 lines, so this grep is the
fail-when-removed check: a tree that still pins an old major reads 21 again (it is also the
reviewer's one-liner on the diff).

The only old-major string left outside `.github/workflows/` in source files is fixture text handed
directly to `nodePinProblems` (`harness/gates/self-test/package-smoke.cases.ts:1122`,
`- uses: actions/setup-node@v4`); it is not a read of the real workflow — the real read is
`NODE_PIN_SOURCES`' `/^\s*node-version:\s*"([^"]+)"/m` on `gate.yml`, whose line 54 is the
unchanged `node-version: "26.13.1"`. Docs that quote the old majors are the goal contract's problem
statement, the plan, and goal 0005's evidence row
(`docs/evidence/0005-bring-your-own-text-tutor.md:396`) — all left untouched.

### Proof 2 — the pinned tags run `node24`, and the newest release each serves

Read 2026-11-26 with `gh api .../releases/latest --jq .tag_name` and
`gh api -H "Accept: application/vnd.github.raw" repos/actions/<action>/contents/action.yml?ref=<major>`,
quoted from the run:

| Pinned | Newest release (date) | `action.yml` at the pinned major |
|---|---|---|
| `actions/checkout@v7` | v7.0.1 (2026-07-20) | `115:runs:` / `116:  using: node24` |
| `actions/setup-go@v7` | v7.1.0 (2026-11-17) | `47:runs:` / `48:  using: 'node24'` |
| `actions/setup-node@v7` | v7.1.0 (2026-10-08) | `43:runs:` / `44:  using: 'node24'` |
| `actions/upload-artifact@v7` | v7.0.2 (2026-10-07) | `73:runs:` / `74:  using: 'node24'` |
| `actions/download-artifact@v8` | v8.0.2 (2026-10-07) | `51:runs:` / `52:  using: 'node24'` |

### Proof 3 — the breaking notes at the new majors, and why each is inert here

Quoted from each action's README at the pinned tag (`gh api .../contents/README.md?ref=<major>`):

- `checkout` v7: "checkout now refuses to check out fork pull request code by default when the
  workflow is triggered by `pull_request_target` or `workflow_run`" (opt-in input
  `allow-unsafe-pr-checkout`), and "This requires a minimum Actions Runner version of v2.327.1".
  The repository's own triggers (`on:` blocks of all five files): `pull_request`, `push` on `main`
  and on `v*` tags, `release: types: [published]`, `workflow_dispatch`, `workflow_call` — no
  `pull_request_target` or `workflow_run` anywhere, so the fork block cannot fire.
- `setup-go` v7 README, "Breaking changes in V6": "Upgraded Node.js runtime from node20 to node24 —
  Make sure your runner is on version v2.327.1 or later"; `setup-node` v7 README, "Breaking changes
  in V5": "Upgraded action from node20 to node24" with the same runner floor. Both workflows run on
  GitHub-hosted runners, past that floor.
- `upload-artifact` / `download-artifact` READMEs at the pinned tags carry no breaking-changes
  section (grep for `breaking` finds only the security-support sentence); their one compatibility
  caveat is GitHub Enterprise Server ("not currently supported on GHES") — not this setup.
- `download-artifact` v8's `action.yml` inputs read back include the three release.yml passes:
  `pattern: "A glob pattern matching the artifacts that should be downloaded."`,
  `path: "Destination path. ... Defaults to $GITHUB_WORKSPACE"`,
  `merge-multiple: "... the downloaded artifacts will be in the same directory specified by path."`;
  and `digest-mismatch:` with `default: 'error'` ("will fail the action") — release.yml downloads
  this repository's own gate artifacts, so the strict default is kept.

### Proof 4 — the workflows still parse and the CI group matrix still reads

`mise exec -- bun Q:/scratch/omp/g23/P2/check-workflows.ts` (parses each file with the same
`Bun.YAML` engine `harness/lib/gate-groups.ts` uses, then runs the repo's own `workflowGroups` on
the real `gate.yml`):

```
cask.yml: parsed; jobs = update, install
frontend-bench.yml: parsed; jobs = bench, linux-smoke
gate.yml: parsed; jobs = gate
keychain.yml: parsed; jobs = experiment
release.yml: parsed; jobs = version, gate, release
gate.yml workflowGroups: ["core","desktop","package","self-test"]
```

`package-smoke.cases.ts`'s fixture and the self-test's `gateGroupProblem` both read the real
`gate.yml`; the parse and the matrix above are the mechanical part of that read.

### CI items (three-OS run is the authority; to be read from this branch's pull request run)

- Gate run: all four groups (`core`, `desktop`, `package`, `self-test`) green on `ubuntu-latest`,
  `macos-latest` and `windows-latest`; the `package` group still builds and installs under the new
  actions.
- `gh run view <PR run id> --log | grep -c "Node.js 20 is deprecated"` = 0. Measured baseline on
  this machine: main's gate run 45784987207 (headSha `943de9d`, push, conclusion success) reads
  `12`, every line the runner's own `##[warning]Node.js 20 is deprecated. The following actions
  target Node.js 20 but are being forced to run on Node.js 24: actions/checkout@v4,
  actions/setup-go@v5, actions/setup-node@v4.` (log saved at
  `Q:/scratch/omp/g23/P2/main-run-45784987207.log`, 837,528 bytes; 12 of 12 lines carry that
  action list). With the new majors pinned, no job in this run can emit that warning for these
  actions.
- Bench (`frontend-bench.yml` is in its own `paths:` filter, so this change triggers it): the
  `bench` matrix `[windows-latest, macos-26]` reports its seven shots byte-identical, and
  `linux-smoke` on `ubuntu-latest` stays green.

## P3 — The request-body recorder (2026-11-26, branch `goal/0023-p3`)

Done-when 2's instrument: the durable host's provider request bodies become recordable, opt-in
through `PASSQUEST_AGENT_SMOKE_DUMP`, so P4 can diff a pre-bump and a post-bump run. Windows 11
Enterprise (10.0.26300), tree `Q:/repos/passquest-tmp`; every run below went through
`mise exec -- bun Q:/scratch/pq-harness-run/run-tmp.ts <gate> <export>`.

### Interface

- `PASSQUEST_AGENT_SMOKE_DUMP=<dir>` is harness-only and unset by default. When set, the gate checks
  `<dir>` once before the four passes — it must exist and accept a probe file, else the gate fails
  naming the variable and the path — and each completed pass writes `<dir>/<label slug>.json`, the
  pass label with non-alphanumerics folded to `-` (the same slug the pass's temp directory uses), as
  `{ label, format, requests }` where `requests` is the stub's own `received` array of
  `{ path, body }`. A write that fails at pass time is a named failure. Unset, nothing is written,
  no check runs, and assertions, failure messages and the pass line are unchanged.
- Files this run wrote (`Q:/scratch/omp/g23/P3/dump`, created before the run):
  `source-host-chat.json`, `source-host-responses.json`, `staged-bundle-chat.json`,
  `staged-bundle-responses.json`.
- Files changed: `harness/gates/agent-smoke.ts` (the check, the option, the per-pass write),
  `harness/gates/self-test/agent-smoke.cases.ts` (the must-fail case),
  `harness/gates/self-test.ts` (`runCase` saves and restores the variable), `docs/harness.md` row 17
  (one sentence). No product file changed.

### Runs

| Run | Result |
|---|---|
| pre-change tree (`01c333c`, no P3 code), `PASSQUEST_AGENT_SMOKE_DUMP=Q:/scratch/omp/g23/P3/blocker/dump` — a path under the regular file `blocker` | PASS — the variable is ignored today: `488 assertions, 11 scenarios x 4 passes (source host, chat, source host, responses, staged bundle, chat, staged bundle, responses), 99 provider requests, permissions skipped on Windows, bundle 1598993 bytes with 7 inlined packages`; nothing written (`blocker` stayed a 32-byte file, no `blocker/dump`). This is the removal side: without the feature the must-fail case's gate run does not fail, so the case reddens. |
| final tree, same impossible path | FAIL in 0 s: `PASSQUEST_AGENT_SMOKE_DUMP="Q:/scratch/omp/g23/P3/blocker/dump" is not a writable directory: ENOENT: no such file or directory, open 'Q:\scratch\omp\g23\P3\blocker\dump\.agent-smoke-write-probe-11364'` |
| final tree, `PASSQUEST_AGENT_SMOKE_DUMP=Q:/scratch/omp/g23/P3/dump` | PASS — `488 assertions … 98 provider requests, permissions skipped on Windows, bundle 1598993 bytes with 7 inlined packages`; the four files above parse with `jq`; labels and formats match their pass; 24 / 24 / 25 / 25 requests, all on `/v1/chat/completions` (chat) and `/v1/responses` (responses); 24 / 25 `role: "system"` messages in the two chat files; 84 / 86 `input` items in the two responses files (untyped message items plus `function_call` and `function_call_output`), and no request body carries `instructions`. |
| final tree, no variable | PASS — same `488 assertions …` line as the pre-change run (the provider-request count is the only field that moves: 99 → 98); the `dump/` listing is byte-identical before and after and no other file appeared in `Q:/scratch/omp/g23/P3`. The unset path enters none of the changed lines: the once-check and the per-pass write both sit behind `if (dump)`. |
| `self-test` gate (this tree, one local run, 1506 s, load 32 % → 69 %) | FAIL on exactly one unrelated window case: `desktop-ux: the non-modal layer broke an earlier stage: desktop-ux prepare: script failed: mygo: javascript error: timed out waiting for a practice item [route #/today] on a layer shown with dialog.show() while the page behind it stays tabbable`. That is the load-caused window flake this machine is known for (the briefing names this exact error), it is in a gate this step does not touch, and only that one problem line was reported, so every other case — the new one included — was green. The three-OS `self-test` job is CI's authority, and P5 re-quotes the case count from its own run. |

Targeted runs of the new case, through its own `files`/`prepare` data and the real gate module with
`runCase`'s logic (`Q:/scratch/omp/g23/P3/case-run.ts`): with the variable set by the prepare, the
gate fails naming the dump failure and the case is green in 1.1 s; with `PASSQUEST_AGENT_SMOKE_DUMP`
unset — what a tree without the feature does to the same invocation — the gate passes and the case
reports `would redden: agent-smoke: status pass on agent smoke whose request dump directory cannot
exist, want fail` (`Q:/scratch/omp/g23/P3/case-removed.log`). With the pre-change-tree run above,
the case reddens exactly when the feature is removed.

The guard move: `dumpDirectoryProblem`'s `existsSync`/`statSync` read now sits inside the same `try`
as the probe write, so a path that disappears between the two calls is a named failure, not a throw.
After that edit, `typecheck` PASS, the impossible-path run and the targeted case fail with the same
message, and the dump-set run was repeated to `Q:/scratch/omp/g23/P3/dump-final`: PASS — `488
assertions, 11 scenarios x 4 passes (source host, chat, source host, responses, staged bundle, chat,
staged bundle, responses), 100 provider requests, permissions skipped on Windows, bundle 1598993
bytes with 7 inlined packages` — with the same four file names, whose `.label`/`.format` read back
`source host, chat chat`, `source host, responses responses`, `staged bundle, chat chat` and
`staged bundle, responses responses`.

One pre-change attempt failed before the pass above: `source host, chat: resume exit 3: … "code": "interrupted", "message": "The host stopped while the model was answering, on attempt 1"` plus its three follow-on lines, under load 73–93 % from the parallel P1/P2/P4 worktrees. The tree carried no P3 code and the identical command passed on the immediate rerun, so the change did not cause it (`Q:/scratch/omp/g23/P3/baseline-dumpvar.log` first attempt, `baseline-dumpvar2.log` pass).

### Sample entries (the facts P4/P5 read from here)

- chat — `source-host-chat.json`, `requests[0].body.messages[0]`:
  `{"role":"system","content":"<passquest-agent-rules>\nYou are the Pass Quest durable study-brief agent. …"}`
- responses — `source-host-responses.json`, `requests[0].body.input[0]`: a message item with no
  `type`, `role: "system"`, `content: "<passquest-agent-rules>\n…"` (the bodies carry no
  `instructions`).
- No dump file contains the stub key `test-key-not-real` or the strings `baselines` /
  `data/private` (`grep -l` over the four files: no match).

### Limits

- Local Windows evidence only. macOS and Linux agent-smoke and self-test runs are CI's: the three-OS
  `core` job runs the default (unset) path and the three-OS `self-test` job runs the must-fail case.
- The pass line's provider-request count moves with machine load; assertions, the pass labels and the
  bundle bytes are the stable fields.

## P4 (2026-11-26) — The Pi packages: the feed check and the blocked branch (branch `goal/0023-p4`)

Base: `main` at `7c36d43` (P1 #80, P2 #81, P3 #82 merged). Contract:
[`../goals/0023-current-dependencies.md`](../goals/0023-current-dependencies.md); row P4 of
[`../plans/2026-11-26-1400-0023-current-dependencies.md`](../plans/2026-11-26-1400-0023-current-dependencies.md);
default Q-0023-03. Every command below ran on this machine (Windows 11, 2026-11-26) through the
configured feed — `npm config get registry` reads `https://packagefeedproxy.microsoft.io/npm/`, and
no other registry was contacted — and its output is quoted from the run.

**What changed.** No package file: the feed still serves 1.5.2 as the newest release of every one of
the six `@earendil-works` entries the lock holds, so Q-0023-03's blocked branch applies. This
branch's diff is this section, its Q-0023-03 outcome line included; `agent/package.json` keeps its
four exact pins and `bun.lock` is untouched.

**The feed check, run before any file was touched.**

```
$ for p in chord pi-ai pi-durable pi-schema pi-telemetry pi-codemode; do printf '%s ' "@earendil-works/$p"; npm view "@earendil-works/$p" version; done
@earendil-works/chord 1.5.2
@earendil-works/pi-ai 1.5.2
@earendil-works/pi-durable 1.5.2
@earendil-works/pi-schema 1.5.2
@earendil-works/pi-telemetry 1.5.2
@earendil-works/pi-codemode 1.5.2
```

Two more readings agree per entry: `npm view <pkg> dist-tags --json` reads `"latest":"1.5.2"` for
all six (`chord` also carries an `initial` tag and `pi-ai` a `legacy-node20` one — neither is
newer), and each entry's `versions` list ends `["1.5.0","1.5.1","1.5.2"]`. The feed answers a 1.6.0
request with its own 404:

```
$ npm view @earendil-works/pi-ai@1.6.0 version
npm error code E404
npm error 404 No match found for version 1.6.0
npm error 404
npm error 404  The requested resource '@earendil-works/pi-ai@1.6.0' could not be found or you do not have permission to access it.
```

Upstream released 1.6.0 on 2026-11-20 and 1.6.1 on 2026-11-25 (the goal's own text); the feed has
picked up neither, so the six entries cannot move together and no mixed set is shipped.
`npm view @earendil-works/pi-ai time --json` dates the feed's own publishes — its last version key
is `1.5.2` → `2026-11-10T12:20:46Z`, sixteen days before this check. Hand-building or vendoring is
declined (owner, 2026-11-26).

**Proof no package file changed.** `git diff --stat` on this branch names one file (this record),
and `agent/package.json`'s four pins read back exactly as before (`jq -r '.dependencies |
to_entries[] | "\(.key) \(.value)"' agent/package.json` → `@earendil-works/chord 1.5.2`,
`@earendil-works/pi-ai 1.5.2`, `@earendil-works/pi-durable 1.5.2`, `@earendil-works/pi-schema
1.5.2`). `bun.lock`'s six entries (line numbers from the tree; `…` marks elision of the long
`dependencies` objects):

```
$ grep -n "@earendil-works" bun.lock
15:        "@earendil-works/chord": "1.5.2",
16:        "@earendil-works/pi-ai": "1.5.2",
17:        "@earendil-works/pi-durable": "1.5.2",
18:        "@earendil-works/pi-schema": "1.5.2",
65:    "@earendil-works/chord": ["@earendil-works/chord@1.5.2", "", { …
67:    "@earendil-works/pi-ai": ["@earendil-works/pi-ai@1.5.2", "", { …
69:    "@earendil-works/pi-codemode": ["@earendil-works/pi-codemode@1.5.2", "", { …
71:    "@earendil-works/pi-durable": ["@earendil-works/pi-durable@1.5.2", "", { …
73:    "@earendil-works/pi-schema": ["@earendil-works/pi-schema@1.5.2", "", {}, …
75:    "@earendil-works/pi-telemetry": ["@earendil-works/pi-telemetry@1.5.2", "", {}, …
```

Lines 15–18 are `workspaces."agent"`'s pins (the root `package.json` lists `"agent"` as its one
workspace); every package entry's registry-URL field is still `""`. `bun.lock` is not part of this
branch's diff, so no version anywhere in it — and no registry URL — moved.

**Q-0023-03 outcome (for the coordinator; `docs/decisions/OPEN-QUESTIONS.md` on this tree still has
no Q-0023 entry to append to, as P1 flagged).** *Outcome (2026-11-26, P4): blocked by the feed.*
All six entries were checked before any file change and every one serves 1.5.2 as its newest, with
`@earendil-works/pi-ai@1.6.0` reading E404, so the four direct pins stay at 1.5.2 and no lock
re-resolve ran. Owner action: ask the feed owner to publish 1.6.x for
`@earendil-works/{chord,pi-ai,pi-durable,pi-schema,pi-telemetry,pi-codemode}`. Re-run: cut
`goal/0023-p4` from `main` again and repeat the six `npm view <pkg> version` lines — only all six at
≥ 1.6.0 opens the row's bump path.

**Q-0023-04 outcome (for the coordinator).** *Outcome (2026-11-26, P4): not fired on the blocked
path.* P3's opt-in recorder is landed (`PASSQUEST_AGENT_SMOKE_DUMP`, harness-only), so the
comparison is ready, but there is no bumped tree to compare against and this branch takes no dump;
when the feed opens the bump path, the row's step 3 takes the `before/` dump on the pre-bump tree
and the `after/` dump on the bumped tree. Q-0006-02 and Q-0021-07 are untouched: no pin moved, and
Q-0021-07's *Revisit when* (the feed serving ≥ 1.6.0 for all six) did not fire.

**Fails when removed.** This section's six listings and 404 are the blocked path's artifact:
without it the branch carries no evidence the feed was checked, and the re-run command above is
what re-reads the same six lines. On this path there is no lock diff, no request-body dump and no
staged-bundle read-back (no rebuild happened); no package byte changed on the branch, so P5's
bundle numbers stay the ones P3 read at 1.5.2 (`1598993` bytes, 7 inlined packages) and the per-OS
size pairs stay P1's.

**For P5.** Feed state at this re-check: blocked, with the six listing lines and the 404 above. The
inventory rows for the six `@earendil-works` entries therefore read *blocked by the feed*, sourced
from this section; there is no lock diff and no after-dump to pair.

**Local checks** (the brief's list for a documentation-only diff; this branch changes no Go file,
no TypeScript file and no workflow, so the Go checks, the smokes and the package/self-test gates
are not this step's to re-run — CI's three-OS matrix re-proves them on the branch's pull request):

| Command (through `Q:/scratch/pq-harness-run/run-0020.ts`) | Result |
|---|---|
| `typecheck.ts typecheck` | `STATUS pass 0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `secrets.ts secrets` | `STATUS pass 818 files, 0 findings` |
| `docs-links.ts docsLinks` | `STATUS pass 397 checked, 0 broken` |

The three ran after this section was written, so its two relative links are inside the 397 checked.

## P5 (2026-11-26) — The inventory, the decisions and the open rows (branch `goal/0023-p5`)

Contract: [`../goals/0023-current-dependencies.md`](../goals/0023-current-dependencies.md).
Plan: [`../plans/2026-11-26-1400-0023-current-dependencies.md`](../plans/2026-11-26-1400-0023-current-dependencies.md).
Defaults Q-0023-01…05 in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).
This branch is stacked on `goal/0023-p4` (PR #83); `main` is `7c36d436` (P1 #80, P2 #81 and P3 #82
merged). Every command below ran on this machine (Windows 11) on 2026-11-26, and every CI number is
read from the run named beside it.

**State at this read: item 1 is shown by the inventory below; item 2 is open, blocked by the feed;
item 3 is shown for the four sets with one open Go row (`modernc.org/memory`, see *Open rows*);
item 4 is shown with the one bench reading recorded red rather than cleared (*The bench records*);
item 5 is shown by the package jobs' pin line. The goal is not closed and no close is claimed.**

### The inventory (Done-when 1)

One table; the `Set` column names the four dependency sets. "Read from" is where the new (or
current) version was read, dated 2026-11-26; a row left in place carries its reason.

| Set | Dependency (how it is pinned) | Old | New | Read from | Notes / reason left in place |
|---|---|---|---|---|---|
| Go modules | `modernc.org/sqlite` (direct) | v1.62.0 | v1.63.0 | the proxy's listing, `go list -m -u all` | the pure-Go SQLite driver; P1 took it and re-proved the store's pinned facts |
| Go modules | `modernc.org/libc` (`require`, indirect) | v1.82.0 | v1.82.13 | same listing | `modernc.org/cc/v4` v4.34.1 → v4.35.0 and `modernc.org/ccgo/v4` v4.39.2 → v4.39.3 followed its requirement; no `require` line names them |
| Go modules | `github.com/klauspost/compress` (`require`, indirect) | v1.20.4 | v1.20.5 | same listing | only `go get -u all` reaches a requirement the main module's packages do not import (Q-0023-01) |
| Go modules | `github.com/ebitengine/purego` (direct) | v0.11.3 | v0.11.3 | same listing (no `[newer]` marker) | already newest |
| Go modules | `github.com/egoist/mygo` (direct; `tool`) | v0.10.0 | v0.10.0 | the proxy's version list ends `v0.10.0`; `releases/latest` is `v0.10.0` (2026-11-17) | already newest; the release notes name no hidden-title-bar change, so goal 0022's precondition does not fire |
| Go modules | `golang.org/x/sys` (direct) | v0.50.0 | v0.50.0 | the proxy's listing | already newest |
| Go modules | `modernc.org/memory` (`require`, indirect) | v1.13.0 | **v1.13.1 served, not taken** | the proxy's listing at this read; the version is dated 2026-11-15T20:32:12Z | **open row**: a `require` with a newer served release; a scratch copy ran `go get -u all` + `go mod tidy` and moved exactly this row (`go.mod` line 28, two `go.sum` lines), leaving the ten graph-only rows; not taken on this docs-only step |
| Go modules | the ten graph-only rows (`google/pprof`, `jfreymuth/oggvorbis`, `mewkiz/pkg`, `stretchr/testify`, `tphakala/go-aac`, `tphakala/simd`, `x/image`, `x/mod`, `x/tools`, `gc/v3`) | versions quoted in P1's section | same | the proxy's listing keeps their `[newer]` markers | graph-only per Q-0023-05: no `require` line names them; each row's `go mod why -m` output (a chain, or `(main module does not need module …)`) is in P1's section |
| Go modules | the remaining indirect rows | as the graph selects | unchanged | the proxy's listing (53 rows: 11 marked, 42 unmarked) | no `[newer]` marker; the five moved rows and MyGo/`purego`/`x/sys` are inside the unmarked 42 |
| npm packages | `@types/bun` (`^1.4.4`, lock 1.4.4) | 1.4.4 | 1.4.4 | the feed's `dist-tags`: `latest` 1.4.4 | already newest; `npm outdated --json` at the root reads `{}` |
| npm packages | `typescript` (`^7.0.2`, lock 7.0.2) | 7.0.2 | 7.0.2 | the feed's `dist-tags`: `latest` 7.0.2 (`beta` 7.1.0-beta) | already newest stable; a beta is not taken |
| npm packages | `@earendil-works/chord`, `pi-ai`, `pi-durable`, `pi-schema` (exact pins) + `pi-codemode`, `pi-telemetry` (transitive) | 1.5.2 | 1.5.2; the target is ≥ 1.6.0 | the feed's `npm view <pkg> version` lines read 1.5.2, and `@earendil-works/pi-ai@1.6.0` answers the feed's E404 (P4's section; re-read here, unchanged) | **blocked by the feed** (Q-0023-03) — item 2 open; no package file changed |
| Toolchains | Go | 1.27.4 | 1.27.4 | `mise ls-remote go` tail `1.27.2 1.27.3 1.27.4` | already newest; pinned in `mise.toml`, `go.mod` and the workflows' `go-version-file` |
| Toolchains | Bun | 1.4.5 | 1.4.5 | `mise ls-remote bun` tail `1.4.3 1.4.4 1.4.5` | already newest; `mise.toml` plus six `bun-version: 1.4.5` sites in four workflows |
| Toolchains | Node | 26.13.1 | 26.13.1 | `mise ls-remote node` 26-line tail `26.12.1 26.13.0 26.13.1` | already newest 26 LTS patch (Q-0013-03); pinned in `mise.toml`, `gate.yml:54`, `harness/lib/package.ts:66` and both install guides; the pin-agreement check is green on every run (Done-when 5) |
| GitHub Actions | `actions/checkout` (10 sites) | v4 | v7 | `releases/latest` v7.0.1 (2026-07-20); `action.yml@v7` reads `using: node24` | v5+'s fork-PR checkout block is inert here — no `pull_request_target`/`workflow_run` trigger exists |
| GitHub Actions | `actions/setup-go` (4 sites) | v5 | v7 | `releases/latest` v7.1.0 (2026-11-17); `node24` | the runner floor v2.327.1 is long passed on GitHub-hosted runners |
| GitHub Actions | `actions/setup-node` (1 site) | v4 | v7 | `releases/latest` v7.1.0 (2026-10-08); `node24` | same floor; the workflow's `node-version: "26.13.1"` (a pin-agreement source) is unchanged |
| GitHub Actions | `actions/upload-artifact` (5 sites) | v4 | v7 | `releases/latest` v7.0.2 (2026-10-07); `node24` | same floor; `archive: false` is unused |
| GitHub Actions | `actions/download-artifact` (1 site) | v4 | v8 | `releases/latest` v8.0.2 (2026-10-07); `node24` | its digest-mismatch default `error` is kept; `release.yml`'s `pattern`/`path`/`merge-multiple` are confirmed against the v8 `action.yml` |
| GitHub Actions | `oven-sh/setup-bun` (6 sites) | v2 | v2 | `releases/latest` v2.2.0 (2026-03-14); `node24` | already the newest major and runtime; floats to its newest release |

The moved rows' old → new numbers and the proxy/feed/grep readings are P1's and P2's sections' own
output; this table re-reads them at the versions the merged tree carries
(`mise exec -- go list -m -f '{{.Path}} {{.Version}}' modernc.org/sqlite modernc.org/libc
github.com/klauspost/compress` reads `v1.63.0`, `v1.82.13`, `v1.20.5`).

### The action-log greps (Done-when 3)

```
$ grep -rnE "uses: +actions/[a-z-]+@v[4-6]" .github/workflows | wc -l
0
$ grep -rnE "uses: +actions/[a-z-]+@v(7|8)" .github/workflows | wc -l
21
$ grep -rn "uses: " .github/workflows/ | grep -c "setup-bun@v2"
6
```

Before P2 the same first grep read 21 (P2's section quotes the per-file split). The runner's own
warning: `gh run view <id> --log | grep -c "Node.js 20 is deprecated"` reads **12** for run
45784987207 (`main` at `943de9d`, the pre-step baseline; its lines name `actions/checkout@v4,
actions/setup-go@v5, actions/setup-node@v4`) and **0** for run 45814888392 (P2's pull request, head
`9978096a`) and run 45825120752 (`main` at `7c36d436`, P1–P3 merged).

### The three-OS gate runs behind every row

| Step | PR | Head | Gate run | Result |
|---|---|---|---|---|
| P1 | #80 | `392e55c8` | 45808814811 | success, all 12 jobs |
| P2 | #81 | `9978096a` | 45814888392 | success, all 12 jobs |
| P3 | #82 | `88f31295` | 45819708765 | success, all 12 jobs |
| P1–P3 merged | — | `main` `7c36d436` | 45825120752 | success, all 12 jobs |
| P4 | #83 | `51de3996` | 45830132616 | in progress at this writing: 10 of 12 jobs green, both `self-test` jobs still running |

The workflows P2's diff self-triggers also ran on its branch and succeeded: `cask` run 45814888515
and `keychain` run 45814888441 (both on a macOS runner, so they exercise the bumped `checkout@v7`
before the next release).

### Rows 17 and 27 of `docs/harness.md`, from the runs

Run 45825120752 (`main` at `7c36d436`) — P3's pull-request run 45819708765 reads the same lines:

- Row 17 (core jobs): `PASS  agent smoke (488 assertions, 11 scenarios x 4 passes (source host,
  chat, source host, responses, staged bundle, chat, staged bundle, responses), 100 provider
  requests, permissions skipped on Windows, bundle 1599023 bytes with 7 inlined packages)` on
  Windows; 496 assertions with `8 permission checks` and `bundle 1599045` / `bundle 1598993` bytes
  on macOS and Ubuntu. The `PASSQUEST_AGENT_SMOKE_DUMP=<dir>` sentence in its body is P3's.
- Row 27 (self-test jobs): `PASS  harness self-test (187 cases (18 must pass), 26/26 gates)` on all
  three OSes — P3's one must-fail dump case took the conflict-marker step's 186 (18) to 187 (18).
- Rows 24 and 25 are unmoved by this goal, and the same run reads them: `PASS  desktop smoke (584
  assertions over 15 scenarios)` on Windows, `583` on macOS, `581` on Ubuntu; `PASS  desktop ux
  (192 audits over 24 states × 4 modes × 2 sizes, 19 probes, 84 focus checks, 0 allowed …)`.
- Row 26: `PASS  package smoke (624 assertions over contents, layout, study, cli-runtime, agents,
  brief, brief-responses, anki, upgrade, credential, broken …)` on Windows, 623 on macOS and
  Ubuntu; every line ends `node pin: 5 sources agree on Node 26.13.1` (Done-when 5).

### The staged bundle and the request bodies (item 2's instrument, blocked)

- The staged bundle: the agent-smoke pass lines above read **7 inlined packages** and the bundle
  bytes on all three OSes, and the package gate proves every inlined package is one `bun.lock` pins
  (`bundlePackages`/`lockProblems`, `harness/lib/package.ts`). P4 changed no package file, so the
  staged bundle is the one P3 measured at Pi 1.5.2.
- The request-body dumps: **absent on this path, by design.** There is no bumped tree, so there is
  no `before/` dump, no `after/` dump and no diff to quote. The instrument is landed (P3,
  `PASSQUEST_AGENT_SMOKE_DUMP=<dir>`), and its sample entries are P3's own — the chat pass's
  `messages[0]` and the responses pass's `input[0]` carry the `role: "system"` rules message, and
  no body carries `instructions`. When the feed opens the bump path, the step takes the `before/`
  dump on the pre-bump tree and the `after/` dump on the bumped tree (Q-0023-04).

### The package sizes, per OS (recorded, not judged)

Read with `stat` from each run's `package-<os>` artifacts (`gh run download <id> -n package-<os>`,
downloaded here into `Q:/scratch/omp/g23/P5/artifacts/`):

| Installer | Before — run 45784987207 (`main` `943de9d`) | After P1 — run 45808814811 | After P1–P3 — run 45825120752 |
|---|---|---|---|
| `pass-quest-setup-0.15.2-amd64.exe` | 32,767,774 B | 32,777,018 B | 32,772,700 B |
| `pass-quest-0.15.2-arm64.dmg` | 37,250,560 B | 37,269,504 B | 37,268,992 B |
| `Pass_Quest-0.15.2-x86_64.AppImage` | 53,647,864 B | 53,643,768 B | 53,643,768 B |

P1's run's package jobs pass the same eleven cases as the merged tree's (624 Windows / 623 macOS
and Ubuntu) and drive the installed tree (`cli-runtime: … opened by the installed binary with no
node on PATH`), so the bumped driver's package builds, installs and runs on all three OSes. The
numbers are recorded, not judged: the exe and dmg move by a few KB between builds and the AppImage
is byte-stable here.

### The bench records

| Run | A / B | Result |
|---|---|---|
| 45808814809 (P1, #80; attempt 1, not rerun) | A `34c884cc` (the plan commit) / B `392e55c8` (P1) | windows-latest **failure**: measurement attempt 1's same-build control pair read ok, so its verdict was taken, and `roomSwitch.anki` read `WORSE` — A n=9 med 32.1 → B n=9 med 46.4, floor 10.7 (control IQR), `q1 43.1 vs base q3 41.6 (above)`; every other metric read ok; `shots: 7/7 state(s) byte-identical between A and B`; `bundle.mainJs` A=B=187714 and `bundle.mainCss` A=B=79885. macOS: every metric ok, shots 7/7 identical. Ubuntu render smoke: green. |
| 45814888421 (P2, #81; attempt 2) | A `04d8380d` (`main` after P1) / B `9978096a` (P2) | windows-latest and macos-26: no `WORSE` line on either job; `shots: 7/7 … byte-identical` on both; Ubuntu render smoke green. Attempt 1's macOS job had failed in the bench's launch guard (`warmup-a-0: the page returned 19 frame duration(s), want at least 20` — the findings log records the same run), and the coordinator reran the failed job. |

P3 and P4 change no bench path, so neither has a bench run.

Attribution for the one red reading, with its evidence: P1's diff is three `go.mod`/`go.sum` lines;
in the recorded pair the frontend bundle bytes are equal on both sides and all seven shots are
byte-identical, and the metric that read `WORSE` is a Windows route-switch timing. The bench's own
rule keeps a readable attempt's verdict, which is why the job is red; the class has the findings
log's precedent (2026-11-24, PR #53: a same-content A/B false positive on the Windows runner,
reran). This record keeps the numbers as measured and does not call them noise: a bench re-run on
the merged tree (`workflow_dispatch` with the full A/B SHAs, goal 0022's precedent) is the one
action that would replace the reading, and it is the coordinator's call.

### Open rows

1. **Item 2 — the Pi packages stay blocked by the feed.** The six entries were re-checked here and
   all read 1.5.2 (P4's section carries the six listing lines and the
   `@earendil-works/pi-ai@1.6.0` 404). Re-run: cut the Pi branch from `main` again and repeat
   `npm view @earendil-works/<chord|pi-ai|pi-durable|pi-schema|pi-telemetry|pi-codemode> version`;
   only all six at ≥ 1.6.0 opens the bump path — the four pins together, `bun install` re-resolving
   the lock from the feed, the `before/`+`after/` dumps and their diff, and the three-OS agent and
   package smokes (Q-0023-03, Q-0023-04). Owner action: ask the feed owner to publish 1.6.x.
2. **`modernc.org/memory` v1.13.0 → v1.13.1.** Served by the proxy, required by `go.mod`
   (line 28, `// indirect`), not taken on this docs-only step. The recipe probe moved exactly this
   row; taking it is one `go.mod` line and two `go.sum` lines and is Done-when 3's remaining item.
   It belongs to the goal's next module-moving step (the re-cut bump step above is the natural
   carrier), never to a silent docs edit.

### Known limits

- macOS and Linux window, package and bench evidence is CI-only: this machine ran none of those
  gates. The named runs are the authority.
- `release.yml` runs only on a tag push, so its action inputs are proven by reading the pinned v8
  `action.yml` (P2's section), not by an executed run.
- Item 2's request-body comparison is absent by the block (above); nothing stands in for it.
- The one bench reading of P1's pair (above) is recorded red; the merged tree has not re-measured
  it.
- P4's own gate run (45830132616) was still in flight at this writing; its result belongs to PR
  #83.

### Fails when removed

Deleting this section leaves Done-when 1's table and the run ids unreadable — the named runs and
listings are the only place the numbers exist; a drifted link reddens `docs-links`, and a drifted
pinned sentence reddens `install-guide`. The cross-check is mechanical: every row names the command
output or run id it was read from, all re-read in this step's own run rather than remembered.

### Local checks on this branch (this step's own run)

The four gates this step's row names, through `mise exec -- bun
Q:/scratch/pq-harness-run/run-0020.ts <gate> <export>` on the edited tree (Windows 11,
2026-11-26):

| Gate | Result |
|---|---|
| `docs-links.ts docsLinks` | `STATUS pass 401 checked, 0 broken` |
| `install-guide.ts installGuide` | `STATUS pass 7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; the release notes name the tap command and both guides; the cask template renders v0.15.0 with the release's SHA-256, and a synthetic pair, into Casks/pass-quest.rb; floor 26.0 from mygo.json` |
| `secrets.ts secrets` | `STATUS pass 818 files, 0 findings` |
| `typecheck.ts typecheck` | `STATUS pass 0 type errors in tsconfig.json, desktop/web/tsconfig.json` |

No Go, TypeScript, workflow or host file changed on this branch, so this step runs no Go check, no
smoke and no package gate; the three-OS runs named above are the authority for those.
