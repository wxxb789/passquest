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
