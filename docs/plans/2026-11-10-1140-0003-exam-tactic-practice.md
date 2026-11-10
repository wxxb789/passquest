# 0003 — A scoring tactic applied on independent questions: increment plan

Contract: [`../goals/archive/0003-exam-tactic-practice.md`](../goals/archive/0003-exam-tactic-practice.md)
Shape reference: goal 0002 (`docs/goals/archive/0002-local-text-baseline.md`, its plan and evidence
record). Conventions: [`../conventions.md`](../conventions.md). Harness:
[`../harness.md`](../harness.md).

## Goal in one sentence

A learner studies one concrete exam-scoring tactic (NOT/EXCEPT polarity), sees a worked counterexample
that breaks blind keyword matching and the "longest answer" folklore, answers a guided question with
the tactic walkthrough visible, then answers a **different**, unattempted, answer-verified question
with no hints; the report keeps assisted success and independent correctness in separate columns, and
the material is either first-party or locally imported and reviewed.

## Public shape

```text
passquest tactic list [--json]
passquest tactic show TACTIC [--json]
passquest tactic practice TACTIC [--source SOURCE] [--stage guided|independent]
                                [--question ID] [--answer LABEL | --skip]
                                [--db PATH] [--json]
```

- `tactic list` and `tactic show` are lesson surfaces. `show` prints the principle, why it works,
  when it applies, ordered steps, and a structured boundary/counterexample with the naive rule it
  defeats. The lesson is first-party authored text, not source material.
- `tactic practice` resolves material in one of two ways:
  - **First-party exercise** (no `--source`): an embedded, independently authored passage with three
    questions. Two questions carry the `not-except-polarity` tag; the third is untagged and must never
    be selected. It is redistributable and is what the automated checks use.
  - **Imported source** (`--source SRC`): only questions whose local author tagged them with the
    tactic, and that pass goal 0002's review gate (reviewed page, reviewed answer, verbatim evidence,
    every option explained). Nothing is extracted automatically from the private source.
- The stage is explicit or derived:
  - `--stage guided` delivers the tactic walkthrough — in the pre-answer document when one is
    requested, and again in the answered document for a one-shot `--answer`/`--skip` — and records
    the attempt as `assisted`.
  - `--stage independent` shows no guidance, refuses a question that has any earlier attempt, and
    refuses a question already used as a guided example for the same tactic. It also refuses until at
    least one guided attempt for the tactic exists (`guided_required`).
  - Without `--stage`, the first tactic attempt is guided; after any guided attempt the default is
    independent.
- The independent question is selected only from tagged, practiceable candidates that are distinct
  from every guided question for that tactic and have zero prior attempts. If none remains, the
  command exits 2 with `not_practiceable` and says what to add or review.
- Exit codes match the existing CLI: `0` success, `2` usage/invalid/unreviewed/not-practiceable,
  `1` storage or internal failure. JSON errors keep the `schema_version: 1` error document.
- `practice`, `report`, import and review commands keep their goal-0001/0002 protocols. `report` gains
  additive counts and per-attempt fields; no existing field changes meaning.

Example:

```bash
passquest tactic list
passquest tactic show not-except-polarity
passquest tactic practice not-except-polarity --stage guided
passquest tactic practice not-except-polarity --stage independent
passquest report
```

## Semantics the contract tests

| State | Stored / shown as | Visible effect |
|---|---|---|
| Guided tactic attempt | `assistance=assisted`, `tactic_id=<id>` | Counted under assisted attempts; never called independent evidence. |
| Independent tactic attempt | `assistance=independent`, `tactic_id=<id>` | Counted under independent attempts; only allowed without guidance and on a first attempt. |
| Ordinary practice attempt | `assistance=independent`, empty `tactic_id` | Unchanged meaning; older rows migrate to this state. |
| Assisted success | `assisted_correct` count and per-question `assisted` count | Separately visible from independent correctness. |
| Independent correctness | `independent_correct` count and per-question `independent` count | Separately visible from assisted success. |
| Repeated independent question | existing `repeat`/`attempt_number` | Not independent evidence; also excluded from independent selection. |
| Unknown tactic / unknown question | exit 2, JSON error code | Nothing recorded. |
| Tactic tag on a source question | `tactics` array in the question JSON, validated against the registered tactic ids | Visible in `questions`; makes the question eligible for that tactic. |
| Untagged or unreviewed source question | not a tactic candidate | Refused material is named instead of silently substituted. |
| Target question leaks no key | pre-answer JSON and human output contain no answer, evidence, reasoning, status or feedback | Same privacy boundary as goal 0001/0002. |

### Why assistance is a separate attempt field

The existing `first`/`repeat` dimension says whether the answer was shown before. That is not the same
question as "was the learner helped during this attempt". A learner can see a tactic walkthrough on a
new question (assisted, first) or answer a repeated question with no help (independent execution but
not independent evidence). Storing both dimensions separately, in the append-only attempts table,
keeps the historical record truthful and recomputable if a later goal changes the reporting policy.

### Tactic content and the counterexample

The shipped tactic is **NOT/EXCEPT polarity** (`not-except-polarity`):

- Principle: in a NOT/EXCEPT stem, the key is the option whose statement the passage contradicts or
  does not support; the other three are supported.
- Why it works: exam items are built so one option has a different truth relation, and negation is
  carried by small words and by the stem, not by the repeated nouns.
- When it works: multiple-choice items that ask which option is NOT stated/true/included, and only
  where the answer mapping has been reviewed.
- Boundary: the lesson includes a concrete counterexample. "Choose the option that repeats the most
  passage words" selects the supported option in a NOT item, which is the wrong answer. A second
  boundary shows that "pick the longest option" fails because the key is frequently short and the
  trap is a wordy paraphrase.

The first-party exercise and the boundary example are distinct so the lesson never reveals the
current question's key.

## Persistence

Schema version 4 adds three columns; `attempts` remains append-only:

- `attempts.assistance TEXT NOT NULL DEFAULT 'independent' CHECK (assistance IN ('independent','assisted'))`
- `attempts.tactic_id TEXT NOT NULL DEFAULT ''`
- `source_questions.tactics_json TEXT NOT NULL DEFAULT '[]'`

Migration is versioned and idempotent. A v0/v1/v2/v3 database keeps its attempts; pre-v4 attempts
become `independent` (no hint existed in that build). A newer schema is still refused. The existing
append-only triggers, id-reusing-insert guard, content-scoped numbering and `0600`/`0700` file rules
are unchanged.

Domain split:

| Package | Truth it owns |
|---|---|
| `internal/tactic` | Registered tactics, their authored lesson and boundary text, and the first-party exercise. It makes no learner-state decisions. |
| `internal/exam` | Additive skill tags on questions, validated as lowercase slugs; evidence/option/key invariants unchanged. |
| `internal/source` | Question tags from local question files, registry validation, and carrying tags into practice content. Review gates unchanged. |
| `internal/attempt` | Assistance and tactic identity on a graded record; `Explain` reports the condition. |
| `internal/store` | Schema v4, tag persistence, query/list of the new attempt fields. |
| `internal/cli` | `tactic list/show/practice`, selection, guided/independent gating, report split. |

## Evidence plan

1. **Go tests**: tactic registry and exercise validation (tags, evidence substrings, two eligible
   questions, an untagged decoy, a keyword trap), assistance/tactic record validation, schema v3→v4
   migration with preserved attempts, tag round-trip in the store, source tag validation and review
   gating, tactic selection/gating (guided required, distinct, unattempted), JSON goldens for
   list/show/awaiting/answered, report counts split, and the pre-answer key-hiding check.
2. **Tier 2 transcript**: the embedded exercise shows the lesson, the guided attempt, the independent
   attempt on a different question, and the report split. A second transcript imports the committed
   baseline, reviews one page under the demo label, authors and reviews two tagged questions, runs the
   tactic guided and independent on that imported page, and shows the source content id and
   provenance in the report. The page text is not reprinted; only selected JSON fields and short
   authored stems are quoted. The review is a workflow demonstration by the agent, not an owner
   verification.
3. **Boundary evidence**: `tactic show` output contains the counterexample and its failure reason; the
   exercise question used by the demo has a keyword-overlap trap whose option explanation states why
   the naive rule fails; the independent attempt is a different question id from the guided attempt.
4. **Confidentiality/local-only**: the existing `local-only` gate (zero network imports, zero
   confidential markers), a strace network check of a tactic run, `git ls-files` filtered for
   PDF/SQLite/private paths, and the new gate asserting the pre-answer document hides the key.
5. **Gates**: `bash scripts/gate.sh` real exit code, counts and the new `tactic-smoke` assertions;
   the harness self-test feeds it a deliberately broken CLI.

## Harness growth (same increment)

A new CLI protocol and a new evidence dimension need a machine check the current set cannot see.
`harness/gates/tactic-smoke.ts` builds the real CLI and drives:

- `tactic list` contains `not-except-polarity`.
- `tactic show` carries non-empty `why_it_works`, `when_it_works`, ordered steps and all three
  boundary fields.
- the awaiting guided document has `assistance=assisted`, a guidance block, and none of `answer`,
  `correct_label`, `correct_text`, `evidence`, `reasoning`, `status`, `feedback` anywhere.
- the guided answer records `tactic_id` and `assisted`.
- the awaiting independent document has `assistance=independent`, no `guidance`, a **different**
  question id, and the same key-hiding check.
- the independent answer records `independent`; a further independent request exits 2.
- `report --json` shows `assisted_attempts=1`, `independent_attempts=1`, and the per-attempt
  assistance split.

`harness/gates/self-test.ts` gains a fake CLI case that prints `{}` for every command; the tactic gate
must fail it. `harness/gates/index.ts` registers the gate after import-smoke and before self-test;
`harness.md` gains the gate row and updated counts; `scripts/gate.sh` remains a shim.

## Trade-offs and defaults taken

- **One tactic, not a catalogue.** The contract says "one real scoring tactic". NOT/EXCEPT polarity is
  the most mechanically checkable choice: it has a crisp counterexample, the answer is deterministic,
  and it does not require the private key to demonstrate. Evidence-location and paraphrase tactics can
  be added to the same registry later without a schema change. (Q-0003-01)
- **Tags are a closed registry, not free text.** A typo in a question file would otherwise make a
  question silently ineligible. Unknown tags are rejected at `question add`/`edit`, naming the
  registered ids. Adding a tactic is a code change, which is acceptable while tactics are authored,
  reviewed content. (Q-0003-03)
- **Assisted means the guided response delivered the tactic walkthrough with the attempt.** The
  pre-answer document carries it when requested, and the answered document carries it for a one-shot
  `--answer`/`--skip`. The walkthrough contains no key, evidence or option-specific answer. Guidance
  is stored as a per-attempt condition, not as a derived learner model.
- **Independent requires a first attempt and a distinct question.** A previously seen question, even
  with no help, is repeat exposure under goal 0001's own rule and cannot support independent
  correctness. A guided attempt for the tactic must exist first, so "after help" is enforced rather
  than merely suggested. (Q-0003-02)
- **Independent selection refuses rather than substitutes.** If no eligible question remains, the
  command says so and exits 2. Silently reusing an attempted question would overstate independent
  evidence.
- **Both material paths.** The embedded exercise makes the capability usable without the private
  source and keeps CI/fixtures redistributable. The `--source` path is the source-grounded route and
  is what the private transcript exercises. Neither path copies source text into the repository.
- **Report adds dimensions, does not rewrite counts.** Overall/first/repeated counts keep their
  meaning; the new assisted/independent split is additive, and every attempt row exposes its own
  values. Goldens are updated for the additive fields only.
- **`tactic_id` is stored on the attempt.** It is provenance for why help was shown and lets a later
  goal compute per-tactic evidence without rewriting attempts. It is not a foreign key because the
  embedded exercise has no source row and tactics are build-time content.

## Out of scope, with reasons

- Automatic tactic tagging or item generation from the private source: that would invent authority
  the source does not have and needs a review workflow of its own.
- Additional tactics (evidence location, paraphrase, elimination, pacing): the registry is ready, but
  the contract asks for one demonstrated tactic; adding more untested lesson content would dilute the
  evidence.
- Adaptive selection (0004), AI tutoring (0005+), Anki (0007), desktop UI (0011): not needed to prove
  a guided/independent tactic pair or the assisted/independent evidence split.
- Scoring, pass probability, exam blueprint or time-limit claims: still unknown and forbidden by the
  core design.
- Committing any PDF, new OCR, private learner database or source-derived question: tests use the
  first-party exercise and small authored fixtures; the private transcript lives in `/tmp`.

## Completion checklist

- [x] Plan written to `docs/plans/` (this file).
- [x] `internal/tactic` registry, lesson, boundary, embedded exercise and tests.
- [x] Question skill tags in `exam`, `source` and the store v4 schema; migration tests.
- [x] Assistance/tactic fields on attempts and the report split; tests and goldens.
- [x] `tactic list/show/practice` CLI with guided/independent gating; human and JSON tests.
- [x] `tactic-smoke` gate, self-test case, harness index and documentation.
- [x] README, `docs/tactics.md`, `docs/source-import.md`, `docs/harness.md` reconciled.
- [x] Private-source and first-party evidence transcript with real output and limits.
- [x] Goal archived unchanged, goals index updated, open questions appended.
- [x] `bash scripts/gate.sh` exit 0 recorded; no test or contract clause weakened.
