# 0022 — The window's title bar belongs to the Pass Quest interface: increment plan

Contract: [`../goals/0022-integrated-title-bar.md`](../goals/0022-integrated-title-bar.md).
Conventions: [`../conventions.md`](../conventions.md). Harness: [`../harness.md`](../harness.md).
Cross-platform rules: [`../cross-platform.md`](../cross-platform.md).
Base: `main` at `771a277` (0019 P9 merged, 2026-11-24). Scouted 2026-11-24; every number below is
read from the source named beside it or from a run named in the evidence plan.
Revised 2026-11-24 in the P0 round 2: Q-0022-01's vertical rule and horizontal mechanism, the
minimum-size (880x600) case, the bench's dark today state, the pointer assertions' step and form, the
per-OS room values, and Q-0022-09. Every finding of the three round-1 reviews is fixed here or
rejected with evidence in the round-2 answer
(`Q:/scratch/omp/runs/2026-11-24/s22-p0-r2/answer.md`), which names the plan line of each.
Revised again 2026-11-24 in the P0 round 3: the drag sample's modal scoping and its must-pass case,
the util group's clearance by `margin` with `titlebar-room-paint`, the fourth conditional check and
its cases, the marker's any-value gate, Q-0022-02's `client.ts` call path, the inactive-look
mechanism, the `os` values, and the Snap risk's wording — every finding of the three round-2 reviews
(`Q:/scratch/omp/runs/2026-11-24/r22-P0-{correct,platform,spec}-r2/answer.md`) is fixed here or
rejected with evidence, named in the round-3 answer.

## Goal in one sentence

Replace the system caption on Windows 11, macOS and Linux with a title area the Stage draws itself —
keeping the system window buttons, the window's behaviour (drag, double-click, resize from every
edge, Snap layouts) and every learner-visible layout — and prove it with per-OS mechanical checks,
per-OS screenshots and a recorded manual pass for what no gate can see.

## Facts this plan rests on (cited or measured, not remembered)

1. **What MyGo v0.10.0 offers for a hidden title bar, per OS.**
   - **The switch.** `WindowOptions.TitleBarStyle: mygo.TitleBarHidden` (macOS `hiddenInset` is the
     variant that adds a toolbar; `HideWindowButtons` drops the buttons; `TitleBarHeight` sizes the
     strip on Linux and Windows, `TrafficLightPosition` moves the macOS lights)
     (`mygo@v0.10.0/docs/windows.md:52-55`; `docs/frontend.md:301-345` frameless, `:350-430` hidden
     title bars).
   - **How the page learns the buttons' room.** Four CSS variables on `:root`, in CSS pixels, from
     the first paint: `--mygo-titlebar-height`, `--mygo-titlebar-inset-left`,
     `--mygo-titlebar-inset-right`, `--mygo-titlebar-controls-bottom` (`docs/frontend.md:386-412`).
     They arrive as a bridge event at document start
     (`internal/bridge/bridge.go:41-43` `TitleBarEvent`, `:108-112` `TitleBarScript`, which is the
     page's only carrier) and the runtime sets them (`packages/bridge/src/bridge.ts:41-52` listens for
     `mygo:title-bar`, `:225` is `setRootVars`, which writes `:root`). They become `0px` in full screen and come back on leave
     (`docs/frontend.md:393-394`). There is **no JS getter and no Window Controls Overlay**: the
     variables are the whole contract.
   - **Drag regions.** `--app-region: drag` on any element (inherited; `no-drag` carves out), read by
     the bridge from the computed style — walking `-webkit-app-region` only where the engine supports
     it (`packages/bridge/src/bridge.ts:125-133`, `:199-211`). A `mousedown` (button 0) on a drag
     element that is not inside `INTERACTIVE` (buttons, links, inputs, and the rest of the bridge's
     list) is taken over and posted as `drag`, or `dblclick` when the click count is 2; interactive
     descendants therefore stay clickable without any page work (the bridge's own `INTERACTIVE`
     selector list, `:199-213`). `dblclick` reaches `Window.TitleBarDoubleClicked`: on Windows
     Maximize/Unmaximize, on Linux the same, on macOS the user's `AppleActionOnDoubleClick`
     (`Minimize` or `performZoom`, `default` = zoom) (`internal/windows/window.go:1579-1586`,
     `internal/linux/window.go:1173-1179`, `internal/darwin/window.go:1598-1610`).
   - **Windows.** The page fills the window; MyGo puts its own caption buttons over the page as a
     layered child window whose pixels carry their alpha (`internal/windows/titlebar.go:11-39`), sized
     **46 DIP wide and as tall as `TitleBarHeight`** (32 by default) with 10 DIP glyphs, at the
     top-right (`titlebar.go:33-39`, laid out in `:185-207`). It answers `WM_NCHITTEST` with the buttons'
     hit-test codes, which is what brings **Windows 11's Snap layouts** over the maximize button
     (`titlebar.go:14-27`, `:268-300` — `captionBar.message` answers `WM_NCHITTEST` with
     `button.hitTest()`; the `:547-616` `SetMaximizeButton`/`snapMessage` pair belongs to the
     *native-UI* surface's own maximize button, not this app's path), and runs the buttons from the
     non-client mouse messages. The top edge is a separate invisible window returning
     `HTTOP`/`HTTOPLEFT`/`HTTOPRIGHT` along the top, up to the window controls, so the window still
     resizes from its top edge (`internal/windows/topedge.go:5-24`), as tall as the frame's own
     resize band — `SM_CYFRAME + SM_CXPADDEDBORDER`, 8 DIP where the system reports none
     (`topedge.go:99-107`) — so the page's top rows in that window's x-range resize the window and
     never reach it. The room it reports is
     `{height: TitleBarHeight, right: 46 x shown buttons, bottom: height}`, zero in full screen or with
     no shown button (`titlebar.go:158-166`). The glyph colours follow the window state and the
     appearance: Windows 11's rest/hover/pressed/active/inactive palette, red close hover
     (`titlebar.go:325-340`); "dark" is the **system** appearance unless the app calls
     `Window.SetWindowButtonsTheme(ThemeDark|ThemeLight|ThemeSystem)`
     (`window.go:1423-1436`; `internal/windows/window.go:1473-1481`; the system reading is
     `AppsUseLightTheme`, `internal/windows/system.go:617-626`).
   - **macOS.** The window is created with `fullSizeContentView`, `titlebarAppearsTransparent` and
     `titleVisibility = hidden`, so the page runs under AppKit's traffic lights
     (`internal/darwin/window.go:208-245`). The room is read from AppKit's own frames: the window's
     title-bar height, the zoom button's right edge, the lights' bottom — zero in full screen, with
     `HideWindowButtons`, or before the buttons exist (`darwin/window.go:1561-1596`). Its height is
     the window frame's minus `contentLayoutRect`'s origin.y and height — AppKit's own title-bar band,
     **~28 pt on a standard window** (`[INFERENCE]`: the formula and the full-size content view are
     the sources, `darwin/window.go:1573-1596`, `:208-245`; P3 reads the value) — and
     **`TitleBarHeight` plays no part on macOS** (`mygo@v0.10.0/window.go:112-122`,
     `docs/windows.md:55`).
     `TitleBarHiddenInset` adds an `NSToolbar` (`internal/darwin/window.go:244-249`) and changes the
     corner radius (macOS 26: 26 with it, 16 without) (`mygo@v0.10.0/docs/windows.md:248-250`).
   - **Linux.** GTK header bars, one per side of the desktop's `gtk-decoration-layout`, sit in an
     overlay above the web view and hold the desktop's title buttons; the room is measured from their
     allocations once GTK laid them out, from their natural sizes before
     (`internal/linux/titlebar.go:11-27`, `:95-153`): `room.Height` is `TitleBarHeight` when the app
     sets one and the bars' own height otherwise, `Left`/`Right` are the bars' allocated widths, and
     `Bottom` is `Height` once either side has one (`titlebar.go:121-146`). The backend is **GTK 3**
     (`internal/desktop/hidden_linux.go:34` opens `libgtk-3.so.0`), so the decoration facts below are
     GTK 3's. A compositor that decorates windows itself gets none (`titlebar.go:16-19`), and full
     screen hides them (`:155-160`). Outside Wayland's CSD case the page's outer 5 px resize the
     window and receive no input; under Wayland GTK draws a shadow and a border the page never sees
     (`docs/frontend.md:333-345`).
   - **What the framework does *not* provide, and where it bites.** (a) No page-visible title getter:
     `window.mygo.window` has `setTitle` and the window-state calls — `isMaximized()` and
     `toggleMaximize()` among them (`packages/runtime/src/types.ts:15-24` `WindowControls`; injected
     at `packages/bridge/src/runtime.ts:389-398`; the app's own
     `desktop/web/src/api/mygo-runtime.d.ts:11-16` declares only the `call`/`on`/`once` subset it
     uses, not `window.mygo.window`) — but nothing that reads the title back, so only the title
     needs a service of its own (fact 3). (b) No page API for the room besides the CSS variables, and
     **no replay of `mygo:title-bar` for a subscriber that registers after document start** (the
     bridge's own listener, `packages/bridge/src/bridge.ts:41-52`, runs at document start;
     `createRuntime`'s `on` delivers future events only) — so the page reads the computed `:root`
     variables once at boot and keeps them current by subscribing, and every gate reads the variables,
     never a JS call. (c) The caption buttons are never part of the page: they
     are a native child window (Windows), AppKit widgets (macOS) or GTK widgets (Linux); a page-level
     hit test inside the reserved room returns the page's own element (measured, fact 3). (d) Windows
     only: the buttons' light/dark defaults to the **system** appearance while this app's appearance is
     its own page theme, so the app must say which it wants (`SetWindowButtonsTheme`, Windows only —
     "the traffic lights of macOS and GTK's title buttons on Linux keep the app's appearance",
     `mygo@v0.10.0/window.go:1431-1434`). (e) `Window.CapturePage()` captures the **visible page**,
     not the window frame or the native buttons (`mygo@v0.10.0/window.go:1811-1824`, quoted by
     `docs/plans/2026-11-22-1800-0019-modern-frontend.md` fact 4), so no page screenshot can show the
     buttons over the drawn title area.
2. **Today's window.**
   - **The window's options.** `internal/desktop/desktop.go:153-162`: `Title: AppName` ("Pass Quest",
     `paths.go:18`), 1280x820, `MinWidth 880`, `MinHeight 600`, `Hidden` from
     `PASSQUEST_DESKTOP_HIDDEN`, `StateKey: "main"`. No `TitleBarStyle`, no buttons theme, no
     appearance call: the caption today is the system's.
   - **The page's top.** `internal/desktop/web/index.html:13-22`: `<div class="app"><header class="top"
     id="top"></header><main class="stage" id="stage">` — a 64 px header row, 58 px under the height
     the audited sizes have (`desktop/web/styles/chrome.css:3`,
     `desktop/web/styles/responsive.css:3-4`, `@media (max-height: 820px)`). `.top` is a
     `1fr auto 1fr` grid with 24 px inline padding (`chrome.css:4`): brand at the left, the route's
     middle slot, `.util` at the right (`desktop/web/src/chrome/top.ts:30-71`, the whole bar's markup).
     The element that becomes the title area is therefore `header#top.top` itself: it spans the window,
     it is the top-most app element, and it already holds the util group (`chrome.css:3-5`).
   - **Measured, top 40 px, 2026-11-24** (hidden launch, this machine's WebView2 runtime, `dpr 1.25`,
     scratch probe `Q:/scratch/omp/g22/P0/probe.js` at `Q:/scratch/omp/g22/P0/run1280x800/result.json`
     and `run880x600/result.json` — a first `run1280/result.json` is a **failed** probe
     (`number 1272 is not iterable`) and contributes nothing; the runtime this machine ran under is
     the one goal 0012's record names, `docs/evidence/0012-opus-grade-visual-experience.md:9`):
     at 1280x800 the viewport is exactly 1280x800 and the app rows read `58px 742px`; `.top` is
     `[0,0,1280,58]`, the brand `[24,17,509,25]`, the middle slot `.tl` `[533,13,214,32]` and `.util`
     `[1098,9,158,40]` holding 中 `[1104,16,34,26]`, EN `[1138,16,36,26]`, theme `[1182,13,32,32]`,
     shortcuts `[1220,13,32,32]`. At 880x600 the same rows are `58px 542px`, `.util` `[698,9,158,40]`
     with EN at `[738,16,36,26]`, theme `[782,13,32,32]`, shortcuts `[820,13,32,32]`. In the top 40 px
     of all seven routes (`today practice feedback assess report anki settings`, driven by hash and
     read per route) sit only `.atmos` (decorative, `pointer-events: none`, full-window) and
     `header#top.top`; the middle slot's content differs per route (run clock on the assessment, the
     session walk on study routes, a resume pill or nothing), and `document.title` is
     `Pass Quest — <route>` on every route. The util group's top is y = 9, one DIP below Windows' 8 DIP
     top resize band (fact 1) — a control moved up to a strip's own top would lose its top rows to the
     resize, which is why Q-0022-01 keeps the header's centring.
   - **Today's room and drag regions.** All four `--mygo-titlebar-*` variables are unset (`""`), and no
     element in the document has a computed `--app-region` (both measured, same probe). So today the
     page reserves nothing and drags by nothing.
3. **What the gates can see** (all three desktop gates drive a real window through the smoke hook,
   hidden unless a launch asks otherwise: `harness/lib/desktop-page.ts:277-300`, the hidden env at
   `:287`; the page script runs in the page, so it reads the DOM, the CSS variables, `document.title`
   and calls bound Go methods, `:63-105` the shared prelude).
   - **The room: yes, and it is live.** In a scratch copy of this tree with
     `TitleBarStyle: mygo.TitleBarHidden, TitleBarHeight: 40` (Windows, hidden window, 1280x800,
     `Q:/scratch/omp/g22/P0/probe2.js`), the page read at document start `height 40px,
     inset-left 0px, inset-right 138px, controls-bottom 40px` — 138 = 3 x 46, MyGo's Windows caption
     button width; maximizing and restoring changed nothing; entering full screen set all four to
     `0px`; leaving restored them. **A hidden Windows window reports the same values a shown one
     would** — `[INFERENCE]` from the source (the room is `GetClientRect` + `titleBarHeight()` +
     `shownButtons()`, none of which reads visibility, `titlebar.go:146-166`); P1's shot record carries
     the room a **shown** launch reads, so the two launch kinds are compared from artifacts rather
     than asserted from one (fact 3's next bullets).
   - **The drag region and covered controls: yes.** The computed `--app-region` is readable per
     element, and the room's rectangles are known from the variables, so a gate can assert both "no
     interactive control lies inside the room" and "the title strip is inside a drag region". The page
     **cannot** see the native buttons: at the room's centre `elementFromPoint` returns the page's own
     control (measured; fact 1(c)) — the room is the only truth in-page.
   - **A page press's fate is observable.** The bridge's takeover is `e.preventDefault()` on the
     press (`packages/bridge/src/bridge.ts:199-211`), so a script that dispatches
     `new MouseEvent("mousedown", {bubbles: true, cancelable: true, button: 0, detail: 2})` reads
     `defaultPrevented === true` exactly when the press became a window double-click — on every OS,
     with no Go-side service; on Windows and macOS the same press then moves
     `window.mygo.window.isMaximized()` (fact 1(a)). A press on a control inside the drag region must
     read `false` (the bridge's `INTERACTIVE` exclusion, fact 1).
   - **Where a room check can run, and the size nobody drives.** The ux gate audits at
     `UX_SIZES = [[1280,800],[1440,844]]` (`harness/gates/desktop-ux.ts:44-47`); **no gate launches or
     resizes to 880x600 today** — a `880` search over `harness/**` and `internal/desktop/**` finds only
     `MinWidth: 880` (`internal/desktop/desktop.go:158`) — so the goal's minimum window needs a case of
     its own (P1). Every gate launch can size its page area with `PASSQUEST_DESKTOP_CONTENT_SIZE`
     (`internal/desktop/smoke.go:31`, `sizeWindow` `:155-159`; the ux gate's own use at
     `desktop-ux.ts:762`), the bench's shot launches included.
   - **The bench's second appearance has a seam.** The bench shoots six states in the app's default
     mode (light, zh) and sets no theme; the launch URL is the app's own, so `?theme=dark` is not on a
     harness path — the page's own theme control is (`[data-act="theme"]`, the one the ux gate's
     `applyMode` clicks, `desktop-ux.ts:162-165`), and a shot driver that clicks it captures the dark
     appearance of the same state.
   - **The window title: yes, with one small read-back.** `Window.Title()` is the platform's own
     read-back — `GetWindowTextW` (`internal/windows/window.go:975-980`), `[NSWindow title]`
     (`internal/darwin/window.go:821-825`), `gtk_window_get_title`
     (`internal/linux/window.go:460-461`) — and a smoke-only bound service returns it. Measured in the
     scratch copy with a `SmokeWindow.Title` binding (`Q:/scratch/omp/g22/P0/probe3.js`): it answered
     `Pass Quest — 今日`, `Pass Quest — 测评`, `Pass Quest — 设置` as the route changed and followed a
     direct `document.title = "probe-xyz"`; a call to an unbound service rejects by name
     (`method SmokeShot.Capture is not bound`), the pattern `desktop-smoke/boot.ts:73-79` already
     asserts.
   - **The designed title area, measured — and what the revision changes.** The round-1 patch
     (asymmetric padding on the header, the strip's height, `--app-region: drag`;
     `Q:/scratch/omp/g22/P0/probe4.js`, `Q:/scratch/omp/g22/P0/tb4run*/result.json`) measured the room
     `[1142,0,138,40]` at 1280x800 with **no interactive control intersecting it**, the strip's centre
     hit-testing a `drag` element, the util group at `[960,0,158,40]`, and **the middle slot 69 px
     left of the window's centre** — the trap Q-0022-01's revised horizontal rule removes by reserving
     the insets inside the outer columns instead of on the header. The revision's own geometry is
     `[INFERENCE]` from that rule and this fact's box list: the util group moves left by the right
     inset alone, the two `1fr` columns stay equal so the middle slot stays centred, and the header
     keeps its `58px` with the util group in its own `[.., 9, .., 40]` band — and it is what P2's
     checks, the 880x600 walk and the per-OS reads exist to measure.
   - **Hidden windows by OS.** Windows and macOS lay out a hidden window at the size it was made with
     (`internal/desktop/hidden_other.go:5-9`); Linux needs `layOutHidden`, because GTK allocates
     nothing while a toplevel is unmapped (`hidden_linux.go:23-31`), and MyGo's Linux room is read from
     the header bars' allocations (`internal/linux/titlebar.go:123-133`) — so on Linux a hidden
     window's room is whatever GTK allocated for it, and the Ubuntu CI job, not this machine, is the
     evidence for that value.
4. **Platform rules to respect.**
   - **Windows 11.** The standard title bar is 32 px, its colours follow light/dark and high contrast,
     all non-interactive title-bar space is draggable, and a double-click there toggles
     maximize/restore (`learn.microsoft.com/en-us/windows/apps/design/basics/titlebar-design`,
     sections *Bar* and *Title*, fetched 2026-11-24). Snap layouts "will automatically show … when a
     user hovers the mouse over the window's maximize button", and an app with a custom caption button
     must answer `WM_NCHITTEST` with `HTMAXBUTTON` for it
     (`learn.microsoft.com/en-us/windows/apps/desktop/modernize/ui/apply-snap-layout-menu`, fetched
     2026-11-24) — MyGo's caption window does exactly that (fact 1). The same page recommends a
     minimum width of at most 500 effective pixels for snapping; Pass Quest's minimum is 880, so a
     narrow snap zone can refuse the window (a disclosed limit, not this goal's to change).
   - **macOS.** The frame holds the window controls and the HIG's advice is "Avoid creating custom
     window UI … don't try to replicate the system-provided appearance"; the key window uses colour in
     the close/minimize/zoom options and inactive windows grey
     (`developer.apple.com/design/human-interface-guidelines/windows`, sections *Best practices* and
     *macOS window states*, fetched 2026-11-24). The transparent title bar only makes sense with a
     full-size content view (`developer.apple.com/documentation/appkit/nswindow/titlebarappearstransparent`,
     fetched 2026-11-24) — which is what MyGo sets (fact 1). The lights hide in full screen and come
     back with the menu bar (`mygo@v0.10.0/docs/frontend.md:393-394`).
   - **Linux.** GTK's `gtk-decoration-layout` decides which title buttons a client-side-decorated
     window shows and on which side (default `menu:minimize,maximize,close`), and a button appears
     only when the window can do it (`docs.gtk.org/gtk4/property.Settings.gtk-decoration-layout.html`,
     the GTK 4 documentation of a property the GTK 3 backend reads the same way —
     `internal/desktop/hidden_linux.go:34` opens `libgtk-3.so.0` — fetched 2026-11-24); the toplevel's
     CSS node carries `.csd`/`.solid-csd`/`.ssd`, and in the CSD case the shadow outside the window
     resizes it (`docs.gtk.org/gtk4/class.Window.html`, section *CSS nodes*, fetched 2026-11-24;
     GTK 3's own pages carry the same CSS-node facts and the one the double-click needs —
     `docs.gtk.org/gtk3/method.Window.maximize.html` and `…/method.Window.is_maximized.html`:
     **maximize is the window manager's**, and its state must not be assumed afterwards "**(or at
     all)**"). Under Wayland, `xdg-decoration` lets the compositor enforce
     server-side decorations — "The compositor can decide not to use the client's mode and enforce a
     different mode instead" — and Mutter does not implement it while KWin, Sway and Hyprland announce
     it (`wayland.app/protocols/xdg-decoration-unstable-v1`, fetched 2026-11-24). CI's Linux desktop
     job installs `libwebkit2gtk-4.1-0 xvfb …` and runs inside `dbus-run-session`
     (`.github/workflows/gate.yml:61-66`): there is no window manager on the runner, so the gate's
     window is never decorated by one and whatever the room reads there is GTK's own bars — and
     `gtk_window_maximize` has no manager to act on it (Q-0022-04).
   - **Windows 11, the rest of the title-bar rule.** The same Microsoft page expects a right-click in
     the title bar to open the system window menu and the title bar to go semi-transparent while the
     window is inactive. MyGo's bridge forwards `contextmenu` to Go and this app draws no system menu
     (`packages/bridge/src/bridge.ts:186-187`; nothing under `desktop/web/src` handles it), so the
     right-click is a manual-pass item and a recorded limit where it is absent (Q-0022-05, Q-0022-09);
     the inactive look under `TitleBarHidden` is the caption window's own — its glyph state follows
     the window's activation (`internal/windows/window.go:578`, palette `titlebar.go:325-340`) — and
     the page's drawn strip does not react today; the bridge sets `data-mygo-window-inactive` on
     `<html>` for a page that wants to (`internal/bridge/bridge.go:113-118`), so the manual pass
     judges the look and the record carries the answer as a limit beside the right-click.
     The page's own guidance that title-bar text responds to text scaling, possibly growing the bar,
     does not hold here either: the header keeps its designed height (Q-0022-01, the goal's own
     constraint), recorded as a limit beside the right-click.
   - **macOS's double-click is the user's setting.** `TitleBarDoubleClicked` reads
     `AppleActionOnDoubleClick`: `None` does nothing, `Minimize` minimizes, anything else (the default,
     unset, is "Zoom") calls `performZoom` (`internal/darwin/window.go:1598-1610`, and
     `IsMaximized()` is `isZoomed && !IsFullScreen`, `:1286`). The macOS assertion therefore states
     the zoom path and its failure message names that default first (Q-0022-03).
5. **Reference shots, and which of them show the window top.**
   - The 17 sheets under `docs/evidence/0012/` are the desktop-ux reference shots: each is a 2x2
     contact sheet (light-zh, light-en, dark-zh, dark-en) of a 1280x800 **page area** downscaled to
     640x400, shot on commit `354f08c` through MyGo's automation build with a seeded data directory
     and loopback stubs (`docs/evidence/0012/README.md:1-11`). **Every one of them shows the page's own
     top bar** — the top 58 px — and none shows an OS frame or the system buttons: a vision read of
     `today.webp` finds the app's top bar at the very top and no caption buttons anywhere
     (`docs/evidence/0012/today.webp`, 1284x848, read 2026-11-24). `README.md:22` and
     `README.zh-CN.md:15` embed `feedback-coach.webp`; `docs/install.md` carries only
     `<!-- screenshot: … -->` placeholders. Nothing in this repository regenerates the sheets: the
     method is the record's own (MyGo's automation build), and no tool under `harness/` or `internal/`
     takes a contact sheet.
   - The bench's own shots are the per-OS visual evidence: one visible launch per state
     (`harness/tools/frontend-bench.ts:614,648,853` — every shot launch passes `hidden: false`),
     compared byte-for-byte against `harness/frontend-visual-allow.json`, whose entries name a state
     and a reason and where **an entry that matches nothing fails the run**
     (`harness/lib/bench.ts:700-740`, `harness/tools/frontend-bench.ts:705-716`; the allowlist is
     empty today). The bench's shot states are `today, sheet-open, practice-independent,
     feedback-coach, settings-row, report` (`harness/lib/bench.ts:32`), each captured in the app's
     default mode (light, zh); P1 adds a seventh, `today-dark`, and the plan's per-step runs carry it
     from then on (Q-0022-05).
   - **The six Done-when-1 cells and the artifact of each.** Windows: the `today` and `today-dark`
     shots in the `frontend bench (windows-latest)` job. macOS: the same two in `(macos-26)`. Linux:
     the same two in the `linux-smoke` job, whose one visible WebKitGTK launch per state captures all
     of `SHOT_STATES` under Xvfb (`.github/workflows/frontend-bench.yml:213-257`) — per OS, per
     appearance, 1280x800. The 0012 sheets are a Windows-only four-appearance contact sheet and are
     not a per-OS artifact (Q-0022-07).

## Defaults (append as Q-0022-01…09; the last entry is Q-0019-06)

1. **Q-0022-01 — The title area's height, content and drag region.** The title area is
   `header#top.top` itself; the header keeps its designed heights (64 px, 58 px under the
   `max-height: 820px` query) and the app sets `TitleBarHeight: 40` — the height the `.util` group
   already has (measured, fact 2) and a Windows-11 sized caption strip.
   **Vertical rule:** the header's contents keep their own centring in the header's own 58/64 px row
   (no shorter band); the header is at least `max(strip, content)` tall at every room value fact 1 or
   P3 reports — the bound `titlebar-strip-clip` checks — so a strip shorter than the content it
   centres (macOS's AppKit band, ~28 pt against a 40 px util group) cannot clip it. This replaces the
   round-1 rule ("centred in its first `--mygo-titlebar-height` px") deliberately, for three reasons:
   it is what keeps the header byte-identical at room `0px`, which is P2's macOS/Linux proof, the
   full-screen look and
   Q-0022-04's server-side-decoration case; a control centred in a 40 px strip would start at y = 0,
   and Windows' top edge is a resize band of `SM_CYFRAME + SM_CXPADDEDBORDER` (8 DIP, fact 1) that
   the page's top rows never reach — the util group's measured top is y = 9 (fact 2), 1 DIP clear of
   it; and a band of exactly the room's height is the one branch plain CSS cannot express, because
   `0px` must mean today's layout. Disclosed consequence: the app's header controls do **not** share
   the caption buttons' vertical centre (Windows: the group's centre is y = 29 against the buttons'
   20; macOS: against a lights band of 0..~28) — P3's 1280x800 read and the manual pass judge the
   look, and the evidence record carries the value that was measured.
   **Horizontal rule:** the insets are reserved inside the header's outer columns — the brand's
   leading padding grows by `--mygo-titlebar-inset-left` (a stretched grid item with no paint of its
   own, so the padding shifts content, not a box), the util group's trailing **margin** by
   `--mygo-titlebar-inset-right` — not on the header itself, so the two `1fr` columns stay equal and
   the middle slot stays centred in the window with no compensation term (the round-1 patch's
   asymmetrically padded grid moved it 69 px left, and its own probe read that). `padding-right` is
   **not** the mechanism: it would grow `.util`'s own box from 158 px to 296 and paint the capsule's
   glass, border and shadow under the caption buttons — invisible to screenshots (fact 1(e)) and
   checked by `titlebar-room-paint` — while `margin` moves the same 158 px box clear (right edge
   1118 at 1280x800, the round-1 probe's own x). Content is unchanged: brand mark, name, prep day;
   the route's middle slot; the utilities.
   **Drag region:** `--app-region: drag` on the header, under `:root[data-titlebar]` — a marker the
   page sets while **any** of the four room values is above zero, not the height alone (fact 1's
   macOS read computes the height and the left inset in one call, and an inset alone still needs its
   clearance; read once at boot from the computed `:root` variables, kept current from
   `mygo:title-bar`, fact 1(b)) — so a zero room leaves both the layout
   and the press handling exactly as today, full screen included. Interactive descendants stay
   clickable through MyGo's own exclusion (fact 1), and what the platform owns stays the platform's:
   Windows' top 8 DIP resize, the Linux page's outer 5 px resize, the button rectangles at the ends.
   **Every page rule is room-driven**: with the variables absent or `0px` the header lays out and
   behaves as today, which is why the macOS and Linux columns of P2's bench run must read
   byte-identical shots.
2. **Q-0022-02 — Which appearance the caption buttons follow.** The app's own theme, on Windows, by
   calling `Window.SetWindowButtonsTheme` when the page's theme changes; macOS and Linux keep the
   system's own appearance (guard: `mygo@v0.10.0/window.go:1431-1434`). The theme lives in the page's
   `localStorage` (`desktop/web/src/core/prefs.ts:23-38`), so the page tells Go: one new bound method,
   bound like `Study` is (`internal/desktop/desktop.go:109`), the generated client regenerated with
   it (`desktop/web/src/api/mygo.ts`, which `desktop-smoke`'s client-fresh scenario compares against a
   fresh `MYGO_GENERATE`), its entry in `desktop/web/src/api/client.ts` — the page's only call path,
   whose `METHODS` list and `Study.`-prefixed `studyCall` fail the type check when a method is
   unlisted, so a new service needs a wrapper there, or the method lives on `Study` — and the page
   calls it at boot and on the theme toggle
   (`desktop/web/src/actions.ts:137-143`). Disclosed: this adds one method to the app's bound surface;
   it does not touch `window.mygo.window`'s behaviour. Reversal: drop the call and accept
   system-appearance buttons, which is legible only while the system and app appearances agree.
3. **Q-0022-03 — macOS hidden-title-bar style.** `TitleBarHidden`, not `hiddenInset`, and no
   `TrafficLightPosition`: the lights stay where macOS puts them for every app, and the page reads
   `--mygo-titlebar-inset-left` / `--mygo-titlebar-controls-bottom` instead of assuming a place.
   `hiddenInset` adds a toolbar and changes the corner radius (fact 1), which is design the goal does
   not ask for. The strip's height on macOS is AppKit's own band, not `TitleBarHeight` (fact 1), so
   Q-0022-01's vertical rule (not a 40 px strip) is what keeps the util group whole under it; P3 reads
   the value from its artifact before it judges anything else. A double-click's macOS action is the
   user's `AppleActionOnDoubleClick` (default: zoom), so the assertion states the zoom path and names
   that default as the first suspect when it fails (fact 4).
4. **Q-0022-04 — Linux decorations.** Linux hides the title bar too (Done-when 1 asks for all three
   platforms) and accepts that a Wayland compositor which enforces server-side decorations keeps its
   own bar with no room for the page — GNOME/X11 and Mutter's CSD give GTK's buttons in MyGo's
   overlay, KWin/Sway/Hyprland do not (fact 4). The disclosed limit is that the page's title area then
   sits below a compositor-drawn bar and reserves nothing; the Ubuntu CI artifact carries the hidden
   window's numbers. **CI has no window manager** (fact 4): `gtk_window_maximize` and
   `gtk_window_is_maximized` are the WM's business, and GTK 3's own docs say not to assume the state
   afterwards or at all — so the CI case asserts the press's takeover (what the page can observe,
   fact 3) and only records the maximize reading; the effect on a real desktop is the manual pass.
   Revisit when a first-tier OS gains a Linux desktop in the matrix.
5. **Q-0022-05 — What the machine owns now, and what the manual pass owns.** Done-when 1's six cells
   (three OSes × two appearances, the today view at 1280x800) are **mechanical**: `today` and
   `today-dark` in each of the Windows and macOS bench jobs, and the same two states from the Linux
   render-smoke job (fact 5). The manual pass therefore carries only what a page capture cannot show
   (fact 1(c)(e)): the frame and the native buttons' legibility over the drawn title area, Snap
   layouts, a real pointer's drag and double-click, the resize from the top edge and both sides, the
   inactive and full-screen looks, and the right-click (fact 4). It is recorded in the evidence record
   with the OS, the appearance and what was seen; on Windows it is a coordinator action, and on macOS
   and Linux it is one where a machine of that OS is available — recorded as an owner action with the
   reason where none is.
6. **Q-0022-06 — Per-OS visual allowlist entries.** `harness/frontend-visual-allow.json` entries gain
   an optional `os` (`"windows" | "macos" | "any"`, default `"any"` — the shot comparison's two
   jobs, `.github/workflows/frontend-bench.yml:48`; the Linux `--smoke` job compares no numbers and
   never reads the allowlist, so a `linux` value would be dead), because each bench job
   would otherwise fail the other OS's job: a step that hides the title bar on Windows only changes
   Windows' page shots, and an allowlist entry covering that state would read as a stale entry in the
   macOS job, where the shots stay identical (`harness/lib/bench.ts:728-740`,
   `harness/tools/frontend-bench.ts:711-716` fail a stale entry). `--self-check` gains the case.
   **An entry belongs to one run's A/B pair, never to the tree in general**: it is added in the branch
   whose own run needs it (P2's Windows entries, P3's macOS entries) and **removed by the next step**
   (P3 removes P2's Windows entries, because that run's A already carries the Windows title bar; P4
   removes P3's macOS entries), so the merged tree's allowlist is empty again and no later pull
   request's run reads them as stale. The goal-level comparison (A = `771a277`, B = the final head)
   therefore runs from a throwaway branch that carries the head plus the entries it needs (one per
   shot state under `windows` and one under `macos` — **fourteen** for the seven states, each with its
   reason); that branch is never merged. **Its cost, disclosed:** from P2's merge until P3 removes the
   entries, another pull request that triggers `frontend-bench` reads those Windows entries in *its
   own* head, matches no difference and fails as stale — the coordinator serialises the steps so no
   other branch's bench run starts inside that window.
7. **Q-0022-07 — The 0012 contact sheets.** They are regenerated for the final head with the method
   their own record documents (MyGo's automation build on Windows, the four appearances, 2x2 of
   1280x800 page areas), and the evidence record lists per sheet what changed — the title area, and
   nothing below it. If that build cannot be reproduced, the record says so and regenerating the
   sheets becomes an owner action — the six per-OS cells keep their own mechanical artifacts (fact 5),
   so this fallback no longer narrows Done-when 1; neither the sheets nor their README claim a state
   they do not show.
8. **Q-0022-08 — The framework's gaps and the workarounds the plan takes.** The room is CSS variables
   only, so every check reads computed styles, never a JS API (fact 1); `mygo:title-bar` is not
   replayed to a late subscriber, so the page reads the variables at boot and subscribes for changes
   (fact 1); there is no page title getter, so P1 adds a smoke-only read-back service (fact 3) — the
   window state needs none, `window.mygo.window.isMaximized()` is the runtime's own (fact 1);
   `TitleBarHeight` does nothing on macOS, so no page rule may assume a 40 px strip (fact 1);
   `CapturePage` cannot show the native buttons or the frame, so their legibility is the manual pass
   (Q-0022-05); the Windows buttons' default appearance is the system's, so the app tells MyGo which
   one it wants (Q-0022-02). Each is a disclosed workaround, and each is revisited when MyGo offers
   the missing capability.
9. **Q-0022-09 — On Windows the caption buttons are MyGo's own drawing, and that is the disclosed
   replacement.** The contract keeps "the system's own window buttons … unless an OS cannot show them
   legibly over the page" and requires any replacement to "behave like the system's, including Snap
   layouts on Windows", disclosed. With `TitleBarHidden` Windows has no system caption left to keep:
   MyGo's backend draws minimize, maximize-or-restore and close itself, over the page, in a layered
   child window sized 46 DIP × `TitleBarHeight` at the top-right
   (`mygo@v0.10.0/internal/windows/titlebar.go:11-25`, `:33-39`). Claimed behaviour, each with its
   source: it answers `WM_NCHITTEST` with the buttons' hit-test codes and lets `DefWindowProc` run the
   non-client mouse move, which is what brings Snap layouts over maximize (`titlebar.go:268-300`); it
   paints Windows 11's state palette, red close hover included (`:325-340`), in the system appearance
   unless the app asks otherwise (Q-0022-02). Verification: the manual pass's Snap hover and its
   OS-level captures of the buttons (Q-0022-05), the per-OS room reading (P2), and the guard that the
   window never sets `HideWindowButtons`. Reversal: none inside MyGo — a page-drawn caption would have
   to answer its own hit test, which is out of this goal's scope.

## Steps (one implementer run each; each leaves every gate green)

| # | Step | Proof |
|---|---|---|
| P1 | The checks, green on today's window: `SmokeWindow.Title`, the boot assertions, the minimum-size (880x600) title-area scenario, the ux library's four conditional checks, the bench's dark today state and its room record, the per-OS allowlist scope, their must-fail self-test cases and the modal must-pass case | `desktop-smoke` and `desktop-ux` green on this machine and on CI's three OSes, with the new checks and the 880x600 room recorded; the self-test cases fail as designed (the modal case passes); the P1 bench run: every metric not worse beyond noise and 7/7 shots byte-identical on both OSes |
| P2 | Windows: `TitleBarHidden` + `TitleBarHeight: 40`, the page's title area and its room-gated drag region, the buttons' theme, the synthetic-pointer assertions | the P2 run (A = P1's merge, B = P2): Windows shots differ and are allowlisted per OS with reasons, macOS shots byte-identical; the three OSes' desktop gates green; the room the page reads recorded per OS, hidden and shown; the takeover and maximize assertions green on Windows and macOS, disclosed on Linux |
| P3 | macOS: the same switch for darwin, the lights where macOS puts them, its room read before anything else is judged | the P3 run: macOS shots allowlisted, Windows shots identical; the macOS job's room readings (hidden and shown), the window-title read-back and the title area at 1280x800 read from the run's own shot pair; `desktop-ux`/`desktop-smoke` green on macOS |
| P4 | Linux: the switch for linux, and the room measured where the desktop decides | the P4 run and the Ubuntu jobs: the hidden window's room reading from `desktop-smoke`, the visible window's from the bench's `--smoke` job; the maximize reading recorded, not asserted (no WM); both rooms recorded, or the difference disclosed |
| P5 | Evidence, decisions, the reference sheets, the manual pass | `docs/evidence/0022-integrated-title-bar.md` with the per-OS readings, the six cells' artifacts, the shot list and the manual-pass record; Q-0022-01…09 in `OPEN-QUESTIONS.md`; `docs-links`, `secrets`, `install-guide` green; the coordinator's review |

Each step is one pull request cut from `main` after the previous step merged; each measurement row
compares **A = the previous step's merge** with **B = the step's head** in one job of the step's own
`frontend-bench.yml` run, on `windows-latest` and `macos-26`, with the same-build control pair
read first (Q-0019-02, fact 5).

### P1 — the checks first (facts 2, 3, 5; no product-visible change)

1. `internal/desktop/smoke.go` gains a `SmokeWindow` service with one method: `Title()`, reading
   `win.Title()` back from the platform. It binds from `SmokeShot`'s own call site
   (`desktop.go:120-121`) but on the smoke hook's condition, a scripted launch — **not** `SmokeShot`'s
   `EnvShotDir` (`smoke.go:199-202`), which the boot and `titlebar` scenarios do not set
   (`harness/lib/desktop-page.ts:277-292`): there, `Title()` would be absent exactly in the launches
   that assert it. The window state needs no Go
   service — `window.mygo.window.isMaximized()` is the injected runtime's own (fact 1) — so the
   round-1 `Maximized()` is dropped. A launch without `PASSQUEST_DESKTOP_SCRIPT` binds nothing; Go
   tests cover the window-less call and the binding condition (`internal/desktop/smoke_size_test.go`
   is the shape).
2. `harness/gates/desktop-smoke/boot.ts` (the scenario that runs first on every OS) adds: the title
   read-back equals the page's `document.title` and starts with `Pass Quest — `; navigating to
   `#/settings` and back changes both together (Done-when 3's "still announced and shown in the
   taskbar, the Dock and the window switcher", read from the platform's own title); and the four room
   values with the presence of the page's `:root[data-titlebar]` marker are recorded in the result, so
   every OS's artifact carries what a hidden window reads. The synthetic double-click the round-1 item
   put here is **not** here: it needs the drag region, so it moves to P2 with the page rule that
   creates it (Q-0022-01).
3. A new scenario `harness/gates/desktop-smoke/titlebar.ts`, registered in `SCENARIOS`
   (`harness/gates/desktop-smoke.ts:37`), drives the **minimum window** Done-when 2 names and no gate
   drives today (fact 3): one launch at `PASSQUEST_DESKTOP_CONTENT_SIZE: "880x600"` walks the seven
   routes in both appearances (the hash, then the page's own theme control), and at each stop records
   the room and the marker and collects the four title-area checks of item 4. It asserts: no
   route×appearance reported a finding; the marker matched the room (present exactly when any of the
   four values is above zero); and the walk really saw both appearances. Green today — the room is
   `0px`, the checks report "no title bar" and the record says with which reason — and real the
   moment P2's room appears.
4. `harness/lib/ux-page.ts` gains the page-side title-area checker (a `TITLEBAR_LIBRARY` string beside
   `UX_LIBRARY`; the ux gate's driver and the scenario above both use it): it parses the four
   variables (unset counts as `0px`), builds the platform's rectangles from them (a right inset makes
   a top-right rectangle, a left inset a top-left one as tall as `controls-bottom`), and reports
   - `titlebar-room-covered`: every visible interactive control whose box a rectangle intersects, by
     path;
   - `titlebar-room-paint`: every visible descendant of the header whose own painted box — a
     background with alpha, a border, or a box-shadow (border box plus the shadow's offset, spread
     and blur) — intersects a rectangle, by path. This is the check the round-2 plan's util
     `padding-right` would have failed (the capsule painting under the buttons while the buttons
     themselves sit clear, invisible to screenshots, fact 1(e)) and the reason Q-0022-01 states
     `margin`;
   - `titlebar-drag-region`: a horizontal sample of the strip (y = height/2; x stepping across the
     window, inside the platform's outer bands and outside the button rectangles) — only sample
     points whose hit lies inside the header count. A point that hits a top-layer modal `<dialog>`
     or its `::backdrop` — an engine's backdrop hit reaches the dialog itself
     (`harness/gates/desktop-ux.ts:222-223`, `desktop/web/src/core/render.ts:133-139`) — or hits
     nothing, is skipped, and the audit's record carries the reason: the top layer takes every strip
     sample while a modal is up, so the strip is not observable in-page there. This is scoping the
     check to what it can observe, not weakening it: every counted point whose hit is not inside an
     interactive control must compute `--app-region: drag`, at least one counted point must exist
     whenever the strip is reachable (a non-zero height and no modal layer), and the first of them
     becomes the probe point P2's assertions press at. The probe point is derived from the layout
     this way, never taken from the window's centre, which the middle slot's own content occupies on
     the report and settings states (the round-1 defect);
   - `titlebar-strip-clip`: a header control whose visible box is not wholly inside the window — the
     strip-shorter-than-its-content case Q-0022-01's rule forbids.
   All four run whenever any of the four room values is above zero — the marker's own condition —
   and read no finding where the geometry they need is absent (a `0px` height has no drag sample; a
   modal state has none either), the record naming why; so they are green today. The room, the
   marker, the paint boxes, the probe point and each skipped sample's reason are part of every
   audit's output, so the artifact shows what the check saw. The ux gate's driver runs them in every
   audit — 24 states × 4 modes × 2 sizes at `UX_SIZES` (1280x800, 1440x844), at no extra launch.
5. `harness/gates/self-test/desktop-ux.cases.ts` gains four must-fail cases, static patches of the
   fixture (the shape `hideFocusAndTitle` already uses, `:14-22`): a stylesheet that fakes the room
   (`--mygo-titlebar-height: 40px; --mygo-titlebar-inset-right: 138px` on `:root`) while leaving the
   util group where it is must report `titlebar-room-covered` naming its controls; the same fake with
   the drag rule neutralised must report `titlebar-drag-region`; the same fake with the util group's
   top pushed above the window's must report `titlebar-strip-clip`; the same fake with the clearance
   applied as `padding-right: 138px` on `.util` (the round-2 defect) must report
   `titlebar-room-paint` naming `.util` while `titlebar-room-covered` stays silent — the case that
   pins `margin`, not padding. The gate's pass list (`passCases()`, `self-test.ts:345-346`) gains
   `desktopUxPassCases()` with the modal case, run through
   `makeDesktopUxGate({sizes: [UX_SIZES[0]], parts: ["one"]})` as the must-fail cases are: the fake
   room, the drag rule and the util margin all present, so the whole restricted audit must read
   **no finding** — the modal states' strip samples are skipped with the reason recorded, and
   without the skip they would red. `postPass` requires the run's detail
   (`harness/gates/desktop-ux.ts:973`, which the step extends with the titlebar segment) to carry
   the modal skip record, so the case proves the skip ran rather than the check never firing. A
   further case in
   `harness/gates/self-test/desktop-smoke.cases.ts` proves the scenario's own assertion path: a stub
   app whose `titlebar` result carries a faked room and one `titlebar-room-covered` finding must fail
   the gate naming that finding.
6. `harness/lib/bench.ts` + `harness/tools/frontend-bench.ts`:
   - `SHOT_STATES` gains `today-dark` (`bench.ts:32`), driven by the page's own theme control
     (`[data-act="theme"]`, the path the ux gate's `applyMode` clicks, fact 3) — one visible launch
     per side and one control-pair launch more, so every later A/B run carries the second appearance
     (Done-when 1, Q-0022-05);
   - the shot script returns the room and the marker, so every shot launch's row carries the room a
     **shown** window reads per OS — the reading the hidden boot result is compared against (fact 3);
   - the `os`-scoped allowlist entry (Q-0022-06) with `--self-check` cases (an entry for another OS is
     neither a finding nor stale; an entry for this OS that matches nothing is still stale), and the
     shot rows carry the OS scope they were read under in the record;
   - the self-check's pinned totals follow the new state (`bench.ts:1523-1532`: `smoke.total` 7 → 8,
     `separate` 39 → 43, `together` 21 → 23) and the workflow's launch-count comment follows
     (`.github/workflows/frontend-bench.yml:42-45`).
7. `docs/harness.md` rows 24, 25 and 27 and the bench's paragraph carry the new scenario, assertions,
   checks and case counts, read from this step's own CI run (the P9 re-quote's rule).

*Audits it must keep green:* `typecheck`, `frontend-baseline`, `docs-links`, `secrets`, `gofmt`,
`go vet`, `go build`, `go test ./internal/desktop/...`, and the whole `desktop` group on all three
OSes (the new checks read as run-and-skipped while the room is zero, and the new scenario passes with
its reason recorded). *Measurement row:* the P1 run of `frontend-bench.yml` — A = B's page, so every
metric must read not worse beyond noise and 7/7 shots byte-identical on both OSes. *Shots it may
change:* none, on either OS.

*Proof:* `mise exec -- bun Q:/scratch/pq-harness-run/run.ts desktop-smoke.ts desktopSmoke` and
`… desktop-ux.ts desktopUx` on this machine (Windows, hidden windows, 1280x800/1440x844): green, with
the room recorded as unset, the marker absent and the four new checks reported as run-and-skipped,
and the new scenario's 880x600 walk recording the same; the four new must-fail self-test cases via
the self-test gate fail with exactly their check names, the modal `PassCase` passes with the skip
record in the detail, and the desktop-smoke case fails with its finding;
`gofmt -l internal cmd`, `go vet`, `go build`, `go test ./internal/desktop/... -count=1`; the P1 run
of `frontend-bench.yml` — no page changed, so all seven states must be byte-identical on both OSes
and no allowlist entry may exist.

### P2 — Windows (facts 1, 2, 4; Done-when 1, 2 on Windows)

- `internal/desktop/desktop.go`: `TitleBarStyle: mygo.TitleBarHidden` and `TitleBarHeight: 40` for
  the windows backend only (a small per-OS switch beside the options, so P3 and P4 turn their own OS
  on).
- The page: the room-driven rules of Q-0022-01 in `desktop/web/styles/chrome.css` (the header's
  insets reserved inside the outer columns — brand `padding-left`, `.util` `margin-right` — and
  `--app-region: drag` scoped to `:root[data-titlebar]`) plus the small module that reads the room
  from `:root` at boot and keeps the marker current from `mygo:title-bar` (a document-level
  attribute; `desktop/web/src/chrome/top.ts` needs no new element).
- The buttons' theme (Q-0022-02): the new bound method, the regenerated `desktop/web/src/api/mygo.ts`,
  its `desktop/web/src/api/client.ts` wrapper, and the call from `desktop/web/src/actions.ts` and
  the boot path.
- The `titlebar` scenario gains the synthetic-pointer assertions, which only now can fire: at the
  probe point item 4 derived, `new MouseEvent("mousedown", {bubbles: true, cancelable: true, button:
  0, detail: 2})` must be taken over (`defaultPrevented`) and must move
  `window.mygo.window.isMaximized()` — asserted on Windows (maximize, then a second press back) and on
  macOS (the zoom path, Q-0022-03), and *recorded but not asserted* on Linux with Q-0022-04's reason;
  at a control inside the title area (`[data-act="theme"]`'s centre) a `mousedown` with `detail: 1`
  must **not** be taken over, and the control's own click must still switch the theme and back —
  Done-when 2's "Interactive controls inside the title area are clickable and do not drag the window".

*Audits it must keep green:* the ux matrix's 192 audits, 19 probes and 84 focus checks with 0 allowed
on all three OSes — including the four new checks, which now see a Windows room (the modal states'
strip samples read skipped with their reason, not findings); `desktop-smoke`'s
14 scenarios (the new one's 880x600 walk included) green; `package-smoke`'s `study` case;
`typecheck`, `frontend-baseline`. *Measurement row:* the P2 run — Windows every metric not worse
beyond noise; macOS the same, with its seven shots byte-identical. *Shots it may change:* Windows
only, all seven states, each with an `os: "windows"` allowlist entry whose reason says "the title area
the page draws under the hidden title bar (0022 P2)"; the macOS shots must stay identical — the
strongest available proof that the page rules are room-driven.

*Proof:* the P2 run (Windows: the seven states differ, every timing metric not worse beyond noise,
guards and the control pair as in 0019; macOS: 7/7 byte-identical); `desktop-ux`'s
`titlebar-room-covered`, `titlebar-room-paint`, `titlebar-drag-region` and `titlebar-strip-clip`
green on Windows with the room read at both audited sizes and at 880x600, and run-and-skipped with
the reason recorded on macOS and Linux (their window options are P3's and P4's); `desktop-smoke`'s
title read-back and the four synthetic-pointer assertions green on Windows, the takeover green on
macOS and Linux;
the scan of the shot pair at 1280x800 by eye against Q-0022-01 in the coordinator's review.

### P3 — macOS (facts 1, 4; Done-when 1, 2, 3 on macOS)

- The per-OS switch turns `TitleBarHidden` on for darwin (Q-0022-03), nothing else: the page rules are
  already room-driven, the traffic lights stay at their default place, and `TrafficLightPosition` is
  not used.
- The room is read before anything else is judged: this run's artifacts carry the hidden window's
  values (`desktop-smoke`'s boot and `titlebar` results) and the shown window's (the shot rows, P1),
  so whether a hidden macOS window reports a room is answered from the pair (fact 1); if a hidden
  window reads `0px` while the shown one reads a room, this step shows the window for that one launch
  — `harness/lib/desktop-page.ts:287` already allows it — with the reason recorded. The AppKit band's
  height is not `TitleBarHeight` (fact 1), and Q-0022-01's rule holds for whatever value comes back,
  so no page change is conditioned on it.
- The title area itself is this step's evidence, not only the allowlist entry: the macOS `today` and
  `today-dark` shots at 1280x800 are read by eye against the same run's own box list (its 880x600
  walk and the ux audits at both sizes), so a clipped or misplaced control is a finding the artifact
  can show.
- The double-click assertion runs on macOS: the zoom path, with `AppleActionOnDoubleClick` named in
  its failure message (Q-0022-03).

*Audits it must keep green:* the same matrix, smoke and package sets as P2, now with a macOS room for
the four checks to read **if the audited launch reports one** — the ux gate's launches and the
880x600 walk are hidden (`harness/lib/desktop-page.ts:285-287`), so a hidden window that reads `0px`
while the shown one reads a room makes the audits run-and-skipped, and this run's readings are
quoted in the record as the coverage limit; the macOS job's focus and accessibility checks
unchanged. *Measurement row:* the P3
run — macOS every metric not worse beyond noise; Windows the same, with its seven shots
byte-identical. *Shots it may change:* macOS only, all seven states, `os: "macos"` entries with the
same reason form — and this step **removes P2's `os: "windows"` entries**, because this run's A
already carries the Windows title bar and they would otherwise read as stale (Q-0022-06).

*Proof:* the P3 run (macOS differ + allowlisted, Windows identical); the macOS job's two room
readings per launch kind, the window-title read-back, the title-area read at 1280x800 and the four
ux checks green where the audited launch reads a room — where only the shown launch reads one, the
audits' recorded skip and the shown reading are the coverage limit, not a green claim; the takeover
and zoom assertions green on macOS; the full-screen and inactive-window
appearances checked by eye against the macOS HIG (the lights' colour is the system's, fact 4).

### P4 — Linux (facts 1, 3, 4; Done-when 1, 2 on Linux)

- The per-OS switch turns `TitleBarHidden` on for linux; Q-0022-04's disclosure covers the
  compositors that decorate windows themselves. The page reserves the room GTK reports, if any: the
  backend is GTK 3 (fact 1), its height is `TitleBarHeight` when the bars have one, and the widths
  come from the desktop's `gtk-decoration-layout`.
- The reading: the hidden window's room from `desktop-smoke`'s boot and `titlebar` results and the
  *shown* window's from the bench's `linux-smoke` job (`--smoke`, visible WebKitGTK windows under
  Xvfb, `.github/workflows/frontend-bench.yml:213-257`) — the shot rows carry the room from P1, so
  the shown value is in this run's own artifact. A hidden window that reads zero while the shown one
  reads a room is written down as a coverage limit of the hidden gates, not smoothed over.
- The double-click is split as Q-0022-04 says: the takeover assertion is real on Ubuntu's bare Xvfb
  (no window manager to act on `gtk_window_maximize`), the `isMaximized()` reading is recorded beside
  it and **not** asserted, and the effect on a real desktop is the manual pass.

*Audits it must keep green:* the Ubuntu `desktop`, `package` and `self-test` jobs, with the four
checks reading the Linux room; Windows' and macOS's gates unchanged. *Measurement row:* the P4 run —
no page or window-option change on Windows or macOS, so both columns' metrics must read not worse
beyond noise and both OSes' shots must stay byte-identical. *Shots it may change:* none in the
comparison jobs (Linux takes no part in the shot comparison; its two `today` cells come from
`linux-smoke`), and this step **removes P3's `os: "macos"` entries** for the same staleness reason
(Q-0022-06).

*Proof:* the P4 run and the Ubuntu desktop jobs: the room readings per launch kind, the four ux
checks green (or the recorded difference with its reason), `desktop-smoke` green on Ubuntu with the
takeover assertion and the recorded maximize reading, the AppImage-built window checked by the
package smoke's study case.

### P5 — evidence, decisions, shots and the manual pass (Done-when 1, 2, 4, 5)

- `docs/evidence/0022-integrated-title-bar.md`: the per-OS room readings quoted from the named runs,
  the window-title read-back per OS, **the six Done-when-1 cells with the artifact path and run id of
  each** (fact 5), the per-OS shot verdicts and allowlist entries with their reasons, the bench's A/B
  tables for each step (N=9, warm-up 2, the control pair's verdict), the manual-pass record — by
  hand, with OS-level captures: hover the maximize button (Snap layouts), right-click the title area,
  double-click it, drag it, resize from the top edge and from both sides, minimize, maximize, close,
  and the full-screen and inactive looks in both appearances — and the known limits (the minimum
  width against narrow snap zones, the Wayland SSD case, no window manager under CI's Xvfb, no
  system window menu on right-click if the pass does not find one, macOS 27 absent, no OS-level
  capture in CI).
- The 0012 sheets regenerated per Q-0022-07 with the per-sheet change list; `README.md` and
  `README.zh-CN.md` keep their embedded sheet (replaced, not remade differently).
- `docs/decisions/OPEN-QUESTIONS.md`: Q-0022-01…09 with their outcomes (appended when the plan was
  accepted, updated here); `docs/harness.md`'s rows carry the final counts.
- The goal-level comparison of Q-0022-06: the coordinator dispatches `frontend-bench.yml` from the
  throwaway branch that carries the final head plus the fourteen entries (`workflow_dispatch` takes
  any ref that carries the workflow file, `a-ref`/`b-ref` decide the two trees), names its run id in
  the record, and throws the branch away.

*Audits it must keep green:* `docs-links`, `secrets`, `install-guide`, `typecheck`, `frontend-baseline`
and the whole gate matrix on the final tree — this step changes no product file, so every earlier row
still reads. *Measurement row:* two runs — P5's own (A = P4's merge, B = the head: identical page, so
all seven shots byte-identical on both OSes and every metric not worse beyond noise) and the goal-level
one from the throwaway branch of Q-0022-06 (A = `771a277`, B = the final head: the title area differs
on both OSes and every entry names it). *Shots it may change:* none in the merged tree; the throwaway
branch's entries exist only in that branch.

*Proof:* `run.ts docs-links.ts docsLinks`, `run.ts secrets.ts secrets`, `run.ts install-guide.ts
installGuide` (untouched, green); the evidence record's numbers cross-checked against the artifacts by
the coordinator; the sheets' diff reviewed per sheet; the coordinator commits and closes the goal.

## Done-when mapping

| Contract item | Steps | Evidence |
|---|---|---|
| 1 the title area is the app's | P2–P5 | the six cells: `today` + `today-dark` per OS at 1280x800 (the Windows and macOS bench jobs, the Linux render smoke), plus the 0012 sheets' four Windows appearances (Q-0022-07); the manual pass's OS-level captures for the frame and the buttons (Q-0022-05); `titlebar-room-covered` and `titlebar-room-paint` green |
| 2 window behaviour is intact | P1, P2–P5 | the titlebar scenario at 880x600 plus the ux matrix at the gate's two real sizes (1280x800, 1440x844); the title read-back and the synthetic-pointer assertions (takeover everywhere, maximize/restore on Windows and macOS, disclose-only on Linux); `titlebar-drag-region` and `titlebar-strip-clip`; the manual pass for Snap, the real pointer drag, the top-edge resize and full screen |
| 3 keyboard, assistive use and the window title | P1, P5 | the platform's own title read back per OS (`GetWindowTextW`/`[NSWindow title]`/`gtk_window_get_title`); `desktop-ux`'s focus and accessibility checks unchanged (84 focus checks, 0 allowed) |
| 4 no regression in speed or stability | P1–P5 | each step's bench run: every metric's verdict against the control pair's floor (Q-0019-02), Windows and macOS; the guards' rows |
| 5 the reference shots are updated honestly | P2–P5 | the per-OS bench artifacts and the allowlist entries with reasons; the 0012 sheets' per-sheet list (Q-0022-07) |
| constraints: only the window chrome and the page area it frees | P1–P5 | the room-driven rule (Q-0022-01): P2's macOS column and P3's Windows column byte-identical; no study view changed below the header; `window.mygo.window` untouched |

## Risks (each with how it is detected)

- **A hidden window reports no room on macOS or Linux** (MyGo reads AppKit frames and, on Linux, GTK
  allocations) → the checks' conditional form reports "no title bar" while the shown window has one.
  Detected by the room reading in the boot result per OS; the ux audits and the 880x600 walk are
  hidden launches too (`harness/lib/desktop-page.ts:285-287`), so their checks then read
  run-and-skipped and the record carries the hidden/shown pair instead of a green claim; the fix is a
  shown launch for that case with the reason recorded, or, where a desktop has no room to report, the
  Q-0022-04 disclosure.
- **The page's change is not room-driven after all** → P2's macOS shot column (or P3's Windows column)
  shows a difference; it is a finding, because only the step's own OS is allowlisted (Q-0022-06).
- **The caption buttons' colours do not follow the app's theme** (the Windows default is the system
  appearance) → the manual pass sees light glyphs on a light title area; the Go test on the new bound
  method and the page's call site are the mechanical half. Detection: the manual pass's captures in
  both appearances while the system is in the other one.
- **A control or a painted capsule sits under the buttons at some state or size** →
  `titlebar-room-covered` names the control and `titlebar-room-paint` the painted box (the `.util`
  capsule is the round-2 `padding-right` case, pinned by the self-test); the contract's own sizes
  (880x600, 1280x800, 1440x844) and the 24 states are the search space, and the self-test fixture
  proves each check fires.
- **Snap layouts do not appear over the caption** (MyGo's `HTMAXBUTTON` answer comes from its layered
  **child** window — `mygo@v0.10.0/internal/windows/titlebar.go:268-300` — while Microsoft's rule and
  samples describe the top-level `WndProc`; whether the shell honors the child's code is MyGo's
  assumption, named in Q-0022-09) → the manual pass: hover the maximize button. On Windows MyGo's
  drawn caption is itself the replacement this goal must disclose and verify (Q-0022-09): this plan
  keeps MyGo's caption window rather than swapping in a page-drawn
  control, and the guard is that the window keeps `TitleBarStyle: TitleBarHidden` with its buttons,
  never `HideWindowButtons`.
- **A room appears without a drag region** (the page's marker and the room disagree, an event lost) →
  `titlebar-drag-region` names the strip with no draggable point, and the `titlebar` scenario records
  the marker beside the room, so the artifact shows which of the two is missing.
- **macOS's room is not the ~28 pt the AppKit formula suggests** (it may report the whole content
  view) → the value is read from P3's artifacts before anything else is judged, Q-0022-01's rule holds
  for any value, the checks are conditional, and a hidden reading of `0px` beside a shown room is a
  recorded coverage limit.
- **The minimum window hides a control the audited sizes do not** → the `titlebar` scenario walks all
  seven routes in both appearances at 880x600 and fails on any finding, while the ux matrix keeps the
  two audited sizes.
- **Right-click in the title area opens no system window menu** (MyGo forwards `contextmenu` to a page
  that draws none) → the manual pass right-clicks once, and the record carries the answer as a limit,
  not as a claim (fact 4, Q-0022-09).
- **The top edge stops resizing** → the manual pass; MyGo's `newTopEdge` runs for every captionless
  window and `captionChanged` lays it out again on every state change
  (`internal/windows/window.go:245`, `internal/windows/titlebar.go:173-179`) — nothing in this plan
  removes it, and the manual pass is what checks it.
- **Full screen shows a reserved strip that is not there** → the room is `0px` in full screen
  (measured, fact 3) and the CSS and the marker fall back to today's layout and press handling; the ux
  audits run at their sizes and the manual pass looks at full screen.
- **Screenshots flap between runs** → the bench's control pair (two launches of one build) must be
  byte-identical before any verdict is read; a flapping state is a run-level failure, never an
  allowlist entry.
- **This machine is at 100 % CPU** (the brief's known fact) → local window runs are advisory; CI's
  three-OS run is the authority, and the bench's guards say whether a local run is readable.
- **The 0012 automation build cannot be reproduced** → Q-0022-07's fallback: the record says so, the
  six cells' own mechanical PNGs carry Done-when 1, and the sheets' regeneration is the owner's
  action.

## Evidence plan

Local (Windows, hidden windows): `run.ts desktop-smoke.ts desktopSmoke`, `run.ts desktop-ux.ts
desktopUx`, `run.ts self-test.ts selfTest` (for the four new must-fail ux cases, the modal pass case
and the desktop-smoke case),
`run.ts typecheck.ts typecheck`, `run.ts docs-links.ts docsLinks`, `run.ts secrets.ts secrets`; the
Go checks the step touches
(`gofmt`, `go vet`, `go build`, `go test ./internal/desktop/... -count=1`). The scratch probes named in
the facts are not evidence: they are how the facts were established, and their results are quoted with
their paths.

CI: the branch's `gate.yml` matrix — `desktop` and `self-test` on ubuntu, macOS and Windows — and one
`frontend-bench.yml` run per step (A = the previous merge, B = the head) plus the macOS and Windows
jobs' artifacts (JSON, log, shots, and the room each launch kind read) and the Ubuntu `linux-smoke`
job's seven visible shots. The numbers in `docs/evidence/0022-integrated-title-bar.md` are read from
those artifacts and named by run id. Disclosed limits: the OS-level capture of the frame and the
buttons is manual (Q-0022-05); macOS 27 is not a runner; Ubuntu's WebKitGTK comes from the archive and
its runner has no window manager (Q-0022-04); the hidden gates on Linux read whatever GTK allocated
(P4).

## Facts each step hands to the next

- **P1 → all:** the exact room values each OS reports (the hidden boot and `titlebar` results against
  the shown shot rows), the title read-back's shape, the four checks' names and messages, the modal
  skip's record, the probe point's derivation, the marker rule, the `os`-scoped allowlist rule, and
  the launch plan the seventh shot state moved.
- **P2 → P3, P4, P5:** the Windows room (`40px / 0px / 138px / 40px` at 1280x800, measured here; the
  page's own reading from CI), the P2 run's shot verdicts per OS (Windows differ, macOS identical),
  and the allowlist entries P2 added.
- **P3 → P4, P5:** macOS's room values, whether a hidden macOS window reports them, its title
  read-back, and the `hiddenInset` question settled (Q-0022-03).
- **P4 → P5:** Linux's two readings (hidden and shown), the `gtk-decoration-layout` in effect on the
  runner, and the SSD disclosure if it applies.
- **Any step that has to show a window or fall back → P5:** the finding quoted from the run, so the
  evidence record's limits are evidence, not opinion.

## Coordinator actions needed

- **The bench run of each step** (P1 first): either open the step's draft pull request — the bench
  runs on `pull_request` — or dispatch `frontend-bench.yml` from the step's own ref with `a-ref`/
  `b-ref` (`workflow_dispatch` takes any ref that carries the workflow file, and `paths` means a
  docs-only step like P5 must be dispatched); the goal-level run of Q-0022-06 is a dispatch from the
  throwaway branch, never a pull request.
- **The manual pass** (Q-0022-05), which now owns only what a page capture cannot show: hover the
  maximize button for Snap layouts, right-click the title area, double-click, drag, resize from the
  top edge and the sides, minimize/maximize/close, and the full-screen and inactive looks in both
  appearances, with OS-level captures that include the frame and the buttons; the captures and the
  checklist go into the evidence record. Required on Windows; on macOS and Linux it is an owner
  action where a machine of that OS is available, and the record says which cells it covered.
  When: after P2 for Windows, after P4 for the final look, again on the final head if a later step
  moved the title area.
- **Regenerate the 0012 contact sheets** for the final head (Q-0022-07) and review the per-sheet
  change list before the goal closes.
- **Append Q-0022-01…09** to `docs/decisions/OPEN-QUESTIONS.md` **before P1's branch is cut** — the
  goal's own rule is that a default is disclosed when it is taken — and take the Q-0022-02 decision
  (the owner's, on the one new bound method) before P2 starts; if P3 or P4 finds that a hidden window
  cannot report its room, decide per Q-0022-04/05 between a shown launch, a disclosure and a contract
  change. Nothing else: no secrets, no remotes, no release.
