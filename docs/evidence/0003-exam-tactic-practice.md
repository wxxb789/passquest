# 0003 — A scoring tactic applied on independent questions: evidence

- Tree: uncommitted round-2 finding fix over `da4bf3393a7263d99da03fc9cc3e8c8b049a9346` (goal-0003
  commits `c6c6c67`, `7bf87e0`, `8ff3423`, `da4bf33`), branch `feat/0003-exam-tactic-practice`; the
  operator commits.
- Date: 2026-11-10. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3 via mise; Bun 1.4.4; `modernc.org/sqlite` v1.62.0 (pure Go); no OCR engine was
  invoked and no PDF was read in this increment.
- Contract: [`../goals/archive/0003-exam-tactic-practice.md`](../goals/archive/0003-exam-tactic-practice.md)
  (moved unchanged at close). Plan:
  [`../plans/2026-11-10-1140-0003-exam-tactic-practice.md`](../plans/2026-11-10-1140-0003-exam-tactic-practice.md).

All commands below ran from the repository root unless a temporary demonstration directory is named.
The demonstration used `/tmp/passquest-0003-demo/`, a separately built binary, a fresh database for
the embedded exercise and another fresh database for the imported baseline. Nothing from the
demonstration was added to the repository. JSON shown below is a selected-field excerpt of the raw
document; the raw key/value protocol is pinned by the golden tests and `tactic-smoke`.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/tactic` | One registered tactic (`not-except-polarity`), its authored principle/why/when/steps, a structured boundary/counterexample, and an embedded first-party exercise with two tagged questions and one untagged decoy. Lesson and exercise are validated by construction. |
| `internal/exam` | Additive `tactics` tags on questions and the lowercase-slug grammar (`ValidSkillTag`). Evidence, option and key invariants are unchanged. |
| `internal/source` | Question-file `tactics`, registry validation at add/edit, tags carried into practice content and included in the content version when present. Review gates unchanged. |
| `internal/attempt` | `Assistance` (`assisted`/`independent`) and `TacticID` on the immutable attempt; ordinary and pre-v4 records default to independent; `WithTactic` validates the condition and tactic id, `WithAssistance` records help without a tactic attribution. |
| `internal/store` | Schema v5: v4's `attempts.assistance`, `attempts.tactic_id` and `source_questions.tactics_json`, plus the append-only `tactic_exposures` ledger; v0–v4 migration keeps attempts and defaults them to independent; an exposure records help shown, not an answer, and is queried by content-scoped question for the ordinary-practice path. |
| `internal/cli` | `tactic list`, `tactic show`, `tactic practice` with guided/independent gating, source or embedded material, and additive report counts/rows. A guided pre-answer document records the exposure before the walkthrough is emitted, so an unanswered question cannot become independent evidence. The shared answer path consults that ledger: ordinary practice on an exposed question is recorded as assisted with no tactic id. Key-hiding protocols unchanged. |
| `harness/` | New `tactic-smoke` gate (70 assertions, including the assisted/independent correctness counters, the answered-document guidance and the guided-exposure lockout), shared JSON readers and key-hiding check in `harness/lib/json.ts` (used by import-smoke and tactic-smoke), and a 19th self-test case over 12/12 gates. |

The guided response carries the walkthrough — in the pre-answer document when the client asks for it,
and again in the answered document — and is stored as `assisted`; the `independent` stage has no
guidance, must use a question distinct from every guided example for the tactic, and must be a first
attempt. Requesting the guided pre-answer document records the exposure first: an exposed question
cannot be the independent question even when no attempt followed, and the next guided request without
`--question` resumes it. A third independent run with no eligible question is refused. Ordinary
`practice` on an exposed question is stored as `assisted` with no tactic id, so the shared recording
path cannot turn prior help into independent correctness.

## Tier 1 — automated checks

| Command | Result |
|---|---|
| `go test ./... -count=1 -v` | exit 0; 115 top-level tests, 88 subtests, 0 failures, 0 skips |
| `bash scripts/gate.sh` | exit 0; 13 passed, 0 failed, 0 skipped (details below) |
| `go vet ./...` | exit 0 |
| `go mod tidy -diff` | empty diff, exit 0 |
| `gofmt -l` (tracked `*.go`) | empty output (the gate checks 25 files) |
| `./node_modules/.bin/tsc --noEmit` | exit 0, 0 type errors |

```text
$ bash scripts/gate.sh
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (25 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (7 packages, 203 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (66 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (283 tracked files, 0 findings)

== local-only CLI
PASS  local-only CLI (7 first-party packages, 0 network imports, 0 confidential markers)

== baseline integrity
PASS  baseline integrity (165 pages (22 native, 143 OCR), 0 fixture references)

== import smoke
PASS  import smoke (35 assertions)

== tactic smoke
PASS  tactic smoke (70 assertions)

== harness self-test
PASS  harness self-test (19 cases, 12/12 gates)

13 passed, 0 failed, 0 skipped
GATE: PASS
```

All files changed by this fix are tracked modifications; no index staging or intent-to-add was used.

Exit code: 0.

The tests that pin this increment's contract clauses:

| Test | Clause it pins |
|---|---|
| `TestRegistryIsValid`, `TestInvalidTacticIsRejected` | A tactic must carry an id, title, summary, principle, why, when, at least three steps and all three boundary fields; a broken lesson is rejected. |
| `TestExerciseCoversEveryRegisteredTactic`, `TestTaggedReturnsOnlyTaggedQuestions` | The first-party exercise has at least two tagged questions per registered tactic and an untagged decoy that selection cannot take. |
| `TestBoundaryAndExerciseExposeTheKeywordTrap` | The boundary names the "most repeated words"/"longest option" folklore and explains word overlap; the exercise contains at least two option explanations that name the keyword/repetition trap; the lesson example is not copied from the exercise. |
| `TestValidSkillTag`, `TestValidateRejects` | Skill-tag grammar; malformed and duplicate question tags are rejected by exam validation. |
| `TestParseQuestionJSONKeepsRegisteredTags`, `TestParseQuestionJSONRejectsBadInput` | Registered tags survive question parsing; unregistered, malformed and duplicate tags are exit-2 invalid input. |
| `TestBuildPracticeExamCarriesTacticTagsAndVersion` | Tags reach practice content; a tag change changes the content version. |
| `TestWithTactic`, `TestBuildRecord`, `TestExplain` | Assisted/independent conditions, tactic id validation, `WithAssistance` without a tactic id, default independence, and feedback assistance. |
| `TestAppendPersistsAssistanceAndTactic` | Attempts persist and read back the assistance/tactic split; an assisted attempt with no tactic id persists for ordinary practice; an unknown assistance value is rejected. |
| `TestMigrationFromVersionThreeAddsAssistanceColumns`, `TestMigrationFromVersionFourAddsGuidedExposures` | v3 attempts migrate as independent with no tactic; a v4 database gains the exposure ledger and keeps its assisted attempt; the database CHECK rejects an unknown assistance value. |
| `TestRecordExposureIsIdempotentAndAppendOnly` | Recording the same exposure twice keeps one row and its first time; the key lookup reports the exposed question and not another one; the database rejects UPDATE/DELETE on the ledger; malformed exposure fields are rejected. |
| `TestQuestionTagsRoundTrip`, `TestMigrationFromVersionThreeDefaultsQuestionTags` | Tags survive add/review/restart and appear in the audit event; a pre-v4 question reads as untagged. |
| `TestTacticLessonSurfacesWhyWhenAndBoundary` | `tactic list`/`show` expose the lesson and boundary in JSON and human form. |
| `TestTacticGuidedThenIndependentKeepsAssistanceSeparate` | Guided is assisted and both guided documents carry guidance; independent is unguided, its awaiting and answered documents carry none, and it is a distinct first attempt; report counts and per-question/try rows split assisted from independent; an exhausted independent request exits 2. |
| `TestTacticIndependentRequiresGuided`, `TestTacticSkipRecordsSkippedAttempt` | Independent before any guided attempt exits 2 with `guided_required` and records nothing; a guided skip satisfies the guided-required gate, records `skipped` with no selected label, and increments the assisted/independent skipped counter. |
| `TestTacticGuidedRerunUsesFirstExample` | A first one-shot guided JSON answer still carries the walkthrough; a third guided run returns the first example with attempt number 2, repeat exposure and assisted assistance. |
| `TestTacticExplicitQuestionRules` | Explicit guided selection; a guided question or an untagged question cannot be the independent question. |
| `TestTacticGuidedAwaitingBlocksIndependentUse`, `TestTacticHumanPromptRecordsExposureOnQuit` | A guided pre-answer document records exposure without creating an attempt; the exposed question is refused as independent with `invalid_input`; the pending guided question is resumed; an interactive quit after the walkthrough is still recorded. |
| `TestOrdinaryPracticeAfterGuidedExposureIsAssisted` | Ordinary practice on a question whose walkthrough was exposed is stored as `assisted` with no tactic id, remains attempt number 1, and is counted as assisted correctness with zero independent attempts. |
| `TestTacticHumanOutputShowsHelpAndCondition` | Human output states the guided/independent condition and the recorded attempt condition. |
| `TestTacticSourcePracticeKeepsProvenance` | Tagged, reviewed source questions are used; the content id, page/answer review provenance, private notice, and assisted/independent split survive into the report. |
| `TestTacticSourceRefusesUntaggedMaterial`, `TestTacticSourceRejectsPreviouslyAttemptedIndependentQuestion` | Untagged material is refused; an ordinary earlier attempt on a tagged question makes it ineligible for independent selection. |
| `TestTacticRejectsBadInput` | Unknown tactic, bad stage, conflicting flags, missing tactic and unknown source exit 2 and record nothing. |
| `TestTacticProtocolGoldens` | Raw keys/values of `tactic list`, `show`, guided awaiting and guided answered. |
| `TestGoldenReportJSON`, `TestCorrectIncorrectSkipAndRepeat`, `TestReportRepeatedIncorrectAndSkipped` | Additive report fields; first/repeated semantics are unchanged; ordinary attempts carry `assistance=independent`, but the independent counters count first attempts only, so a repeated attempt is not independent correctness. |

The test count is not the evidence that the product works; the Tier 2 transcripts are.

## Tier 2 — embedded first-party exercise

Binary: `/tmp/passquest-0003-demo/passquest`; database: `demo.sqlite`.

### Lesson and the "after help" gate

```text
$ passquest tactic list
Pass Quest — scoring tactics
  not-except-polarity  NOT/EXCEPT polarity
    Read the stem's negative word first, then match statements to the passage instead of matching words.

$ passquest tactic show not-except-polarity
Pass Quest — tactic not-except-polarity
Title: NOT/EXCEPT polarity
...
Boundary — Choose the option that repeats the most words from the passage, and prefer the longest option.
  Example: A NOT item says: 'The reading room is closed on Mondays. Which of the following is NOT true?' Option A says 'The reading room is closed on Mondays'; option B says 'The reading room is open on Mondays.' Option A repeats the passage exactly, but under a NOT stem it is the supported statement and therefore wrong. Option B reverses the fact and is the key. The longer option is not more likely to be right; it is often a wordy paraphrase that copies vocabulary.
  Why it fails: Word overlap measures vocabulary reuse, not truth. The stem decides which truth relation the key needs, and a small negation word changes that relation without changing the repeated nouns.

$ passquest tactic practice not-except-polarity --stage independent --json --db demo.sqlite
{
  "schema_version": 1,
  "error": {
    "code": "guided_required",
    "message": "complete a guided not-except-polarity attempt before the independent question; the guided walkthrough is the help this stage follows"
  }
}
exit=2
```

The lesson is 16 lines in human form. `tactic show --json` and `tactic list --json` are pinned by the
goldens rather than repeated here.

### Guided attempt (help shown)

```text
$ passquest tactic practice not-except-polarity --json --db demo.sqlite
state: awaiting_answer
tactic: {id: not-except-polarity, stage: guided, assistance: assisted}
guidance: principle, why_it_works, when_it_works, 6 steps, boundary {folklore, example, why_it_fails}
content: tactic-first-party-not-except
question: t1 — "Which of the following is NOT stated in the trip notice?"
```

A recursive key check over the awaiting document (`jq -r '[.. | objects | keys[]] | unique | .[]'`)
reported **0** matches for `answer`, `correct_label`, `correct_text`, `evidence`, `reasoning`,
`status` and `feedback` (grep exit 1).

```text
$ passquest tactic practice not-except-polarity --answer D --json --db demo.sqlite
state: answered
tactic: {id: not-except-polarity, stage: guided, assistance: assisted}
has_guidance: true
attempt: {id: 1, question_id: t1, status: correct, assistance: assisted, tactic_id: not-except-polarity, repeat: false}
feedback: {assistance: assisted, exposure: first_attempt}
```

The answered document carries the same guidance block as the pre-answer document, so the one-shot
form of the guided command still shows the walkthrough that makes the attempt assisted.

### Independent attempt (no hint, different question)

```text
$ passquest tactic practice not-except-polarity --json --db demo.sqlite
state: awaiting_answer
tactic: {id: not-except-polarity, stage: independent, assistance: independent}
has_guidance: false
content: tactic-first-party-not-except
question: t2 — "All of the following are true EXCEPT:"

$ passquest tactic practice not-except-polarity --answer C --json --db demo.sqlite
state: answered
tactic: {id: not-except-polarity, stage: independent, assistance: independent}
has_guidance: false
attempt: {id: 2, question_id: t2, status: correct, assistance: independent, tactic_id: not-except-polarity, repeat: false}
feedback: {assistance: independent}
```

The independent awaiting document also had **0** forbidden-key matches. The question id differs from
the guided example (`t2` vs `t1`), and `guidance` is absent from both independent documents rather
than empty.

### Guided exposure without an answer

A guided pre-answer document is help shown even when the learner never answers. The exposure is
recorded before the walkthrough is emitted, so the same question cannot later be submitted as
independent:

```text
$ passquest tactic practice not-except-polarity --json --answer D --db exposure.sqlite
state: answered
question: t1
attempt: {attempt_number: 1, assistance: assisted}

$ passquest tactic practice not-except-polarity --stage guided --question t2 --json --db exposure.sqlite
state: awaiting_answer
question: t2
has_guidance: true

$ passquest tactic practice not-except-polarity --stage independent --question t2 --json --db exposure.sqlite
{
  "schema_version": 1,
  "error": {
    "code": "invalid_input",
    "message": "question t2 was already shown with the guided walkthrough; choose a different question"
  }
}
exit=2

$ passquest report --json --db exposure.sqlite | jq -c '.counts | {attempts, assisted_attempts, independent_attempts}'
{"attempts":1,"assisted_attempts":1,"independent_attempts":0}

$ passquest tactic practice not-except-polarity --stage guided --json --db exposure.sqlite
state: awaiting_answer
question: t2
stage: guided
```

The awaiting call recorded an exposure and no attempt: the report still shows one assisted attempt
and zero independent attempts. The next guided request resumed the exposed `t2` rather than switching
questions, and the exposure is permanent, so a later independent run cannot select it.

### Human feedback condition

```text
$ passquest tactic practice not-except-polarity --answer D --db human.sqlite
...
First attempt on this question; no earlier answer or evidence was shown.
Assisted attempt: a tactic walkthrough was shown before this answer, so it is not independent evidence.
Recorded attempt #1 (question t1, attempt 1 on this question, 0 ms, assistance assisted, tactic not-except-polarity).

$ passquest tactic practice not-except-polarity --answer C --db human.sqlite
...
First attempt on this question; no earlier answer or evidence was shown.
Independent attempt: no hint was shown and this question had no earlier attempt.
Recorded attempt #2 (question t2, attempt 1 on this question, 0 ms, assistance independent, tactic not-except-polarity).
```

### Report and exhaustion

```text
$ passquest report --db demo.sqlite
Pass Quest — attempt evidence (tactic-first-party-not-except v1)
Tactic practice — NOT/EXCEPT polarity

Attempts: 2 (correct 2, incorrect 0, skipped 0)
First attempts: 2 (correct 2, incorrect 0, skipped 0)
Repeated attempts (answer already shown): 0 (correct 0, incorrect 0, skipped 0)
Independent attempts (no hint): 1 (correct 1, incorrect 0, skipped 0)
Assisted attempts (help shown): 1 (correct 1, incorrect 0, skipped 0)

Questions
  t1  attempts 1  first correct  latest correct  repeated 0  assisted 1  independent 0
  t2  attempts 1  first correct  latest correct  repeated 0  assisted 0  independent 1
  t3  attempts 0  first not attempted  latest —  repeated 0  assisted 0  independent 0

Attempts
  #1  t1  correct  first  assisted (not-except-polarity)  selected D  0 ms  session 197d0ced44feb868
  #2  t2  correct  first  independent (not-except-polarity)  selected C  0 ms  session 29cfeccb2aaa6536
```

`Independent attempts` counts first attempts only: a no-hint repeat carries `assistance=independent`
and is counted under `Repeated attempts`, not independent correctness.

`report --json` carried `assisted_attempts: 1`, `independent_attempts: 1`,
`assisted_correct: 1`, `independent_correct: 1`, per-question `assisted`/`independent`, and
per-attempt `assistance`/`tactic_id` for the same two rows.

```text
$ passquest tactic practice not-except-polarity --stage independent --json --db demo.sqlite
{
  "schema_version": 1,
  "error": {
    "code": "not_practiceable",
    "message": "no unattempted question tagged \"not-except-polarity\" remains distinct from the guided example (2 candidate(s), 1 shown with guidance); add and review another tagged question, or repeat the guided example with --stage guided"
  }
}
exit=2
```

The untagged decoy `t3` was never selectable: every tactic call chose only `t1`/`t2`.

## Tier 2 — source-grounded material

```text
$ passquest import --path data/baselines/2026-crsc-english --db source.sqlite
Imported source src-029b9408ceddd473 (165 pages, 0 reviewed, 0 questions).
Title: 2026-crsc-english
Origin: txt-directory
Database: /tmp/passquest-0003-demo/source.sqlite

$ passquest page review src-029b9408ceddd473 21 --reviewer demo-operator \
    --note "checked the page against the committed baseline text" --db source.sqlite
Page 21 reviewed by demo-operator.
Flags: reading_order_unverified
Database: /tmp/passquest-0003-demo/source.sqlite
```

Two local question files under `/tmp/passquest-0003-demo/` tagged `not-except-polarity` were added
and their answers reviewed. They are not committed. `q001` is a NOT-stated question on the boarding
instructions; `q002` is an EXCEPT question whose distractor repeats "American citizens" while reversing
the passage's exception. The page text is not reprinted in this record; the only quoted fragments are
the short evidence strings in the local question files, and those strings are already present in the
owner-authorised committed baseline.

```text
$ passquest questions src-029b9408ceddd473 --page 21 --db source.sqlite
Pass Quest — questions for src-029b9408ceddd473
  q001  page 021  answer D (reviewed)    review reviewed by demo-operator  flags —  Which of the following is NOT stated in the boarding instructions?  tactics not-except-polarity
  q002  page 021  answer C (reviewed)    review reviewed by demo-operator  flags —  All of the following are true EXCEPT:  tactics not-except-polarity

Database: /tmp/passquest-0003-demo/source.sqlite
```

### Guided and independent on imported material

```text
$ passquest tactic practice not-except-polarity --source src-029b9408ceddd473 --json --db source.sqlite
state: awaiting_answer
tactic: {id: not-except-polarity, stage: guided, assistance: assisted}
content: src-029b9408ceddd473-p021
content.title: 2026-crsc-english — page 21
content.provenance.rights: Private local source: do not redistribute, publish or upload without a recorded rights/consent decision.
question: q001
question.review: page_status reviewed / question_status reviewed, both by demo-operator
has_guidance: true
```

The pre-answer recursive key check again reported **0** forbidden-key matches.

```text
$ passquest tactic practice not-except-polarity --source src-029b9408ceddd473 --answer D --json --db source.sqlite
tactic: {stage: guided, assistance: assisted}
has_guidance: true
attempt: {question_id: q001, content_id: src-029b9408ceddd473-p021, content_version: 1-6edbc6d1,
          status: correct, assistance: assisted, tactic_id: not-except-polarity}

$ passquest tactic practice not-except-polarity --source src-029b9408ceddd473 --json --db source.sqlite
state: awaiting_answer
tactic: {stage: independent, assistance: independent}
question: q002
has_guidance: false
```

The second awaiting document had 0 forbidden-key matches. `q002` is distinct from the guided `q001`.

```text
$ passquest tactic practice not-except-polarity --source src-029b9408ceddd473 --answer C --json --db source.sqlite
tactic: {stage: independent, assistance: independent}
has_guidance: false
attempt: {question_id: q002, content_id: src-029b9408ceddd473-p021,
          status: correct, assistance: independent, tactic_id: not-except-polarity}

$ passquest report --db source.sqlite
Pass Quest — attempt evidence (src-029b9408ceddd473-p021 v1-6edbc6d1)
2026-crsc-english — page 21

Attempts: 2 (correct 2, incorrect 0, skipped 0)
First attempts: 2 (correct 2, incorrect 0, skipped 0)
Repeated attempts (answer already shown): 0 (correct 0, incorrect 0, skipped 0)
Independent attempts (no hint): 1 (correct 1, incorrect 0, skipped 0)
Assisted attempts (help shown): 1 (correct 1, incorrect 0, skipped 0)

Questions
  q001  attempts 1  first correct  latest correct  repeated 0  assisted 1  independent 0
  q002  attempts 1  first correct  latest correct  repeated 0  assisted 0  independent 1

Attempts
  #1  q001  correct  first  assisted (not-except-polarity)  selected D  0 ms  session 4c3589f791cae6ea
  #2  q002  correct  first  independent (not-except-polarity)  selected C  0 ms  session 6d6769c90a41b3b4
```

The page text was stored only in the local SQLite database. The source content notice names the page
reviewer/time, both answer reviews, and states that this is private local material, not a live exam,
not an exam blueprint and not an external answer key. The answer reviews were performed by the agent
under the label `demo-operator`; this is a **workflow demonstration**, not an owner verification of
page 21 or of the 165-page baseline.

The source DB was `0600`; the embedded DB was `0600`.

### Ordinary practice after an exposed walkthrough

The exposure ledger is also read by the shared answer path, so ordinary practice after help keeps the
assisted condition. This transcript uses the independently authored fixture under
`internal/cli/testdata/import` on a fresh database, not the private baseline. A temporary copy of
`question-q1.json` carried `"tactics": ["not-except-polarity"]`; it was not committed.

```text
$ passquest import --path internal/cli/testdata/import --json --db demo-source.sqlite
source: src-bb4d9a5479518365

$ passquest question add src-bb4d9a5479518365 1 --file /tmp/question-q1-tagged.json --db demo-source.sqlite
$ passquest page review src-bb4d9a5479518365 1 --reviewer demo-reviewer --db demo-source.sqlite
$ passquest question review src-bb4d9a5479518365 q1 --answer B --reviewer demo-reviewer --db demo-source.sqlite

$ passquest tactic practice not-except-polarity --source src-bb4d9a5479518365 \
    --stage guided --question q1 --json --db demo-source.sqlite
{
  "state": "awaiting_answer",
  "question": "q1",
  "tactic": {
    "id": "not-except-polarity",
    "title": "NOT/EXCEPT polarity",
    "stage": "guided",
    "assistance": "assisted"
  },
  "has_guidance": true
}

$ passquest practice --source src-bb4d9a5479518365 --question q1 --answer B --json --db demo-source.sqlite
{
  "attempt": {
    "attempt_number": 1,
    "status": "correct",
    "assistance": "assisted",
    "tactic_id": absent
  },
  "feedback_assistance": "assisted"
}

$ passquest report --json --db demo-source.sqlite
{
  "attempts": 1,
  "correct": 1,
  "assisted_attempts": 1,
  "assisted_correct": 1,
  "independent_attempts": 0,
  "independent_correct": 0
}
```

The ordinary attempt remains attempt number 1 and keeps no tactic id, but the earlier walkthrough makes
it assisted: it is counted as assisted correctness and contributes nothing to independent correctness.

## Tier 3 — local-only and source boundary

| Check | Command | Result |
|---|---|---|
| Runtime network syscalls | `strace -f -e trace=network -o network-exposure.log passquest tactic practice not-except-polarity --json --answer D --db network.sqlite` | exit 0; 9 log lines; **0** `socket`/`connect`/`bind`/`sendto`/`recvfrom`/`getaddrinfo` matches |
| Default state directory | run without `--db` / `PASSQUEST_DB` | `.passquest/` mode `0700`; database mode `0600` |
| Tracked private files | `git ls-files` filtered for `.pdf`, `.sqlite*`, `.db`, `data/private/`, `data/local/`, `local-data/`, `.passquest/` | no matches |
| Private PDF ignored | `git check-ignore -v data/private/source/2025-crsc-english-review.pdf` | `.gitignore:2:/data/private/` |
| First-party network imports | `bun run gate` local-only gate | 7 first-party packages, 0 network imports, 0 confidential markers |
| Baseline provenance | `bun run gate` baseline gate | 165 pages (22 native, 143 OCR), 0 fixture references |
| Secrets / private sources | `bun run gate` secrets gate | 283 tracked files, 0 findings |
| No new source artifacts | `git status --short` and `git ls-files` | no PDF, new OCR, SQLite database or private learner data; the only new source-like testdata is the first-party exercise and the authored import/tactic fixtures |
| Quotation boundary | this record | no page text reprint; only short evidence strings that already exist in the committed baseline and authored question stems/tags are shown |

`net` remains linked transitively by the pure-Go SQLite chain; no Pass Quest package imports or calls
it. The strace run is the runtime proof for this increment.

## Contract clause → evidence

| "Done when" clause | Evidence |
|---|---|
| A learner can practice a concrete evidence-based exam tactic on source-grounded material and understand why/when it works. | `tactic show` returns principle, why, when, 6 steps and a boundary; guided practice on both the first-party exercise and imported page 21; `q001`/`q002` answer reviews and source provenance; the pre-answer document hides the key. |
| After help, the learner can try a distinct answer-verified question without hints; assisted success and independent correctness stay separately visible. | `--stage independent` before help is exit 2 `guided_required`; the independent awaiting document has no `guidance` and question `t2`/`q002` differs from guided `t1`/`q001`; attempts store `assisted` vs `independent`; report prints and JSON returns separate counts and per-question/try rows (1 assisted correct, 1 independent correct in both transcripts); a third independent request is refused. A question whose guided walkthrough was requested but never answered is recorded in `tactic_exposures` and refused as independent (`invalid_input`, zero attempts added), and the next guided request resumes it. Ordinary `practice` on an exposed question is stored as `assisted` with no tactic id (attempt 1) and adds zero independent correctness (Tier 2, "Ordinary practice after an exposed walkthrough"; `TestOrdinaryPracticeAfterGuidedExposureIsAssisted`). |
| The material includes a counterexample or boundary showing why naive folklore or blind keyword matching can fail. | The lesson's boundary names "repeat the most words"/"longest option" and explains word overlap versus truth; exercise `t1` option D and `t2` option C explain the repeated-vocabulary trap; source `q002` reverses the American-citizens exception while repeating its words; `TestBoundaryAndExerciseExposeTheKeywordTrap` pins the lesson and at least two trap explanations. |
| Question provenance, local data confidentiality and deterministic scoring continue to hold. | Tags and reviewed answers are stored in the private DB; source content id/version, page/answer reviewers and private notice appear in practice and report; pre-answer JSON has 0 forbidden keys; the existing append-only attempt schema remains; migration keeps attempts; strace shows 0 network syscalls; DB `0600` in a `0700` default directory; no private source or database is tracked; same deterministic grading/feedback path as goal 0001/0002. |

## Harness growth

Goal 0003 adds a new CLI protocol and a new evidence dimension, so it adds
`harness/gates/tactic-smoke.ts` (70 assertions, including the assisted/independent correctness
counters, the answered-document guidance and the guided-exposure lockout) and a self-test input that
makes it fail when the CLI prints `{}` instead of the tactic protocol. The tick comes from the real
CLI, the real database and the real report; it does not re-implement scoring.

Shared JSON readers moved to `harness/lib/json.ts` and are used by both `import-smoke` and
`tactic-smoke`; `import-smoke` keeps its 35 assertions, and the self-test now reports 19 cases over
12/12 gates. `harness.md` lists the new gate and the updated counts. `bash scripts/gate.sh` remains
the only documented shell entry point and still execs `bun run gate`.

## Known limits

- One tactic is registered. The schema and CLI accept more, but each needs its own authored lesson,
  boundary and exercise; this increment does not claim a tactics catalogue.
- Tactic tags are a closed registry; adding a tactic is a code change. A local question cannot invent
  a new tag.
- "Independent" here means no tactic walkthrough was shown on that question (no exposure row exists),
  a different question from the guided example, and zero earlier attempts on that content-scoped
  question. Ordinary practice after an exposure is assisted, not independent. It is not a delayed,
  timed or held-out assessment; those belong to goal 0009.
- The guided attempt is still an attempt and counts as repeat exposure for that question. Re-running
  the guided example is allowed and remains visibly assisted/repeated. A separate append-only
  `tactic_exposures` row records that the walkthrough was shown before an answer; it is not an
  attempt, does not change first/repeat counts, and is not printed by `report`.
- The report's overall `attempts`/`correct` counts include assisted attempts. The separate
  `assisted_*`/`independent_*` counts distinguish help from independent correctness; `independent_*`
  counts first attempts only, so a no-hint repeat appears under `repeated_*` and is not independent
  evidence.
- The source demonstration reviewed page 21 with the agent label `demo-operator`. The 165-page
  baseline remains unreviewed as a whole; no claim is made about page 21's text beyond the two local
  questions' reviewed evidence.
- The demonstration's local question files (`/tmp/passquest-0003-demo/q001.json`, `q002.json`) are not
  committed. They are reconstructible from the committed baseline and the stems/tags shown above; the
  private source is not published.
- No OCR, PDF extraction, automatic tactic tagging or item generation is performed.
- Windows has no POSIX mode; the import-smoke database assertion is reported as skipped there. The
  tactic smoke is platform-neutral apart from the binary name.
