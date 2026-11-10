# Scoring tactics

Goal 0003 adds one inspectable exam-scoring tactic, a guided example that records help, and an
independent question that records unassisted first-attempt correctness. The tactic is authored
first-party content; questions come either from the shipped exercise or from locally imported,
reviewed source material.

No exam blueprint, pass threshold or readiness probability is implied. The tactic teaches how an
item's polarity works, not what the live examination will ask.

## Commands

```bash
passquest tactic list
passquest tactic show not-except-polarity
passquest tactic practice not-except-polarity --stage guided
passquest tactic practice not-except-polarity --stage independent
passquest report
```

With `--source SOURCE`, the same commands use reviewed questions from a private local import:

```bash
passquest tactic practice not-except-polarity --source SOURCE
passquest tactic practice not-except-polarity --source SOURCE --stage independent
```

`--json` uses the existing `schema_version: 1` protocol. The pre-answer document carries `tactic`
(always) and `guidance` (guided only); it never contains the answer key, evidence, reasoning, status
or post-answer feedback. The answered document carries the tactic condition, the attempt, the
deterministic feedback, and — for a guided attempt — the same `guidance` block, so a one-shot
`--answer`/`--skip` still receives the walkthrough that makes the attempt assisted. An independent
answered document carries no `guidance`.

## NOT/EXCEPT polarity

- **Principle:** in a NOT/EXCEPT item, three options are supported by the passage and one is not (or
  is false). The key is the odd statement, not the option that reuses the most vocabulary.
- **Why it works:** the item author gives the key a different truth relation. Negation is carried by
  the stem and by small words (`not`, `except`, `free`, `after`), so an option can repeat passage
  nouns and still be false.
- **When it works:** stems that ask what is NOT stated/true/included, or that contain EXCEPT, and
  only when the answer mapping has been reviewed. It does not decide a "best title" or inference
  question.
- **Boundary:** the lesson's counterexample shows that "choose the option that repeats the most words"
  and "prefer the longest option" both select the supported statement in a NOT item, which is the
  wrong answer. Word overlap measures vocabulary reuse, not truth. The shipped exercise contains a
  second keyword-overlap trap whose option explanation states exactly why the shortcut fails.

The lesson text and the boundary example are embedded in the binary and are redistributable. The
boundary example is deliberately different from the exercise so the guided walkthrough cannot reveal
the current question's key.

## Guided and independent attempts

An attempt records two independent dimensions:

| Field | Values | Meaning |
|---|---|---|
| `assistance` | `assisted`, `independent` | Whether a tactic walkthrough accompanied the attempt. The guided response always carries it; an independent response carries none. |
| `first`/`repeat` (existing) | first, repeated | Whether that content-scoped question was answered before. |

- A `guided` attempt shows the tactic walkthrough and is stored as `assisted`: the pre-answer
  document carries it when one is requested, and the answered document carries it again for a
  one-shot `--answer`/`--skip`. It can succeed or fail; it is never counted as independent
  correctness.
- An `independent` attempt shows no guidance, uses a question **different from every guided question
  for the same tactic**, and requires zero earlier attempts on that content-scoped question. It is
  stored as `independent`.
- The first tactic attempt defaults to `guided`; after one guided attempt the default becomes
  `independent`. `--stage independent` before any guided attempt exits 2 with `guided_required`.
- If no eligible independent question remains, the command exits 2 with `not_practiceable` and names
  the reason. It never substitutes an attempted question.
- Existing ordinary `practice` attempts are stored as `independent` with no tactic id. Older rows
  migrate to that state because no help existed in those builds.

`report` keeps the existing attempts/first/repeated counts and adds:

```text
Independent attempts (no hint): 2 (correct 1, incorrect 1, skipped 0)
Assisted attempts (help shown): 1 (correct 1, incorrect 0, skipped 0)
```

The independent row counts first attempts only, because a no-hint repeat still comes after the answer
and evidence were shown. A repeated attempt carries `assistance=independent` but is counted under
`Repeated attempts`, not independent correctness. The assisted row counts every guided attempt,
including a repeated guided example.

Each question row adds `assisted` and `independent` counts; each attempt row names its assistance and
tactic. The per-question `independent` count follows the same first-attempt rule. The same values are
fields in `report --json` (`assisted_attempts`, `independent_attempts`, and their status splits).

## Tactic tags on imported questions

A locally authored question file may name the tactics it exercises:

```json
{
  "id": "q26",
  "stem": "Which of the following is NOT stated in the passage?",
  "options": [
    {"label": "A", "text": "..."},
    {"label": "B", "text": "..."},
    {"label": "C", "text": "..."}
  ],
  "answer": "A",
  "evidence": "verbatim passage substring",
  "reasoning": "why the odd statement is the key",
  "option_reasoning": {"A": "...", "B": "...", "C": "..."},
  "tactics": ["not-except-polarity"]
}
```

- Tags are lowercase slugs and must name a registered tactic. An unknown or misspelled tag is an
  exit-2 question-file error; it is not silently ignored. This keeps a typo from making a question
  permanently invisible to tactic practice.
- Tags survive review, editing and restart. A tag change changes the practice content version (the
  material is not identical), but not the content id, so attempts stay content-scoped.
- Tactic practice uses only questions that pass goal 0002's review gate: reviewed page, reviewed
  answer, every option explained, and evidence still a verbatim substring of the current page text.
  Untagged or unreviewed material is refused with an actionable message, never replaced by another
  question.
- The source page text, the locally authored questions and the review labels stay in the private
  SQLite database. The tactic lesson itself contains no source text.

## Current limits

- One tactic is registered. The registry and the question-tag field are designed for more, but each
  addition needs its own authored lesson, boundary and tested material.
- Tags are a closed vocabulary validated against the build's registry. A user cannot define a new
  tactic id without a code change.
- "Independent" means no tactic walkthrough and no prior attempt on that question. It does not mean
  delayed, time-limited or held-out assessment; those belong to goal 0009.
- The guided attempt is still a recorded attempt with repeat exposure. Re-running a guided example is
  allowed and remains visibly assisted/repeated.
- No automatic tactic detection, item generation or answer-key extraction is performed. A question is
  tagged only by a local author's question file.
