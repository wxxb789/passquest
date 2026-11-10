# 0001 — First independent exam attempt: increment plan

Contract: [`../goals/archive/0001-first-independent-exam-attempt.md`](../goals/archive/0001-first-independent-exam-attempt.md)
(moved unchanged at close). Shape reference: the harness increment `0f904b0` — every new surface gets a gate.

## Goal in one sentence

A learner answers a self-authored, answer-keyed multiple-choice exercise through a local CLI, sees
deterministic feedback with passage evidence, and finds the same attempt evidence after the process is
gone.

## Public shape

```text
passquest practice [--db PATH] [--question ID] [--answer A|B|C|D | --skip] [--json]
passquest report   [--db PATH] [--json]
passquest version
```

- `practice` prints the passage, stem and options only. The answer, evidence quote and reasoning are
  rendered after a selection (or after `skip`) and cannot be reached from the pre-answer path.
- Default question = first question with no stored attempt; otherwise the first question, so repeats
  are possible and are labelled as repeats. `--question` selects a stable question id.
- `skip` records a skipped attempt with no selection, then shows the key and reasoning.
- `--json` is non-interactive: without `--answer`/`--skip` it emits an `awaiting_answer` document and
  exits without reading stdin. Human mode reads `A`–`D`, `skip`, `quit`; EOF/`quit` records nothing.
- Exit codes: `0` success (including "no attempt recorded"), `2` usage/unknown question/invalid
  answer, `1` storage or internal failure. JSON errors go to stderr as
  `{"schema_version":1,"error":{"code":...,"message":...}}`.
- Default database: `$PASSQUEST_DB`, else `./.passquest/passquest.sqlite` (already gitignored).
  The file is created `0600` inside a `0700` directory.

### Attempt semantics (the part the contract tests)

| Situation | Stored `status` | `selected_*` | Report class |
|---|---|---|---|
| First try, chosen label equals key | `correct` | label + option text | first attempt |
| First try, chosen label differs | `incorrect` | label + option text | first attempt |
| First try, `skip` | `skipped` | `NULL` | first attempt |
| Any try after an earlier stored attempt on the same question | same three values | same rules | repeated attempt; answer/evidence were already shown |

`attempt_number = prior attempts on that question + 1`, computed inside the insert transaction and
recomputed on read from row order. A repeated attempt is never folded into first-attempt counts: the
report prints first and repeated counts separately and has no percentage, exam score, pass
probability or "mastery" claim. Elapsed milliseconds are recorded, not enforced — the exam time
budget is unknown.

### JSON schemas (schema_version 1)

- `awaiting_answer`: `content{id,version,title,notice,provenance{author,source,rights,
  exam_relation}}`, `question{id,number,total,passage,stem,options[{label,text}]}`. **No**
  answer/evidence/reasoning/status field exists in this document
  or in the human pre-answer output.
- `answered`: `content`, `attempt{id,session_id,question_id,attempt_number,prior_attempts,repeat,
  status,selected_label,selected_text,correct_label,correct_text,started_at,answered_at,
  elapsed_ms,content_id,content_version}`, `feedback{verdict,correct_label,correct_text,evidence,
  reasoning,selected_reasoning,exposure}`.
- `report`: `content`, `counts` (total/correct/incorrect/skipped, split into first and repeated),
  `questions[{id,attempts,first_status,latest_status,repeated}]`, `attempts[...]`, `limitations[]`.
  No database path, no learner identity, no score.

## Content

One passage, three four-option questions, authored for this repository (a library-hours notice with
comprehension and detail questions). Stored at `internal/exam/testdata/sample-0001.json`, embedded
with `go:embed`, so the binary needs no filesystem content and no network. Every question carries the
key label, a verbatim evidence quote, one-line reasoning and per-option reasoning. Machine checks:

- the evidence quote is a substring of the passage;
- exactly one key label, every option explained, labels unique;
- the sample contains no `CRSC`/Chinese confidential marker, and its notice/provenance says it is
  self-authored, not from a live exam and not an exam blueprint.

The key is authored and internally cross-checked, not externally verified exam material. No passing
score is invented.

## Persistence

SQLite via `modernc.org/sqlite` v1.62.0 (pure Go, no cgo). Standard library has no SQLite; SQLite is
already the decided durable store in `docs/core-design.md` §6 and `docs/conventions.md`, so this is
not a new architectural choice. One `attempts` table, `PRAGMA user_version = 1`, idempotent
migration, and triggers that abort `UPDATE`/`DELETE` on attempts (append-only evidence). Stored
columns snapshot the selected and correct option text plus content version, so a later content
revision cannot silently rewrite what was answered.

## Evidence plan

1. Unit/integration tests in `internal/exam`, `internal/attempt`, `internal/store`, `internal/cli`:
   validation, grading, invalid input, skip, repeat, append-only triggers, migration from version 0,
   reject newer schema, file permissions, key non-disclosure, JSON goldens, and a re-exec subprocess
   test that writes in one process and reports in another.
2. Real CLI transcript in `docs/evidence/0001-first-independent-exam-attempt.md`: fresh temp dir,
   awaiting output, correct/incorrect/skip/repeat, report from a new process, Python `sqlite3` table
   dump, `strace -f -e trace=network` showing no socket/connect syscalls.
3. New gate `scripts/check-local-only.sh`, wired into `scripts/gate.sh`: fails if a first-party
   package imports `net`, `net/http`, `net/rpc` or `net/smtp`, if the CLI links an HTTP/RPC/SMTP
   client, or if `cmd/`/`internal/` contain a confidential source marker. The `net` package itself
   is linked transitively by the pure-Go SQLite chain and disclosed by `docs/harness.md` gate 8,
   not failed. The schema/JSON surface is covered by tests + goldens, so the harness grows in the
   same increment.
4. `bash scripts/gate.sh` with its real output and exit code.

## Trade-offs and defaults taken

- **One passage, three questions, no timer.** A coherent exercise and all four attempt outcomes are
  demonstrable with a tiny authored sample. A timer or per-section budget would imply a blueprint the
  project does not have; elapsed time is recorded instead.
- **Skip shows the answer.** Otherwise a skip would be indistinguishable from an unanswered question;
  the contract asks for feedback, and the exposure is recorded by the next attempt being a repeat.
- **No percentage.** Counts only. The pass threshold and blueprint are unknown (goal 0009 owns
  readiness claims).
- **No hint feature.** Tactics/hints belong to goal 0003; adding an unrequested hint channel would
  create a second evidence dimension that goal 0001 cannot verify.
- **Repeat counting is per question id**, across content versions, because a question id is the
  stable identity of an item and exposure is about the learner having seen its key.
- **Archive link.** Moving the contract unchanged makes its `../core-design.md` link resolve from
  `docs/goals/archive/` to a missing path. A one-line redirect at `docs/goals/core-design.md` keeps
  the archived contract byte-identical, clickable and inside the docs-link gate. Reversible; delete
  the redirect if the owner prefers a link-check exemption for archives.
- **Staging.** Files are `git add`-ed (not committed) so `git ls-files`-based gates — gofmt, docs
  links, secrets — see the new surface.

## Out of scope, with reasons

- PDF/TXT import and page provenance — goal 0002.
- Scoring-tactic teaching or classification — goal 0003.
- Adaptive selection, Anki, providers/LLMs, desktop UI — goals 0004–0013.
- Exam blueprint, pass threshold, readiness probability — unknown and explicitly forbidden to invent.
- Multi-learner profiles/accounts — local single-learner evidence is sufficient for this slice.

## Completion checklist

- [x] Plan written to `docs/plans/` (this file).
- [x] Sample content + validation.
- [x] Store/migration/append-only + CLI.
- [x] Tests, goldens, subprocess reopen test.
- [x] `scripts/check-local-only.sh` wired into `gate.sh`; `docs/harness.md` updated.
- [x] README usage; evidence record with real output.
- [x] Goal archived unchanged, `docs/goals/README.md` state updated, open question appended.
- [x] `bash scripts/gate.sh` exit 0 recorded.

## Post-review corrections (2026-11-10)

Review findings F5–F7 changed three statements above without changing the goal contract:

- Repeat numbering and exposure are scoped to `(content_id, question_id)`, not `question_id` alone.
  A new content version under the same content id still inherits exposure; the same question id
  under a different content id is a separate item.
- The report's question rows carry `content_id`; the human report qualifies a row or attempt with its
  content id only when it differs from the report content.
- The append-only triggers also reject an id-reusing `INSERT OR REPLACE`; the schema is version 2,
  so an existing version-1 database gains that trigger and the composite index on open.
- `--json` flag-parse or extra-argument errors emit the documented JSON error document.
