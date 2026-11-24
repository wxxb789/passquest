# Desktop web frontend (goal 0011)

Plain TypeScript, no framework, bundled by Bun (Q-0011-02). It ports the owner-chosen "C · Stage"
prototype (`design/prototypes/c-stage.html`) into modules; the DOM structure and class names are the
prototype's, so its CSS applies unchanged.

## Engine baseline

The frontend is written for the engines Pass Quest ships on, and
[Q-0019-01](../../docs/decisions/OPEN-QUESTIONS.md) names them. CI run `45045599642` on the P1 pull
request's head `aa1580a` (its merge on main is `71c366b`) records the engines: its Windows and macOS
jobs write every launch's engine UA into `launches.rows[].engine` (each tree keeps the same string
once at `trees.<a|b>.userAgent`), and its `frontend bench (ubuntu-latest render smoke)` job renders
every state green in a visible WebKitGTK window on Xvfb 1920x1080. The OS-level versions come from
the runner images and the Ubuntu archive named per row; those images move, so the gate pins the
policy, not the build:

| Platform | Engine | Baseline | Where the version is recorded |
|---|---|---|---|
| Windows 11+ | WebView2 (Chromium) | Evergreen: current stable Chromium (Chrome/157 at scout time, 2026-11-22; the Evergreen runtime updates to the latest stable, `learn.microsoft.com/…/webview2/concepts/versioning`) | the bench record's `launches.rows[].engine` per launch; run `45045599642` recorded `Edg/157.0.0.0` (Chromium 157) |
| macOS 26+ | WKWebView (system WebKit) | Safari 26.x WebKit (the final goal-wide run's runner read macOS 26.7.2; macOS 27 is above the floor, not the baseline) | the `macos-26` runner image readme (scout time, fetched 2026-11-22: macOS 26.6.1, Safari 26.6 `21624.4.5.11.5`; plan fact 1) and the final run `45338748380` attempt 2 (its macOS job read 26.7.2; record `runner.ImageOS`/`runner.ImageVersion` = `macos26`/`20261117.0516.1`) — the images move, so each run's build is named, not pinned; the bench record's `launches.rows[].engine` per launch, which run `45045599642` recorded as `AppleWebKit/605.1.15`, Safari's UA component |
| Linux (second-tier) | WebKitGTK | 2.52.6 on `ubuntu-latest` (the archive's `libwebkit2gtk-4.1-0`; MyGo `dlopen`s it) | `packages.ubuntu.com/{noble,resolute}/libwebkit2gtk-4.1-0` (`2.52.6-0ubuntu0.24.04.1` / `2.52.6-0ubuntu0.26.04.1`, fetched 2026-11-22); run `45045599642`'s `frontend bench (ubuntu-latest render smoke)` job renders every state green on its Xvfb display and records only the engine's user agent, not the WebKitGTK version |

The final goal-wide run `45338748380` re-read the same two UA strings on its Windows and macOS jobs
(`Edg/157.0.0.0`; `AppleWebKit/605.1.15`), and its `frontend bench (ubuntu-latest render smoke)` job
renders the six states green in a visible WebKitGTK window (`frontend-bench-smoke.json`: `ok: true`,
7 launches, `guards 6 checked, 0 failed`); P9's evidence record names every run behind these numbers.

- **JavaScript level.** `desktop/web/tsconfig.json` holds the compiler half of the baseline; the
  `frontend-baseline` gate (`harness/gates/frontend-baseline.ts`) pins these values:

  | tsconfig key | Value |
  |---|---|
  | target | `"target": "ES2024"` |
  | lib | `"lib": ["ES2024","DOM","DOM.Iterable"]` |

  The bundle step stays one Bun call
  (`bun build desktop/web/src/main.ts --outdir … --target browser --minify`): `bun build` has no
  browser-version target (`--target` is `browser|bun|node`), so nothing downlevels the output and the
  three engines' gate runs are the runtime proof.
- **CSS level:** the features today's stylesheets use are supported by all three baseline engines
  (WebKitGTK 2.52 is a later trunk generation than the Safari release each feature landed in; the
  citations are in the plan's fact 1), except `scrollbar-color` (Safari 26.2): on the macOS 26.0/26.1
  floor it is ignored and the `::-webkit-scrollbar*` rules beside it paint the bar
  (`styles/base.css:34-37`). The `frontend-baseline` gate fails on any vendor-prefixed
  construct not on its allowlist. P3 removed the two dead twins (`-webkit-backdrop-filter` and
  `-webkit-mask-image`, each beside its unprefixed property) and the `:-webkit-any()`/`:-moz-any()`
  selectors the minifier injected from `styles/desktop.css:38`'s `:not(.q, .c)` list, which now reads
  `:not(.q):not(.c)`; the source's vendor tokens fell from 26 to 6 and the bundle's from 28 to 6: the
  `6`s are the `frontend-baseline` gate's pass line (`6 vendor token(s) in the source, 6 in the
  bundle`), the `26` is `git grep -oE -- "-webkit-[a-z-]*|-moz-[a-z-]*|-ms-[a-z-]*" 98752a7 --
  desktop/web/styles | wc -l`, and the `28` the same `grep -oE` over the `main.css` of a
  `git archive 98752a7 desktop/web` tree built with the bundle step above. That leaves the six
  WebKit-only constructs (`-webkit-font-smoothing` and the five `::-webkit-scrollbar*`
  pseudo-elements) that the gate's allowlist keeps with their reasons (Q-0019-06).
- **Linux is second-tier** (Q-0000-07): it must still render and work, and no feature is adopted that
  WebKitGTK 2.52.6 cannot run — the bench's `linux-smoke` job renders every state in a visible
  WebKitGTK window on the runner's Xvfb.

## Entry and build

- Entry: `src/main.ts`. It imports `styles/index.css`, which declares the cascade layers in file order
  (`@layer tokens … desktop`), `@import`s the prototype CSS split by section (`tokens.css` …
  `responsive.css`, values verbatim, one `layer(…)` per file) plus `desktop.css` (port-only
  additions), and imports `motion.css` unlayered: the reduce-motion and shot-mode `!important`
  blocks must stay outside the layers, which would outrank the harness's own injected settle/shot
  styles.
- Build (what the harness runs):

  ```sh
  bun build desktop/web/src/main.ts --outdir internal/desktop/web/dist --target browser --minify
  ```

  Bun emits `main.js` (ES module) and `main.css` alongside it. The output is generated and gitignored.
- Typecheck: `node node_modules/typescript/bin/tsc --noEmit -p desktop/web/tsconfig.json`
  (own project: strict, DOM libs, no Bun types).

## Host page contract

`internal/desktop/web/index.html` (owned by the shell) loads `dist/main.css` and
`<script type="module" src="dist/main.js">`, and provides the prototype skeleton:

```html
<body data-room="calm">
<div class="atmos" aria-hidden="true"></div>
<div class="app">
  <header class="top" id="top"></header>
  <main class="stage" id="stage"></main>
</div>
<nav id="dock"></nav>
<div id="orb"></div>
<div id="layer"></div>
</body>
```

`<html>` gets `data-theme`, `lang` and the `shot` class from the app. URL parameters `?theme=`,
`?lang=`, `?shot=1` work as in the prototype; theme and language persist in `localStorage`
(`pq-stage-theme`, `pq-stage-lang`) and nothing else does.

## Layout

```text
src/main.ts         error bar, delegated events, boot
src/actions.ts      ACT registry (data-act), routing and boot recovery
src/controllers/    per-room controllers; service.ts is the only StudyApi call path (busy guard, failure toast)
src/screens.ts      composes views + chrome + layers from state
src/core/           state, i18n (T(zh, en)), html (escaping tag), router, render/layers/focus,
                    keyboard, clock (display-only), toast (textContent), prefs (theme + lang)
src/chrome/         top bar, dock, orb
src/components/     satellite, constellation, evidence dots, passage (+ marks), options,
                    condition capsule, call strip, failure line, sheet, dialogs (incl. the hint gate)
src/views/          today, practice, feedback, coach, assess, report, anki, settings, states
src/api/mygo.ts     generated by MyGo from the bound Go service (MYGO_GENERATE); never hand-edited
src/api/types.ts    re-exports the generated types, plus StudioError and CoachMode
src/api/client.ts   StudyApi = the generated `Study` client, called through window.mygo;
                    parseCallError, bindApi(), onGoEvent()
```

`main.ts` binds the StudyApi before boot; outside a MyGo window every screen renders its
"not connected" state and no number is shown.

## Rules the code keeps

- Every interpolation goes through `html\`\``, which escapes `& < > " '`. `raw()` is used only by
  `icons.ts` for the static SVG path table. Model replies, learner text and imported text are text.
- The Anki card preview is the core's rendered card inside `<iframe sandbox="" srcdoc>`.
- Clocks display the distance to server timestamps (`presented_at`, deadlines); they never count.
- No demo data: views take every number from typed data parameters.
- Ids, hashes, error codes and CLI commands are never the learner's text: a failure shows its
  localized title and next step (`components/failure.ts`); the core's message folds into a
  `[data-raw]` details line. Registry names and model ids carry `translate="no"`.
- Each audited state marks its one primary action with `data-primary` (desktop-ux gate,
  `harness/ux-allow.json`); after a route change focus lands on `#stage`, never on `<body>`.
