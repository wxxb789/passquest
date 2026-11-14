# 0011 — Study comfortably in the MyGo desktop application

**State:** Active
**Prerequisite:** 0007 — Anki notes and 0009 — unseen assessment, with 0005 — AI coaching

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Move the proven headless exam experience into a genuinely usable MyGo desktop reading and practice workspace without changing the domain model or evidence semantics. This is the **functional UI goal**, intentionally late.

## Refined contract (activated 2026-11-14)

The increment is a **MyGo desktop workspace that drives the same Go core the CLI drives**. Goals 0001–0010
and 0014 provide every study truth headlessly: verified practice with assisted/independent conditions,
adaptive `next`, tactic and skill practice, the BYOK tutor with consent and per-provider budget, Anki notes
with an outbox, a durable study brief, find, and timed unseen assessment. What does not exist is a window: the
orchestration lives in unexported `internal/cli` functions that print to a writer, the two-step JSON practice
protocol loses the learner's elapsed time, and nothing can be done without a terminal.

The owner chose the **"C · Stage"** design direction
([`design/prototypes/c-stage.html`](../../design/prototypes/c-stage.html), brief
[`design/prototypes/BRIEF.md`](../../design/prototypes/BRIEF.md)). This goal adopts its tokens, layout and
component vocabulary so that 0012 refines one design language instead of replacing a placeholder, but its
evidence is functional behaviour, not visual polish.

**In scope**

1. **A desktop entry point.** `cmd/passquest-desktop` is a MyGo application (pinned version, no cgo) with one
   web window whose frontend is TypeScript bundled by Bun and embedded in the binary. It is a separate binary
   so `cmd/passquest` keeps linking no HTTP client. Invoked with CLI arguments it behaves exactly like
   `passquest` (so the durable host can call `agent record-step` on it).
2. **One Go core, two surfaces.** A typed workspace service exported by the Go core is bound to the window.
   It reuses the CLI's selection, grading, tutor, note, assessment and model code; it does not reimplement a
   rule. Every bound call returns structured data, never CLI text.
3. **Today.** The next action from the transparent policy with its reason and per-skill evidence, the
   unseen-assessment pool, the last assessment, Anki counts, a paused study brief (resume/cancel), find
   status, and the active tutor model with its budget. Exam date and daily study budget are optional local
   settings shown as `not set` until the learner sets them; the pass threshold is always `unknown`.
4. **Practice and feedback.** Select local practice (adaptive next, a skill, a tactic, or the sample),
   answer by mouse or keyboard (A–D / 1–4, Enter, S skip, H hint, ? shortcut sheet), and see before answering
   whether the attempt can count. Opening a hint warns first and records the attempt as assisted. The
   recorded attempt carries the real elapsed time between presentation and answer. Feedback shows the
   deterministic grade, the verified key, every option's authored explanation (or the "not provided" state),
   the evidence sentence highlighted in the passage, and the tactic link. Selecting passage text can seed an
   Anki note.
5. **Context-aware AI help.** From feedback the learner can preview and send `explain`, `probe` and
   `micro_lesson` requests; the panel shows external vs local, provider and model, the exact payload summary,
   the source consent state and the budget slot before sending. Replies are advisory, rendered as text, and
   never change the grade. Consent can be granted and revoked from the window.
6. **Assessment and report.** Start a timed unassisted run when the pool allows it, answer item by item with
   the run and per-item clocks, leave and resume (the clock keeps running), and read the report with the
   like-for-like comparison and the per-item annotations that were withheld during the run.
7. **Anki.** Propose a note from an attempt or a passage mark, edit fields with a live rendered preview using
   the real note model, accept or skip, push through the existing outbox, and see pushed / queued / already
   present / conflict states.
8. **Settings.** Providers, models, capabilities and authorization as four separate facts; `model use` and
   `model test`; credentials entered in the window are stored in the OS credential store and shown only as
   present/missing; per-provider budget; the consent ledger; local services (Node availability, AnkiConnect
   endpoint, search endpoint).
9. **Interruption.** Closing or killing the app mid-item and relaunching resumes the same presented item with
   its original presentation time and no attempt written; an interrupted assessment resumes under the
   existing run rules; a paused brief shows as paused.
10. **Runtime lifecycle.** The desktop process opens no TCP listener; Node hosts are spawned per call exactly
    as the CLI spawns them and none is left running after the window closes; no credential is written to
    SQLite, logs, the DOM snapshot or exported files.

**Out of scope**

Visual polish beyond adopting the chosen tokens and layout (goal 0012); installers, first-run onboarding,
upgrade and relaunch guarantees on a clean machine (goal 0013); a second learner or encrypted store
(Q-0002-02); fetching Anki-side field values to offer "keep Anki's version" (Q-0011-07); new practice content.

**Defaults taken (reversible, disclosed)**

- Verified platform: Windows 11 with WebView2 locally; macOS and Linux build, vet, test and launch the
  smoke in CI (Q-0011-01).
- Frontend toolchain: plain TypeScript, no framework, bundled by Bun; the bundle is generated, not
  committed (Q-0011-02).
- Credentials: an environment variable still wins; otherwise the OS credential store (Q-0011-03, revisits
  Q-0005-01).
- Data location: the desktop app defaults to its per-user data directory; `PASSQUEST_DB` overrides
  (Q-0011-04).
- Hints: offered only where an authored guided walkthrough exists; opening one is recorded as an exposure
  and the attempt is assisted (Q-0011-05).
- Exam date and daily study budget: optional, display-only learner settings; never inputs to scoring,
  selection or assessment (Q-0011-06).
- Presentation: the window records a presentation when an item is displayed, which spends assessment units
  exactly as `show` and `questions` do (Q-0011-08).
- All-providers-unusable state: one aggregated line plus the per-provider table (resolves Q-0010-06's
  revisit, Q-0011-09).

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | The app starts and renders real study state without the CLI | `desktop-smoke` launches the built binary with a hidden window on an isolated data directory and asserts a non-blank Today built from bound calls |
| 2 | Practice is real evidence | the smoke answers through the DOM by keyboard; a second process's `passquest report --json` shows the attempt with `elapsed_ms > 0` and the condition the window showed |
| 3 | Hints turn an attempt assisted | the smoke opens the hint gate, confirms, answers; the report shows the attempt assisted |
| 4 | Feedback is the verified truth | the DOM shows the verified key, every option explanation or its missing state, and the evidence sentence; Go tests cover the service DTOs |
| 5 | AI help is advisory and consent-gated | loopback provider stub: preview payload matches the sent body; a reply renders as text; imported text is withheld before consent; a 401 leaves the attempt report byte-identical |
| 6 | Assessment and report work in the window | on a generated reviewed source the smoke starts a run, answers, leaves, resumes, and reads a report showing `pass threshold: unknown`; the navigator never shows correctness |
| 7 | Anki flow works in the window | loopback AnkiConnect stub: propose, edit, accept and push land one note; an unreachable endpoint queues it |
| 8 | Credentials stay secret | a key entered in Settings is stored in the OS credential store under a test service, shown as `present`, and absent from SQLite, logs and the DOM |
| 9 | Interruption is recoverable | kill mid-item, relaunch, the same item and presentation time return; no attempt was written |
| 10 | Lifecycle is clean | the smoke finds no listening TCP socket owned by the app and no child process after exit |
| 11 | Nothing headless regresses | all earlier gates stay green; `cmd/passquest` still links no `net/http`; the local-only rule additionally covers the desktop's first-party packages |

## Known limits

The smoke drives the real window through in-page script, not a human; usability with long imported pages is
judged on the owner's machine and recorded in the evidence record. macOS and Linux are covered only as far as
CI launches the hidden window. No real provider or real Anki deck is contacted. Visual fidelity to the
prototype is a 0012 claim, not a 0011 one.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
