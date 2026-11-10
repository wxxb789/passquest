# 0004 — Choose practice based on learner evidence: increment plan

Contract: [`../goals/archive/0004-adaptive-next-exercise.md`](../goals/archive/0004-adaptive-next-exercise.md)
Shape reference: goal 0003 (`docs/goals/archive/0003-exam-tactic-practice.md`, its plan and evidence
record). Conventions: [`../conventions.md`](../conventions.md). Harness:
[`../harness.md`](../harness.md).

## Goal in one sentence

For one learner, recompute an inspectable recommendation — **explain**, **practice a new question**,
or **review a seen question** — from the append-only attempts, elapsed time, assistance and repeat
exposure, with a small skill set: one scoring tactic and two minimal language prerequisites.

The recommendation is pure inference. Nothing is written to the database when it is computed, and no
old attempt is rewritten. The recorded attempt row is the evidence; the recommendation is a view.

## Public shape

```text
passquest skill list [--json]
passquest skill show SKILL [--json]
passquest skill practice SKILL [--source SOURCE] [--question ID]
                                [--answer LABEL | --skip] [--db PATH] [--json]
passquest next [--source SOURCE] [--db PATH] [--json]
```

- `skill list` / `skill show` expose the small registry. `not-except-polarity` is the scoring skill
  and keeps its full lesson available through `tactic show`; `vocabulary-in-context` and
  `sentence-structure` are the two language prerequisites. Each carries an authored principle,
  ordered steps and a watch-out. The scoring entry's `tactic_id` links to the goal-0003 lesson rather
  than duplicating the boundary text.
- `skill practice` is ordinary unassisted practice on a question tagged with the skill. It shows no
  explanation and no key; the awaiting document is the same key-hiding protocol as `practice` and
  `tactic practice`. It accepts embedded first-party material or one imported source. It prefers a
  **fresh passage** (a content id with no earlier attempt), then an unattempted question, then a
  repeat, and labels the passage exposure in every document. A repeat is allowed but never counted
  as independent transfer.
- `next` is a read-only recommendation surface. It loads embedded content plus practiceable tagged
  questions from imported sources and returns the recommended action, the skill, the recorded
  evidence, the inferred state, the uncertainty, and an exact follow-up command. `--source` restricts
  imported material to that source; embedded first-party material is always available.
- Exit codes follow the existing CLI: `0` success, `2` unknown skill / invalid input / not
  practiceable / unknown source, `1` storage or internal failure. JSON errors keep
  `schema_version: 1` and the `{error: {code, message}}` shape.
- Existing `practice`, `report`, import, review, and `tactic` protocols keep their meaning. The
  attempt JSON gains an additive `skills` tag snapshot; no existing field changes meaning.

## Skill set (deliberately small)

| Skill | Kind | Why it is here | Material |
|---|---|---|---|
| `not-except-polarity` | scoring | The goal-0003 tactic; the only scoring skill | Goal-0003 embedded exercise plus one first-party scoring question on a second passage |
| `vocabulary-in-context` | language | Minimum prerequisite for guessing meaning from the sentence, not the word's usual translation | Three first-party passages, one question each; the third keeps a fresh passage after two correct attempts so time can change the action |
| `sentence-structure` | language | Minimum prerequisite for finding the subject/verb and attaching a modifier | Two first-party passages, one question each |

No ontology beyond these three. The source question field remains `tactics` for protocol
compatibility, but its values are now validated against this skill registry; a scoring tactic is a
kind of skill. A language tag is therefore accepted by `question add`/`question edit` and carried
into practice content and the attempt snapshot.

## Evidence classes (recorded facts)

Each attempt is classified against a skill only from data recorded at answer time:

| Class | Rule | What it can show |
|---|---|---|
| Independent transfer | `assistance=independent`, attempt number 1, no earlier attempt on the same content id | The only class counted as independent correctness. |
| Passage re-exposure | `assistance=independent`, attempt number 1, an earlier attempt already exists on the same content id | A new question on a seen passage; retained as familiarity, never transfer. |
| Assisted | `assistance=assisted` (tactic walkthrough or ordinary practice after a walkthrough) | Help was visible; it is not independent correctness, even when the answer is correct. |
| Revisit | attempt number > 1 on the same content-scoped question | Repeat exposure; retained, never independent transfer. |

Attempts are processed in insertion order. The first attempt on a content id marks the whole passage
seen, regardless of question or skill: feedback on any question reveals the passage and its
evidence. This is stricter than goal 0003's attempt-level `assistance` field, which remains stored
unchanged. Goal-0003 independent question `t2` on the same passage as `t1` is therefore an
independent **attempt** but a passage re-exposure for the goal-0004 policy. The evidence record
demonstrates the difference explicitly.

The skill snapshot on new attempts (`attempts.skills_json`) makes the mapping historical. A pre-v6
attempt has no snapshot; it is mapped from the question's current tags and marked
`mapped_from_current_tags`, with that uncertainty printed. An edited tag therefore cannot silently
rewrite old evidence for new rows; it can only affect pre-v6 rows, which say so.

## Inferred states and actions

`answered` means independent transfer attempts with status correct or incorrect. Skipped independent
attempts are shown but cannot support a skill either way.

| State | Condition | Action | Reason shape |
|---|---|---|---|
| `no_evidence` | no attempt for the skill | scoring: `explain`; language: `practice_new` | Scoring starts with the guided walkthrough; language needs a first measurement. |
| `familiarity_only` | attempts exist, zero independent transfer attempts | `practice_new` if a fresh passage exists; else `review`; scoring with no guided attempt gets `explain` | Assisted, revisited or passage-exposed work is not transfer. |
| `insufficient` | 0 or 1 answered independent transfer attempts | `practice_new` if a fresh passage exists; else `review` | A single observation cannot distinguish skill from luck; skips record no answer. |
| `weak` | at least 2 answered, zero correct | `explain` | A repeated wrong outcome supports a gap; the explanation is the skill rule, not a causal diagnosis. |
| `mixed` | at least 2 answered, some correct | latest answered independent incorrect: `explain`; latest answered correct: `practice_new` if fresh, else `review` | The latest answered evidence decides whether to re-teach or confirm on a fresh passage. |
| `supported` | at least 2 answered, all correct | slow latest: `review`; otherwise `practice_new` if fresh, else `review` | All correct is still a small sample; a fresh passage tests transfer, while a slow latest answer calls for consolidation. |

Definitions:

- **Fresh passage** = a content id with at least one practiceable question tagged for the skill and
  zero attempts on that content id. This is the only material that can add independent transfer
  evidence.
- **Slow** = at least two independent correct attempts, latest independent status is correct, a
  positive fastest independent-correct elapsed, and latest elapsed `>= 2 * fastest`. Time is local
  prompt-to-answer time, not an exam time limit. It changes the supported-skill action from
  `practice_new` to `review`; no other state uses time.
- **Review** repeats a seen question. It is recorded normally but cannot be independent transfer, and
  the reason says so.
- Recommendation priority, highest first: `weak` / `mixed` with latest incorrect → `familiarity_only`
  → `insufficient` → `mixed` with latest correct → `supported` slow or no fresh passage → `no_evidence`
  → `supported` fresh. Ties follow registry order, so output is deterministic.

## JSON shape

```json
{
  "schema_version": 1,
  "state": "recommendation",
  "recommendation": {
    "action": "practice_new",
    "skill": {"id": "vocabulary-in-context", "kind": "language", "title": "Vocabulary in context"},
    "state": "insufficient",
    "reason": "...",
    "command": "passquest skill practice vocabulary-in-context",
    "follow_up_command": null,
    "evidence_attempt_ids": [3, 4]
  },
  "skills": [
    {
      "skill": {"id": "...", "kind": "...", "title": "..."},
      "evidence": {
        "attempts": 2,
        "assisted_attempts": 1,
        "assisted_correct": 1,
        "independent_attempts": 1,
        "independent_correct": 1,
        "independent_incorrect": 0,
        "independent_skipped": 0,
        "revisits": 0,
        "passage_repeats": 1,
        "distinct_passages": 1,
        "questions_seen": 2,
        "fresh_passages": 1,
        "fastest_independent_ms": 1200,
        "latest_independent_ms": 1200,
        "latest_independent_status": "correct",
        "independent_attempt_ids": [2],
        "mapped_from_current_tags": 0
      },
      "inference": {
        "state": "insufficient",
        "action": "practice_new",
        "reason": "...",
        "causal_diagnosis": "not_attempted",
        "uncertainty": ["..."]
      }
    }
  ],
  "unclassified_attempts": [],
  "limitations": ["..."]
}
```

`evidence` contains recorded facts only. `inference` contains the recomputed state, action, reason and
uncertainty. The human output prints the same split under `Evidence (recorded)` and
`Inference (recomputed from evidence)`. `unclassified_attempts` names attempts with no registered
skill tag or unavailable content; they are never silently dropped.

`next` follows the existing key-hiding boundary: it prints skill ids, titles, content ids, question
ids, counts and commands, never passage text, answer labels, evidence sentences or reasoning.

## Persistence

Schema version 6 adds one append-only snapshot column; no derived state table is added:

- `attempts.skills_json TEXT NOT NULL DEFAULT '[]'`

`attempt.BuildRecord` copies `question.Tactics` into `Attempt.Skills`; `WithTactic` appends the tactic
id when it is not already present. `store.Append` validates lowercase slugs, uniqueness and persists
the JSON array; `store.List` reads it back. Attempts remain append-only with the existing triggers.
A v0–v5 database keeps its rows; pre-v6 rows read as an empty snapshot. A newer schema is still
refused. `next` writes nothing, so reopening the database recomputes the same recommendation from the
same rows.

## Domain split

| Package | Truth it owns |
|---|---|
| `internal/skill` | The small skill registry, authored language lessons and six first-party language/scoring exercise passages. It validates that every registered skill has at least two distinct passages, so independent transfer is possible; vocabulary has three so a supported state can still have a fresh passage. |
| `internal/adaptive` | The pure evidence classifier and recommendation policy. It owns no storage and no CLI syntax. |
| `internal/attempt` | The skill snapshot on a graded record; existing assistance/tactic/repeat fields unchanged. |
| `internal/store` | Schema v6, snapshot persistence, migration from v5. |
| `internal/source` | Question tags validated against the skill registry instead of the tactic registry. Review gates unchanged. |
| `internal/cli` | `skill list/show/practice`, `next`, command rendering, and the passage-exposure label. |

## Evidence plan

1. **Go tests**:
   - Skill registry: valid kinds, lesson fields, scoring link to the tactic, every skill has at least
     two distinct passages, all tags registered, unique question ids, untagged decoy excluded.
   - Adaptive policy: no evidence, assisted only, one observation, repeated incorrect, mixed with
     latest correct/incorrect, supported fast/slow, passage re-exposure, assisted passage exposure,
     revisits, skips, no fresh passage, priority order, deterministic ties, current-tag fallback,
     multi-skill attempts.
   - Store: v5→v6 migration keeps attempts and defaults the snapshot; snapshot round-trip; invalid or
     duplicate skill slugs rejected; append-only triggers unchanged.
   - Source: a language skill tag is accepted, an unregistered tag is still rejected, tags reach
     practice content and the attempt snapshot.
   - CLI: `skill list/show`, key-hiding awaiting document, fresh vs seen passage labels, explicit
     source practice, unknown skill/question, `next` evidence/inference split, time-driven
     `practice_new` vs `review`, hint/revisit effect, source-scoped command, reopen stability, golden
     documents and human output.
2. **Tier 2 transcripts** (real binary, fresh databases under `/tmp/passquest-0004-demo/`):
   - Empty database → `next` recommends `explain` for the scoring skill; language skills show
     `no_evidence` and `practice_new`.
   - Guided tactic `t1` then independent `t2` on the same passage → `next` shows 1 assisted, 0
     independent transfer, 1 passage re-exposure; it does not call the pair independent.
   - Two fresh vocabulary passages, correct → `supported`; repeat one question → `revisits=1` and
     independent correctness unchanged.
   - Two fresh sentence-structure passages, wrong → `weak`, action `explain`, causal diagnosis
     `unresolved`.
   - Two correct vocabulary attempts with real interactive delays (for example ~150 ms and ~900 ms) →
     `supported` with `review`, reason quoting the two elapsed values.
   - Imported fixture page with a reviewed tagged question → source practice and `next --source` show
     the source content id in the evidence.
   - A sample-exam attempt (untagged) → `next` lists it under `unclassified_attempts`, not under a
     skill.
   - `next --json` twice in separate processes on the same database → byte-identical output; the
     attempt rows are unchanged.
3. **Boundary evidence**: the same-passage transcript above; the key-hiding check on
   `skill practice`; the JSON shows `assisted`, `revisits`, `passage_repeats` separately from
   `independent_*`; the reason for an explanation says the wrong answer does not identify a cause.
4. **Confidentiality/local-only**: existing local-only gate (zero network imports/markers), a strace
   network check of `next` and `skill practice`, `git ls-files` filtered for PDF/SQLite/private
   paths, database `0600`, default directory `0700`, and no private source text in any output.
5. **Gates**: `bash scripts/gate.sh` real exit code and counts, including the new
   `adaptive-smoke` gate and its self-test case.

## Harness growth (same increment)

A new CLI protocol and a new evidence dimension need a machine check:

- `harness/gates/adaptive-smoke.ts` builds the real CLI and drives: `skill list/show`;
  a fresh-passage `skill practice` awaiting document with key hiding; the answered attempt's skill
  snapshot; `next` on an empty database; the assisted/passage-re-exposure classification after
  `tactic practice t1/t2`; two fresh correct attempts; revisits; two wrong attempts → `explain`;
  reopened `next` stability; and an unknown-skill exit 2.
- `harness/gates/self-test.ts` gains a fake-CLI case that prints `{}`; the adaptive gate must fail it.
  The case count becomes 20 over 13/13 gates.
- `harness/gates/index.ts` registers the gate after `tactic-smoke` and before `self-test`;
  `harness.md` gains the gate row and updated counts; `scripts/gate.sh` remains the shim.

## Trade-offs and defaults taken

- **Passage-level transfer is stricter than goal 0003.** Goal 0003 required a distinct question and
  no earlier attempt on that question. Goal 0004 additionally refuses to call a new question on a
  seen passage independent transfer, because the passage and feedback were already exposed. Stored
  attempts are untouched; only the adaptive classification is stricter. (Q-0004-01)
- **Three skills, not a taxonomy.** One scoring skill and two language prerequisites are enough to
  make selection real and to show accuracy, help and time changing the recommendation. Adding more
  needs its own authored material and tested policy branch. (Q-0004-02)
- **No stored recommendation or derived learner state.** `next` is a pure function of attempts and
  the current material catalog. Reopening recomputes it; there is no table to migrate or rewrite.
- **Skill snapshots on attempts.** New attempts store the tags that were visible when the answer was
  recorded. This keeps historical grouping stable if a question is edited later. Pre-v6 rows fall
  back to current tags and say so.
- **Time is relative, not an exam clock.** The only time rule compares the learner's latest correct
  independent answer with their own fastest correct answer for the same skill. No exam time limit is
  invented.
- **Review is honest.** A repeat can consolidate but never counts as independent transfer. The
  recommendation says this and the report continues to mark it as a repeat.
- **`next` includes imported sources by default.** A real learner's local material is primary
  context. `--source` narrows imported material to one source; embedded first-party material remains
  available so the command works without a private import.

## Out of scope, with reasons

- A measured pass probability, readiness score, exam blueprint or time limit: still unknown and
  forbidden by the core design.
- A broad skill ontology, automatic skill tagging, or an LLM diagnosis: the registry is authored and
  closed; a wrong answer alone is not a causal diagnosis.
- Delayed or held-out assessment (goal 0009): the strictest current rule is passage-level
  re-exposure, not a timed holdout.
- Adaptive scheduling across days, retention curves or Forgetting-index claims: one learner with a
  handful of local attempts cannot support them.
- New question generation, OCR, PDF extraction, Anki, provider calls, desktop UI: later goals.
- Committing any PDF, OCR output, private learner database or source-derived fixture: tests use
  first-party content and the existing authored import fixture.

## Completion checklist

- [x] Plan written to `docs/plans/` (this file).
- [x] `internal/skill` registry, language lessons, six first-party passages and tests.
- [x] `internal/adaptive` classifier/policy and tests: time, hints, revisits, passage exposure,
      uncertainty, priority.
- [x] Attempt skill snapshot, schema v6 migration, store tests, source tag validation.
- [x] `skill list/show/practice` and `next` CLI; key hiding; human/JSON goldens.
- [x] `adaptive-smoke` gate, self-test case, harness index and documentation.
- [x] README, `docs/adaptive.md`, `docs/tactics.md`, `docs/source-import.md`, `docs/harness.md`
      reconciled.
- [x] Tier 1/2/3 evidence record with real commands, numbers and limits.
- [x] Goal archived unchanged, goals index updated, open questions appended.
- [x] `bash scripts/gate.sh` exit 0 recorded; no test or contract clause weakened.
