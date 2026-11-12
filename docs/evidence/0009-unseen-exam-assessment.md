# 0009 — Show defensible independent exam readiness: evidence

- Tree: working tree on top of `fda985eccd1ea521c5de9061efea09be9c37c513` (`docs(0009,0010):
  activate both contracts with the owner's decisions`). The operator owns the commit; this run did
  not commit, push or open a pull request.
- Date: 2026-11-12. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3; Node v22.23.2; Bun 1.4.4.
- Contract: [`../goals/archive/0009-unseen-exam-assessment.md`](../goals/archive/0009-unseen-exam-assessment.md).
  Plan: [`../plans/2026-11-12-1255-0009-unseen-exam-assessment.md`](../plans/2026-11-12-1255-0009-unseen-exam-assessment.md).
  Behaviour: [`../assess.md`](../assess.md).

The demonstration used `/tmp/pq-assess-demo/`, a locally built CLI and generated, independently
authored fixtures. No private source text, course PDF, baseline page or learner data was used or
added to the repository. The demonstration databases and JSON documents stayed under `/tmp`.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/assess` | The holdout unit rule, whole-unit selection, the per-item clock, median/p90, per-skill aggregation, contamination classification and comparison dimensions. No database, network or attempt writer. |
| `internal/store` | Schema v14: `assessment_runs`, `assessment_items`, `assessment_events`, append-only `content_presentations`, an optional glossary column, and a rebuilt append-only `attempts` table that admits `assistance='none'` only with an `assessment_run_id`. |
| `internal/cli` | `assess pool/start/resume/abandon/report/compare`, the interactive and blind-batch run loop, the coaching guard, source/next presentation recording, and the human/JSON documents. |
| `internal/attempt` | `AssistanceNone`, `AssessmentRunID`, `WithAssessment` and `IndependentEvidence`; assessment answers remain ordinary append-only attempts. |
| `internal/exam`, `internal/source` | The optional bounded glossary and its validation, storage and carry-through into practice content. |
| `harness/gates/assess-smoke.ts` | Real CLI, generated reviewed source, 259 assertions over the nine acceptance rows. |

## Acceptance

| # | Acceptance | Evidence |
|---|---|---|
| 1 | Only unseen content units enter a pool | `assess pool` on a 40-page source reported 43 items / 40 units; one practice on page 1 reported 39 items / 39 units, page 1 `spent by attempt,practice` with 4 questions, page 2 still unseen. `assess-smoke` asserts the same drop and the fresh unit. |
| 2 | A run cannot consult coaching | an interrupted run left active; `tutor history`, `skill show` and `find` each exited **2** with `{"error":{"code":"assessment_active",...}}`; `assess-smoke` asserts all three. |
| 3 | Timing is measured, not asserted | every run item carries a stored `elapsed_ms`; a real `(sleep 3.2; yes B \| head -20)` run with `--size 20 --minutes 1` recorded 1 timed-out item at exactly `3000 ms` and `timed out 1`. |
| 4 | Annotations are withheld and later visible | the human run transcript shows the passage, stem and options but no reasoning, option reasoning or glossary; `assess report` prints `Evidence`, `Reasoning`, both option explanations and `Glossary: closes — shuts`. |
| 5 | Contamination is visible, not averaged away | a `show SOURCE --page 15` during a run produced `scored: 19; contaminated: 1`, `prior_exposure: 1`, and item `#18 …p015/p15-q1 … reasons prior_exposure`; the headline was 19/19, not 20/20. |
| 6 | Comparison refuses unlike runs | `assess compare 1 2` exited **2** with `window_minutes differs (60 vs 30)`; `assess compare 1 1` exited 0 with state `compared`. |
| 7 | Unknowns are stated | every report prints `pass threshold: unknown`, `real item count: unknown`, `real time limit: unknown`, `readiness probability: not computed`; no pass/fail verdict or probability appears. |
| 8 | Interruption does not lose a run | a real process was killed with SIGTERM after five answers; `assess resume 1` completed the remaining items, and the report showed `Resumes: 1; answered after a resume: 15`, with 15 items marked `after resume`. |
| 9 | The pool floor is enforced | a 4-item database exited **2** with `assessment pool has 4 qualifying item(s) on 4 unseen content unit(s); at least 20 are required, so 16 more reviewed item(s) must be imported`. |

## Pool and unit spending

```text
Pass Quest — unseen assessment pool
Qualifying items: 43
Qualifying units: 40
Practiceable units: 40 (spent 0)
Pool floor: 20 items (met)
Default run: --size 40 --minutes 60
```

After `practice --source SOURCE --page 1 --question p01-q1 --answer B`:

```text
Qualifying items: 39
Qualifying units: 39
Practiceable units: 40 (spent 1)
Pool floor: 20 items (met)
```

and the unit table shows `src-…-p001 page 1 questions 4 spent by attempt,practice`, while
`src-…-p002` remains `unseen`. The drop is four items, not one.

The 4-item floor refusal, exit 2:

```json
{
  "schema_version": 1,
  "error": {
    "code": "pool_floor",
    "message": "assessment pool has 4 qualifying item(s) on 4 unseen content unit(s); at least 20 are required, so 16 more reviewed item(s) must be imported (the unit count is the binding constraint on independent assessments)"
  }
}
```

## A completed run and its report

`assess start --size 20 --minutes 60` on an unspent 82-item source selected page 1's four questions
plus 16 single-question pages. All answers were `B`.

```text
Status: completed
Window: 60 minute(s); per-item timeout: 180000 ms; annotations: none
Selected: 20; presented: 20; finalized: 20; scored: 20; contaminated: 0; pending: 0; unavailable: 0
Resumes: 0; answered after a resume: 0
Correct: 20/20 (100.0%)
Accuracy band: strong (descriptive; not a pass threshold)
Elapsed: median 12 ms; p90 33 ms; timed out 0; skipped 0; basis 20 scored item(s)
Speed band: comfortable (descriptive)

Per-skill accuracy (scored items)
  not-except-polarity      17/17 (100.0%)
  vocabulary-in-context    3/3 (100.0%)
```

The same report's annotation section:

```text
Items and withheld annotations
  #1 …-p001/p01-q1  correct  10 ms
    Evidence: closes at 4 p.m. on Saturdays
    Reasoning: Reasoning for page 1 question 1.
    Option A: Nine is the weekday opening time.
    Option B: The passage states 4 p.m. on Saturdays.
    Glossary: closes — shuts
```

The human run transcript for the same item set contains the passage, stem and options and the line
`No coaching, glossary, reasoning, evidence or correctness feedback is shown during the run.` It
contains no `Reasoning:`, `Option A:` explanation or `Glossary:` line. The passage text necessarily
contains the evidence sentence; the withheld annotation is the separate `Evidence` field.

## Forced per-item timeout

`(sleep 3.2; yes B | head -20) | passquest assess start --size 20 --minutes 1 --json` with a
3,000 ms per-item clock:

```text
Window: 1 minute(s); per-item timeout: 3000 ms; annotations: none
Selected: 20; presented: 20; finalized: 20; scored: 20; contaminated: 0; pending: 0; unavailable: 0
Correct: 19/20 (95.0%)
Elapsed: median 10 ms; p90 31 ms; timed out 1; skipped 1; basis 20 scored item(s)
...
  #1 …-p001/p01-q1  timed_out  3000 ms [timed out]
```

The timed-out item is stored as a skipped attempt with `assistance='none'`, its run id and
`elapsed_ms = 3000`; it is in the scored total as not correct and is separately counted.

## Interruption and resume

The run process was killed with SIGTERM after five answers (`exit -15`). `assess resume 1` read the
remaining 15 answers and exited 0. The report:

```text
Status: completed
Selected: 20; presented: 20; finalized: 20; scored: 20; contaminated: 0; pending: 0; unavailable: 0
Resumes: 1; answered after a resume: 15
Correct: 20/20 (100.0%)
```

The resumed portion is marked per item (`after_resume: true` and `[after resume]` in human output).
The run was not dropped and no duplicate attempt was created.

## Contamination

A `show SOURCE --page 15` ran while the assessment held item 3 open. Page 15's unit was still
unseen by the run.

```text
Selected: 20; presented: 20; finalized: 20; scored: 19; contaminated: 1; pending: 0; unavailable: 0
Correct: 19/19 (100.0%)
...
Contamination (excluded from the headline)
  prior_exposure: 1
  repeated_variant: 0
  hints_during_run: 0
  resumed_after_window: 0
  #18 …-p015/p15-q1  correct  17 ms  reasons prior_exposure
```

The contaminated item is absent from the headline denominator (19, not 20) and remains listed with
its own status and elapsed time.

## Coaching refusal

With run 1 active:

```text
tutor history --json          exit 2  {"error":{"code":"assessment_active","message":"tutor is unavailable while assessment run 1 is active (deadline …); …"}}
skill show vocabulary-in-context --json  exit 2  {"error":{"code":"assessment_active", …}}
find --json                    exit 2  {"error":{"code":"assessment_active", …}}
```

After `assess abandon 1`, `find --json` exits 0 instead of `assessment_active`.

## Comparison

Runs 1 and 2 had 20 items and the same pool family but 60 and 30 minutes:

```text
assess compare 1 2 --json   exit 2
{"schema_version":1,"error":{"code":"incomparable_runs","message":"cannot compare runs 1 and 2: window_minutes differs (60 vs 30)"}}
assess compare 1 1 --json   exit 0
{"state":"compared", ...}
```

The comparison document carries the two run summaries and deltas; its limitations state that it
does not show that AI, coaching or the app caused a change.

## Store boundary

Schema v14 adds:

```sql
content_presentations(id, content_id, question_id, surface, at)  -- append-only
assessment_runs(id, …, status, resume_count, pool_family, …)     -- one active row
assessment_items(run_id, position, …, state, presented_at, item_deadline_at, answered_at, attempt_id)
assessment_events(id, run_id, kind, position, at, detail)        -- append-only
```

The attempts table was rebuilt with an explicit copy so its `assistance` CHECK admits `none` and
`assessment_run_id`; a CHECK pairs the two. `TestMigrationFromVersionThirteenKeepsAttemptsAndAddsAssessment`
opens a v13 fixture, preserves its attempt, and proves the append-only triggers still reject update
and delete. `TestAssessmentRunLifecycle` proves one active run, answer atomicity, duplicate refusal,
timeout state and completion. `TestAssessmentRunResumeAbandonAndReconcile` proves resume counting,
idempotent abandon and expired-window abandonment. `TestSpendsUnionIncludesOldAndNewEvidence` proves
the pool sees old attempts, guided exposures, presentations, note drafts and assessment items.

## Gates

`bash scripts/gate.sh` on the final tree, exit code **0**:

```text
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (88 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (17 packages, 702 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (148 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (409 files, 0 findings)

== local-only CLI
PASS  local-only CLI (16 first-party packages, 0 network imports, 0 confidential markers)

== baseline integrity
PASS  baseline integrity (165 pages (22 native, 143 OCR), 0 fixture references)

== import smoke
PASS  import smoke (35 assertions)

== tactic smoke
PASS  tactic smoke (70 assertions)

== adaptive smoke
PASS  adaptive smoke (120 assertions)

== tutor smoke
PASS  tutor smoke (156 assertions)

== agent smoke
PASS  agent smoke (120 assertions, 11 scenarios, 25 provider requests, 2 permission checks)

== note smoke
PASS  note smoke (806 assertions)

== find smoke
PASS  find smoke (113 assertions)

== assess smoke
PASS  assess smoke (259 assertions)

== portable paths
PASS  portable paths (88 Go files, no platform-bound path literal)

== harness self-test
PASS  harness self-test (31 cases, 19/19 gates)

20 passed, 0 failed, 0 skipped
GATE: PASS
GATE_EXIT=0
```
