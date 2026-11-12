# 0009 — Show defensible independent exam readiness: increment plan

Contract: [`../goals/archive/0009-unseen-exam-assessment.md`](../goals/archive/0009-unseen-exam-assessment.md).
Shape references: goal 0008's plan/evidence and `find-smoke`, the append-only attempt and
exposure ledgers (`internal/store/exposure.go`), the source review gate
(`internal/source/question.go`), and the goal-0006 interruption vocabulary
(`internal/cli/agent.go`). Conventions: [`../conventions.md`](../conventions.md). Harness:
[`../harness.md`](../harness.md). Owner decisions already recorded: Q-0009-01..05 in
[`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

## Goal in one sentence

Measure timed, unassisted performance on questions the learner has never been shown, by spending a
whole content unit as soon as any question on it is presented, running the selected items under a
visible per-item clock, and reporting what the stored answers support separately from what they do
not.

## Public shape

```text
passquest assess pool   [--db PATH] [--json]
passquest assess start  [--size N] [--minutes M] [--annotations none] [--db PATH] [--json]
passquest assess resume RUN [--db PATH] [--json]
passquest assess abandon RUN [--db PATH] [--json]
passquest assess report RUN [--db PATH] [--json]
passquest assess compare RUN_A RUN_B [--db PATH] [--json]
```

Exit codes: `0` normal / pool report / completed run / report / comparison · `2` invalid input,
pool-floor or pool-short refusal, `run_active`, `run_abandoned`, `assessment_active`, incomparable
runs, unknown run · `4` an interactive run interrupted by EOF or `quit` (the run stays `active`;
`assess resume` continues it) · `1` local storage failure.

`--size` defaults to 40 and `--minutes` to 60 (Q-0009-01). `--annotations none` is the only accepted
value; any other value is exit 2. `assess start`/`resume` accept `--json` as a **blind batch** mode:
no question is printed, one answer line is read per item, and one summary document is written at the
end. Human mode is interactive and prints the item without key, evidence, reasoning or glossary.
Both modes record the same rows. `pool`, `report`, `compare` and `abandon` emit one JSON document.

## Content unit and holdout pool

The content unit is the existing `content_id`: one imported source page (`<source>-pNNN`, Q-0002-03),
which is the passage boundary this material actually has. The pool is built only from imported
sources, never from the embedded sample or the skill passages; the known limit in the contract says
holdout items come from reviewed, imported material.

A unit qualifies when its page is reviewed and at least one question on it passes the existing
practice gate (reviewed page, reviewed answer, evidence still a substring). Qualifying items are the
practiceable questions on qualifying units. Unit order is source creation order, then page ascending,
then question id ascending — the order the stored source presents its pages and questions. A run
takes whole units in that order until it reaches `--size`; a unit is never split across runs, so the
actual item count is printed and may exceed `--size` by at most the last unit's question count.

A unit is spent when any of its questions has been presented. The spent set is a **union**, so old
databases need no backfill:

| Source | Timestamp used |
|---|---|
| `attempts` (ordinary, tactic, skill or assessment answer) | `started_at` |
| `tactic_exposures` (guided walkthrough delivered) | `exposed_at` |
| `content_presentations` (new append-only ledger for `practice`, `tactic`, `skill`, `show`, `questions`, `next`) | `at` |
| `note_drafts` and `notes` (a derived note) | `created_at` |
| `assessment_items` (presented, answered or timed out) | `presented_at` |

Surfaces that render a question now append to `content_presentations`: `practice`, `tactic
practice`, `skill practice`, `show` (when a page or source is displayed with its question lines),
`questions`, and `next` for the material the recommendation names. An assessment presentation is
already an `assessment_items` row, so it is not duplicated in this ledger. `note
propose` and `note accept` are already covered by the draft/note rows; `tutor` requires a stored
attempt, so its unit is already spent. Skill and tactic lesson views (`skill show`, `tactic show`)
are not question presentations and spend nothing. `next` prints only a command, not a stem; it
still spends the named item because the contract lists `next` explicitly, and this plan treats that
as the conservative reading. If the owner wants `next` to be non-spending, only the single
`recordPresentation` call in `runNext` is removed.

`assess pool` prints qualifying items, qualifying units, practiceable units, spent units, the floor,
the shortfall, and one line per spent unit with the surfaces that spent it, plus the unseen units.
That makes acceptance #1 directly inspectable: exposing one question removes every question on its
unit, and a fresh unit still counts.

## Run semantics

`assess start` reads the pool, refuses below 20 qualifying items (`pool_floor`, naming the item
shortfall and the unit count), then refuses a requested size larger than the pool (`pool_short`,
naming the shortfall) instead of silently shrinking it. It creates one `assessment_runs` row and one
`assessment_items` row per selected question.

- Per-item timeout is `floor(minutes × 60000 / actual_size)` milliseconds, at least 1; it is stored
  on the run and printed. The whole-run deadline is `started_at + minutes`; each item's deadline is
  `min(presented_at + per_item_timeout, run_deadline)`.
- The first presentation writes `presented_at` and the item deadline. A question is re-rendered on
  resume with the original `presented_at`, so an interruption does not reset its clock.
- An answer read after the item deadline is ignored and the item is recorded as a timed-out
  `skipped` attempt with `assistance='none'` and `elapsed_ms = item deadline - presented_at`. No
  correctness feedback is printed during the run. The report shows the annotation and the outcome
  afterwards.
- A process killed or EOF-interrupted between items leaves the run `active`; `assess resume RUN`
  increments `resume_count`, appends a `resumed` event and continues. Any run whose whole-run
  deadline has passed is reconciled to `abandoned` with a reason on the next assess/guard command;
  its completed items remain readable, and it is never deleted.
- `assess abandon RUN` is the explicit learner exit; it records `abandoned` and an event.
- One active run at a time. `assess start` during an active run refuses with `run_active` and the
  resume command. `tutor`, `skill` and `find` refuse with `assessment_active` (exit 2) while a run is
  active and inside its window; an expired run is abandoned first, so it stops blocking.
- `assistance='none'` is a new attempt condition. The attempts table is rebuilt in schema v14 to
  widen its CHECK; old rows keep their values, and a CHECK pairs `assistance='none'` with a positive
  `assessment_run_id`. Assessment answers are ordinary append-only attempts, so the existing
  `elapsed_ms`, `skills_json`, start/answer timestamps and report/adaptive consumers see the same
  evidence. Adaptive selection and the find level bucket treat `none` like an independent first
  attempt because it is timed and unassisted; repeat/timeout rules still apply.

## Annotations withheld

Authored question files gain an optional `glossary` array of `{term, meaning}` entries. It is
validated, stored on `source_questions.glossary_json`, carried into `exam.Question`, and shown by
ordinary practice, `show`, `questions` and `report`. The assessment renderer omits the glossary,
question reasoning, option reasoning and evidence sentence. `assess report` prints all four per item
under `Annotations (withheld during the run)`, so the learner can learn from them after the fact
(Q-0009-05). A question without a glossary prints `glossary: none`; the field is optional so no
existing fixture changes.

## Report

The report is recomputed from run rows, item rows and attempt rows. It prints:

- status, selected/presented/finalized/scored counts, correct/total over **scored** items, and the
  accuracy ratio;
- per-skill correct/total over scored items (an item with two skills contributes to both; an item
  with no registered skill is listed as `unclassified`);
- median and p90 `elapsed_ms` over scored finalized items, using the median (average of the two
  middle values for an even count) and nearest-rank p90; timed-out items contribute their
  per-item-timeout elapsed value and are separately counted;
- timed-out count, skipped count, unavailable-item count and pending count;
- accuracy and speed bands, explicitly labelled descriptive: accuracy `low` below 50%, `mixed`
  50–69%, `developing` 70–84%, `strong` at or above 85%; speed `comfortable` below 50% of the
  per-item timeout, `steady` 50–74%, `pressed` at or above 75%;
- a contamination section with four named categories: `prior_exposure`, `repeated_variant`,
  `hints_during_run`, `resumed_after_window`. Every contaminated item is excluded from the headline
  and per-skill accuracy, and listed with its position, ids, status, elapsed time and reasons. It is
  never averaged away;
- the per-item annotations;
- explicit unknowns: `pass threshold: unknown`, `real item count: unknown`, `real time limit:
  unknown`, `readiness probability: not computed`; there is no pass/fail verdict.

Contamination rules are pure functions over the spend union and the run/event rows:

- `prior_exposure`: a spend on the item's content unit with `at < item.presented_at`, excluding this
  run's own assessment rows.
- `repeated_variant`: the same question id was presented under a different content id before this
  item, or the content unit appeared in an earlier assessment run.
- `hints_during_run`: a spend on the item's content unit after the run's `started_at` and before its
  answer, excluding assessment surfaces. This is what a `show`, `questions`, `practice` or `next`
  call from another process during the run produces.
- `resumed_after_window`: the run was resumed at or after its whole-run deadline. Such a run is
  reconciled to `abandoned`; any answer recorded after the deadline is excluded, and the event is
  listed even when no item was affected.

## Comparison

`assess compare A B` loads both reports and refuses with `incomparable_runs` (exit 2) unless both
runs are `completed` and their actual item count, `window_minutes` and pool family are equal. The
first differing dimension in that fixed order is named with both values. The pool family is the
SHA-256 of the sorted `source_id@sha256` lines of every imported source at run start; importing or
removing a source changes the family, and page corrections inside a source do not. A successful
comparison prints side-by-side counts, accuracy, median/p90 and timed-out counts, plus per-skill
accuracy and deltas. It never says that AI, coaching or the app caused the change; the limitations
say the comparison is two local samples and that unequal scored-item counts after contamination are
shown rather than normalized away.

## Persistence (schema v14)

`internal/store/assess.go` adds:

| Table | Ownership |
|---|---|
| `assessment_runs` | One row per run: window, requested/actual size, per-item timeout, deadline, annotations, pool family, status, resume counters, completion/abandon timestamps. A partial unique index allows one `active` row. |
| `assessment_items` | Selected question order and state: `pending`/`presented`/`answered`/`timed_out`/`unavailable`, content version, skills snapshot, presented/deadline/answered times, attempt id. |
| `assessment_events` | Append-only audit: `created`, `presented`, `answered`, `timed_out`, `resumed`, `completed`, `abandoned`, `unavailable`. |
| `content_presentations` | Append-only presentation ledger: content id, question id, surface, at. |

Migration v14 also rebuilds `attempts` (widen the assistance CHECK to `none`, add
`assessment_run_id`, recreate the composite index and the three append-only triggers) and adds
`source_questions.glossary_json`. The rebuild copies every row explicitly; a migration test opens a
v13 fixture, keeps its attempt, proves the append-only triggers still reject update/delete, and
records an assessment attempt.

## Evidence plan

1. **Go tests**
   - `internal/exam` / `internal/source`: glossary parsing, validation, storage round-trip,
     practice-exam carry-through; existing fixtures unchanged.
   - `internal/store`: v13→v14 migration and attempt rebuild; append-only attempt triggers; the
     `none`/run-id CHECK; presentation append-only; run lifecycle (create, present, answer, timeout,
     resume, complete, abandon, one-active); spent union includes old attempts, exposures, notes and
     new presentations; items and events round-trip.
   - `internal/assess`: pool unit spending, item/unit counts, source order, whole-unit selection,
     per-item timeout derivation, median/p90, per-skill aggregation, each contamination category,
     exclusion from the headline, compare dimension order and matching.
   - `internal/cli`: pool floor and shortfall, size shortfall, annotations flag, active-run refusal
     for tutor/skill/find and for a second start, run rendering without annotations, JSON report
     shape, contaminated item listed, compare refusal, abandon, interrupted/resume, forced timeout
     with injected clock and reader, glossary visible in report.
2. **Tier 2 real transcript** in `/tmp/passquest-0009-demo/` with the real binary: import a generated
   reviewed fixture source, `assess pool` before/after one exposure, a real run with a forced
   timeout, kill/resume, a mid-run `show` contamination, report, compare refusal and success.
3. **Gate**: `harness/gates/assess-smoke.ts` drives the real CLI against a generated reviewed source
   in a temp directory; it asserts all nine acceptance rows, registers in `harness/gates/index.ts`,
   and gets a broken-CLI case in `harness/gates/self-test.ts`. It does not run beside the Go test
   suite; the gate runner is sequential.
4. **Docs**: `docs/assess.md` (commands, pool rule, run clock, annotations, report, contamination,
   comparison, limits), `docs/source-import.md` glossary field, README goal 0009 entry,
   `docs/adaptive.md`/`docs/find.md` note that `assistance='none'` counts as independent evidence,
   harness/cross-platform gate counts.
5. **Close**: move the contract unchanged to `docs/goals/archive/`, update `docs/goals/README.md`,
   append the new defaults to `OPEN-QUESTIONS.md`, write
   `docs/evidence/0009-unseen-exam-assessment.md` with real commands/numbers/exit codes, and run
   `bash scripts/gate.sh` once on the final tree.

## Trade-offs and defaults taken

- **Attempts, not a second answer table.** Assessment answers reuse the append-only attempt row,
  because the contract's premise is that the existing facts already describe the run. The cost is a
  v14 table rebuild to admit `assistance='none'`; the migration test pins old rows and triggers.
- **Whole units, not exact item counts.** A unit is the holdout boundary; splitting one across runs
  would spend it while leaving its siblings unusable. `--size` is a target, the actual count is
  stored and printed, and compare uses the actual count.
- **First unseen units, not a random sample.** Deterministic and reproducible, but it over-represents
  early pages. The report's limitations say the sample is deterministic rather than random and that
  item difficulty is not calibrated.
- **Timeout is enforced when input returns.** A terminal CLI cannot preempt a blocked read
  portably. The printed deadline and the computed elapsed are real; a late answer is recorded as a
  timeout, and the report says so.
- **Glossary is an optional authored field.** The contract names glossary and reasoning; reasoning
  already exists, so a small optional field makes the withholding claim inspectable without inventing
  content or changing existing fixtures.
- **`next` spends the item it names.** The contract lists `next`; the learner is told the exact next
  item even though its stem is not printed. This is the conservative reading and is reversible in
  one call.

## Explicitly out of scope

Pass probability, a fabricated pass threshold, adaptive difficulty inside a run (goal 0004), item
authoring, a second looser practice pool (Q-0009-03), and any claim that this replaces the real
exam. No real exam blueprint, item count or duration is invented.
