# 0015 — Know when a newer Pass Quest exists: evidence

Contract: [`../goals/0015-know-when-a-newer-version-exists.md`](../goals/0015-know-when-a-newer-version-exists.md).
Plan: [`../plans/2026-11-19-1900-0015-update-check.md`](../plans/2026-11-19-1900-0015-update-check.md).
Guide: [`../install.md`](../install.md#upgrade). Design: the one network exception in
[`../core-design.md`](../core-design.md) §6. Decisions: Q-0015-01…06 in
[`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

## Environment

Windows 11 Enterprise (10.0.26300), Go 1.27.4 (with `GOROOT` unset: mise keeps a Go 1.27.3 root whose
compiler mismatches the 1.27.4 `go` on `PATH`, and every build then fails), Bun 1.4.4, Node v26.13.1,
MyGo 0.10.0 (Go tool), WebView2 runtime installed. macOS arm64 and Linux x86_64 were not run locally;
CI's `macos-latest` and `ubuntu-latest` runners are their only evidence. Branch `goal/0015-update-check`:
P1 `1e90997`, P2 `206148b`, P3 `0336e98`, P4 `b6b5bad`, P5 this docs tree.

## Gate runs

| Command | Result |
|---|---|
| CI, run 44552103953 (head `b6b5bad`, the last code change; P5 changes docs only, so these counts hold on the final tree) | 12/12 jobs green. `harness self-test`: 74 cases (7 must pass) over 24/24 gates on all three OSes; `desktop smoke`: 488 assertions over 13 scenarios on Windows and 487 on macOS and Linux, including the `update` scenario; `desktop ux`: 192 audits over 24 states × 4 modes × 2 sizes, 19 probes, 72 focus checks, 0 allowed on all three |
| `bun Q:/scratch/pq-harness-run/run.ts self-test.ts selfTest` (this tree) | PASS 74 cases (7 must pass), 24/24 gates `[488s]` |
| `bun Q:/scratch/pq-harness-run/run.ts local-only.ts localOnly` | PASS 37 first-party packages, 1 allowed network package: internal/update -> api.github.com, 0 confidential markers, 0 boundary violations `[15s]` |
| `bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck` | PASS 0 type errors in tsconfig.json, desktop/web/tsconfig.json `[1s]` |
| `bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS 904 files, 0 findings `[0s]` |
| `bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS 254 checked, 0 broken `[0s]` |
| `gofmt -l internal cmd`; `go vet ./...`; `go build ./...` | No output, exit 0 each |
| `go test ./internal/... -count=1` | exit 0: 25 packages `ok`, 0 `FAIL`, 0 `SKIP` (slowest: `internal/cli` 31.1s, `internal/store` 12.1s) `[37s]` |
| CI (`.github/workflows/gate.yml`, four groups × three runners) | TBD (final head) — the coordinator fills this row |

## Acceptance → evidence

| # | Done when | Evidence |
|---|---|---|
| 1 | The learner decides | The check is on demand only (default taken, Q-0015-01): no start-up check, no timer, no stored preference. The Settings row (`updates:` key, `desktop/web/src/views/settings.ts:184`) shows the running version (`:200`), the check state and the **Check for updates** button (`:203`); opening it shows the one-line disclosure of what is contacted and sent (`:211`), and only the click runs the check (`desktop/web/src/controllers/settings.ts:50-58`; the session starts `idle`, `desktop/web/src/core/state.ts:58-59,162`). desktop ux audits `settings-update-idle`, `settings-update-newer` and `settings-update-unavailable` (`harness/gates/desktop-ux.ts:66-68`) in light/dark × zh/en at 1280×800 and 1440×844 — the state before the first check and the two states after one — and the desktop smoke's script reads `data-update-state` before and after each press (`harness/gates/desktop-smoke/update.ts:66-81`). |
| 2 | It sends nothing about the learner | `internal/update` is the whole request surface: one `GET` to the constant endpoint with `Accept: application/vnd.github+json` and `User-Agent: PassQuest`, no version, query, body or cookie (`internal/update/update.go:33,68-69,199-211`). `TestCheckCapturesTheRequest` (`internal/update/update_test.go:348`) records the method, path, raw query, every header, the body and the cookies and fails on anything outside the allowed set; the desktop smoke re-asserts the same set, and the exact path, on every request the real window made (`harness/gates/desktop-smoke/update.ts:254-269`). No other host or path is reachable: the endpoint and release-prefix literals are the only http(s) strings in the package's non-test sources (local-only rule 4), they are pinned against `harness/lib/update-endpoint.ts`, and the stub refuses to stand in for anything else (`harness/lib/update-stub.ts:23-33`). The CLI links no HTTP client and no `internal/update` (local-only rule 2). |
| 3 | It tells the truth about versions | Every state has a Go test against an `httptest` server — `TestCheckStates` (`internal/update/update_test.go:69`) covers newer, equal, older, draft, pre-release, 404, 403 with `x-ratelimit-remaining: 0`, 429, 500, malformed JSON and an invalid tag, beside `TestCheckCapsTheBody` (`:228`), `TestCheckTimeout` (`:238`), `TestCheckStalledBodyTimeout` (`:254`), `TestCheckRefusedConnection` (`:276`) and `TestCheckRefusesRedirects` (`:315`) — and the desktop scenario walks the row through newer (version, date and both links), current (equal, older and pre-release; the ignored tag is never surfaced), no release (404), a malformed body, a rate limit, a stall past the launch's own 3 s timeout and the stub closed mid-request (`harness/gates/desktop-smoke/update.ts:193-227` expectations, `:293-330` assertions). The window renders one view per state (`desktop/web/src/views/settings.ts:143-176`) with the reason as a short clause (`desktop/web/src/core/i18n.ts:449-460`); study is never blocked (`:172-175`; the scenario checks Today still renders, `update.ts:329`). The version the compare uses is pinned: `TestVersionMatchesMygoJSON` (`update_test.go:451`) fails when `internal/update.Version` and `mygo.json` differ, and desktop ux compares the version the row shows with `mygo.json` (`harness/gates/desktop-ux.ts:807-808`). |
| 4 | It never changes the install | The scenario takes the data directory's recursive listing (names, sizes, mtimes) before the launch and after the exit and requires the only change to be the app's own boot database, any other add/change/removal named (`harness/gates/desktop-smoke/update.ts:118-160,237,284-293`). The newer view's two links are `target="_blank" rel="noopener"` and must carry the repository's release page built from the tag, so no URL from the response is used (`desktop/web/src/views/settings.ts:156-157`; `TestLinksComeFromTheRepository`, `update_test.go:407`); the scenario never clicks a link, so no browser opens on a runner, and `internal/update` has no file, store or download path (its imports are stdlib only, `update.go:12-22`). |
| 5 | The exception is enforced | local-only rules 1–4 (`harness/gates/local-only.ts:41-55,124-247`): the one allowed package and its allowed import (`:41-42`), the CLI's transitive dependencies free of `net/http`, `net/rpc`, `net/smtp` and `internal/update` (`:184-188`), only `internal/desktop` and `cmd/passquest-desktop` importing `internal/update` (`:44,165-166`), and the http(s) literals of the whole package subtree read with the `go/parser` tool `harness/tools/urllits` and pinned to the constant endpoint plus the release prefix (`:190-219`). Self-test cases fail for each stated reason — another package or a new `cmd/` package importing `net/http`, a `net/http` sub-package, the CLI importing `internal/update`, a package outside the desktop importing it, a foreign or drifted endpoint literal, two endpoint literals, an absent or unreadable literal file (`harness/gates/self-test/local-only.cases.ts:23,73,124,154,179,195,212,233,258,272,286,309,331,348`) — with accepted cases for the one allowed package, the desktop importer and a nested package with no literals (`:376,391,442`); the endpoint pin also runs as a self-test unit check (`harness/gates/self-test.ts:174-243`). Written down: [`../core-design.md`](../core-design.md) §6 and the Updates paragraph of [`../install.md`](../install.md#upgrade). |

## Known limits

- **No release is published.** Publishing needs the owner's go-ahead (Q-0013-07 keeps packages as
  7-day artifacts), so the live check against the real endpoint answers `no_release` (HTTP 404), and
  no test ever contacts the real endpoint: every state is a loopback stub (`httptest` in Go,
  `harness/lib/update-stub.ts` in the gates).
- **The browser hand-off is MyGo's documented default, not observed on CI.** With no
  `SetWindowOpenHandler`, MyGo 0.10.0 opens `target="_blank"` http(s) URLs in the system browser and
  denies everything else (MyGo 0.10.0 `window.go`, `docs/architecture.md`); the scenario asserts the
  `href`, `target` and `rel` and never clicks the link, so no runner opens a browser.
- **A learner behind a mandatory proxy gets "could not check".** The client sets `Proxy: nil` on
  purpose (Q-0015-02), so an environment proxy is ignored and the request either reaches the
  endpoint directly or fails as `offline`/`timeout`; the guide says so.
- **The CLI already links the raw `net` package** transitively — on Windows through
  `modernc.org/libc` → `golang.org/x/sys/windows`, on Linux through `modernc.org/libc` →
  `github.com/google/uuid` — and never calls it, so rule 2 proves only "no HTTP/RPC/SMTP client"
  (and no `internal/update`), not "no `net`".
- **Rules 1 and 3 read `go list` direct imports, test imports excluded, over `./...`.** A `_test.go`
  file importing a network package in another package, or a package in a directory `go list ./...`
  skips (for example `testdata/` or a `_`-prefixed directory), is not seen; rule 4 reads only files
  under `internal/update` in the working-tree list.
- **The data-directory listing cannot tell the check's writes from boot's.** Each launch starts on a
  fresh data directory, so the "before" listing is taken before the app creates its database, and the
  boot database is the one permitted change. A write by the check into that database would therefore
  pass the listing. The guarantee rests on the code instead: `Study.CheckForUpdate` runs through
  `begin(false)` and never opens the workspace (`internal/desktop/study.go`), and `internal/update`
  imports no store, file or first-party package.
- **The English UI strings are not pinned by a test.** The smoke and audit expectations accept either
  the Chinese or the English form of each text, so the assertions are satisfied by the Chinese
  strings alone and an English-only rewording passes.
- **The package smoke does not drive the update check.** The installed app's Settings renders the row
  (the credential case enters a key there), but no packaged launch presses **Check for updates**, and
  the browser hand-off is not exercised on any runner; the states are covered in a source checkout.
- **Windows local runs only for these gates.** macOS and Linux desktop behaviour is CI's evidence;
  the smoke-only overrides and the single endpoint are OS-independent by construction, but macOS was
  not run here.
