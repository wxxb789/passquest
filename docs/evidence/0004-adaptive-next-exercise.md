# 0004 — Choose practice based on learner evidence: evidence

- Tree: working tree on top of `2d0cb4a39fe7a1ab8480febb2125adb1ba92f89f` (the reviewed goal-0004
  head), branch `feat/0004-adaptive-next-exercise`; the fixes below are uncommitted and the operator
  owns the commit. The final gate ran on this tree, not on a staged snapshot.
- Date: 2026-11-10. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3 via mise; Bun 1.4.4; `modernc.org/sqlite` v1.62.0 (pure Go). No OCR engine was
  invoked and no PDF was read in this increment.
- Contract: [`../goals/archive/0004-adaptive-next-exercise.md`](../goals/archive/0004-adaptive-next-exercise.md).
  Plan: [`../plans/2026-11-10-1313-0004-adaptive-next-exercise.md`](../plans/2026-11-10-1313-0004-adaptive-next-exercise.md).

All commands below ran from the repository root unless a temporary demonstration directory is named.
The demonstration used `/tmp/passquest-0004-demo/` and a separately built binary. Nothing from the
demonstration was added to the repository. JSON shown below is a selected-field excerpt of the raw
document; the raw protocol is pinned by the golden tests and `adaptive-smoke`.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/skill` | The small registry (`not-except-polarity` scoring, `vocabulary-in-context` and `sentence-structure` language), authored lessons, and six first-party exercise passages. Validation requires at least two distinct passages per skill; vocabulary has three so a supported state can still have a fresh passage. |
| `internal/adaptive` | Pure evidence classifier and policy. It classifies independent transfer, passage re-exposure, assisted attempts and revisits; infers `no_evidence`/`familiarity_only`/`insufficient`/`weak`/`mixed`/`supported`; selects `explain`, `practice_new` or `review`; states uncertainty. It writes nothing. |
| `internal/attempt` | `Skills` snapshot on the graded record; `WithTactic` adds the tactic id to the snapshot without duplicating it. Existing assistance/tactic/repeat fields are unchanged. |
| `internal/store` | Schema v6 adds a nullable `attempts.skills_json`; `NULL` marks a pre-v6 row, while a recorded array (including `[]`) is authoritative. Migration keeps rows; append-only triggers unchanged; invalid or duplicate skill slugs are rejected. |
| `internal/source` | Question tags validate against the skill registry (the `tactics` field keeps its name). Review gates unchanged. |
| `internal/cli` | `skill list/show/practice`, `next`, passage-exposure labels, key-hiding documents, and additive `skills` on attempt JSON. |
| `harness/` | New `adaptive-smoke` gate (80 assertions) and a 20th self-test case over 13/13 gates. |

The policy distinguishes recorded evidence from inference: the JSON has separate `evidence` and
`inference` objects, and the human output has `Evidence (recorded attempts)` and `Inference
(recomputed from evidence)` sections. A recommendation carries the exact follow-up command.

## Tier 1 — automated checks

| Command | Result |
|---|---|
| `go test ./... -count=1` | exit 0; 9 packages; 283 test runs, 283 passes, 0 skips, 0 failures (`go test -json` count) |
| `go vet ./...` | exit 0 |
| `go mod tidy -diff` | empty diff, exit 0 |
| `gofmt -l` (tracked `*.go`) | empty output, exit 0; the gate checks 33 files |
| `./node_modules/.bin/tsc --noEmit` | exit 0, 0 type errors |
| `bash scripts/gate.sh` | exit 0; 14 passed, 0 failed, 0 skipped (final output below) |

The final gate output, run on the working tree at `2d0cb4a` plus the review fixes (nothing staged or
committed):

```text
$ bash scripts/gate.sh
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (33 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (9 packages, 283 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (76 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (308 tracked files, 0 findings)

== local-only CLI
PASS  local-only CLI (9 first-party packages, 0 network imports, 0 confidential markers)

== baseline integrity
PASS  baseline integrity (165 pages (22 native, 143 OCR), 0 fixture references)

== import smoke
PASS  import smoke (35 assertions)

== tactic smoke
PASS  tactic smoke (70 assertions)

== adaptive smoke
PASS  adaptive smoke (80 assertions)

== harness self-test
PASS  harness self-test (20 cases, 13/13 gates)

14 passed, 0 failed, 0 skipped
GATE: PASS
```

Exit code: 0.

The tests that pin this increment's contract clauses:

| Test | Clause it pins |
|---|---|
| `TestRegistryIsValid`, `TestInvalidSkillIsRejected` | The three skills carry a kind, principle, why/when, watch-out and at least three steps; the scoring skill reuses the tactic lesson and boundary. |
| `TestValidateContent`, `TestTaggedReturnsOnlyTaggedQuestions` | Every first-party tag is registered; ids are unique; each skill has at least two distinct passages; an untagged question is never selected. |
| `TestScoringExerciseCarriesTheKeywordTrap` | The second scoring passage contains a real keyword-overlap trap whose key explanation says repetition is not truth. |
| `TestBuildRecordSnapshotsSkillTags`, `TestWithTactic` | A graded record snapshots the question's tags; a tactic attempt adds its tactic id exactly once; the caller's slice is not aliased. |
| `TestAppendPersistsSkillSnapshot`, `TestAppendRejectsBadSkillSnapshot`, `TestMigrationFromVersionFiveAddsSkillSnapshot` | The snapshot round-trips; malformed/duplicate slugs are rejected; a v5 database keeps attempts with no recorded snapshot, while a new untagged attempt has a recorded empty one. |
| `TestParseQuestionJSONKeepsRegisteredTags`, `TestQuestionsHumanOutputLabelsTagsAsSkills` | A language skill tag is accepted by the question-file parser; an unregistered tag is still rejected; the human question list labels tag values `skills`. |
| `TestNoEvidence`, `TestAssistedOnlyIsNotIndependent`, `TestAssistedPracticeWithoutGuidedTacticStillExplains`, `TestOneIndependentObservationIsInsufficient` | Empty, guided-assisted-only and ordinary-assisted-only states select the documented action and never call help independent; scoring re-teaches until a guided tactic attempt exists. |
| `TestRepeatedIncorrectSelectsExplanation`, `TestMixedEvidenceUsesLatestIndependentAttempt`, `TestSkipAfterAnswersKeepsAnsweredEvidence` | Repeated or mixed errors select an explanation; the reason says the cause is unresolved; the latest answered independent attempt decides re-teach vs confirm; a later skip does not erase earlier answered evidence or make the reason claim the skip was an answer. |
| `TestSupportedEvidenceUsesTime` | Two correct observations select `practice_new` when fast, `review` when the latest is at least twice the fastest, and `practice_new` when the latest is faster. |
| `TestPassageExposureIsNotIndependentTransfer`, `TestAssistedFirstAttemptExposesThePassage` | A second question on a seen passage is a passage re-exposure; an assisted first attempt exposes the passage for later attempts. |
| `TestRevisitsAreNotIndependentTransfer`, `TestSkipsRecordNoAnswerEvidence` | A repeat is a revisit, not independent correctness; a skip records no answer evidence. |
| `TestNoFreshPassageSelectsReview`, `TestPriorityOrder`, `TestTiesFollowRegistryOrder` | No fresh passage selects review; weak/familiarity/insufficient evidence outranks no-evidence material; ties follow registry order. |
| `TestCurrentTagFallbackIsDisclosed`, `TestMultiSkillRecordCountsForEachAndExposesContent`, `TestNoMaterialSkill` | Pre-v6 tags disclose the fallback; one attempt counts for each tagged skill but exposes the passage once; no material produces no recommendation. |
| `TestSkillPracticeHidesKeyAndSnapshotsSkill`, `TestSkillPracticeSecondPassageAndRepeat`, `TestSkillPracticeSourcePath` | Skill awaiting JSON hides the key; the attempt snapshots the skill; fresh/seen passages are labelled; source review provenance survives. |
| `TestSkillPracticeScoringRequiresGuidedAttempt`, `TestNextSendsUnGuidedScoringToTacticWalkthrough`, `TestSkillPracticeSkipsGuidedExposure`, `TestSkillPracticeAssistedFreshPreambleIsTruthful`, `TestNextRecordedEmptySnapshotDoesNotRegroupAfterTagEdit` | Scoring skill practice refuses without a guided attempt and records no attempt; `next` sends unguided scoring evidence to the tactic walkthrough and then offers a runnable scoring practice; an already-shown walkthrough is skipped when a fresh alternative exists; assisted-fresh output never claims independent transfer; a recorded empty snapshot survives a later tag edit without being regrouped. |
| `TestNextNoEvidence`, `TestNextPassageExposureAndHints`, `TestNextTimeChangesAction`, `TestNextRevisitsAndSkips` | `next` separates evidence from inference; hints and same-passage exposure are not independent; time changes the action; revisits and skips are retained. |
| `TestNextUnclassifiedAttempt`, `TestNextReopenRecalculatesWithoutRewriting` | Untagged attempts are unclassified; reopening recomputes the same JSON and does not rewrite attempt rows. |
| `TestNextRecordClassification`, `TestNextIncludesImportedSourceWithoutScope` | A known untagged question, unavailable content and an unregistered snapshot are classified apart with a reason; an attempt on an imported source is included by an unscoped `next` without being reported unavailable. |
| `TestGoldenSkill*`, `TestGoldenNext*` | Raw keys/values of skill list/show/awaiting/answered and `next` empty/after-one/slow. |
| `TestTacticProtocolGoldens` | The goal-0003 answers gain only the additive `skills` snapshot. |

The test count is not the evidence that the product works; the Tier 2 transcripts are.

## Tier 2 — real CLI transcripts

Binary: `/tmp/passquest-0004-demo/passquest`; all databases are fresh files under that directory.

### Empty evidence

```text
$ passquest next --json --db empty.sqlite
state: recommendation
recommendation: explain — not-except-polarity (scoring)
state: no_evidence
reason: "No attempt is recorded for not-except-polarity. The scoring tactic starts with a guided walkthrough, ..."
command: passquest tactic practice not-except-polarity --stage guided
follow_up_command: passquest skill practice not-except-polarity
skills:
  not-except-polarity    no_evidence      explain       fresh passages 2
  vocabulary-in-context  no_evidence      practice_new  fresh passages 3
  sentence-structure     no_evidence      practice_new  fresh passages 2
unclassified attempts: 0
```

The scoring skill starts with the explanation because goal 0003 requires a guided walkthrough before
its independent stage. Language skills start with a fresh measurement.

### Same passage, hints and a new question

```text
$ passquest tactic practice not-except-polarity --answer D --db tactic.sqlite
$ passquest tactic practice not-except-polarity --answer C --db tactic.sqlite
$ passquest next --json --db tactic.sqlite
recommendation: practice_new — not-except-polarity
state: familiarity_only
reason: "2 attempt(s) are recorded for not-except-polarity but none is independent transfer
         (assisted 1, revisits 0, passage re-exposures 1). A fresh passage is selected; help and
         repeat exposure cannot show transfer."
command: passquest skill practice not-except-polarity
evidence:
  attempts 2
  assisted_attempts 1, assisted_correct 1
  independent_attempts 0, independent_correct 0
  revisits 0, passage_repeats 1
  distinct_passages 1, questions_seen 2
  fresh_passages 1
uncertainty:
  - "1 independent first attempt(s) on an already-seen passage are familiarity, not independent transfer."
  - "1 assisted attempt(s) cannot count as independent correctness."
```

This is the key boundary: both goal-0003 attempts are recorded (one assisted, one independent), but
the adaptive view does not call the second question independent transfer because it shares the
passage. The report still shows the stored assistance values; the policy is stricter, not a rewrite.

### Time changes a supported-skill action

Two correct vocabulary attempts on fresh passages with real interactive delays:

```text
$ { sleep 0.15; printf 'A\n'; } | passquest skill practice vocabulary-in-context --db time.sqlite
$ { sleep 0.9;  printf 'B\n'; } | passquest skill practice vocabulary-in-context --db time.sqlite
$ passquest next --json --db time.sqlite
recommendation: review — vocabulary-in-context
state: supported
reason: "All 2 answered independent transfer(s) for vocabulary-in-context are correct, but the latest
         correct independent answer took 897 ms against your fastest 134 ms (at least twice as slow).
         A seen question is selected for review; a repeat is not independent transfer."
command: passquest skill practice vocabulary-in-context --question v2
evidence:
  independent_attempts 2, independent_correct 2
  fastest_independent_ms 134
  latest_independent_ms 897
  fresh_passages 1
uncertainty:
  - "Elapsed time is local prompt-to-answer time, not an exam time limit."
```

With the same two correct attempts and no slow flag, the action is `practice_new` (pinned by
`TestSupportedEvidenceUsesTime` and the `adaptive-smoke` two-correct stage). The time rule only
compares the learner with their own fastest correct answer for the same skill.

### Revisits are retained, not transfer

```text
$ passquest skill practice vocabulary-in-context --answer A --db revisit.sqlite
$ passquest skill practice vocabulary-in-context --answer B --db revisit.sqlite
$ passquest skill practice vocabulary-in-context --answer A --db revisit.sqlite
$ passquest skill practice vocabulary-in-context --question v1 --answer A --db revisit.sqlite
attempt: {id: 4, question_id: v1, attempt_number: 2, repeat: true, assistance: independent,
          skills: [vocabulary-in-context]}
$ passquest next --json --db revisit.sqlite
recommendation: review — vocabulary-in-context
state: supported
evidence: attempts 4, independent_attempts 3, independent_correct 3, revisits 1,
          distinct_passages 3, questions_seen 3, fresh_passages 0
reason: "All 3 answered independent transfer(s) ... are correct, but no fresh passage remains. A seen
         question is selected for review; a repeat is not independent transfer."
```

The report still has 4 rows; the fourth is a repeat and does not become a fourth independent
observation.

### Repeated errors select an explanation, not a cause claim

```text
$ passquest skill practice sentence-structure --answer A --db weak.sqlite
$ passquest skill practice sentence-structure --answer A --db weak.sqlite
$ passquest next --json --db weak.sqlite
recommendation: explain — sentence-structure
state: weak
reason: "All 2 answered independent transfer(s) for sentence-structure are incorrect. The explanation
         is selected because the gap repeated; the wrong options do not identify whether the cause is
         the skill, vocabulary, time or a misread stem."
causal_diagnosis: unresolved
command: passquest skill show sentence-structure
follow_up_command: passquest skill practice sentence-structure --question s2
```

The second wrong answer repeated the gap; it did not prove a misconception. The uncertainty list says
a wrong option does not identify the cause.

### Source-grounded evidence

The committed first-party import fixture was imported, its page 1 and tagged question `q2` were
reviewed under the label `demo-operator`, and the question was answered through the source path:

```text
$ passquest import --path internal/cli/testdata/import --json --db source.sqlite
source: src-bb4d9a5479518365
$ passquest question add src-bb4d9a5479518365 1 --file .../question-q2-tagged.json --db source.sqlite
$ passquest page review src-bb4d9a5479518365 1 --reviewer demo-operator --db source.sqlite
$ passquest question review src-bb4d9a5479518365 q2 --answer C --reviewer demo-operator --db source.sqlite
$ passquest tactic practice not-except-polarity --answer D --json --db source.sqlite
attempt: {content_id: tactic-first-party-not-except, question_id: t1, status: correct,
          assistance: assisted, tactic_id: not-except-polarity, skills: [not-except-polarity]}
$ passquest skill practice not-except-polarity --source src-bb4d9a5479518365 --answer C --json --db source.sqlite
attempt: {content_id: src-bb4d9a5479518365-p001, question_id: q2, status: correct,
          assistance: independent, skills: [not-except-polarity]}
skill: {id: not-except-polarity, kind: scoring, passage_exposure: fresh}
review: page reviewed by demo-operator; question reviewed by demo-operator
$ passquest next --json --source src-bb4d9a5479518365 --db source.sqlite
source_scope: src-bb4d9a5479518365
source_tagged_questions: 1
recommendation: practice_new — not-except-polarity
state: insufficient
reason: "Only 1 answered independent transfer observation(s) exist ... A single observation cannot
         distinguish a skill pattern from luck; a fresh passage is selected."
scoring evidence: attempts 2, assisted_attempts 1, assisted_correct 1, guided_attempts 1,
                  independent_attempts 1, independent_correct 1,
                  distinct_passages 2, fresh_passages 1
```

The guided attempt is required before scoring `skill practice` (Q-0004-05); it is the goal-0003
walkthrough, not an extra source claim. The source question is still answered independently.

The fixture is first-party and redistributable; the private CRSC baseline was not imported or read.
The review is a workflow demonstration by the agent, not an owner verification.

### Untagged attempt is visible, not counted

```text
$ passquest practice --question q1 --answer B --json --db unclassified.sqlite
$ passquest next --json --db unclassified.sqlite
unclassified_attempts:
  [{id: 1, content_id: sample-0001, question_id: q1, reason: "question has no registered skill tag"}]
scoring evidence: attempts 0
```

The untagged goal-0001 attempt remains in the database and is shown; it does not silently update a
skill.

### Key hiding and reopen stability

```text
$ passquest skill practice vocabulary-in-context --json --db keyhide.sqlite > keyhide.json
$ python3 -c 'recursively collect keys; check answer, correct_label, correct_text, evidence,
              reasoning, status, feedback'
forbidden_matches 0 []
```

The recursive check reports zero forbidden-key matches. The awaiting document has no answer, evidence,
reasoning, status or feedback.

```text
$ passquest next --json --db revisit.sqlite > revisit-a.json
$ passquest next --json --db revisit.sqlite > revisit-b.json
$ cmp revisit-a.json revisit-b.json; echo $?
0
$ passquest report --json --db revisit.sqlite | jq '.counts.attempts, [.attempts[].id]'
4 [1,2,3,4]
```

Two separate processes produced byte-identical `next` documents; the attempt rows and ids did not
change.

## Tier 3 — local-only and source boundary

| Check | Command | Result |
|---|---|---|
| Runtime network syscalls | `strace -f -e trace=network -o network-next.log passquest next --json --db network.sqlite` | exit 0; 6 log lines; **0** `socket`/`connect`/`bind`/`sendto`/`recvfrom`/`getaddrinfo` matches |
| Runtime network syscalls | `strace -f -e trace=network -o network-skill.log passquest skill practice vocabulary-in-context --json --answer A --db network.sqlite` | exit 0; 7 log lines; **0** matches |
| Default state directory | run without `--db` / `PASSQUEST_DB` | `.passquest/` mode `0700`; database mode `0600` |
| Tracked private files | `git ls-files` filtered for `.pdf`, `.sqlite*`, `.db`, `data/private/`, `data/local/`, `local-data/`, `.passquest/` | no matches |
| Private PDF ignored | `git check-ignore -v data/private/source/2025-crsc-english-review.pdf` | `.gitignore:2:/data/private/` |
| First-party network imports | `bun run gate` local-only gate | 9 first-party packages, 0 network imports, 0 confidential markers |
| Baseline provenance | `bun run gate` baseline gate | 165 pages (22 native, 143 OCR), 0 fixture references |
| Secrets / private sources | `bun run gate` secrets gate | 308 tracked files, 0 findings |
| New-file markers | local-only gate on the working tree | 9 first-party packages, 0 confidential markers |

`net` remains linked transitively by the pure-Go SQLite chain; no Pass Quest package imports or calls
it. The strace runs are the runtime proof. The final gate ran on the working tree; the tracked-file
gates scanned the same paths because the fixes changed tracked files only. Nothing was staged,
committed or pushed. The evidence and report state that the operator owns the commit.

## Contract clause → evidence

| "Done when" clause | Evidence |
|---|---|
| Independently recorded attempts, time, hints and revisits change the recommendation, and the reason is inspectable. | Assisted-only tactic evidence selects `practice_new` and says 1 assisted, 0 independent; two correct fresh vocabulary attempts select `supported`; 134 ms vs 897 ms changes the action to `review` and the reason quotes both values; a revisit adds `revisits=1` but independent correctness stays 3; two wrong sentence attempts select `explain`. `next --json` exposes `evidence`, `inference`, `reason`, `uncertainty` and the exact command. |
| Same question or passage exposure does not masquerade as independent transfer; repeat exposure and uncertain causal diagnosis are retained. | Goal-0003 `t1`/`t2` on one passage: 1 assisted, 0 independent, 1 passage re-exposure, `familiarity_only`; the report still has both stored attempts. `revisits` and `passage_repeats` are separate fields. A repeated wrong answer yields `causal_diagnosis: unresolved` and the reason says the wrong option does not identify the cause. |
| The user can see what evidence supports a gap, what is uncertain, and why an explanation, new question or review was selected. | Human `next` prints `Evidence (recorded attempts)` and `Inference (recomputed from evidence)`; JSON has per-skill `evidence` counts/attempt ids and `inference.state/action/reason/causal_diagnosis/uncertainty`; the selected recommendation repeats its reason and follow-up command. |
| Reopening keeps historical evidence intact and recalculates recommendations without rewriting attempts. | Schema v6 snapshot migration test (a v5 row has no snapshot; a new untagged row has a recorded empty one); a recorded empty snapshot is not regrouped by a later tag edit; `next` is read-only; two separate `next` processes produced byte-identical JSON (`cmp` exit 0); `report` after the reopen still shows 4 attempts with ids 1–4; `mapped_from_current_tags` records the pre-v6 fallback. |
| Do not claim measured pass probability or introduce a broad ontology. | Three skills only; every recommendation has a limitation line "not an exam score, pass estimate or readiness claim"; exact-count states; no pass threshold or exam time limit is invented. |

## Harness growth

Goal 0004 adds a new CLI protocol and an evidence dimension, so it adds
`harness/gates/adaptive-smoke.ts` (80 assertions) and a self-test input that makes it fail when the CLI
prints `{}` instead of the skill/next protocol. The tick comes from the real CLI, the real database and
the real report; the gate does not re-implement scoring or the policy. The self-test now reports 20
cases over 13/13 gates. `harness.md` lists the new gate and the updated counts.
`bash scripts/gate.sh` remains the only documented shell entry point and still execs `bun run gate`.

## Known limits

- Three skills are registered. The registry accepts more, but each addition needs its own authored
  lesson, passages and tested policy branch.
- State thresholds are exact counts on a small sample, not a psychometric model. Two or fewer answered
  independent observations are always labelled insufficient or just-supported; no confidence
  interval is computed.
- Passage-level independence is stricter than goal 0003's stored attempt condition. A new question on
  a seen passage is recorded as an independent attempt but classified as a passage re-exposure by
  `next`. Goal 0003's report fields are unchanged.
- Pre-v6 attempts have no skill snapshot (`NULL`); they are mapped from current question tags and
  marked `mapped_from_current_tags`. A later tag edit can regroup only those rows. A recorded empty
  snapshot means "no registered tag at answer time" and is never regrouped; it is listed as
  unclassified.
- Scoring `skill practice` is refused until a guided tactic attempt exists, so the plan's "ordinary
  unassisted practice" is narrower for the scoring skill than for the language skills (Q-0004-05).
- A database created by the reviewed intermediate v6 build (`skills_json NOT NULL DEFAULT '[]'`)
  cannot distinguish its migrated pre-v6 rows after the column becomes nullable. No released v6
  database existed because goal 0004 was uncommitted; a fresh v5→v6 migration is correct.
- The time rule is relative to the learner's own fastest correct answer for the same skill. The demo
  used interactive sleeps (134 ms and 897 ms); it is not an exam clock and says so.
- `next` loads embedded material plus every imported source by default, which reads all source pages
  to build the catalog. It is a local read-only pass; no page text is printed.
- The source demonstration reviewed a first-party fixture page under the agent label `demo-operator`.
  The private CRSC baseline was not read or imported; no claim is made about its pages.
- Windows has no POSIX mode, so the database-mode assertion remains skipped there. The adaptive smoke
  is platform-neutral apart from the binary name.
- The fixes are uncommitted on top of `2d0cb4a` for operator verification. The final gate ran on
  that working tree; a commit would let CI reproduce the same tracked-file scan from a clean
  checkout.
