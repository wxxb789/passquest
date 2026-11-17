# 0012 — A distinctive, restrained and memorable study experience

**State:** Active
**Prerequisite:** 0011 — usable desktop study workspace

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Reserve the high-fidelity interface, interaction design and Anki card look-and-feel for a later Opus-led pass. Distinguish meaningful visual memorability from decorative image spam.

## Refined contract (activated 2026-11-15)

Goal 0011 delivered a functional MyGo workspace that already carries the owner-chosen **"C · Stage"**
tokens and layout ([`design/prototypes/c-stage.html`](../../../design/prototypes/c-stage.html)). A
route-by-route comparison with the prototype, an Anki model audit and an in-window quality audit (80 audits
over 2 window sizes × 10 states × 2 themes × 2 languages) found what still separates it from a finished,
memorable study experience: a primary action pushed out of view in guided practice, internal identifiers, hashes
and CLI commands shown to the learner, contrast and font-size failures, focus lost after navigation, an Anki
preview that is not the C-Stage card, and two workflows (AI refinement of a note, coach history) whose domain API
exists but has no window. This increment refines **one design language**; it changes no scoring, consent,
budget, assessment or sync rule.

**In scope**

1. **Every high-value flow without friction.** Today → practice → feedback → coach → note → push, and
   assessment → report, reach their primary action without scrolling at 1280×800 and 1440×844; whether an
   attempt counts is stated at every point it changes (picker, practice, next item); the report shows the
   like-for-like comparison by default and the annotation sheet shows stem, your choice and the verified key.
2. **Nothing internal leaks.** No content id, hash, sha256, raw error, CLI command or untranslated code is the
   primary text of any learner-facing element; codes appear only as muted secondary detail where an action needs
   them. Budgets use one denominator per provider everywhere.
3. **Type, contrast, density and focus.** WCAG AA text contrast in light and dark on the real translucent layers;
   no text below 12 px; a visible focus indicator on every control; focus never falls to `<body>` after an action
   or navigation; dialogs trap and restore focus; motion respects reduced-motion and the rooms (independent cool,
   assisted warm, assessment still) transition when motion is allowed.
4. **The Anki card is the C-Stage card.** The note model (same name, fields and template name) gets a C-Stage
   template and CSS: system fonts only, no remote asset or script, light and Anki night mode, readable at phone
   width. Existing decks are upgraded in place on the next push when their model is a known earlier version;
   a model the learner customised in Anki is never overwritten. The desktop preview renders the exact same
   template and CSS as the deck, front and back, light and night, desktop and phone width.
5. **Workflows with an existing API get a window.** AI refinement of a note draft (`note propose --refine`
   semantics: advisory, external, consent and budget shown before sending) and the coach history for an attempt.
6. **Meaningful visuals only.** The evidence constellation, satellites and depth layers stay because they encode
   evidence, condition and hierarchy; nothing decorative becomes a prerequisite for any action.

**Out of scope**

Packaging, installers and first-run onboarding (goal 0013); new practice content; a free-text coach composer
(no domain API — Q-0012-03); "keep Anki's version" of a conflict (Q-0011-07); per-purpose model routing.

**Defaults taken (reversible, disclosed)**

- Quality is measured by a new `desktop-ux` gate against an explicit allowlist that may only shrink; it must be
  empty at close except for entries whose reason is exam content or a learner-entered value (Q-0012-01).
- Note model upgrade: in place under the same model name via `updateModelStyling`/`updateModelTemplates`, only
  when each of the stylesheet, front and back is a part Pass Quest itself shipped (line endings and surrounding
  whitespace aside), so no learner text can be overwritten (Q-0012-02).
- The tactic name stays on the back of the card, so the front does not hint the answer (Q-0012-04).
- Window sizes measured: 1280×800 and the largest 1440-wide height the development display allows (1440×844);
  a smoke-only content-size switch fixes the viewport (Q-0012-05).

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | Primary actions visible without scrolling | `desktop-ux`: per route × theme × language at 1280×800 and 1440×844, the primary action of each state is inside its scroll container's visible box and the viewport |
| 2 | No overflow, clipping or covered controls | `desktop-ux`: page and text-container overflow, out-of-viewport elements, `elementFromPoint` coverage of every control |
| 3 | AA contrast and ≥ 12 px text | `desktop-ux`: composited contrast per text element in both themes (cross-checked against screenshot pixels during development), font-size floor |
| 4 | Keyboard and focus | `desktop-ux`: every `[data-act]` focusable with a visible indicator; no focus on `<body>` after the scripted navigation; dialogs trap and restore focus |
| 5 | Nothing internal leaks | `desktop-ux`: zh screens contain no raw code, id, hash or CLI text outside exam content, identifiers marked `data-raw`, and the allowlist |
| 6 | Motion is purposeful and optional | `desktop-ux`: with reduced motion emulated no animation runs > 1 ms and none loops; without it the room transition runs on a condition change |
| 7 | The card is one source | `note-smoke`: model CSS/template hashes pinned, `createModel` sends exactly `note model`, upgrade from v1 sends styling+templates once, a customised model is reported and not overwritten, no remote URL/script; `desktop-smoke`: preview `srcdoc` equals the Go-rendered card document; Go tests: night-mode selectors and contrast ≥ 4.5 for every text token |
| 8 | Refinement and history work in the window | `desktop-smoke`: refine previews payload, consent and budget, sends to the loopback stub, renders an advisory suggestion as text, accept keeps the learner in control; history lists the attempt's calls |
| 9 | The gate catches what it claims | self-test: a stylesheet that removes focus outlines and drops a title to background colour fails `desktop-ux`; every in-page probe runs against planted defects on each run |
| 10 | Nothing regresses | all earlier gates; CLI goldens unchanged |

Rendered screens (light/dark × zh/en for each route at 1280×800) are attached to the evidence record as the
reviewable artifact; they are evidence of the state the gates checked, not a substitute for them.

## Known limits

Real Anki desktop, AnkiDroid and AnkiMobile rendering and the sync behaviour of a template update are owner
evidence: no Anki installation exists on the development machine. Contrast on translucent layers is computed,
not measured by eye; the in-window audit cannot press a real Tab key in a hidden window, so reachability is
approximated by focusability. macOS and Linux are covered as far as CI runs the audit.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
