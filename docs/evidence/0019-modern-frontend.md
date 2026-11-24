# 0019 — A modern frontend written for the engines Pass Quest supports: evidence

Contract: [`../goals/archive/0019-modern-frontend.md`](../goals/archive/0019-modern-frontend.md). Plan:
[`../plans/2026-11-22-1800-0019-modern-frontend.md`](../plans/2026-11-22-1800-0019-modern-frontend.md).
Decisions: Q-0000-07, Q-0000-09, Q-0011-02, Q-0012-01, Q-0019-01…06 in
[`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

**State: P1–P8 and the P6 fix are merged on `main`** (P1 `71c366b` … P8 `78290cf`, the fix `bb94a9d`);
this record is P9's deliverable and the coordinator commits it with the goal close. Every
number below is read from the named CI run's own artifact: the bench records
(`frontend-bench-<os>/frontend-bench-record.json`; on Linux `frontend-bench-ubuntu-smoke/frontend-bench-smoke.json`)
and the run's log, downloaded per run id, or from the gate run's log line named in the row. Values in
tables are the tool's own rounded log values where the log prints them (for example `15.7`, `31.2`), and
the record's exact fields where a table quotes a record path. Nothing is quoted from memory, and no
number is compared across runs.

## The runs this record cites

| Run | What it is | A → B | Result |
|---|---|---|---|
| 45045599642 | P1's pull-request run (`aa1580a`, merge `71c366b`): the method's first run and its control pair | `63ce6e4` → `aa1580a` | both OS jobs `ok`; every metric `worse=false` (the run predates Amendment 13's control-pair floor and prints the fixed 5 %/5 ms one, e.g. `gap 23.7 vs floor 72.67`); shots: tree A predates the capture hook so the A/B pair is `not-measured`, and the run's own control pair reads `shots … 6/6 state(s) byte-identical`; guards `64 checked, 0 failed, not measured 1` per OS |
| 45090760750 | P2 the baseline written down | `71c366b` → `019884d` | both jobs `ok`, every verdict `worse=false`; shots A/B and control `6/6` |
| 45099610142 | P3 the workarounds go | `98752a7` → `8d8fe7c` | both jobs `ok`; shots `6/6`, control `6/6` |
| 45123200561 | P4 native dialogs | `89a9494` → `62aace2` | both jobs `ok`; shots `6/6`, control `6/6` |
| 45139959664 | P5 popovers | `74a8283` → `5c777f1` | both jobs `ok`; shots `6/6`, control `6/6` |
| 45174298421 | P6 entry transitions | `a0e2b3e` → `47470d3` | both jobs `ok`; shots `6/6`, control `6/6` |
| 45291381011 | P7 nesting and cascade layers | `5aa4155` → `897e588` | both jobs `ok`; shots `6/6`, control `6/6` |
| 45312448788 | P8's gate run (`e46bff3`; the branch was docs-only, so the bench's paths filter ran no bench) | — | `ok`: the desktop group green on ubuntu, macOS and Windows by this run's own lines — `PASS  desktop smoke (493 assertions over 13 scenarios)` on Windows, `492` elsewhere; `PASS  desktop ux (192 audits …, 19 probes, 84 focus checks, 0 allowed …)` on the three |
| 45315096427 | `workflow_dispatch`: P1 vs P8 | `71c366b` → `78290cf` | Windows `failure`: `metric frames.p95 is worse beyond noise … median 31.3 -> 46.8, gap 15.5 (beyond)`; macOS `ok` |
| 45316313813 | `workflow_dispatch`: P1 vs P8, second draw | `71c366b` → `78290cf` | Windows `failure`: `frames.p95 … median 15.7 -> 31.2` **and** `frames.longest … median 31.3 -> 46.9, gap 15.6 (beyond)`; macOS `ok` |
| 45316326864 | `workflow_dispatch`: P1 vs P5 | `71c366b` → `a0e2b3e` | both jobs `ok`; frames.p95 15.7 → 15.7 on Windows |
| 45318388579 | `workflow_dispatch`: P1 vs P6 | `71c366b` → `5aa4155` | both jobs `ok`; Windows frames.p95 31.22 → 31.3 (a two-tick base) |
| 45318421329 | `workflow_dispatch`: P6 vs P7 | `5aa4155` → `7056351` | both jobs `ok`; Windows frames.p95 31.2 → 30.42 |
| 45320531877 | `workflow_dispatch`: P6 vs P7, second draw | `5aa4155` → `7056351` | Windows `failure`: `the same-build control pair of frames.p95 reports a worse head … this run's comparison is not readable`; macOS `ok` |
| 45320505404 | `workflow_dispatch`: P5 vs P6 | `a0e2b3e` → `5aa4155` | Windows `failure`: `frames.p95 … median 15.7 -> 31.2`, and the same run's control pair also read worse (readyMs and frames.p95), so the run is unreadable; macOS `ok` |
| 45330890948 | `workflow_dispatch`: P5 vs the rejected `will-change` head `1bcbf9d` | `a0e2b3e` → `1bcbf9d` | Windows `failure`: `state sheet-open differs from the base and no allowlist entry names it`; macOS `ok`; the candidate was rejected (Amendment 19(c)) |
| 45335594677 | `workflow_dispatch`: P5 vs the keyframe-fix head `6220f46` | `a0e2b3e` → `6220f46` | both jobs `ok`; Windows frames.p95 31.22 → 31.22 (the run's base state hides a one-tick step) |
| 45336617606 | the fix PR's gate run (`ecd11f0`), the final tree's CI counts | — | `ok` on all three OSes; the counts table below quotes its lines |
| 45336617620 | the fix PR's bench run: P8 vs the fix | `78290cf` → `ecd11f0` | both jobs `ok`; shots `6/6`, control `6/6`; Windows frames.p95 31.3 → 31.2 |
| 45338748380 | the final goal-wide run: P1 vs the final tree | `71c366b` → `bb94a9d` | attempt 1: macOS `failure` (`frames.longest … median 81 -> 107, gap 26 (beyond)`); attempt 2: all three jobs `ok`; both attempts Windows `ok` |

## The method every run records

The record's `method` block (quoted from `45338748380/frontend-bench-windows-latest`): `samples: 9`,
`warmup: 2`, `contentSize: "1280x800"`, `alternation: "A then B, launch by launch, serially in this
job"`, `control: "two interleaved sets of 9 launches of the B build, after the comparison phase"`, and
the verdict rule `head Q1 > base Q3 and the median gap >= the floor = max(5 ms, 5% of the base median,
the larger of the two control-pair sides' IQRs (q3 - q1)) where the metric has control-pair samples, and
max(5 ms, 5% of the base median) where it has none` (Amendment 13; Q-0019-02). `method.guards`:
`document.visibilityState === "visible" and 30 requestAnimationFrame ticks within 4000 ms; a launch
whose guard fails records no sample; the prepare launch is not measured and its window is hidden; a
hidden window's page draws no frames on macOS and Linux (mygo@v0.10.0 page.go, ThrottlingOff), so
its script waits on no animation frame`. Every
run of the goal reads `guards: 64 checked, 0 failed, not measured 1` per OS job (the not-measured launch
is the hidden prepare); the final run's guard ticks fit the budget with room — `ticks 30..30, elapsed
444..530 ms` (Windows) and `460..1087 ms` (macOS). Shots: six states (`today`, `sheet-open`,
`practice-independent`, `feedback-coach`, `settings-row`, `report`), one visible launch per state per
side, byte equality against `harness/frontend-visual-allow.json` (Q-0019-04).

## Done when 1 — the baseline is written down

The documented baseline and the artifacts that carry the same values:

| Reading | Where written | Where measured |
|---|---|---|
| Windows: WebView2 Evergreen = current stable Chromium; runner read `Chrome/157.0.0.0 … Edg/157.0.0.0` | `desktop/web/README.md` engine table, `docs/conventions.md` stack section (both name WebView2/Evergreen, WKWebView Safari 26.x, WebKitGTK 2.52.6, ES2024) | every record's `trees.<a\|b>.userAgent`; run 45338748380's Windows record reads `Edg/157.0.0.0`, the same string as the P1 control run 45045599642 |
| macOS: WKWebView = the system WebKit, Safari 26.x (runner macOS 26.7.2, image `macos26 20261117.0516.1`) | same table, with the runner image readme as the source | record `runner.ImageOS`/`ImageVersion`; `trees.b.userAgent` reads the frozen `AppleWebKit/605.1.15` UA component in every run |
| Linux second-tier: WebKitGTK 2.52.6 (the archive's `libwebkit2gtk-4.1-0`; MyGo `dlopen`s it) | same table | the render-smoke record's UA (`Version/60.5 Safari/605.1.15` on run 45338748380) and the job's own green line, below |
| JavaScript level ES2024 | `desktop/web/tsconfig.json` `"target": "ES2024"`, `"lib": ["ES2024","DOM","DOM.Iterable"]`; the README carries both strings | gate run 45336617606: `PASS  frontend baseline (ES2024 target/lib pinned, README carries both, all 4 allowlist entries fire on their own side; 6 vendor token(s) in the source, 6 in the bundle)` on all three OSes |
| No feature is adopted that WebKitGTK 2.52.6 cannot run | the README's Linux bullet; Q-0019-01 | the ubuntu job of every bench run is green (Linux tier, below) |

## Done when 2 — old-engine workarounds are gone

| Reading | Command / line | Value |
|---|---|---|
| source vendor tokens, before | `git grep -oE -- "-webkit-[a-z-]*\|-moz-[a-z-]*\|-ms-[a-z-]*" 98752a7 -- desktop/web/styles \| wc -l` | `26` (re-run 2026-11-24 for this record); the plan's fact 2 carries the same command with the per-file split (chrome 7, base 6, anki 4, coach 2, layers 2, orbit 2, primitives 2, assess 1) |
| source vendor tokens, after | same command on `bb94a9d -- desktop/web/styles` | `6` — `base.css:8` `-webkit-font-smoothing`, `:34-37` the `::-webkit-scrollbar*` family; the gate's own line: `6 vendor token(s) in the source, 6 in the bundle` (run 45336617606) |
| built vendor tokens, before | the `main.css` of the bundle step, built from a `git archive 98752a7 desktop/web` tree; the after side is the gate's bundle line on the final tree (`6`) | `28`: the +2 over the source are the `:-webkit-any()`/`:-moz-any()` pair Bun's minifier injected from `desktop/web/styles/desktop.css`'s `:not(.q, .c)` list, now written `:not(.q):not(.c)` (P3; Amendment 11) |
| what stayed, and why | `harness/gates/frontend-baseline.ts` `ALLOWLIST` (4 entries, 6 tokens: `-webkit-font-smoothing`, the `-webkit-scrollbar`/`-button`/`-corner` family, `-track`, `-thumb`, each with its reason) | Q-0019-06; an entry that matches nothing fails the gate (`all 4 allowlist entries fire on their own side`) |
| the check cannot silently regress | `harness/gates/self-test/frontend-baseline.cases.ts` (a tree at `ES2022`; a stray `-webkit-backdrop-filter` that must fail in the source and in the bundle it travels into; a hand-written `:-moz-any(` selector; a bundled `:-webkit-any(` fixture; an entry matching nothing) | the final self-test: `PASS  harness self-test (163 cases (15 must pass), 26/26 gates` on all three OSes (run 45336617606) |

## Done when 3 — modern features: adopted, and left out

Each adopted feature and the hand-built code it removed (details and the per-round measurements are in
the plan's amendments 14–19; the gates below are the behaviour oracle):

| Adopted | Removed / replaced | Proof it changed nothing visible |
|---|---|---|
| `<dialog>` + `showModal()` for the centered dialog and the bottom sheet | the scrim element and its animation, the manual `inert` toggling, the autofocus fallback where it covered the dialog, the `ui.ret` restore where it could land (the app keeps its own click dismissal, focus entry and opener record — Amendment 14) | P4's run 45123200561 (`6/6` shots both pairs); run 45336617606's `desktop ux` and `desktop smoke` lines; the `outside-click-not-closing`, `dialog-focus-*` and `modal-not-trapping` checks |
| `popover="auto"` for the mark popover, `popover="manual"` for the toast | the mark popover's `z-index: 45` and its hidden-state handling (the app keeps its own dismissal because native light dismiss cannot clear `pendingRange`, and no gate can drive the engine path — Amendment 15); the toast's dead `z-index: 60` | P5's run 45139959664; the probe pairs in Amendment 15 (toast-alone and coachpop byte-identical; markpop 241 px at ≤2/255; the two intended stacking differences are disclosed there, outside the bench's six states) |
| `@starting-style` entry transition for `.dlg` (the sheet keeps the base's `sheetin` keyframe; the backdrop keeps the base's `fade` keyframe) | the `.dlg`'s one-shot `dlgin` keyframe | P6's run 45174298421, the fix runs 45335594677/45336617620, and the final run; the motion check's rows (`entry-transition-missing` when removed) with the backdrop rows capability-gated (Amendment 16(e)) |
| CSS nesting (source-time only; Bun flattens it) and `@layer` in file order, `motion.css` unlayered | file-order-dependent specificity; two `!important` blocks moved out of the layers so the harness's injected settle/shot styles still win | P7's run 45291381011; the computed-style oracle (0 differing cells over 216 groups / 130,616 cells, Amendment 17(e)); the final tree's 192 audits / 0 allowed |
| (not adopted) the P6 sheet entry and container queries | see the left-out list in Q-0019-05, with each candidate's evidence | P8's gate run 45312448788 green at both audited sizes; the boundary probe's numbers in Amendment 18 |

Left out, with the evidence each rests on (the full list is Q-0019-05): the coach pop as a popover
(reverted — the manual popover's box-shadow painted over the orb; Amendment 15); native light dismiss as
the only outside-click path (native dismissal cannot clear `pendingRange` and no gate can drive the
engine path); the backdrop as a transition / `transition-behavior: allow-discrete` (the engines run the
backdrop animation but do not name the `::backdrop` pseudo in `getAnimations()`; Amendment 16(e));
`@starting-style` for the sheet's slide (the restored keyframe is the base's own entry and the measured
frame step went back to the base's level; Amendment 19); container queries (the compact mode's gutter
change makes the stage's content box non-monotone at the 1340 boundary: 1292.00 px at 1340 vs 1261.33 px
at 1341, and 1330/1362 both present 1282.00 px; with the coach column the stage is 952 px at 1440 —
Amendment 18); anchor positioning and `<dialog>`-free focus trapping (no Linux proof on WebKitGTK
2.52.6); the dock tooltip (no gain); `@scope`, scroll-driven animations, view transitions and
`content-visibility` (no hand-built part they would replace, or a behaviour change the goal forbids).

## Done when 4 — performance, before and after

### The steps and their runs

One row per step; A = the previous step's merge, B = the step's own head, one job per OS. "Verdicts"
counts every metric of the 13 the tool records (`worse=true` would be a failure). Bundle bytes are the
B side of that run's record (`trees.b.bundles`), the basis of the per-step table further down.

| Step | Run | A → B | Verdicts | Shots A/B + control |
|---|---|---|---|---|
| P1 (method) | 45045599642 | `63ce6e4` → `aa1580a` | 13/13 `worse=false` (predates Amendment 13's control-pair floor) | A/B `not-measured` (A predates the hook); control `6/6` |
| P2 (baseline) | 45090760750 | `71c366b` → `019884d` | 13/13 ok on both jobs | `6/6` + `6/6` |
| P3 (workarounds) | 45099610142 | `98752a7` → `8d8fe7c` | 13/13 ok | `6/6` + `6/6` |
| P4 (dialogs) | 45123200561 | `89a9494` → `62aace2` | 13/13 ok | `6/6` + `6/6` |
| P5 (popovers) | 45139959664 | `74a8283` → `5c777f1` | 13/13 ok | `6/6` + `6/6` |
| P6 (entry) | 45174298421 | `a0e2b3e` → `47470d3` | 13/13 ok (the tick the dispatch runs later read is in the Windows section) | `6/6` + `6/6` |
| P7 (layers) | 45291381011 | `5aa4155` → `897e588` | 13/13 ok | `6/6` + `6/6` |
| P8 (container queries, left out) | no bench run (`docs/**` is outside the workflow's `paths` filter); gate run 45312448788 | `7056351` → `78290cf` | desktop group green on three OSes | — |
| P6 fix | 45336617620 | `78290cf` → `ecd11f0` | 13/13 ok | `6/6` + `6/6` |
| final | 45338748380 | `71c366b` → `bb94a9d` | attempt 2: 13/13 ok on both jobs; attempt 1: 12/13 on macOS, 13/13 on Windows | `6/6` + `6/6` in both attempts |

### Windows tier (WebView2, Chromium 157)

The final run, attempt 2 (`45338748380/frontend-bench-windows-latest`, `metrics[]`; medians and floors in
ms, the tool's log rounding):

| Metric | A | B | Floor (source) | Verdict |
|---|---|---|---|---|
| readyMs | 1593.7 | 1614.9 | 79.7 (5%) | ok |
| answerSelectMs | 1.1 | 1.0 | 5 (5ms) | ok |
| submitFeedbackMs | 75.1 | 70.5 | 7.9 (control IQR) | ok |
| coachReplyMs | 189.2 | 200.2 | 21.4 (control IQR) | ok |
| roomSwitch.report | 43.7 | 44.4 | 20.1 (control IQR) | ok |
| roomSwitch.anki | 33.1 | 35.8 | 17.4 (control IQR) | ok |
| roomSwitch.settings | 34.1 | 34.7 | 5.1 (control IQR) | ok |
| roomSwitch.assess | 54.9 | 47.4 | 29.3 (control IQR) | ok |
| frames.median | 15.6 | 15.6 | 5 (5ms) | ok |
| frames.p95 | 15.7 | 15.7 | 5 (5ms) | ok |
| frames.longest | 46.9 | 46.9 | 5 (5ms) | ok |
| bundle.mainJs | 186489 | 186908 | 9324 (5%) | ok |
| bundle.mainCss | 79071 | 79477 | 3954 (5%) | ok |

The frame tier's resolution, and the P6 step it could and could not see. Windows frame durations are
quantized to the 15.6 ms timer tick: `frames.median` reads 15.6 in every run of the goal, and
`frames.p95`/`frames.longest` read a small multiple of the tick. The runner's baseline level is 1 tick
(~15.7 ms) or 2 ticks (~31.2 ms) and can differ between a run's A/B phase and its control phase. The
dispatch runs in the P6 window, `frames.p95` (median) and `frames.longest`:

| Run | Pair | p95 A → B | longest A → B | longest control A/B | longest floor | Verdict |
|---|---|---|---|---|---|---|
| 45316313813 | P1 → P8 | 15.7 → 31.2 **worse** | 31.3 → 46.9 **worse** | 46.8 / 46.8 | 15.6 | saw the step |
| 45320505404 | P5 → P6 | 15.7 → 31.2 **worse** | 46.9 → 46.9 | 46.9 / 46.9 | 5 | saw the step; run unreadable (control readyMs/p95 worse) |
| 45316326864 | P1 → P5 | 15.7 → 15.7 | 31.3 → 31.3 | 31.3 / 31.3 | 15.6 | no P6 in the window; P4+P5 paid no tick |
| 45315096427 | P1 → P8 | 31.3 → 46.8 **worse** | 62.6 → 47.0 | 62.5 / 62.4 | 15.6 | a 2-tick base still showed a one-tick step (as `+1 tick` on p95) |
| 45318388579 | P1 → P6 | 31.22 → 31.3 | 46.9 → 62.4 | 62.5 / 62.6 | 15.6 | base at 2 ticks: cannot separate a one-tick step |
| 45335594677 | P5 → fix `6220f46` | 31.22 → 31.22 | 62.5 → 62.5 | 62.5 / 62.5 | 15.6 | base at 2 ticks: the fix is invisible here |
| 45336617620 | P8 → fix `ecd11f0` | 31.3 → 31.2 | 62.5 → 62.5 | 62.5 / 62.5 | 15.6 | base at 2 ticks: the fix is invisible here |
| 45338748380 attempt 1 | P1 → final | 15.7 → 15.7 | 31.3 → 31.3 | 31.3 / 46.8 | 15.5 | base at 1 tick: the fix holds |
| 45338748380 attempt 2 | P1 → final | 15.7 → 15.7 | 46.9 → 46.9 | 46.9 / 46.9 | 5 | base at 1 tick: the fix holds |

**Disclosed limit of this tier:** a one-tick difference is only separable when at least one side moves
to a different tick level than the other. The runs that could see the P6 step read it as WORSE
(45316313813, 45320505404 — the latter's control pair also read worse, so it proves the step's size but
not a readable comparison; 45315096427 read the same +1 tick on a 2-tick base). The runs whose A and B
both sat at the 2-tick level could not resolve it (45318388579 31.22 → 31.3, 45335594677
31.22 → 31.22, 45336617620 31.3 → 31.2, 45320531877 35.95 → 35.95 and 45330890948 31.22 → 31.3). The
final run's A sat at the 1-tick level in both attempts and B read 15.7 in both: the
restored `sheetin` keyframe holds the base's level. No cause beyond the measured one tick is claimed
(Amendment 19(b)); the `will-change` alternative was rejected by its own shot difference (run
45330890948).

### macOS tier (WKWebView 26.x, runner macOS 26.7.2)

The final run, attempt 2 (`45338748380/frontend-bench-macos-26`):

| Metric | A | B | Floor (source) | Verdict |
|---|---|---|---|---|
| readyMs | 3655 | 3641 | 1286 (control IQR) | ok |
| answerSelectMs | 2 | 2 | 5 (5ms) | ok |
| submitFeedbackMs | 126 | 112 | 35 (control IQR) | ok |
| coachReplyMs | 224 | 209 | 63 (control IQR) | ok |
| roomSwitch.report | 89 | 61 | 25 (control IQR) | ok |
| roomSwitch.anki | 45 | 48 | 21 (control IQR) | ok |
| roomSwitch.settings | 27 | 27 | 7 (control IQR) | ok |
| roomSwitch.assess | 137 | 133 | 49 (control IQR) | ok |
| frames.median | 17 | 17 | 5 (5ms) | ok |
| frames.p95 | 76.75 | 78 | 48.65 (control IQR) | ok |
| frames.longest | 146 | 154 | 71 (control IQR) | ok |
| bundle.mainJs | 186489 | 186908 | 9324 (5%) | ok |
| bundle.mainCss | 79071 | 79477 | 3954 (5%) | ok |

Attempt 1 of the same run (`45338748380-attempt1.log`; its uploaded artifacts were replaced by the
rerun and the log is the record):

| Reading | Value |
|---|---|
| macOS `frames.longest` | A n=9 q1=74 med=81 q3=88 | B n=9 q1=106 med=107 q3=123 | control 95 / 86 | floor 23 (control IQR) | **WORSE** — `q1 106 vs base q3 88 (above); median 81 -> 107, gap 26 (beyond)` |
| macOS `frames.p95` | 45.2 → 39.8, floor 12.2 — ok |
| macOS readyMs | 2907 → 2855, floor 1212 — ok |
| Windows job | all 13 verdicts ok: `frames.p95 15.7 -> 15.7`, `frames.longest 31.3 -> 31.3` (floor 15.5) |
| shots, both OSes | `6/6 state(s) byte-identical between A and B`; control `6/6` |

**Disclosed macOS observation (accepted by the owner on 2026-11-24; Q-0019-02).** Across the
goal-wide/bisect dispatch runs, macOS `frames.longest` reads the B median above the A median in 8 of
the 12 rows below, one of them beyond its floor (attempt 1 above). No cause is claimed: the macOS jobs
are shared runners, this metric's same-build control pairs read medians of 86–201 ms and set floors of
23–121 ms (the `control A/B` and `floor` columns), and the owner said the macOS tests may be somewhat
more lenient (2026-11-22; Q-0019-02) — that remark did not by itself accept the attempt-1 WORSE. The
owner accepted this reading on 2026-11-24 without further investigation (Q-0019-02), which closes
Done-when 4's macOS `frames.longest` item; whether it is the P6 cost showing through macOS noise,
runner state, or a real macOS regression stays unknown.
Per-run rows
(`frontend-bench-macos-26/frontend-bench-record.json`; ms):

| Run | Pair | longest A → B | control A/B | floor (source) | Verdict |
|---|---|---|---|---|---|
| 45315096427 | P1 → P8 | 157 → 169 | 177 / 186 | 84 (control IQR) | ok |
| 45316313813 | P1 → P8 | 59 → 90 | 86 / 89 | 36 (control IQR) | ok |
| 45316326864 | P1 → P5 | 126 → 162 | 201 / 119 | 121 (control IQR) | ok |
| 45318388579 | P1 → P6 | 108 → 139 | 201 / 200 | 66 (control IQR) | ok |
| 45318421329 | P6 → P7 | 210 → 217 | 129 / 135 | 61 (control IQR) | ok |
| 45320531877 | P6 → P7 | 89 → 88 | 120 / 96 | 50 (control IQR) | ok |
| 45320505404 | P5 → P6 | 239 → 265 | 121 / 136 | 95 (control IQR) | ok |
| 45330890948 | P5 → `will-change` | 159 → 133 | 111 / 104 | 24 (control IQR) | ok |
| 45335594677 | P5 → fix | 186 → 174 | 194 / 194 | 73 (control IQR) | ok |
| 45336617620 | P8 → fix | 122 → 110 | 138 / 158 | 56 (control IQR) | ok |
| 45338748380 attempt 1 | P1 → final | 81 → 107 | 95 / 86 | 23 (control IQR) | **WORSE** |
| 45338748380 attempt 2 | P1 → final | 146 → 154 | 200 / 175 | 71 (control IQR) | ok |

macOS `frames.p95` moves both ways in the same rows (worst: 45318421329 86.05 → 107, median gap
20.95 beyond its 14.1 ms floor and read ok only because the `q1 > base q3` leg is not met —
`q1 77.1 vs base q3 107.25 (not above)`; 45330890948 78 → 65; the final attempt 2 76.75 → 78 within
48.65).

### Linux tier (WebKitGTK, second-tier)

One job per bench run, `--smoke`: one visible window per state, six states, no numbers compared (Linux
is second-tier; its numbers would come from another engine). Every run's job is green; the final run's
record (`45338748380/frontend-bench-ubuntu-smoke/frontend-bench-smoke.json`): `ok: true`, 6 states, `7`
launches, `guards checked 6, failed 0, not measured 1`, empty `failures`, UA
`Mozilla/5.0 (X11; Ubuntu; Linux x86_64) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/60.5
Safari/605.1.15`. The gate matrix's `ubuntu-latest` desktop job is the second half of the Linux
evidence (counts table below).

### Bundle bytes per step

`trees.<side>.bundles` of the named run (`internal/desktop/web/dist/main.js` / `main.css`):

| Step | Merge | Read from | main.js | main.css | Δjs | Δcss |
|---|---|---|---|---|---|---|
| P1 | `71c366b` | 45045599642 (both sides) | 186489 | 79071 | — | — |
| P2 | `98752a7` | 45099610142 A | 186489 | 79071 | 0 | 0 |
| P3 | `89a9494` | 45123200561 A | 186489 | 78111 | 0 | −960 |
| P4 | `74a8283` | 45139959664 A | 186738 | 78513 | +249 | +402 |
| P5 | `a0e2b3e` | 45174298421 A | 186908 | 78662 | +170 | +149 |
| P6 | `5aa4155` | 45291381011 A | 186905 | 78654 | −3 | −8 |
| P7 | `7056351` | 45291381011 B | 186905 | 79486 | 0 | +832 |
| P8 | `78290cf` | 45336617620 A (P8's own PR ran no bench) | 186905 | 79486 | 0 | 0 |
| fix | `bb94a9d` | 45338748380 B (45336617620 B for `ecd11f0`) | 186908 | 79477 | +3 | −9 |
| **total** | `71c366b` → `bb94a9d` | 45338748380 | **186489 → 186908** | **79071 → 79477** | **+419 (0.22%)** | **+406 (0.51%)** |

The goal deliberately pays for the dialog machinery (P4) and the cascade layers (P7); every bundle
verdict stays within its 5%-of-base floor (final run above), and P3 returns 960 css bytes. The P6 cost
(a one-tick frame step) was **not** accepted: the fix restores the base's own entry (Amendment 19).

## Done when 5 — nothing visible changes unintentionally

| Reading | Run / artifact | Value |
|---|---|---|
| every step's shot pair | each run's `shots.rows` | `6/6 state(s) byte-identical between A and B`, with the same-build control `6/6` in every run that has the pair (P2…P7, the fix, both final attempts); the P1 run's A predates the hook (`shots not-measured (base predates the capture hook)`) and its own control pair reads `6/6` |
| the allowlist | `harness/frontend-visual-allow.json` | `entries: []` — no state was allowed to differ (the intended P4/P5 stacking differences are outside the bench's six states and are recorded in the plan's amendments 14–15) |
| the companion has teeth | run 45330890948 | `state sheet-open differs from the base and no allowlist entry names it` — the `will-change` candidate was rejected on it (its Windows pair read `5/6`, macOS `6/6`) |
| the audits | gate run 45336617606 | `desktop ux (192 audits over 24 states × 4 modes × 2 sizes, 19 probes, 84 focus checks, 0 allowed …)` on ubuntu, macOS and Windows (Windows: `backdrop rows required`; macOS/ubuntu: `backdrop rows not reported by this engine`) |
| the smoke | gate run 45336617606 | `desktop smoke (493 assertions over 13 scenarios)` on Windows; `492` on macOS and Linux |
| the package | gate run 45336617606 | `package smoke (539 assertions over contents, layout, study, agents, brief, anki, upgrade, credential, broken …)` on Windows; `538` on macOS and Linux |
| the design and the text | this goal's diff | no file under `design/` changed; the port's DOM, class names and text are the prototype's (README); the ux gate's `zh-ascii-words`/`en-cjk-text`/`raw-code` audits are green in the same run |
| the keyboard behaviour | gate run 45336617606 | the ux focus checks (84) and the gates' `dialog-focus-*`, `focus-lost`, `card-end-under-sheet`, `chosen-option-hidden` checks are green; the P4–P6 probes measured the dismissal, focus entry and Escape returns per Amendment 14/15 |

## Constraints

- No framework or new runtime dependency: `package.json`, `bun.lock` and `mygo.json` unchanged across
  this goal's merges (`git diff --name-only 63ce6e4..bb94a9d -- bun.lock package.json mygo.json` prints
  nothing); no new module is imported by the frontend (the diff adds no dependency).
- No change to the Go core, the bound API, scoring, consent or data: the only Go change in the goal is
  the P1 shot hook (`internal/desktop/smoke.go` `bindShots`; `desktop.go`'s one line; `desktop_test.go`'s
  two tests for it, `TestShotService` and `TestShotDirComesOnlyFromAScriptedLaunch`) which binds
  `SmokeShot` only while `PASSQUEST_DESKTOP_SHOT_DIR` (and the script env) are set — a normal launch
  binds nothing (the boot smoke proves the unbound call rejects; run 45336617606's self-test and smoke
  lines).
- Linux keeps working: the ubuntu job of every bench run and of the gate matrix is green.

## Known limits of this record

- **Hidden windows in the gates.** The gates and the smoke launch hidden windows on purpose (timers
  throttled off; WebKit also suspends animations in a hidden window), so no rendering number comes from
  them; all frame numbers come from the bench's visible launches, and every measured launch records the
  visible window it requested (`hidden: false`; a measured launch that requested the hidden one fails
  the record-shape check).
- **macOS 27 is absent** and is not run: the runner is `macos-26` (26.7.2) and the baseline is the OS's
  system WebKit, Safari 26.x (Q-0019-01).
- **Ubuntu's WebKitGTK version moves with the archive**: the record names the UA it measured
  (`Version/60.5 Safari/605.1.15`, run 45338748380) and the plan pins `2.52.6` at scout time; a future
  archive update changes what the second-tier run exercises.
- **The frame tier's resolution** is the 15.6 ms tick, and the Windows rows above state which runs
  could and could not separate the P6 step; a difference smaller than the run's own level cannot be
  read there.
- **The floor rule's cost**: where a metric has control-pair samples, a regression whose median gap is
  below the larger control side's IQR cannot read WORSE (Q-0019-02's disclosed limit) — the macOS
  `frames.longest` floors of 23–121 ms are the sharpest instance of it.
- **macOS `frames.longest`**: the observation above; B above A in 8 of 12 dispatch rows, one
  beyond its floor, no cause claimed (accepted by the owner on 2026-11-24; Q-0019-02).
- **The final run has two attempts**: attempt 1 failed on macOS `frames.longest` and its artifacts were
  replaced by the rerun; attempt 1's log is the record for it (quoted above). Attempt 2 is what the
  uploaded artifacts carry.
- **LoAF** totals are Chromium-only enrichment; the macOS and Linux launches record `loaf: null`
  (Amendment 5).
- **The bench runs on Windows and macOS only** (the goal's scope); Linux numbers would come from
  another engine and are not compared.

## Local checks of this round (2026-11-24, Windows 11)

The P9 round is docs and decisions only; it changes no code, so the gate set below is the proof it
names.

| Check | Result |
|---|---|
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts docs-links.ts docsLinks` | PASS — `STATUS pass 347 checked, 0 broken` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts secrets.ts secrets` | PASS — `STATUS pass 787 files, 0 findings` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts install-guide.ts installGuide` | PASS — `STATUS pass 7 sections, 4 code blocks and 7 screenshot places agree; 6 links and 5 URLs agree; 6 quoted app strings match their sources; the release notes name the tap command and both guides; the cask template renders v0.15.0 with the release's SHA-256, and a synthetic pair, into Casks/pass-quest.rb; floor 26.0 from mygo.json` |
| `mise exec -- bun Q:/scratch/pq-harness-run/run.ts frontend-baseline.ts frontendBaseline` | PASS — `STATUS pass ES2024 target/lib pinned, README carries both, all 4 allowlist entries fire on their own side; 6 vendor token(s) in the source, 6 in the bundle` (the README's engine table is what this gate pins) |

The final numbers of P9's edited documents are cross-checked against the artifacts by the coordinator
(the runs and record paths above); the goal file's archive move is theirs.
