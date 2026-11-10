# Adaptive next practice

Goal 0004 makes the next practice choice depend on recorded evidence instead of a fixed order. The
recommendation is a pure view over the append-only attempts: it is recomputed on every `next`, and
nothing about the inference is stored or allowed to rewrite an old attempt.

No pass probability, readiness score, exam blueprint or time limit is computed. The only time signal
is the learner's own prompt-to-answer time inside this local practice.

## Commands

```bash
passquest skill list
passquest skill show vocabulary-in-context
passquest skill practice vocabulary-in-context
passquest skill practice not-except-polarity --source SOURCE --question q2
passquest next
passquest next --json
passquest next --source SOURCE
```

`next` loads embedded first-party material plus practiceable tagged questions from imported sources.
`--source SOURCE` puts that source first and restricts the imported material considered; embedded
first-party material remains available so the command works without a private import.

`next` is read-only. It does not record an attempt, write a recommendation table or change a
question tag.

## Skill set

| Skill | Kind | Purpose |
|---|---|---|
| `not-except-polarity` | scoring | The goal-0003 NOT/EXCEPT tactic; the only scoring skill. |
| `vocabulary-in-context` | language | Infer a word's meaning from its sentence, not its usual translation. |
| `sentence-structure` | language | Find the subject and verb, then attach modifiers and condition words. |

Each skill carries an authored principle, why/when guidance, at least three steps and a watch-out.
`skill show` is the lesson surface. For the scoring skill, `tactic show not-except-polarity` remains
the full lesson with the counterexample.

There is no larger ontology. A wrong option alone does not identify a cause; the policy keeps the
causal diagnosis `unresolved` and says so.

## What counts as independent transfer

The policy classifies every attempt against a skill:

| Class | Rule | Counted as independent correctness? |
|---|---|---|
| Independent transfer | `assistance=independent`, attempt number 1, and no earlier attempt on the same content id | Yes |
| Passage re-exposure | `assistance=independent`, attempt number 1, but an earlier attempt exists on the same content id | No |
| Assisted | `assistance=assisted` (walkthrough or other help visible) | No |
| Revisit | attempt number greater than 1 on the same content-scoped question | No |

The passage rule is deliberately stricter than goal 0003's question-level assistance rule. Feedback
on any question reveals the passage and its evidence, so a new question on a seen passage is
familiarity, not transfer. Goal-0003 `t1`/`t2` show this: the report keeps one assisted attempt and
one independent attempt, while `next` shows one passage and zero independent transfers.

`skill practice` prefers a fresh passage, then an unattempted question, then a repeat. Every document
carries `passage_exposure: fresh | already_seen`, so the learner can see whether the attempt can add
transfer evidence before answering.

## States and actions

`answered` means independent transfer attempts with status `correct` or `incorrect`. Skipped
independent attempts are shown but count as no answer evidence.

| State | Condition | Action |
|---|---|---|
| `no_evidence` | no attempt for the skill | scoring: `explain`; language: `practice_new` |
| `familiarity_only` | attempts exist, but no independent transfer attempt | `practice_new` if a fresh passage exists; otherwise `review`; scoring without a guided attempt gets `explain` |
| `insufficient` | 0 or 1 answered independent observations | `practice_new` if a fresh passage exists; otherwise `review` |
| `weak` | at least 2 answered, zero correct | `explain` |
| `mixed` | at least 2 answered, some correct | latest answered independent incorrect: `explain`; latest answered correct: `practice_new` if fresh, otherwise `review` |
| `supported` | at least 2 answered, all correct | slow latest: `review`; otherwise `practice_new` if fresh, otherwise `review` |

A **fresh passage** is a content id with a tagged question and zero attempts on that content. It is
the only material that can add independent transfer evidence. `review` repeats a seen question and is
never independent transfer; the reason says so.

**Slow** means at least two independent correct attempts, the latest independent status is correct,
and the latest elapsed time is at least twice the learner's fastest correct independent time for that
skill. It changes only the `supported` action, from `practice_new` to `review`. This is a relative
pace signal, not an exam clock.

Recommendation priority is: `weak` / `mixed` with latest incorrect, then `familiarity_only`, then
`insufficient`, then `mixed` with latest correct, then `supported` slow or without a fresh passage,
then `no_evidence`, then `supported` fresh. Ties follow registry order, so the output is
deterministic.

## Inspecting the evidence

`next --json` separates the two sides explicitly:

- `evidence` — recorded facts: attempts, assisted, independent correct/incorrect/skipped, revisits,
  passage re-exposures, distinct passages, questions seen, fresh passages, fastest/latest elapsed and
  the attempt ids.
- `inference` — recomputed state, action, reason, causal diagnosis and uncertainty lines.

The human output prints `Evidence (recorded attempts)` and `Inference (recomputed from evidence)`
separately. `unclassified_attempts` lists attempts with no registered skill tag or unavailable
content; they are never silently dropped, and they still mark their passage as seen.

A recommendation carries an exact follow-up command:

| Action | Scoring skill command | Language skill command |
|---|---|---|
| `explain` | `passquest tactic practice <id> --stage guided` | `passquest skill show <id>` |
| `practice_new` | `passquest skill practice <id>` | `passquest skill practice <id>` |
| `review` | `passquest skill practice <id> --question <seen>` | `passquest skill practice <id> --question <seen>` |

Every `explain` recommendation also carries a `follow_up_command` for the next practice step when
material exists: `skill practice <id>` after the guided tactic walkthrough, or after the language
lesson. Source material adds `--source SOURCE` to the command.

## Question tags

A local question file's `tactics` field keeps its goal-0003 name but accepts any registered skill id,
including `vocabulary-in-context` and `sentence-structure`. Unknown tags are rejected at
`question add`/`question edit`; see [`source-import.md`](source-import.md).

New attempts snapshot the question's current tags in `attempts.skills_json` (schema version 6), so a
later tag edit cannot regroup old evidence. Attempts written before version 6 have no snapshot; they
are mapped from the question's current tags and marked `mapped_from_current_tags` with an uncertainty
line.

## Limits

- The state thresholds are exact counts on a small sample, not a psychometric model. Two or fewer
  answered observations are always labelled insufficient or just-supported.
- Only the three registered skills are inferred. Untagged questions and unavailable content are
  listed as unclassified.
- A separately viewed lesson is not tracked as assistance. Only a tactic walkthrough delivered with
  the attempt (or an ordinary attempt after an exposed walkthrough) is recorded as assisted.
- `next` uses the current material catalog for fresh-passage counts. If a question is edited or
  removed, historical attempts remain evidence but the review command may not be available.
- Nothing here is a held-out, delayed or timed assessment; those belong to goal 0009.
