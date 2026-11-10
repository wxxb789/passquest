# 0003 — A scoring tactic applied on independent questions

**State:** Ready
**Prerequisite:** 0002 — usable private baseline

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Teach one real scoring tactic (such as NOT/EXCEPT polarity, locating textual evidence, or avoiding distractor paraphrases), oriented to getting correct answers with limited English and time, rather than teaching a whole grammar curriculum.

## Done when
- A learner can practice a concrete evidence-based exam tactic on source-grounded material and understand why/when it works.
- After help, the learner can try a distinct answer-verified question without hints; assisted success and independent correctness stay separately visible.
- The material includes a counterexample or boundary showing why naive folklore or blind keyword matching can fail.
- Question provenance, local data confidentiality and deterministic scoring continue to hold.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
