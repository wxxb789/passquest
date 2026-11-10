# 0005 — Helpful AI tutor with user-supplied text model

**State:** Ready
**Prerequisite:** 0004 — adaptive next exercise

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Let a learner use their own authorized text-LLM configuration for concise, question-grounded explanations, misconception probes and tactical micro-lessons. Text-only is sufficient; no speech, image or search prerequisite. Provider-specific marketing claims are not an acceptance criterion.

## Done when
- A learner can configure one supported BYOK text endpoint and get useful, bounded coaching tied to an actual attempted question and their observed difficulty.
- AI suggestions are clearly distinct from verified answer keys and deterministic grades; a bad/failed model response cannot corrupt attempts or turn unverified output into an exam fact.
- The learner can inspect, avoid or authorize any private source material being sent to the provider, and can still practice verified local questions when the provider is unavailable.
- Credentials are not written into learning data, exported cards, logs or publicly committed artifacts; practical failure/usage information is visible.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
