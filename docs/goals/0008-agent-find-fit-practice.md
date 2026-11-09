# 0008 — Find practice only when learning evidence needs it

**State:** Provisional
**Prerequisite:** 0006 — resumable agent work and 0004 — adaptive next exercise

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Turn an observed training gap into bounded, source-aware resource discovery. Do not preinstall a general-purpose English corpus or allow unlimited scraping/search costs.

## Provisional basis for acceptance
- A learner's observed skill gap can trigger a specific search intent, yielding at least one relevant and provenance-bearing candidate if an authorized accessible source exists.
- The learner can distinguish external link-only resources, content legally available for local study, and generated unverified variants.
- Unverified AI answers, copyright restrictions and instructions embedded in web pages do not become authoritative study/assessment state.
- The agent visibly reports lack of suitable material, budget limits and failures instead of inventing articles or answers.

**Requires refinement before activation:** Refine the search provider, rights policy and representative learner gap when activating. Availability of real external sources is a dependency, not evidence of model competence.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
