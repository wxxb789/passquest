# 0004 — Choose practice based on learner evidence

**State:** Ready
**Prerequisite:** 0003 — scoring tactic practice

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Make adaptive selection real for one learner, with a small set of scoring and minimal language prerequisite skills. Distinguish current evidence from inferred skill state; do not claim a measured pass probability or introduce a broad ontology.

## Done when
- A user's independently recorded attempts, time, hints and revisits change what the system recommends next, and the reason for that choice is inspectable.
- The same question or passage exposure does not masquerade as independent transfer; repeat exposure and uncertain causal diagnosis are retained.
- The user can see what evidence supports a skill gap, what is uncertain, and why an explanation, new question or review was selected.
- After reopening, the historical evidence is intact and inferred recommendations can be recalculated without rewriting old attempts.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
