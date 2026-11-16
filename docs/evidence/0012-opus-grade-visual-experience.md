# 0012 — A distinctive, restrained and memorable study experience: evidence

Contract: [`../goals/archive/0012-opus-grade-visual-experience.md`](../goals/archive/0012-opus-grade-visual-experience.md).
Design: [`../../design/prototypes/c-stage.html`](../../design/prototypes/c-stage.html) (owner-chosen, Q-0012-00).
Domain: [`../desktop.md`](../desktop.md), [`../notes.md`](../notes.md).

## Environment

Windows 11 Enterprise (10.0.26300), WebView2 runtime 157.0.4341.60, Go 1.27.4, Bun 1.4.4, Node 26.13.1,
MyGo v0.9.0. Linux procps parsing was additionally checked against real `ps` output in WSL Ubuntu 24.04.
macOS was not run locally; CI is its only evidence.

## Gate runs

| Command | Result |
|---|---|
| `bun run gate` (final tree, Windows, landscape display) | 22 passed, 2 failed, 0 skipped. Failed: `agent smoke`, `provider smoke` — see below |
| `go test` gate | 22 packages, 862 tests, 0 skipped (831 at goal 0011) |
| `desktop smoke` | 349 assertions over 12 scenarios (Windows); the 11-scenario version (328 assertions) also passed in WSL Ubuntu 24.04 under `xvfb-run` (327 there: the Windows-only Credential Manager check does not run) |
| `desktop ux` | 128 audits over 16 states × 4 modes × 2 sizes, 17 probes, 58 focus checks, 0 allowed (Windows); the 13-state version also passed in WSL Ubuntu 24.04 under `xvfb-run` |
| `harness self-test` | 46 cases (2 must pass) over 23/23 gates |
| `note smoke` / `assess smoke` | 1367 / 275 assertions |
| `golden contract` | no golden changed against `cf66996` |
| CI (ubuntu, macOS, Windows) | CI_RESULT |

**Why gates fail locally.** As in goal 0011 (Q-0011-16): the npm feed this machine must use
(`packagefeedproxy.microsoft.io`) still lacks the locked Pi Durable packages (`@earendil-works/*@1.5.1`,
checked 2026-11-16), so `agent smoke` and `provider smoke` cannot start the durable host. The lock file was
not changed; CI installs from the public registry and is their evidence.

## Acceptance → evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | Primary actions visible without scrolling | `desktop ux`: 16 declared states (Today, practice sheet, shortcut sheet, independent practice, hint gate, feedback with a coach reply, Anki draft editor, the editor with the refine panel open, guided practice, the notes list, report, report annotations, Settings row, run form, running assessment, practice on an imported question whose stem and four options run to several sentences each) × light/dark × zh/en at 1280×800 and 1440×844; each state's one visible `[data-primary]` lies inside the viewport and its scroll containers, and so does every `[data-must-see]` element (the consent and budget rows of a provider call strip); text and controls out of view are reachable by a scroll the learner can make (`unreachable`); a missing or duplicated state is a harness failure |
| 2 | No overflow, clipping or covered controls | `desktop ux`: page and text-container overflow, out-of-viewport elements, `elementFromPoint` coverage of every control, in every audit above |
| 3 | AA contrast and ≥ 12 px text | `desktop ux`: composited contrast per text element over the translucent layers, including text below the fold and form-control values/placeholders; font-size floor including generated text. The goal-0011 window had 124 distinct findings; the final allowlist `harness/ux-allow.json` is empty |
| 4 | Keyboard and focus | `desktop ux`: every `[data-act]` focusable with a visible change under `.qa-fv`; modal trap; dialogs take and return focus; focus never on `<body>` after the scripted navigations. `desktop smoke` refine/coach flows keep focus inside the panel after open, send, retry and close |
| 5 | Nothing internal leaks | `desktop ux`: zh screens contain no raw code, id, hash, CLI text or ASCII word outside exam content and the marked exceptions (`[lang=en]`, `kbd`, `code`, `.mono`, `[data-raw]`, `[translate=no]`); allowlist empty. Review fixes removed a source id from the consent dialog title, a CLI command from the refine panel, the attempt's database id from the draft line and an English core instruction from the local-services sheet |
| 6 | Motion is purposeful and optional | `desktop ux`: with no preference, leaving an independent item for the assessment room runs the room-colour transition; with reduced motion emulated, no animation runs > 1 ms, none loops, and no scripted change starts any transition (`transition-under-reduce`; the prototype's 1 ms rule fails it with 676 findings) |
| 7 | The card is one source | `note smoke` (1367 assertions): v2 model CSS/template hashes pinned, `createModel` sends exactly `note model`, an upgrade from v1 sends styling + templates once, a customised model is reported (`model_customized`) and not overwritten, a half-updated deck is matched part by part (Q-0012-02), no remote URL or script. `desktop smoke`: every card side's `srcdoc` equals the Go-rendered `CardDocument` byte for byte (question/answer × light/night), sized with nothing clipped at desktop and phone width, `css_sha256` equals `note model --json`. Go tests: night-mode selectors and contrast ≥ 4.5 for every text token |
| 8 | Refinement and history work in the window | `desktop smoke` against the loopback provider stub and the real Node tutor host: the refine preview's sha256 and size equal the raw body received; consent and budget shown before sending; a non-JSON reply is a typed `invalid_response` whose Retry only reloads the preview; the suggestion renders as text and fills the fields only when used; an edited suggestion is recorded `edited`; with a hand-configured endpoint (no registry provider id) the coach and the refine panel show the preview and a Send naming the endpoint's host (`manual` scenario). `TestWorkspaceRefinementStaysAdvisory`: after a relaunch the draft keeps its own fields and returns the suggestion beside them, and an emptied field falls back to the draft's own value. History lists exactly the attempt's recorded calls in order |
| 9 | The gate catches what it claims | `harness self-test`: a stylesheet that removes focus outlines and drops the hero title to background colour fails `desktop ux` with `no-focus-indicator` and `contrast-fail`; a page area 16 px short fails the viewport precondition. Every `desktop ux` run first plants 17 defects into Today and fails unless all 17 are reported. Unit checks: the process-tree matcher ignores stale-ppid and reused-pid processes and rejects an unreadable `ps` table, the coverage check rejects a missing state, no page script loses an escape |
| 10 | Nothing regresses | all earlier gates pass with their documented counts (or fail for the environment reasons above); `golden contract`: no golden changed against `cf66996`; `go test`: 862 tests (831 at goal 0011) |

## Rendered screens

Contact sheets of 17 states (light/dark × zh/en at a 1280×800 page area) are in [`0012/`](0012/README.md),
shot from the e2e build of the final tree with seeded local data and loopback stubs. They show the state the
gates checked; they are not a substitute for them. Shooting them found two defects the gate did not yet audit —
a stray scrollbar in the notes list and a refine panel whose Send, and after a first fix whose consent and budget
rows, were out of view — which led to the `anki-notes`/`anki-refine` states, `scroll-without-content` and
`must-see-hidden`.

## Review

A multi-lens review of the goal diff (learner contract, evidence semantics, harness integrity, UX leaks,
security) produced 24 findings; independent skeptics refuted 3 and confirmed 21 (9 P2, 12 P3). All 21 were
fixed with a regression check that failed before and passed after (or, for two coverage gaps, a mutation
check). A second, read-only review of the fixes found two medium issues it had introduced or left open — a
suggestion lost when leaving the Anki route mid-request, and a POSIX `ps` table that could parse as empty and
silently pass the orphan check — plus two low ones (DST fall-back start times, an unordered preview reply);
all four were fixed and adversarially re-verified, and a one-second start-time slack on Linux found during that
verification was applied.

The first CI run found what a Windows machine cannot: on Linux a hidden GTK window is never allocated, so the
page laid out at 0×0 (fixed by allocating the unmapped window to its default size, `internal/desktop/hidden_linux.go`);
hidden WebKit pages throttle timers and suspend CSS animations, which left card frames at their default height on
macOS and stalled the motion stage (frames are now fitted by timers after insertion, the motion check counts only
transitions the condition change starts, and hidden windows run with background throttling off); Linux's wider
system font pushed the coach header's close button out of the sheet (the advisory chip now wraps); and the Windows
runner's 1024×768 display clamped the window (CI now sets 1920×1080). A local portrait 125 % display fails the
viewport precondition the same way, by design. One behaviour beyond the window changed: `assess report` no longer prints the
evidence or reasoning of an unavailable item (CLI and window alike; no golden covers that case).

## Known limits

- Real Anki desktop, AnkiDroid and AnkiMobile rendering and the sync behaviour of a template update are owner
  evidence; no Anki installation exists on the development machine.
- Contrast on translucent layers is computed, not measured by eye; reachability is approximated by
  focusability because a hidden window cannot receive a real Tab key.
- macOS and Linux are covered as far as CI runs the audit.
- The gate's stub model is verified, so its refine panel is shorter than one for an unverified model; the
  unverified layout was checked with the same audit library on the screenshot build, not by the gate.
- While the refine panel is open it covers the lower part of the card preview.
- A refinement whose request is still running when the learner closes the refine panel is kept in the store
  and shown again after a relaunch, not immediately.
