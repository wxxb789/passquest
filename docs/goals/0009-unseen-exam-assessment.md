# 0009 — Show defensible independent exam readiness

**State:** Active
**Prerequisite:** 0004 — adaptive next exercise and 0002 — private source baseline

## Larger purpose

Pass Quest is a local-first, desktop-first AI exam coach. Its concrete target is helping an adult with basic English pass a specific written exam within about 90 days, not maximizing general English proficiency. The actual blueprint, pass threshold and daily study budget are unknown. Its original 165-page source PDF is labeled internal-only and must not be committed, bundled, or exported to public providers without explicit rights/consent. Core principles: evidence and scoring correctness before AI confidence; exam tactics and time-to-score before broad coursework; recoverable, idempotent local state; agent freedom within permission/quality boundaries. Follow [Core Design](../core-design.md).

## Intended outcome

Give learners a meaningful picture of whether they are becoming able to score independently under time constraints. Hold out full passages or source clusters from training; no invented pass probability or exam standard.

## Refined contract (activated 2026-11-12)

The increment is a **timed, unassisted assessment on items the learner has never been shown**, plus a report
that separates what the stored numbers support from what they do not. Existing `attempts` already record
`started_at`, `answered_at`, `elapsed_ms`, `assistance`, `tactic_id` and `skills_json`, so "timed, unassisted,
per-skill" is a query over evidence rather than new state. What does not exist yet is a holdout concept, a
timed run, and a like-for-like comparison.

**In scope**

1. **Holdout pool.** `passquest assess pool` reports how many items qualify as unseen: imported items with an
   answer-verified status and **zero prior exposure**. Exposure is recorded when an item is *presented*
   (`practice`, `next`, `show`, or a derived note), not when it is answered — so a question whose key was
   shown is spent even if the learner never typed an answer.
2. **Assessment run.** `passquest assess start --size N --minutes M` presents one item at a time, refuses
   `tutor`, `skill` and `find` during the run, records per-item `elapsed_ms`, a per-item timeout and
   `assistance='none'`. An interrupted run resumes under the goal-0006 rules; a run left past its window is
   recorded `abandoned`, never silently dropped.
3. **Annotations withheld.** During a run the question renders without glossary or reasoning
   (`--annotations none` is the only accepted value in this goal); the withheld annotations are printed in the
   report instead, so the learner can still learn from them after the fact.
4. **Report.** `passquest assess report <run>` returns correct/total, per-skill accuracy, median and p90
   `elapsed_ms`, timed-out count, and an explicit **contamination section**: prior exposure, repeated
   variants, hints used during the run, resumed-after-window. A contaminated item is excluded from the
   headline score *and* listed — never averaged away.
5. **Comparison.** `passquest assess compare <a> <b>` compares two runs only when item count, time window and
   pool family match; otherwise it refuses and names the dimension that differs. It never attributes a change
   to "AI was used" or "the coach worked".
6. **Honest unknown.** Where the real passing score, item count or time limit are unknown, the report prints
   them as unknown instead of a pass probability or a fabricated threshold.
7. **Pool floor.** A run refuses to start below 20 qualifying items and states how many more items must be
   imported before it can.

**Out of scope**

Predicting a real exam score or pass probability; adaptive difficulty inside a run (that is goal 0004); item
authoring (items come from reviewed imported sources); a second, looser practice pool (recorded as
Q-0009-03); any claim that this assessment replaces the real exam.

**Defaults taken (reversible, disclosed)**

- Item count and time window: `--size 40 --minutes 60`, both required-looking flags, and the report always
  prints the values it used. The real CRSC spec is unknown (Q-0009-01).
- Pass threshold: none. The report shows accuracy and speed bands and prints `pass threshold: unknown`
  (Q-0009-02).
- Minimum holdout: 20 qualifying items (Q-0009-03).
- Exposure: any presentation of the item, not only a wrong answer (Q-0009-04).
- Annotations: hidden during a run, shown in the report (Q-0009-05).

## Acceptance and evidence

| # | Acceptance | Evidence |
|---|---|---|
| 1 | Only unseen items enter a pool | `assess pool` over a database with exposed items prints the exposure events; a control run exposes one item and the pool count drops by one |
| 2 | A run cannot consult coaching | real transcript where `tutor` and `skill` are refused with a typed error and a nonzero exit code |
| 3 | Timing is measured, not asserted | stored `elapsed_ms` per item, plus a run with a forced per-item timeout |
| 4 | Annotations are withheld and later visible | run transcript showing no glossary/reasoning, then the same annotations in `assess report` |
| 5 | Contamination is visible, not averaged away | a run seeded with one previously exposed item: excluded from the headline, listed in the contamination section |
| 6 | Comparison refuses unlike runs | `assess compare` on two runs differing in size or minutes → refusal naming the dimension and an exit code |
| 7 | Unknowns are stated | report over this database prints `pass threshold: unknown` and no pass/fail verdict |
| 8 | Interruption does not lose a run | kill the process mid-run, resume, and the report shows the resumed portion and marks it |
| 9 | The pool floor is enforced | a database below 20 unseen items refuses with the shortfall, exit code shown |

## Known limits

No real exam standard is available, so the report cannot say "you would pass". Holdout items are drawn from
the imported 165-page source, so the pool only covers what was reviewed and imported. Item difficulty is not
calibrated, so a cross-run comparison is only valid inside the same pool family. The 40/60 defaults are a
disclosed approximation, not the real examination.

## Execution boundary

At run start, stop/report if an input only the user can supply makes the specified outcome impossible. Otherwise select reversible defaults and disclose them. Stop for irreversible/unauthorized actions; do not invent a missing exam passing score or publish restricted text. Show real behavior with evidence and explicit limitations, not only a diagram, unit stub, screenshot or progress report. Pair each acceptance condition with its evidence at completion.
