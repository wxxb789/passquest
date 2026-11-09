# 0007 — Useful Anki notes created during study

**State:** Provisional
**Prerequisite:** 0006 — resumable agent work

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Without implementing an Anki clone, allow a desktop learner to turn high-value words, exam patterns or short tactics encountered in real exercises into beautiful, concise Anki notes. Anki Desktop/AnkiConnect is the primary local path, and Anki's existing mobile syncing remains authoritative.

## Provisional basis for acceptance
- A learner can propose/preview/edit and accept or skip a note from a question they just studied, with useful source context and no irrelevant decoration.
- An accepted note appears correctly in a connected local Anki Desktop deck with a styled template and works with Anki's mobile-supported HTML/CSS; the app explains actual AnkiWeb sync rather than falsely claiming a direct phone push.
- Repeated submission/restart doesn't duplicate notes and later revisions preserve Anki review history and user edits, or surface a conflict.
- Offline queueing and a practical export fallback are demonstrable; confidential content is not sent to AnkiWeb without authorization.

**Requires refinement before activation:** Refine exact Anki setup and sample card expectations against the environment available after goal 0006. The output is a user-visible Anki note, not a fabricated sync screenshot.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
