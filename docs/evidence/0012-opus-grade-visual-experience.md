# 0012 — A distinctive, restrained and memorable study experience: evidence

Contract: [`../goals/archive/0012-opus-grade-visual-experience.md`](../goals/archive/0012-opus-grade-visual-experience.md).
Design: [`../../design/prototypes/c-stage.html`](../../design/prototypes/c-stage.html) (owner-chosen, Q-0012-00).
Domain: [`../desktop.md`](../desktop.md), [`../notes.md`](../notes.md).

## Environment

Windows 11 Enterprise (10.0.26300), WebView2 runtime 157.0.4341.60, Go 1.27.4, Bun 1.4.4, Node 26.13.1,
MyGo v0.9.0. Linux procps parsing was additionally checked against real `ps` output in WSL Ubuntu 24.04.
macOS was not run locally; CI is its only evidence.

## Gate runs

GATE_RUNS

**Why gates fail locally.** As in goal 0011 (Q-0011-16): the npm feed this machine must use
(`packagefeedproxy.microsoft.io`) still lacks the locked Pi Durable packages (`@earendil-works/*@1.5.1`,
checked 2026-11-16), so `agent smoke` and `provider smoke` cannot start the durable host. The lock file was
not changed; CI installs from the public registry and is their evidence.

## Acceptance → evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | Primary actions visible without scrolling | `desktop ux`: 13 declared states (Today, practice sheet, shortcut sheet, independent practice, hint gate, feedback with a coach reply, Anki draft editor, guided practice, report, report annotations, Settings row, run form, running assessment) × light/dark × zh/en at 1280×800 and 1440×844; each state's one visible `[data-primary]` lies inside the viewport and its scroll containers; a missing or duplicated state is a harness failure |
| 2 | No overflow, clipping or covered controls | `desktop ux`: page and text-container overflow, out-of-viewport elements, `elementFromPoint` coverage of every control, in every audit above |
| 3 | AA contrast and ≥ 12 px text | `desktop ux`: composited contrast per text element over the translucent layers, including text below the fold and form-control values/placeholders; font-size floor including generated text. The goal-0011 window had 124 distinct findings; the final allowlist `harness/ux-allow.json` is empty |
| 4 | Keyboard and focus | `desktop ux`: every `[data-act]` focusable with a visible change under `.qa-fv`; modal trap; dialogs take and return focus; focus never on `<body>` after the scripted navigations. `desktop smoke` refine/coach flows keep focus inside the panel after open, send, retry and close |
| 5 | Nothing internal leaks | `desktop ux`: zh screens contain no raw code, id, hash, CLI text or ASCII word outside exam content and the marked exceptions (`[lang=en]`, `kbd`, `code`, `.mono`, `[data-raw]`, `[translate=no]`); allowlist empty. Review fixes removed a source id from the consent dialog title, a CLI command from the refine panel, the attempt's database id from the draft line and an English core instruction from the local-services sheet |
| 6 | Motion is purposeful and optional | `desktop ux`: with no preference, leaving an independent item for the assessment room runs the room-colour transition; with reduced motion emulated, no animation runs > 1 ms and none loops |
| 7 | The card is one source | `note smoke` (1367 assertions): v2 model CSS/template hashes pinned, `createModel` sends exactly `note model`, an upgrade from v1 sends styling + templates once, a customised model is reported (`model_customized`) and not overwritten, a half-updated deck is matched part by part (Q-0012-02), no remote URL or script. `desktop smoke`: every card side's `srcdoc` equals the Go-rendered `CardDocument` byte for byte (question/answer × light/night), sized with nothing clipped at desktop and phone width, `css_sha256` equals `note model --json`. Go tests: night-mode selectors and contrast ≥ 4.5 for every text token |
| 8 | Refinement and history work in the window | `desktop smoke` against the loopback provider stub and the real Node tutor host: the refine preview's sha256 and size equal the raw body received; consent and budget shown before sending; a non-JSON reply is a typed `invalid_response` whose Retry only reloads the preview; the suggestion renders as text and fills the fields only when used; an edited suggestion is recorded `edited`. `TestWorkspaceRefinementStaysAdvisory`: after a relaunch the draft keeps its own fields and returns the suggestion beside them, and an emptied field falls back to the draft's own value. History lists exactly the attempt's recorded calls in order |
| 9 | The gate catches what it claims | `harness self-test`: a stylesheet that removes focus outlines and drops the hero title to background colour fails `desktop ux` with `no-focus-indicator` and `contrast-fail`; a page area 16 px short fails the viewport precondition. Every `desktop ux` run first plants 13 defects into Today and fails unless all 13 are reported. Unit checks: the process-tree matcher ignores stale-ppid and reused-pid processes and rejects an unreadable `ps` table, the coverage check rejects a missing state, no page script loses an escape |
| 10 | Nothing regresses | all earlier gates pass with their documented counts (or fail for the environment reasons above); `golden contract`: no golden changed against `cf66996`; `go test`: 862 tests (831 at goal 0011) |

## Rendered screens

SCREENS

## Review

A multi-lens review of the goal diff (learner contract, evidence semantics, harness integrity, UX leaks,
security) produced 24 findings; independent skeptics refuted 3 and confirmed 21 (9 P2, 12 P3). All 21 were
fixed with a regression check that failed before and passed after (or, for two coverage gaps, a mutation
check). A second, read-only review of the fixes found two medium issues it had introduced or left open — a
suggestion lost when leaving the Anki route mid-request, and a POSIX `ps` table that could parse as empty and
silently pass the orphan check — plus two low ones (DST fall-back start times, an unordered preview reply);
all four were fixed and adversarially re-verified, and a one-second start-time slack on Linux found during that
verification was applied. One behaviour beyond the window changed: `assess report` no longer prints the
evidence or reasoning of an unavailable item (CLI and window alike; no golden covers that case).

## Known limits

- Real Anki desktop, AnkiDroid and AnkiMobile rendering and the sync behaviour of a template update are owner
  evidence; no Anki installation exists on the development machine.
- Contrast on translucent layers is computed, not measured by eye; reachability is approximated by
  focusability because a hidden window cannot receive a real Tab key.
- macOS and Linux are covered as far as CI runs the audit.
- A refinement whose request is still running when the learner closes the refine panel is kept in the store
  and shown again after a relaunch, not immediately.
