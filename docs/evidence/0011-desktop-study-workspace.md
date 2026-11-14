# 0011 — Desktop study workspace: evidence

Contract: [`../goals/archive/0011-desktop-study-workspace.md`](../goals/archive/0011-desktop-study-workspace.md).
Plan: [`../plans/2026-11-14-1600-0011-desktop-study-workspace.md`](../plans/2026-11-14-1600-0011-desktop-study-workspace.md).
Domain: [`../desktop.md`](../desktop.md).

## Environment

Windows 11 Enterprise (10.0.26300), WebView2 runtime 157.0.4341.60, Go 1.27.4, Bun 1.4.4, Node 26.13.1,
MyGo v0.9.0. Linux paths were additionally exercised in WSL Ubuntu 24.04 (WebKitGTK 4.1 under `xvfb-run`,
gnome-keyring inside `dbus-run-session`). macOS was not run locally; CI is its only evidence.

## Gate runs

| Command | Result |
|---|---|
| `bun run gate` (final tree, Windows) | 21 passed, 2 failed, 0 skipped, 7 m 30 s. Failed: `agent smoke`, `provider smoke` — see below |
| `bun run gate` on `main` before any change (`1f409b5`), same machine | 20 passed, 2 failed: the same two gates with byte-identical failure lines |
| `go test` gate | 22 packages, 830 tests, 0 skipped (786 before this goal) |
| `typecheck` | 0 errors in `tsconfig.json` and `desktop/web/tsconfig.json` |
| `local-only` | 21 first-party packages (incl. `cmd/passquest-desktop`), 0 network imports, 0 confidential markers; `go list -deps ./cmd/passquest` contains no `net/http` |
| `desktop smoke` | 239 assertions over 11 scenarios (four full local runs: 78–94 s each) |
| `desktop smoke` in WSL Ubuntu 24.04 (dbus + gnome-keyring + xvfb) | pass twice, before the fresh-item scenario rewrite (223 assertions, 46–64 s) |
| `harness self-test` | 44 cases (2 must pass) over 22/22 gates |

**Why two gates fail locally.** This machine's npm feed (`packagefeedproxy.microsoft.io`) does not yet carry
the locked Pi Durable packages (`@earendil-works/*@1.5.1`, `@aws-sdk/core@3.980.2`, … — 24 packages; the feed's
newest are 1.5.0 / 3.980.1), so `agent/node_modules` is incomplete and the durable host cannot start. The lock
file was not changed. Both gates fail identically on unmodified `main`; CI (`bun install --frozen-lockfile`
from the public registry on ubuntu, macOS and Windows) is the evidence for them.

## Acceptance → evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | The app starts and renders real study state without the CLI | `desktop smoke` `boot`: hidden window on a fresh data directory, `window.__pq.ready`, non-blank `#stage`, no error bar; Today is built from bound calls only |
| 2 | Practice is real evidence | `practice`: keyboard answers on a guided item (assisted), a fresh skill item (independent, counted) and a tactic item on the already-read passage (re-exposure, not counted); for each, the condition shown before answering equals the feedback condition and `passquest report --json` in a second process (`elapsed_ms > 0` equal to the window's, `started_at` = presentation time); `next --json` counts exactly the one independent attempt |
| 3 | Hints turn an attempt assisted | `hint`: a fresh countable item, hint gate requires confirmation, condition flips to assisted with the same token and presentation time, report shows it assisted and not counted |
| 4 | Feedback is the verified truth | `feedback`: verified key and recorded status in the DOM, every option explanation or its missing state, the evidence sentence highlighted; Go tests `TestWorkspace*` cover the DTOs |
| 5 | AI help is advisory and consent-gated | `coach` against a loopback OpenAI-compatible stub and the real Node tutor host: call strip `data-sha`/`data-bytes` equal the raw body received; a reply containing markup renders as literal text with no element created; imported text withheld before consent and included after a grant in the window; a 401 shows `auth_failed` and `report --json` is byte-identical across the failed call |
| 6 | Assessment and report work in the window | `assess`: generated reviewed source (23-item pool), start, answer, leave, relaunch, resume the same item at the same presentation time, finish; `pass threshold: unknown`; navigator cells only `pending`/`presented`/`answered`/`timed_out`/`unavailable`; agrees with `passquest assess report`. `TestWorkspaceAssessmentWindowElapseAbandons` proves an item on screen when the run window passes is recorded timed out, as in the CLI |
| 7 | Anki flow works in the window | `anki` against a loopback AnkiConnect stub: propose, edit (preview hash changes, marked edited), accept, push exactly one `addNote` carrying the edit; an unreachable endpoint queues the next note |
| 8 | Credentials stay secret | `settings`: a generated key entered in the window is stored under a per-run credential service, shown only as `present`, reaches the stub on the next call with no environment variable set, and is absent from SQLite and data files, result files, app output and the DOM text/HTML; cleared in the window, service entry deleted. `TestWorkspaceLegacyCustomCredentialBeatsStoredKey` proves an environment variable still wins |
| 9 | Interruption is recoverable | `resume`: the app is killed mid-item, relaunched to the same token and presentation time (read from SQLite before the kill), no attempt written |
| 10 | Lifecycle is clean | every launch: no listening TCP socket owned by the app or its descendants while running, no surviving child after exit. `internal/hostproc` + `TestHostRunnerHostAndItsChildrenEndWithHostproc` (POSIX; run in WSL) prove a durable host and its grandchild are reaped on quit; Windows uses a kill-on-close job object |
| 11 | Nothing headless regresses | all earlier gates pass with their documented counts (or fail identically to `main` for the environment reason above); CLI goldens unchanged (`golden contract`); `local-only` covers the desktop's first-party packages |

## Manual verification in the real window

Every route was driven in the real window on a seeded database, light and dark, Chinese and English
(28 screenshots, kept outside the repository by the operator). Checked by hand: the C-Stage layout and rooms
(independent cool, assisted warm, assessment still), feedback depth layering, the coach credential strip,
1280×800 layout, the empty, shortfall and failure states.

## Review

A five-lens review (evidence semantics, security, harness integrity, platform/lifecycle, learner contract)
produced 21 findings; independent skeptics refuted 1 and confirmed 20 (1 P1, 4 P2, 15 P3). All 20 were fixed
with regression checks; the P1 (Node durable host surviving window close on macOS/Linux) is fixed by
`internal/hostproc`. One residual is recorded as Q-0011-13 (CLI `next` during an active run, unchanged).

## Known limits

- macOS is evidenced only by CI; the Keychain path there assumes the runner's unlocked login keychain.
- The smoke drives the window by in-page script, not a human; long imported pages were judged by hand.
- Notes accept only an all-provider consent (Q-0011-12); "keep Anki's version" of a conflict is not offered
  (Q-0011-07).
- A `desktop:build` binary started outside the repository finds the tutor, Anki and search hosts copied beside
  it, but the durable brief still needs the workspace's `node_modules`; packaging is goal 0013.
- The Windows binary is a console program (Q-0011-14).
