# 0001 — First independent exam attempt

**State:** Ready
**Prerequisite:** None

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Make a real, inspectable, answer-verified exam-style practice interaction usable without a GUI, an LLM, Anki or the private PDF. A tiny redistributable self-authored sample is sufficient; this is the earliest valuable product slice.

## Done when
- A learner can answer an actual, coherent multiple-choice exercise without being shown the answer and see deterministic correctness feedback plus concise evidence-based reasoning.
- Correct, incorrect, skipped and repeated attempts are represented truthfully, with no fictional scores or knowledge claims.
- A fresh local invocation demonstrates the end-to-end attempt and shows durable evidence of its result after reopening the app/process.
- The demonstration contains no original confidential CRSC text, no external calls and no claim that the sample is from the live exam.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
