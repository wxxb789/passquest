# 0011 — Study comfortably in the MyGo desktop application

**State:** Provisional
**Prerequisite:** 0007 — Anki notes and 0009 — unseen assessment, with 0005 — AI coaching

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Move the proven headless exam experience into a genuinely usable MyGo desktop reading and practice workspace without changing the domain model or evidence semantics. This is the **functional UI goal**, intentionally late.

## Provisional basis for acceptance
- A user can start the app, select local practice, answer exam-style questions, use context-aware AI help, inspect feedback and see study/Anki state without using CLI commands.
- Long passages, keyboard navigation, selection, hints, source context, different text lengths and interrupted sessions work as a coherent desktop experience.
- Verified attempts, user content, model failure and source rights behave consistently with the earlier headless acceptance, not as UI-only demonstrations.
- The MyGo/Go-to-agent runtime can be started/stopped/recovered locally without an unexpected public endpoint or orphaned secrets.

**Requires refinement before activation:** Refine the primary desktop OS and concrete UX acceptance examples against the functioning engine. At this point prioritize functional clarity, not final visual artistry.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
