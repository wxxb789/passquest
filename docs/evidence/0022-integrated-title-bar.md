# 0022 — The window's title bar belongs to the Pass Quest interface: evidence

Contract: [`../goals/archive/0022-integrated-title-bar.md`](../goals/archive/0022-integrated-title-bar.md). Plan:
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
- The 2026-11-26 sitting below captured the hover backplate on the maximize button in both
  appearances (2 856 and 2 852 differing pixels, the button's own box only) and the inactive
  drawing's presence in the dark appearance; the Snap layouts flyout did **not** appear on hover in
  either appearance, and a standard Win32 window's own maximize button, hovered as a control,
  showed no flyout either (the hit-test codes and the control are in the manual pass below).

### The manual pass (Q-0022-05)

Machine: Windows 11 `10.0.26300`, one 2560x1440 desktop at 125% — each launch of this sitting reads
`SM_CXSCREEN` 2560x1440, screen 2560x1440 and work area `60,0`…`2560,1440`, and the P5 sitting's
`2048x1152` is 2560 ÷ 1.25 (inference from the ratio: the same screen read DPI-virtualized) — page
`dpr 1.25`. Second sitting, 2026-11-26, in the owner's RDP session, **whose pointer input works**; the
2026-11-25 sitting's no-input evidence is kept at the end of this section. App: the published
v0.15.2 package, already installed on this machine — the uninstall entry
`com.github.wxxb789.passquest` reads `DisplayVersion 0.15.2`, the install directory is
`%LOCALAPPDATA%\Programs\Pass Quest` with NSIS's own `Uninstall.exe` (its `QuietUninstallString`
carries `/S`) and the package's `agent/`/`node/` trees, and the release asset
`pass-quest-setup-0.15.2-amd64.exe` (sha256
`ab7c7bde1256f64aaa537ca72e07dcb4e1e09d54069ba1667b8b99f6f8e7fa`, equal to the release's
`SHA256SUMS.txt`) sits under `Q:/scratch/omp/g22/close/install/`; `docs/install.md` documents the
installer as interactive only, so no silent install is claimed. Every launch below is that
executable with `PASSQUEST_DESKTOP_CONTENT_SIZE=1280x800`, a fresh data directory, the loopback stub
registry and the smoke hook's script; the window reads outer `501,215 1618x1010`, client
`1600x1000` at scale 1.25 — the 1280x800 page area — and the page's own log reads `marker=true`,
`room 40px///`, `theme=light` at boot.

**Real input this sitting.** `SendInput` returns 1 and the cursor lands on the intended point
(`cap.ps1`; every drive JSON's `pointer` rows read `sendInput: 1`, `atTarget: true`), `SetCursorPos`
and `GetCursorPos` agree, screen `BitBlt` captures hold real pixels (the light and dark pages read
`241,244,248` and `9,15,30` at the same point), and `GetForegroundWindow` follows the driven
clicks.

Captures are under `Q:/scratch/omp/g22/close/manual/` with one JSON per launch beside them
(`run-light.json`, `run-light2.json`, `run-light3.json`, `run-tap.json`, `run-last.json`,
`run-dark.json`, `run-dark2.json`, `run-dark3.json`, `run-dark4.json`, `run-control.json`,
`run-c.json`; the page's own log in the matching `*.result.json`). Driven, with what was seen:

| Item | Appearance | What was seen |
|---|---|---|
| Hover the maximize button for 1.5 s and 3 s — does Windows 11's Snap layouts flyout appear? | light | the pointer to screen `2024,241` (css `1211,20`; the maximize button — MyGo's three 58 px buttons fill css 1140.8–1280 of the strip): `topLevelBefore` = `topLevelAt1500ms` = `topLevelAt3000ms`, so no flyout window, menu or XAML island appears, and the at-rest/hover capture pair differs in the maximize button's own box only (2 856 px there; the full-window diff's whole extent is `1496,4`–`1548,48`, the button — `z01-light-at-rest-window.png` / `z02-light-hover-1500ms-window.png`); the light launch's screens `l02-hover-max-1500ms-light-screen.png`, `l03-hover-max-3000ms-light-screen.png` and `l04-hover-left-light-screen.png` show the button's backplate and nothing else |
| Hover the maximize button (same) | dark | the same: no new top-level window at 1.5 s or 3 s (`run-last.json`, `run-dark4.json`), the pair differs in the same box (2 852 px in that region; the full-window diff adds 79 px on the theme control, the earlier click's hot state — `z03-dark-at-rest-window.png` / `z04-dark-hover-1500ms-window.png`), screens `z04-dark-hover-1500ms-screen.png`, `d03-hover-max-1500ms-dark-screen.png`, `d04-hover-max-3000ms-dark-screen.png`, `d05-hover-left-dark-screen.png` |
| Double-click the drawn title area (css `400,29` → screen `1010,252`), then again | light | first: `IsZoomed` false → true, rect `501,215 1618x1010` → `51,-9 2518x1458` (`l05-dblclick-max.png`, `l05-dblclick-max-screen.png`); second: true → false, rect back to `501,215 1618x1010` (`l06-dblclick-restore.png`); the page's own log reads `max=false` → `true` → `false` with `innerWidth` 1280 → 2000 (`tap.result.json`) |
| Drag the title area by (200,100) | light | rect `501,215` → `701,315`, delta exactly `{x:200, y:100}` (`l07-drag-after.png`) |
| Resize from the top edge (css `400,3`, the strip the window answers `HTTOP` for) | light | rect `701,315 1618x1010` → `701,455 1618x870` (top +140, height −140) (`l08-resize-top-after.png`) |
| Resize from the left side (rect.left+4) and from the right side (rect.right−5) | light | left: `701,455 1618x870` → `821,455 1498x870` (left +120, width −120, `l09-resize-left-after.png`); right: → `821,455 1378x870` (width −120, `l10-resize-right-after.png`) |
| Minimize with the button, then restore from the taskbar | light | css `1165,20` → screen `1966,241`: `IsIconic` true (`l21-minimized-light-screen.png`); the taskbar's own element `Pass Quest - 1 running window` at `30,1024` clicked: `IsIconic` true → false, rect back to `501,215 1618x1010` (`l22-restored-window.png`, `l22-restored-screen.png`) |
| Close with the button | light | css `1257,20` → screen `2081,241`: the process is gone 500 ms later, `IsWindow` false, `processGone: true` (`c01-before-close.png`, `c02-after-close-screen.png`) |
| Right-click the drawn title area — does the system window menu appear? | light | the click lands (`foregroundAtMenu: true`), **no menu and no new top-level window appears**, and the window's own capture changes by 79 pixels, all inside the theme control's 36x36 box (its focus state) (`l31-rightclick-light-screen.png`, `l31-rightclick-light-window.png`; `l11-rightclick-light-*` with the window then at `821,455 1378x870`) |
| Right-click the drawn title area (same) | dark | the same outcome: no menu, 79 pixels changed, all in the theme control's box (`z05-rightclick-dark-window.png`, `z05-rightclick-dark-screen.png`, `run-last.json`) |
| The inactive look — click away so the app loses the foreground | light | this desktop has **no bare-desktop point**: the app is surrounded by the owner's Edge, Explorer and MarkTurbo windows and a scan of the work area outside the app's rect finds no `Progman`/`WorkerW`/`SysListView32` point (`probe.json` `freeDesktop: null`), so the click that takes the foreground went to the empty part of the 60 px taskbar strip (`30,60`, class `Shell_TrayWnd`); after it `GetForegroundWindow` is not the app (`appActive: false`) and the app's window is **pixel-identical** to its active look (0 differing pixels over the whole window rect in the screen pair `l31` → `l32-inactive-light-screen.png` and in the window pair `l30-fresh-window.png` → `l32-inactive-light-window.png`); a click back on the drawn title area activates it again (`l33-reactivated-light-window.png`, `foreground: true`) |
| The inactive look (same) | dark | the same taskbar click deactivates it (`appActive: false`, `run-dark.json`); the inactive window capture `d07-inactive-dark-window.png` differs from the active dark one (`z03-dark-at-rest-window.png`) by 490 pixels along the title strip's top line and over the caption buttons — MyGo's `!active` glyph palette (`foreground.a` 0x72) reads in the dark appearance and not in the light one above |
| The page's own appearance control, clicked for real (needed for the dark rows) | both | a real click at css `1060,29` → screen `1835,252` switched the page: `tap.result.json`'s log reads `theme=light` → `dark` +0.53 s and back +0.37 s after that launch's two clicks; the same click worked in `run-dark` (dark by +2.6 s), `run-dark4` (log `theme=dark` through its end) and `run-last` (page pixels `9,15,30` in `z03-dark-at-rest-window.png`). In one launch (`run-dark2`) the same single click produced no switch at all in the following 35 s (`dark2.result.json`: `theme=light` from start to end) — recorded as seen |

**The Snap flyout (Q-0022-09's assumption) did not appear.** Beyond the hover rows above, the
OS-level facts: the app's own `WM_NCHITTEST` answers, sent by the probe exactly as the system sends
them (`run-light2.json` `hit-tests`), are `HTCLIENT 1` from the top-level window at the maximize,
minimize and close buttons, `HTMINBUTTON 8` / `HTMAXBUTTON 9` / `HTCLOSE 20` from the `MyGoCaption`
child each point falls in, `HTTOP 12` from the top-edge strip at css `400,3`, and `HTCLIENT 1` from
the page at the drawn title area. A control hover over a standard Win32 window's own maximize
button — a scratch `System.Windows.Forms` window at `760,380`, whose `WM_NCHITTEST` answers 9 from
the window itself at `1582,392` — produced **no flyout** in 1.8 s and no new top-level window
either (`run-control.json`, `c-control-before.png` / `c-control-hover.png`). On this machine the
flyout appeared for neither window; the app's top-level `HTCLIENT` answer over its caption buttons
is the difference a later check has to weigh.

**The caption-button clicks were not all delivered.** The close button closed the app on its first
click; the minimize button minimized the window in two launches of four (`run-light2.json`,
`run-dark4.json`) and did nothing in two (`run-light3.json`; `run-tap.json` sent it twice —
`iconic: false`, the rect and the captures unchanged); the maximize button maximized the window on
its first click and did nothing on the following restore click (`run-tap.json`
`maximize-button-2` `zoomed: true`). Each of those clicks reported `sendInput: 1` with the cursor on
the button and the guard's `atTarget.ours true`, so delivery, not aiming, is the observation. The
page's own controls answered every real click in this sitting (the theme control in four launches,
the title-area clicks and the drag above), and the taskbar button answered twice.

**The owner's hand check (2026-11-26).** On this machine's own console (not the RDP session the
probe ran in), the owner hovered the maximize button of the installed v0.15.2 and the Snap layouts
flyout appeared, and the minimize, maximize and close buttons each worked when clicked by hand. The
two observations above are therefore of the remote session and the synthetic input, not of the app:
a standard window showed no flyout there either, and every hand click landed.

Two right-click attempts could not be driven, with the exact observation: in `run-dark.json` and
`run-dark4.json` the guard refused because the point was covered by another application's window —
`the window at the target is not the app's (root class 'NshWindow', pid 15988)`, the owner's
MarkTurbo window — and those two launches' later items (inactive, reactivate) ran with the app
already covered or minimized. The driven right-clicks are the light one and `run-last.json`'s dark
one above.

Not driven on this machine — owner actions where a machine of that OS is available:

_Deferred by the owner on 2026-11-26 (Q-0022-05): the goal closes without them; they stay here for
when a Mac or Linux machine is at hand._

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

The 2026-11-25 sitting, kept as history: `SendInput` returned 0 with `GetLastError` 5
(`ERROR_ACCESS_DENIED`), `SetCursorPos` left `GetCursorPos` at `0,0`, a launch whose page listened
for `mousemove`/`mousedown` recorded none (`listen.result.json` `"seen":[]`) although the same
launch renders, `GetForegroundWindow` was 0 and desktop captures were black while the per-window
`PrintWindow` renders; its captures are `PrintWindow` of the real window only
(`Q:/scratch/omp/g22/P5/manual/`, the light/dark/maximized/full-screen/restored states) and every
pointer item was an owner action. Its rects stand (`501,215 1618x1010` → `51,-9 2518x1458`
maximized, full screen `0,0 2560x1440`).

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
- The manual pass above is the remainder: the real pointer drove the drag, the double-click and the
  three resizes on 2026-11-26; the Snap flyout did not appear in that RDP session (a standard window
  showed none either) and the caption-button clicks were delivered in four of eight synthetic
  attempts; the owner's hand check on the console the same day saw the flyout and every button
  click work. The inactive cue reads only in the dark appearance, and the owner list's macOS and
  Linux items stay owner actions.

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
- The right-click window menu: nothing in the app draws one, and the driven right-clicks of
  2026-11-26 (both appearances) showed no menu and no new top-level window appear; the app keeps
  the foreground.
- On macOS and Linux the title read-back returns MyGo's cached title, not `[NSWindow title]` or
  `gtk_window_get_title` ([`../harness.md`](../harness.md) row 24); the three-stop assertion is the
  evidence either way.
- macOS 27 is not a runner; the macOS cells are macOS 26 (`macos26@20261117.0516.1`, Safari 26.x).
- No CI job captures the frame or the native buttons at the OS level (`CapturePage` shows the page
  only); that half is the manual pass above (Q-0022-05).
- The manual pass on this machine could not drive the real pointer on 2026-11-25 (evidence above);
  on 2026-11-26 it did — the drag, the double-click, the top-edge and both-side resizes, the
  minimize/restore and the close are recorded there. The Snap layouts flyout did not appear on hover
  in either appearance (a standard window's own maximize button showed none as a control either),
  and two of four minimize clicks and two of three maximize-button clicks were not delivered — the
  counts and the captures are in the manual-pass table. The owner's hand check on the console
  (2026-11-26) saw the Snap flyout and every caption button work, so both readings belong to the
  remote session and the synthetic input.
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

## The 2026-11-26 sitting's local checks

Run on this tree after the manual-pass update; each line is the gate's own pass line.

| Check | Result |
|---|---|
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | `STATUS pass 381 checked, 0 broken` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | `STATUS pass 815 files, 0 findings` |
