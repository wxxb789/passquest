# 0009 — Show defensible independent exam readiness

**State:** Provisional
**Prerequisite:** 0004 — adaptive next exercise and 0002 — private source baseline

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Give learners a meaningful picture of whether they are becoming able to score independently under time constraints. Hold out full passages or source clusters from training; no invented pass probability or exam standard.

## Provisional basis for acceptance
- A learner can complete a timed unassisted assessment on reviewed, previously unseen items and inspect correctness, speed and high-impact errors.
- Prior exposure, hints and repeated variants are excluded from the uncontaminated score or explicitly reported as contamination; source and answer verification are inspectable.
- Repeated assessments compare like-for-like observable outcomes without attributing improvements to AI merely because it was used.
- Where exam scoring information is absent, output states what is known and unknown, rather than fabricating a passing threshold.

**Requires refinement before activation:** Refine the holdout contents, time windows and reporting shape once enough verified independent material exists.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
