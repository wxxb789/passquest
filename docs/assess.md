# Timed unseen assessment

Goal 0009 measures timed, unassisted performance on questions the learner has never been shown.
It does not invent an exam blueprint, a pass threshold or a pass probability. The contract is
[`goals/archive/0009-unseen-exam-assessment.md`](goals/archive/0009-unseen-exam-assessment.md)
(archived at close); the plan is
[`plans/2026-11-12-1255-0009-unseen-exam-assessment.md`](plans/2026-11-12-1255-0009-unseen-exam-assessment.md).

## Commands

```bash
passquest assess pool
passquest assess start  --size 40 --minutes 60 --annotations none
passquest assess resume RUN
passquest assess abandon RUN
passquest assess report RUN
passquest assess compare RUN_A RUN_B
```

`--size` defaults to 40 and `--minutes` to 60; both are local flags, not a discovered examination
specification. `--annotations none` is the only accepted value. `pool`, `report` and `compare`
accept `--json`. `start` and `resume` accept `--json` as a blind batch mode: no question is printed,
one answer line is read per item, and one summary document is written at the end. Human mode is
interactive and prints the item without its key, evidence, reasoning or glossary.

Exit codes: `0` normal · `2` invalid input, pool-floor/shortfall refusal, `run_active`, `run_abandoned`,
`assessment_active`, unknown run or incomparable runs · `4` a run interrupted by EOF or `quit` (it stays
`active` and `assess resume` continues it) · `1` local storage failure.

## The content unit and the holdout pool

The content unit is the existing `content_id`: one imported source page (`<source>-pNNN`). The pool
contains only reviewed, practiceable imported material; the embedded sample and the skill passages
never enter it.

Presenting **any question on a page spends the whole page**. The spent set is a union, so an older
database needs no backfill:

| Evidence of exposure | Timestamp |
|---|---|
| an attempt (ordinary, tactic, skill or assessment) | `started_at` |
| a guided tactic walkthrough | `exposed_at` |
| a content presentation (`practice`, `tactic`, `skill`, `show`, `questions`, `next`) | `at` |
| a derived note (draft or accepted note) | `created_at` |
| an assessment item | `presented_at` |

`assess pool` prints qualifying items, qualifying units, practiceable units, spent units, the floor
and the shortfall, and lists every unit as `unseen` or `spent by …`. A page with four questions and
one exposed question therefore reports one spent unit and four fewer qualifying items; a fresh page
still counts. The unit count, not the item count, is the binding constraint on how many independent
assessments the imported material supports.

`show SOURCE` without `--page` prints every question stem it lists, so it spends every unit whose
stem is shown. `show SOURCE --page N` spends the units whose questions on that page are shown.
`questions SOURCE` prints the same stems and therefore spends the same units.
`next` prints only a command, not a stem, but the contract lists it as a presentation surface, so it
spends the unit its recommendation names. `skill show` and `tactic show` are lessons, not question
presentations, and spend nothing.

## Run rules

- A run refuses below **20 qualifying items** (Q-0009-03) and refuses a requested `--size` larger
  than the pool. It never silently shrinks the request.
- Selected units keep source order: source creation order, page ascending, question id ascending.
  Whole units are taken until the target size is reached; the actual count is stored and printed and
  may exceed `--size` by the last unit's question count. A unit is never split across runs.
- The per-item clock is `floor(minutes × 60000 / actual_items)` ms, at least 1. The whole-run
  deadline is `started_at + minutes`; each item's deadline is the earlier of its own timeout and the
  run deadline. Both are stored and printed.
- A late answer is ignored and recorded as a timed-out `skipped` attempt with
  `assistance='none'` and `elapsed_ms` equal to the item deadline minus its first presentation.
  No correctness feedback is shown during the run. A terminal command cannot interrupt a blocked
  read, so the deadline is enforced when input returns.
- A process killed or interrupted between items leaves the run `active`. `assess resume RUN`
  increments `resume_count`, appends a `resumed` event and continues at the first unfinished item.
  The original `presented_at` is kept, so an interruption does not reset the item clock. An EOF or
  `quit` exits 4 and leaves the run resumable. A run whose whole window has passed is reconciled to
  `abandoned` on the next assess or coaching command; its completed items stay readable.
- One run may be active at a time. `assess start` during an active run refuses with `run_active`.
  `tutor`, `skill` and `find` refuse with `assessment_active` (exit 2) while a run is active and
  inside its window. An expired run is abandoned first, so it stops blocking.
- `assess abandon RUN` is the explicit learner exit. It records `abandoned` and cannot rewrite a
  completed run.

Assessment answers are ordinary append-only attempts with `assistance='none'` and
`assessment_run_id` set. Adaptive selection and the find level bucket treat them as independent
first-attempt evidence; repeat and timeout rules still apply.

## Annotations

An authored question file may add a `glossary` array of `{term, meaning}` entries. Ordinary
practice, `show`, `questions`, `report` and the pre-answer JSON show it. A run omits the glossary,
question reasoning, option reasoning and evidence sentence. `assess report` prints all four per item
under `Items and withheld annotations`, so the learner can still learn from them after the fact. A
question without a glossary prints `glossary: none`. See
[`source-import.md`](source-import.md#annotations).

## Report

`assess report RUN` is recomputed from the run, item and attempt rows:

- correct/total over **scored** items, with contaminated items excluded from both the headline and
  per-skill accuracy;
- per-skill correct/total; an item carrying two skills contributes to both and an untagged item is
  listed as `unclassified`; skipped and timed-out items count as not correct;
- median and nearest-rank p90 `elapsed_ms` over scored items, with timed-out items contributing
  their per-item-timeout value and a separate timed-out count;
- selected/presented/finalized/scored/contaminated/pending/unavailable counts;
- accuracy and speed bands, explicitly descriptive: `low` below 50%, `mixed` 50–69%, `developing`
  70–84%, `strong` at or above 85%; `comfortable` below 50% of the per-item clock, `steady` 50–74%,
  `pressed` at or above 75%;
- a contamination section, never averaged away;
- the withheld annotations;
- explicit unknowns: `pass threshold: unknown`, `real item count: unknown`, `real time limit:
  unknown`, `readiness probability: not computed`, `exam blueprint: not available`. No pass or fail
  verdict is printed.

### Contamination

| Reason | Rule |
|---|---|
| `prior_exposure` | a spend on the item's content unit before the item's first presentation, excluding this run's own rows |
| `repeated_variant` | the same question id was presented under another content id, or the content unit appeared in an earlier assessment run |
| `hints_during_run` | a non-assessment spend on the item's content unit after the run started and before the item's answer |
| `resumed_after_window` | the run was resumed at or after its whole deadline; such a run is abandoned, and any answer after the deadline is excluded |

Contaminated items are listed with their position, ids, state, elapsed time and reasons. Sibling
questions share a contaminate unit: if one page was exposed, every selected item on that page is
excluded and listed.

## Comparison

`assess compare A B` refuses (`incomparable_runs`, exit 2) unless both runs are `completed` and
their actual item count, `window_minutes` and pool family match. The first differing dimension in
that order is named with both values. The pool family is the SHA-256 of the sorted
`source_id@sha256` lines of every imported source at run start; importing or removing a source
changes it, while a page correction inside a source does not.

A successful comparison prints side-by-side counts, accuracy, median/p90, timed-out counts and
per-skill accuracy, with deltas. It never says that AI, coaching or the app caused a change:
the runs are local samples, and an unequal scored-item count after contamination is shown rather
than normalized away.

## Limits

- No real exam standard is available, so the report cannot say "you would pass".
- The selected units are the first unseen material in source order, not a random sample.
- Item difficulty is not calibrated; a cross-run comparison is only valid inside the same pool
  family and item count.
- The 40/60 defaults and the 20-item floor are disclosed approximations, not the real examination.
- Because holdout is per content unit, a page carrying several questions is spent as fast as a page
  carrying one. The pool is reported in both items and units, and the unit count is the binding
  constraint. Every new run needs 20 unseen items remaining, so repeated assessments require a
  larger imported and reviewed pool; this is the disclosed cost of never reusing an exposed item.
- Only imported, reviewed material counts. Completing the 165-page review remains open-ended content
  work.
