# 0022 — The window's title bar belongs to the Pass Quest interface: evidence

Contract: [`../goals/0022-integrated-title-bar.md`](../goals/0022-integrated-title-bar.md). Plan:
[`../plans/2026-11-24-1800-0022-integrated-title-bar.md`](../plans/2026-11-24-1800-0022-integrated-title-bar.md).
Decisions: Q-0022-01…09 in [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

**State: P1–P4 are merged on `main`** (P1 `b338c67` #55, P2 `5faaae4` #56, P3 `53e0ac8` #57, P4
`63803a1` #61); this record is P5's deliverable and the coordinator commits it with the goal close.
Every number below is read from the named run's own artifact or log line, downloaded per run id;
nothing is quoted from memory, and no number is compared across runs. Values in tables are the
tool's own log values where the log prints them and the record's exact fields where a row quotes a
record path. Each step's bench head is tree-identical to its merge (checked with `git diff --stat`:
`fdcbcc9`/`b338c67`, `a321c88`/`5faaae4`, `2d4553d`/`53e0ac8`, `be2744c`/`63803a1` all empty), so the
runs below measure the merged steps.

## The runs this record cites

| Run | What it is | A → B | Result |
|---|---|---|---|
| 45442487602 | P1's pull-request bench (the branch's successful bench run, attempt 3; the earlier head `4139694`'s failed run 45427302458 contributes nothing) | `5964d89` → `fdcbcc9` | both jobs `ok`; shots 7/7 identical and control 7/7 on both OSes; every metric `worse=false` and every control-pair verdict `worse=false`; guards 68 checked / 0 failed / 1 not measured; allowlist empty |
| 45533227954 | P2's pull-request bench | `b338c67` → `a321c88` | both jobs `ok`; Windows 0/7 identical, all seven allowed by its `os: "windows"` entries; macOS 7/7 identical; control 7/7 both; every metric `worse=false`; guards 68/0/1 |
| 45568297337 | P3's pull-request bench (attempt 1) | `5faaae4` → `2d4553d` | both jobs `ok`; macOS 0/7 identical, all seven allowed by its `os: "macos"` + `branch: "goal/0022-p3"` entries; Windows 7/7 identical; control 7/7 both; every metric `worse=false`; guards 68/0/1 |
| 45596813256 | P4's pull-request bench | `3a4faeb` → `be2744c` | both jobs `ok`; 7/7 identical on Windows and macOS (P4 changes Linux only); control 7/7 both; every metric `worse=false`; guards 68/0/1; the tree carries only 0021 P2's branch-scoped allowlist entry |
| 45596813363 | P4's pull-request gate run | — | all four group jobs green on all three OSes, including the three `desktop` jobs and the three `self-test` jobs quoted below |
| 45599616722 | `main`'s own gate run on `63803a1` (the final merged tree; P5 changes no code) | — | green on all three OSes; the three `desktop` jobs and the three `self-test` jobs carry the lines quoted below |
| 45610151815 | the goal-level bench (Q-0022-06), a dispatch from the throwaway `tmp/0022-goal-bench` | `4913932` → `0a5b688` (the throwaway branch = `63803a1` plus the fourteen entries) | both OS jobs `ok`; all seven states differ and are allowed per OS, no stale entry; control 7/7 both; every metric `worse=false`; guards 68/0/1 |
| P5's own bench | this step's pull-request `frontend-bench` run (A = P4's merge `63803a1`); read when the goal closes | `63803a1` → P5's head | the page is identical, so both OSes must read 7/7 identical with no finding and no stale entry |

P4's run measured `A = 3a4faeb` — `main`'s tip when the final bench was dispatched, i.e. P3's merge
plus five merges: 0020's P1 (`96a2a1f`), P2 (`61427d6`) and P3 (`94dd55c`), and 0021's P1 (`4a7528b`)
and P2 (`3a4faeb`) (0021 P2 changes the settings row's text; both sides of the run carry it), with
`B = be2744c` = A plus P4's own change. The plan's nominal A for P4 (P3's merge `53e0ac8`) differs by
those merges only; the window options and the page rules are identical in both.

The goal-level run's A is `4913932`, not the plan's own line (`A = 771a277`): `4913932` is that base
plus the docs-only close of goal 0019 (merge `#52`; `git diff 771a277 4913932` is five docs files and
no page file), the then-current `main` when the dispatch was prepared — recorded here because the
plan names `771a277`. Its A→B also carries the 0020 and 0021 merges above, so for the shots it means
the `settings-row` state differs by the title area **and** 0021 P2's endpoint-formats line (A
predates that merge): the line sits at y≈580–595 of the Windows job's
`frontend-bench-shots/shot-b-settings-row/settings-row.png` and is absent from the A side's own shot
(a pixel diff of the pair reads bands at rows 5–58 and 114–778).

## The method every bench run records

The `method` block of each record (quoted from `45596813256/frontend-bench-windows-latest`):
`samples: 9`, `warmup: 2`, `contentSize: "1280x800"`, `alternation: "A then B, launch by launch,
serially in this job"`, `control: "two interleaved sets of 9 launches of the B build, after the
comparison phase"`, and the verdict rule `head Q1 > base Q3 and the median gap >= the floor = max(5
ms, 5% of the base median, the larger of the two control-pair sides' IQRs) …` (Q-0019-02). Every run
of this goal reads `guards: 68 checked, 0 failed, not measured 1` per OS job; the shots are the seven
`SHOT_STATES` (`today`, `today-dark`, `sheet-open`, `practice-independent`, `feedback-coach`,
`settings-row`, `report`), one visible launch per state per side, compared byte-for-byte against
`harness/frontend-visual-allow.json` with the run's own control pair first. The runners and engines
each run read: `win25-vs2026@20261116.315.1` with `Edg/157.0.0.0` (Chromium 157) and
`macos26@20261117.0516.1` with `AppleWebKit/605.1.15` in every run of the goal; P4's
`linux-smoke` job reads `ubuntu26@20261116.202.1` and `Version/60.5 Safari/605.1.15`.

## Done when 1 — the title area is the app's

### The six cells (three OSes × two appearances, the today view at 1280x800)

All six are read from run 45596813256's artifacts (`B` = `be2744c` = the merged tree). Every cell's
own `geometry` row reads `cssWidth 1280`, `cssHeight 800`, `dpr 1`.

| OS | Appearance | Artifact (inside the run's artifacts) | Bytes | sha256 |
|---|---|---|---|---|
| Windows | light | `frontend-bench-windows-latest/frontend-bench-shots/shot-b-today/today.png` | 379498 | `736bc157ea90b752eae9926950d162a88a11e7bbde47308b40023dfb5d6a40f7` |
| Windows | dark | `frontend-bench-windows-latest/frontend-bench-shots/shot-b-today-dark/today-dark.png` | 540401 | `14c00f3a52a9c8a007b11f5c726c7d673a781580f0e52f22ac47ddd79ed13a61` |
| macOS | light | `frontend-bench-macos-26/frontend-bench-shots/shot-b-today/today.png` | 800632 | `5b1e338b3dad4dcbc75faca9206bb496ddda931c0189358bb708bd56c0b407dd` |
| macOS | dark | `frontend-bench-macos-26/frontend-bench-shots/shot-b-today-dark/today-dark.png` | 955228 | `0ad0b4c7fa28fe20c7c02103a216fe17468714962263747b0131f5c7211024ab` |
| Linux | light | `frontend-bench-ubuntu-smoke/frontend-bench-smoke-shots/smoke-today/today.png` | 179411 | `3e8a0054624e8ef97214e71a263ae9abb238537bc9963ac717b6687c357b4a7c` |
| Linux | dark | `frontend-bench-ubuntu-smoke/frontend-bench-smoke-shots/smoke-today-dark/today-dark.png` | 396391 | `36f9f171c1aff95ce8115ecd0245af84505617b8fe067c2b8057330aada8c1ef` |

The Windows and macOS cells are the bench's visible launches; the Linux cells are the
`ubuntu-latest render smoke` job's visible WebKitGTK launches under Xvfb, all seven states written
per launch (`frontend bench: shots 7/7 written` in its log). The macOS job's display is
`displayplacer`'s scaling:off mode (its log reads `displayplacer: the display is 1920x1080 points at
density 1`), and the Linux screen is Xvfb's `1920x1080x24` (the workflow's own `xvfb-run -a -s
"-screen 0 1920x1080x24"`).

### The room each OS reports, hidden and shown

`height/left/right/bottom` in CSS px; the hidden readings are the `desktop` jobs of run 45599616722
(the merged tree), the shown readings are the shot launches' `titlebar` rows of run 45596813256.

| OS | Hidden (boot + 880x600 walk) | Shown (shot rows) | Row |
|---|---|---|---|
| Windows | `40/0/138/40`, marker true | `40/0/138/40`, marker true | 138 = 3 × 46 DIP, MyGo's caption strip |
| macOS | `32/69/0/23`, marker true | `32/69/0/23`, marker true | AppKit's own band; **a hidden window reads it too** |
| Linux | `40/0/207/40`, marker true | `40/0/207/40`, marker true | the right header bar's allocated width; `height` is the app's own `TitleBarHeight` request (Q-0022-04 amended) |

The title-button-layout step of the two Ubuntu jobs — run 45599616722's `desktop` job (the hidden
room) and run 45596813256's `linux-smoke` job (the shown one) — logs the runner's GNOME key
`gsettings get org.gnome.desktop.wm.preferences button-layout` = `'appmenu:close'`; the runner has no
GNOME session, and the room above is GTK's own measurement of the bars it built. On this machine the final build
(shown, 1280x800, dpr 1.25) read `--mygo-titlebar-height 40px`, `inset-left 0px`, `inset-right
138px`, `controls-bottom 40px`, marker true — the same shape as the Windows CI reading. Full screen
reads `0px` with the marker false (the plan's fact 3 probe; re-measured on this machine, below).

### The four title-area checks

Every `desktop` job of run 45599616722 reads `PASS  desktop ux (192 audits over 24 states × 4 modes ×
2 sizes, 19 probes, 84 focus checks, 0 allowed …)`; the titlebar segment each job prints:

| OS | Room, audits, marker | Drag samples | Skipped modal-layer | Probe | Walk at 880x600 |
|---|---|---|---|---|---|
| Windows | `room 40/0/138/40 px x192 at 192/192 audits, marker 192/192` | 10944 counted | 3648 | 8,20 | `room 40/0/138/40 px x14 at 14 stops, marker 14/14` |
| macOS | `room 32/69/0/23 px x192 at 192/192 audits, marker 192/192` | 11664 counted | 3888 | 69,16 | `room 32/69/0/23 px x14 at 14 stops, marker 14/14` |
| Linux | `room 40/0/207/40 px x192 at 192/192 audits, marker 192/192` | 10368 counted | 3456 | 8,20 | `room 40/0/207/40 px x14 at 14 stops, marker 14/14` |

Each job's `skipped states` reason is the modal-layer one (`48x the top layer takes every strip
sample while a modal is up: the strip is not observable in-page here`), and no job reports a
`titlebar-room-covered`, `titlebar-room-paint`, `titlebar-drag-region` or `titlebar-strip-clip`
finding. In P1's run (45442487386) all three jobs read the checks' zero form instead — `no title bar
at 192/192 audits (marker 0/192), the four checks run-and-skipped: 192x no title bar: every
--mygo-titlebar-* variable is 0px or unset` — and the 880x600 walk `room 0/0/0/0 px x14 at 14 stops,
marker 0/14`, which is the checks' green-on-today form the step required.

### The caption buttons (Q-0022-02, Q-0022-09)

- The bound method (`Study.SetWindowButtonsTheme`) is called at boot and on the page's theme toggle;
  the boot scenario calls both sheets and asserts the call answers (`boot: the title buttons' theme
  call failed: …` green in all three `desktop` jobs of run 45599616722, so GTK parsed both sheets on
  Ubuntu).
- On this machine the OS-level captures of the real shown window (below) read dark glyphs on the
  light page and light glyphs on the dark page, with the restore glyph after a maximize — MyGo's
  caption window following the app's theme.
- The active/inactive glyph palette and the Snap flyout cannot be produced on this machine (no input
  desktop); they are owner actions, recorded in the manual pass.

### The manual pass (Q-0022-05)

Machine: Windows 11 `10.0.26300`, `SM_CXSCREEN` 2048x1152 (the session reports a 2560x1440 virtual
screen), page `dpr 1.25` read by the launch. App: `bun run desktop:build` from this branch's tree,
shown at a 1280x800 page area (`PASSQUEST_DESKTOP_CONTENT_SIZE`), a fresh data directory and the
loopback stub registry per launch. Captures: `PrintWindow` (`PW_RENDERFULLCONTENT`) of the real
window — the OS's own composition of the window, its frame area and MyGo's caption child — saved
under `Q:/scratch/omp/g22/P5/manual/`, with each launch's script result beside them.

Driven, with what was seen (capture file names are inside that directory):

| State | What was seen |
|---|---|
| Light, shown at 1280x800 (`m01-light-window.png`, 1618x1010 = the page area at dpr 1.25 plus the invisible borders) | the page's own title area at the top; three caption glyphs at the top-right, dark and legible on the light page; a single-square maximize glyph; the util group to their left with clear space (no control under the buttons) |
| Page theme switched through the app's own control (the same `[data-act="theme"]` click the ux gate's `applyMode` uses) | the launch's result reads `theme dark`; the caption glyphs repaint light (`m10-dark-window.png`), legible on the dark page; the util group stays clear |
| Maximized through `window.mygo.window.toggleMaximize()` | rect `501,215 1618x1010` → `51,-9 2518x1458`; `m11-maximized.png` shows the two-square restore glyph and the wider layout clear of the buttons; the script reads `isMaximized true` |
| Full screen through `toggleFullScreen()` | rect `0,0 2560x1440`; the script reads `--mygo-titlebar-height 0px` and marker false; `m12-fullscreen.png` shows the page filling the screen with no caption glyphs |
| Leaving full screen | rect back to `501,215 1618x1010`; the script reads `--mygo-titlebar-height 40px` and marker true again (`m13-restored.png`) |

Not drivable on this machine, with the evidence: the session delivers no pointer input —
`SendInput` returns 0 with `GetLastError` 5 (`ERROR_ACCESS_DENIED`), `SetCursorPos` leaves
`GetCursorPos` at `0,0`, a launch whose page listened for `mousemove`/`mousedown` recorded none
(`listen.result.json` `"seen":[]`) although the same launch renders, `GetForegroundWindow` is 0 and
`SetForegroundWindow` returns false, and desktop captures are black while the per-window
`PrintWindow` renders. The double-click attempt left the window rect unchanged
(`m03-dblclick-max.png` / `m04-dblclick-restore.png`), which is this fact, not a window defect.
Re-checked in P5's fix round (2026-11-25): unchanged — `SendInput returned 0 (last error 5); cursor
now 0,0` and `GetForegroundWindow` 0.

Owner actions this machine cannot do (each a few seconds on a machine with input):

- **Windows:** hover the maximize button — does Windows 11's Snap layouts flyout appear (Q-0022-09's
  assumption)? Double-click the title area (maximize, then restore); drag it (the window moves);
  resize from the top edge (the top 8 DIP band) and from both sides; minimize,
  maximize and close with the caption buttons and restore from the taskbar; right-click the title
  area (does the system window menu appear? if not, that limit stands); click the desktop for the
  inactive look in both appearances (the caption glyphs grey; the drawn strip does not react — the
  plan's recorded limit).
- **macOS (a Mac with macOS 26/27):** the traffic lights stay at the top left over the page and no
  header control sits under them (the owner decision of 2026-11-25, Q-0022-03); the lights in both
  app appearances and inactive; drag and double-click the drawn title area (zoom per the user's
  `AppleActionOnDoubleClick`); resize from every edge; minimize and close with the lights; full
  screen hides the lights with the menu bar and brings them back.
- **Linux (a CSD desktop, GNOME/X11):** the GTK title buttons' glyphs follow the page's appearance in
  both (the `Theme.SetGTKCSS` sheet) and inactive; drag, double-click, maximize/restore, minimize
  and close; resize from the top edge and from both sides.
  On a compositor that enforces server-side decorations (KWin, Sway, Hyprland): the disclosed
  Q-0022-04 limit — the compositor keeps its own bar and the page reserves nothing.

## Done when 2 — window behaviour is intact

- The 880x600 walk (the minimum window): the three lines in the room table's last column above, from
  run 45599616722's `desktop` jobs, with the marker matching the room at all 14 stops and no finding.
- The ux matrix at the two audited sizes (1280x800, 1440x844): 192 audits per OS job, 19 probes, 84
  focus checks, 0 allowed, the titlebar segment in the table above.
- The synthetic pointer (the bridge's takeover): asserted in the `titlebar` scenario of every
  `desktop` job of run 45599616722 — green on Windows and macOS by the job's own green (the
  double-click press at the strip's derived probe is taken over and moves `isMaximized` true, then
  false; the theme control's press is not taken over and the control still switches the theme), and
  on Linux the takeover is asserted with the maximize reading *recorded*: `titlebar: maximize reading
  recorded, not asserted (no window manager under CI's Xvfb, Q-0022-04): false -> true -> false`. The
  ux jobs' `probe` column above is the same derived probe point (Windows/Linux 8,20; macOS 69,16).
- The manual-pass limits above are the honest remainder: the real pointer, the top-edge resize, the
  Snap flyout and the inactive look are owner actions.

## Done when 3 — keyboard, assistive use and the window title

- `desktop-ux`'s accessibility checks are unchanged: each OS job of run 45599616722 reads the same
  `192 audits … 19 probes, 84 focus checks, 0 allowed` line as the 0019 record's final run.
- The window title is read back in the `boot` scenario at three stops (Today, `#/settings`, back),
  eight checks per launch: the read equals the page's `document.title`, starts with `Pass Quest — `,
  and `document.title` changes and returns with the route. Run 45599616722's `desktop` jobs are green
  with those checks inside their `584` (Windows), `583` (macOS) and `581` (Linux) assertions.
- The read path per OS: on Windows the smoke-only service reads the OS title through the window's
  HWND (`GetWindowTextW`); on macOS and Linux it returns MyGo's cached title, because reading
  `[NSWindow title]`/`gtk_window_get_title` back needs a cgo or Objective-C binding this project does
  not carry — the disclosed limit in [`../harness.md`](../harness.md) row 24. On this machine the
  final build read back `Pass Quest — 今日`, `Pass Quest — 设置` and `Pass Quest — 今日` at the three
  stops, each equal to `document.title` (scratch launch, `Q:/scratch/omp/g22/P5/manual/title.result.json`).

## Done when 4 — no regression in speed or stability

Each step's run, read from its own record (`N = 9`, warm-up 2, the control pair's verdict first):

| Step | Run | A → B | Windows | macOS |
|---|---|---|---|---|
| P1 | 45442487602 | `5964d89` → `fdcbcc9` | 13 metrics `worse=false`; shots 7/7 identical; control 7/7 | same |
| P2 | 45533227954 | `b338c67` → `a321c88` | 13 metrics `worse=false`; shots 0/7, all seven allowed; control 7/7 | 13 metrics `worse=false`; shots 7/7 identical; control 7/7 |
| P3 | 45568297337 | `5faaae4` → `2d4553d` | 13 metrics `worse=false`; shots 7/7 identical; control 7/7 | 13 metrics `worse=false`; shots 0/7, all seven allowed; control 7/7 |
| P4 | 45596813256 | `3a4faeb` → `be2744c` | 13 metrics `worse=false`; shots 7/7 identical; control 7/7 | same |

No metric's verdict or control-pair verdict reads `worse=true` in any of the eight OS jobs, so no run
is unreadable and none needed a second draw. The metrics are `readyMs`, `answerSelectMs`,
`submitFeedbackMs`, `coachReplyMs`, the four `roomSwitch.*`, `frames.median`, `frames.p95`,
`frames.longest`, `bundle.mainJs`, `bundle.mainCss`. `bundle.mainJs` grows in P2 by 488 bytes
(`186908 → 187396`, `worse=false`: Q1 not above the base Q3 and the gap within the floor) — the
title-area module and the client entry — and stays flat in P3/P4.

## Done when 5 — the reference shots are updated honestly

- The per-OS bench artifacts and entries above are the updated reference: the six cells of Done-when
  1 are the new head's own shots, and every difference is allowlisted per OS in the step's own run
  (P2's seven `{state, os: "windows", reason: "the title area the page draws under the hidden title
  bar (0022 P2)"}` entries; P3's seven `{state, os: "macos", branch: "goal/0022-p3", reason: "…(0022
  P3)"}` entries), each read from the branch's own file at its head commit and from the run's
  `shots.allowed` rows. P4's run reads both OS columns 7/7 identical, and the merged tree's allowlist
  holds only 0021 P2's `branch: "goal/0021-p2"` entry, inert in every other run.
- The 0012 contact sheets are **not regenerated**. Their method is MyGo's automation (e2e) build of
  `cmd/passquest-desktop` plus the state walk and 2x2 composer that goal 0012's own record documents
  ([`0012/README.md`](0012/README.md)) and that this repository does not carry — nothing under
  `harness/` or `internal/` takes a contact sheet, and the `design/` tree holds only prototypes (its
  `shoot.sh` shoots the prototype HTML files, not the app), so it is not a source for the sheets.
  Q-0022-07's fallback is therefore invoked: the six mechanical cells above keep Done-when 1, the
  sheets keep showing commit `354f08c` and their README now says so with a dated note (added in P5's
  fix round), and regenerating them for the final head is an owner/coordinator action (the per-sheet
  change list is due with that regeneration). `README.md` and `README.zh-CN.md` embed the same
  pre-0022 `feedback-coach.webp`, stale with the sheets, and are replaced when the sheets are
  regenerated.

## Known limits

- The minimum width is 880 px while Microsoft recommends at most 500 effective px for snap zones, so
  a narrow snap zone can refuse the window (the plan's disclosed limit; not this goal's to change).
- A Wayland compositor that enforces server-side decorations keeps its own bar and the page reserves
  nothing (Q-0022-04); no first-tier Linux desktop is in the CI matrix.
- CI's Ubuntu runner has no window manager: `gtk_window_maximize` is the WM's business, so the
  maximize reading is recorded, not asserted (Q-0022-04), and the reading itself was
  `false -> true -> false` in run 45599616722.
- The right-click window menu: nothing in the app draws one; whether the shell shows one is checked
  only by the owner action above (Q-0022-05, Q-0022-09).
- On macOS and Linux the title read-back returns MyGo's cached title, not `[NSWindow title]` or
  `gtk_window_get_title` ([`../harness.md`](../harness.md) row 24); the three-stop assertion is the
  evidence either way.
- macOS 27 is not a runner; the macOS cells are macOS 26 (`macos26@20261117.0516.1`, Safari 26.x).
- No CI job captures the frame or the native buttons at the OS level (`CapturePage` shows the page
  only); that half is the manual pass above (Q-0022-05).
- The manual pass on this machine could not drive the real pointer (evidence above); the Snap
  flyout, the real drag/double-click/top-edge resize, the minimize/restore and the inactive look
  remain owner actions.
- The macOS and Linux manual items are owner actions where a machine of that OS is available; the
  record says which cells they cover.
- The goal-level bench's allowlist entries live only in the throwaway dispatch branch
  (`tmp/0022-goal-bench`, tip `0a5b688`, still on the remote at this record's writing); the merged
  tree's allowlist keeps only 0021 P2's branch-scoped entry.

## The two coordinator dispatches

**P5's own bench** — this step's pull-request `frontend-bench` run (A = `63803a1` = P4's merge, B =
this branch's head), read when the goal closes; the page is identical, so both OSes must read 7/7
identical with no finding and no stale entry. `docs/**` is outside the workflow's `paths`, so the
run is dispatched on this branch's ref:

```bash
gh workflow run frontend-bench.yml -R wxxb789/passquest --ref goal/0022-p5 -f a-ref=63803a1
```

(No `b-ref`: an empty one means B is the dispatched ref itself, and the workflow's ref check rejects
a non-commit string.)

**The goal-level comparison (Q-0022-06)** — the throwaway branch carries the final head plus fourteen
entries and is never merged; the dispatch passes no `head_ref`, so entries without `branch` are the
ones that apply, and the bench reads the allowlist from B's tree, which is why B is the throwaway
branch's own commit:

```bash
# 1. On a throwaway branch from the goal's final head, write harness/frontend-visual-allow.json
#    with exactly these fourteen entries (no `branch`): today, today-dark, sheet-open,
#    practice-independent, feedback-coach, settings-row and report, once under "windows" and once
#    under "macos":
#
#      { "version": 1, "entries": [
#        { "state": "today",                  "os": "windows", "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "today-dark",             "os": "windows", "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "sheet-open",             "os": "windows", "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "practice-independent",   "os": "windows", "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "feedback-coach",         "os": "windows", "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "settings-row",           "os": "windows", "reason": "the title area the page draws under the hidden title bar and the settings legend's endpoint-formats line (0021 P2) (goal 0022, goal-level comparison)" },
#        { "state": "report",                 "os": "windows", "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "today",                  "os": "macos",   "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "today-dark",             "os": "macos",   "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "sheet-open",             "os": "macos",   "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "practice-independent",   "os": "macos",   "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "feedback-coach",         "os": "macos",   "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" },
#        { "state": "settings-row",           "os": "macos",   "reason": "the title area the page draws under the hidden title bar and the settings legend's endpoint-formats line (0021 P2) (goal 0022, goal-level comparison)" },
#        { "state": "report",                 "os": "macos",   "reason": "the title area the page draws under the hidden title bar (goal 0022, goal-level comparison)" }
#      ] }
#
# 2. Commit and push the throwaway branch (never merged).
gh workflow run frontend-bench.yml -R wxxb789/passquest --ref tmp/0022-goal-bench -f a-ref=4913932
#    (b-ref stays empty: B is the dispatched ref itself, i.e. the throwaway branch's commit.)
#    Expected: both OSes differ in all seven states, every difference allowed by its os entry, no
#    stale entry; metrics judged first against the run's own control pair.
# 3. Delete the throwaway branch.
```

Run as 45610151815 on 2026-11-25 (`tmp/0022-goal-bench` = `63803a1` + this file: one commit ahead,
`harness/frontend-visual-allow.json` only, +67/−3): A `4913932` → B `0a5b688`, both OS jobs `ok`,
`shots: 0/7 state(s) byte-identical` with all seven `allowed:` lines on both OSes (no stale entry),
control 7/7 both, every metric `worse=false`, guards 68/0/1; the Ubuntu render smoke reads
`40/0/207/40` at all seven states and `shots 7/7 written`. The dispatched `settings-row` entries'
reason named the title area only — the entry is a state×OS allow, so the run passed — while that
state's difference also carries 0021 P2's line (verified above); the two `settings-row` reasons here
name both for a re-dispatch.

## P5's local checks

Run on this branch's tree on the coordinator's Windows machine; each line is the gate's own pass
line.

| Check | Result |
|---|---|
| `bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | `PASS 376 checked, 0 broken` |
| `bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | `PASS 814 files, 0 findings` |
| `bun Q:/scratch/pq-harness-run/run.ts install-guide.ts installGuide` | `PASS 7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; the release notes name the tap command and both guides; the cask template renders v0.15.0 with the release's SHA-256, and a synthetic pair, into Casks/pass-quest.rb; floor 26.0 from mygo.json` |
| `bun Q:/scratch/pq-harness-run/run.ts typecheck.ts typecheck` | `PASS 0 type errors in tsconfig.json, desktop/web/tsconfig.json` |
| `bun Q:/scratch/pq-harness-run/run.ts frontend-baseline.ts frontendBaseline` | `PASS ES2024 target/lib pinned, README carries both, all 4 allowlist entries fire on their own side; 6 vendor token(s) in the source, 6 in the bundle` |
