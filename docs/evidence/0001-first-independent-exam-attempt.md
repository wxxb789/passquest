# 0001 — First independent exam attempt: evidence

- Tree: working tree over `0f904b0` (staged, not committed), branch `feat/0001-independent-exam-attempt`.
- Date: 2026-11-10. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3 via mise; `CGO_ENABLED=1` in the environment but no cgo package is imported;
  SQLite is `modernc.org/sqlite` v1.62.0 (pure Go).
- Contract: [`../goals/archive/0001-first-independent-exam-attempt.md`](../goals/archive/0001-first-independent-exam-attempt.md)
  (moved unchanged at close). Plan: [`../plans/2026-11-10-0044-0001-first-independent-exam-attempt.md`](../plans/2026-11-10-0044-0001-first-independent-exam-attempt.md).

All commands below were run from the repository root with
`export PATH="$HOME/.local/share/mise/shims:$PATH"`. The demonstration used a fresh temporary
directory (`/tmp/passquest-0001-demo`), a fresh database, and a separately built binary.

## What was built

A real CLI (`cmd/passquest`) over four first-party packages:

| Package | Truth it owns |
|---|---|
| `internal/exam` | Self-authored passage + three four-option questions, key, verbatim passage evidence, one-line reasoning and a distractor explanation per option. Embedded in the binary; no runtime file or network lookup. |
| `internal/attempt` | Deterministic grading, selection parsing (`A`–`D`, `skip`), snapshotting selected/correct text and elapsed time, and the feedback/exposure model. |
| `internal/store` | One SQLite table `attempts` (`PRAGMA user_version = 1`), idempotent migration, per-question attempt numbering inside the insert transaction, and triggers that abort `UPDATE`/`DELETE`. |
| `internal/cli` | `practice`, `report`, `version`; human and `schema_version: 1` JSON protocols; `$PASSQUEST_DB` / `./.passquest/passquest.sqlite` default; `0600` database in a `0700` directory. |

The exercise is a library-opening-hours notice written for this repository. It is not drawn from any
live examination, and no exam blueprint, time limit, pass threshold or readiness estimate is encoded.

## Tier 1 — automated checks

| Command | Result |
|---|---|
| `go test ./... -count=1 -v` | exit 0; 34 top-level tests, 37 subtests, 0 failures, 0 skips (re-measured 2026-11-10 after the simplification pass; the original increment measurement was 26/26 at `a13772e`) |
| `go vet ./...` | exit 0 |
| `go mod tidy -diff` | empty diff, exit 0 |
| `gofmt -l` (tracked `*.go`) | empty output |

The tests that pin the contract clauses:

| Test | Clause it pins |
|---|---|
| `TestAwaitingJSONHidesTheKey` | `awaiting_answer` JSON has no `answer`, `correct_*`, `evidence`, `reasoning`, `status` or `feedback` key; no stored attempt |
| `TestHumanPromptDoesNotRevealTheKey` | Human prompt has no `Correct answer:`, `Result:`, `Why:` or `Recorded attempt`; EOF records nothing |
| `TestCorrectIncorrectSkipAndRepeat` | Correct/incorrect/skipped records, `attempt_number`, `prior_attempts`, `repeat`, first/repeated exposure, report counts |
| `TestInvalidInputRecordsNothing` | Invalid label, unknown question and conflicting flags exit 2 and store 0 attempts |
| `TestInteractiveInput` | Lowercase answer, invalid-then-skip loop, `quit` stores nothing |
| `TestProcessReopenSeesStoredEvidence` | Writes in one OS process, reads in a second OS process, same attempt id and session |
| `TestGoldenAwaitingJSON`, `TestGoldenReportJSON` | Protocol golden files (`internal/cli/testdata/*.golden.json`) |
| `TestAttemptsAreAppendOnly` | `UPDATE` and `DELETE` are rejected by SQLite triggers; count unchanged |
| `TestMigrationKeepsOlderDatabaseAndIsIdempotent` | Version-0 database with unrelated data migrates to 1, data preserved, second open idempotent |
| `TestOpenRejectsNewerSchema` | Version 99 is refused instead of silently misread |
| `TestOpenCreatesSchemaAndProtectsFiles` | `0600` file, `0700` new directory |
| `TestSampleIsValidAndEvidenceGrounded` | Every evidence quote is a verbatim passage substring; 3 questions, 4 options, every option explained |
| `TestSampleMakesNoConfidentialOrLiveExamClaim` | Sample contains no `CRSC`/internal-only markers and states self-authored/not-live-exam provenance |
| `Exam`/`attempt` validation tables | Bad keys, evidence, labels, status and timestamps are rejected |

The test count is deliberately not the evidence that the product works; the Tier 2 transcript is.

## Tier 2 — real CLI demonstration

Commands (each invocation is a separate process):

```bash
demo=/tmp/passquest-0001-demo
go build -o "$demo/passquest" ./cmd/passquest
db="$demo/passquest.sqlite"

"$demo/passquest" practice --db "$db" --question q1 </dev/null
"$demo/passquest" practice --db "$db" --json --question q1
"$demo/passquest" practice --db "$db" --answer B
"$demo/passquest" practice --db "$db" --answer A
"$demo/passquest" practice --db "$db" --skip
"$demo/passquest" practice --db "$db" --question q1 --answer B
"$demo/passquest" report --db "$db"
```

### Pre-answer prompt (stdin closed, no attempt recorded)

```text
Pass Quest — practice (sample-0001 v1)
Westbridge Library notice

Notice: Self-authored practice sample. Not from any live exam; not an exam blueprint or a claim about exam content, difficulty or pass threshold.

Passage: Westbridge Community Library — opening hours
The Westbridge Community Library will change its opening hours next month. From 3 March, the library will open at 9 a.m. and close at 7 p.m. from Monday to Friday. On Saturdays it will close at 4 p.m., two hours earlier than before. The library will stay closed on Sundays.

The change follows a six-month survey of visitors. Most people who answered the survey said they wanted longer weekday hours after work. The library director, Ms. Chen, said the Saturday change was necessary because few people visited late on Saturdays and the library needs to save money for new computers.

During the first week of March, the library will offer a free class on using the online catalogue. The class is limited to twelve people, and visitors should register at the front desk. Anyone who cannot attend on a weekday may join the same class on Saturday morning.

Question 1 of 3 (q1)
What time will the library close on Saturdays from 3 March?

  A. At 7 p.m.
  B. At 4 p.m.
  C. At 8 p.m.
  D. At 9 a.m.

Your answer (A, B, C, D; skip or quit):
No attempt recorded.
```

The key is not marked: the four options are printed neutrally and the evidence/reasoning strings
appear only after an attempt. The passage necessarily contains the fact the question asks about; that
is the reading task, not an answer disclosure.

Awaiting JSON (fresh process, no answer): the document has exactly the keys
`content`, `question`, `schema_version`, `state`; the question has exactly
`id`, `number`, `options`, `passage`, `stem`, `total`. No key/evidence/reasoning/status field exists.
`report --json` immediately afterwards reported `attempts: 0`.

### Correct attempt (q1, answer B)

```text
Result: CORRECT
Your answer: B — At 4 p.m.
Correct answer: B — At 4 p.m.
Evidence: "On Saturdays it will close at 4 p.m., two hours earlier than before."
Why: The notice gives the new Saturday closing time directly: 4 p.m., two hours earlier than the old time.
First attempt on this question; no earlier answer or evidence was shown.
Recorded attempt #1 (question q1, attempt 1 on this question, 0 ms).
Database: /tmp/passquest-0001-demo/passquest.sqlite
```

### Incorrect attempt (next question q2, answer A; key B)

```text
Result: INCORRECT
Your answer: A — The library wants to open on Sundays instead.
Why your answer is wrong: The notice says the library will stay closed on Sundays; it does not plan to open then.
Correct answer: B — Few people visited late on Saturdays and the library needs to save money.
Evidence: "few people visited late on Saturdays and the library needs to save money for new computers."
Why: Both reasons are stated in one sentence: low late-Saturday attendance and saving money for new computers.
First attempt on this question; no earlier answer or evidence was shown.
Recorded attempt #2 (question q2, attempt 1 on this question, 0 ms).
```

### Skipped attempt (next question q3)

```text
Result: SKIPPED (no answer chosen)
Correct answer: B — By registering at the front desk.
Evidence: "The class is limited to twelve people, and visitors should register at the front desk."
Why: The notice states the class is free and that visitors should register at the front desk; no fee or call is mentioned.
First attempt on this question; no earlier answer or evidence was shown.
Recorded attempt #3 (question q3, attempt 1 on this question, 0 ms).
```

### Repeated attempt (q1 again, answer B)

```text
Result: CORRECT
Your answer: B — At 4 p.m.
Correct answer: B — At 4 p.m.
Evidence: "On Saturdays it will close at 4 p.m., two hours earlier than before."
Why: The notice gives the new Saturday closing time directly: 4 p.m., two hours earlier than the old time.
Repeated attempt: the correct answer and evidence were shown on an earlier attempt, so this is not independent evidence.
Recorded attempt #4 (question q1, attempt 2 on this question, 0 ms).
```

### Report from a new process

```text
Pass Quest — attempt evidence (sample-0001 v1)
Westbridge Library notice

Attempts: 4 (correct 2, incorrect 1, skipped 1)
First attempts: 3 (correct 1, incorrect 1, skipped 1)
Repeated attempts (answer already shown): 1 (correct 1, incorrect 0, skipped 0)

Questions
  q1  attempts 2  first correct  latest correct  repeated 1
  q2  attempts 1  first incorrect  latest incorrect  repeated 0
  q3  attempts 1  first skipped  latest skipped  repeated 0

Attempts
  #1  q1  correct  first  selected B  0 ms  session 2cb913a02d6b56fe
  #2  q2  incorrect  first  selected A  0 ms  session deacbb70278f68b2
  #3  q3  skipped  first  selected —  0 ms  session edc35840e7fbc432
  #4  q1  correct  repeat  selected B  0 ms  session cdc36be186c4bfe1

Limitations
  - Counts cover only this self-authored sample; they are not an exam score, pass estimate or readiness claim.
  - Repeated attempts come after the correct answer and evidence were shown and are not independent evidence.
  - The exam blueprint, time limit and pass threshold are unknown, so no pass probability is computed.

Database: /tmp/passquest-0001-demo/passquest.sqlite
```

The four different session ids are the four separate processes; the report still sees all four
attempts because they are durable rows, not in-memory state.

### Direct SQLite inspection after all processes exited

```text
user_version: 1
tables: ['attempts', 'sqlite_sequence']
triggers: ['attempts_append_only_delete', 'attempts_append_only_update']
rows (id | question | status | selected | key | elapsed_ms):
  (1, 'q1', 'correct', 'B', 'B', 0)
  (2, 'q2', 'incorrect', 'A', 'B', 0)
  (3, 'q3', 'skipped', None, 'B', 0)
  (4, 'q1', 'correct', 'B', 'B', 0)
UPDATE rejected: attempts are append-only
DELETE rejected: attempts are append-only
```

`stat -c '%a'` on the database returned `600`.

## Tier 3 — local-only and source-boundary checks

| Check | Command | Result |
|---|---|---|
| First-party network imports | `go list -f '{{.ImportPath}}:{{range .Imports}} {{.}}{{end}}' $(go list ./cmd/... ./internal/...)` | 5 first-party packages, 0 `net`/`net/http`/`net/rpc`/`net/smtp` imports |
| Linked HTTP/RPC/SMTP clients | `go list -deps ./cmd/passquest` | no `net/http`, `net/rpc`, `net/smtp` |
| Runtime network syscalls | `strace -f -e trace=network -o net.log passquest report ...` (also practice) | 0 `socket`/`connect`/`sendto`/`recvfrom`/`bind` lines; the log contains only process-exit lines |
| Confidential markers in runtime files | `bash scripts/check-local-only.sh` | 0 hits in `cmd/` and `internal/` (tests are excluded because one test asserts absence) |
| Sample provenance | `TestSampleMakesNoConfidentialOrLiveExamClaim` + CLI notice output | self-authored, "not from any live exam", "not drawn from a live examination"; no `CRSC`/internal-only marker |
| Seed/tracked private state | `bash scripts/check-secrets.sh` | clean; no `.sqlite`, `.pdf` or local-state file tracked |

`net` itself is linked transitively by `modernc.org/sqlite` → `modernc.org/libc` →
`github.com/google/uuid`. No Pass Quest code imports or calls it; the strace run above is the runtime
proof that the CLI makes no network call. This is a known structural limit of the chosen pure-Go
SQLite driver, not a hidden feature.

## Gate results

`bash scripts/gate.sh` — a full run on this tree, exit code 0 (PASS on all 8 gates; 0 failures,
0 skips). Test timings vary between identical runs; the PASS lines, counts and exit code are the
invariant.

```text
== gofmt
PASS  gofmt

== go vet
PASS  go vet

== go build
PASS  go build

== go test
?   	github.com/wxxb789/passquest/cmd/passquest	[no test files]
ok  	github.com/wxxb789/passquest/internal/attempt	0.003s
ok  	github.com/wxxb789/passquest/internal/cli	0.459s
ok  	github.com/wxxb789/passquest/internal/exam	0.004s
ok  	github.com/wxxb789/passquest/internal/store	0.365s
PASS  go test

== go mod tidy -diff
PASS  go mod tidy

== docs links
docs links: 41 checked, 0 broken
PASS  docs links

== secrets/sources
secrets/sources: clean (45 tracked files)
PASS  secrets/sources

== local-only CLI
local-only: 5 first-party packages, 0 network imports, 0 confidential markers
PASS  local-only CLI

GATE: PASS
```

Re-measured after the simplification pass (2026-11-10, tree at `ff80f2b` plus the uncommitted apply
diff): `bash scripts/gate.sh` → exit 0, 8/8 `PASS`, 0 `SKIP`; docs links 45 checked/0 broken;
secrets/sources clean (214 tracked files); local-only 5 first-party packages. The 41/45 counts in
the transcript above are the original increment measurement at `a13772e`; owner commits `5e89188`
and `4a861ca` added docs and data before this re-measurement.

## Contract clause → evidence

| "Done when" clause | Evidence |
|---|---|
| A learner can answer an actual, coherent multiple-choice exercise without being shown the answer, and see deterministic correctness feedback plus concise evidence-based reasoning. | Pre-answer prompt and awaiting JSON above (no key fields); correct/incorrect/skip responses with key, verbatim passage evidence and authored reasoning; `TestAwaitingJSONHidesTheKey`, `TestHumanPromptDoesNotRevealTheKey`, `TestSampleIsValidAndEvidenceGrounded`, awaiting golden. |
| Correct, incorrect, skipped and repeated attempts are represented truthfully, with no fictional scores or knowledge claims. | Stored rows and report above: 4 attempts, first 3 vs repeated 1, statuses 2/1/1, skip has `NULL` selection; report limitations; `TestCorrectIncorrectSkipAndRepeat`, `TestAttemptsAreAppendOnly`. No percentage, pass estimate or mastery claim is produced. |
| A fresh local invocation demonstrates the end-to-end attempt and shows durable evidence after reopening the app/process. | Four separate CLI processes wrote attempts; a fifth `report` process read them; SQLite dump after exit; `TestProcessReopenSeesStoredEvidence`; `TestMigrationKeepsOlderDatabaseAndIsIdempotent`. |
| The demonstration contains no original confidential CRSC text, no external calls and no claim that the sample is from the live exam. | Self-authored sample and provenance notice; marker test and `check-local-only.sh`; 0 network syscalls under strace; notice says "Not from any live exam". |

## Known limits and residual risks

1. **One sample, three questions.** Counts describe this sample only. The key is authored for this
   repository and machine-checked against the passage; it is not an externally verified live-exam
   key (none exists for self-authored content). A real key review belongs to the import goal 0002.
2. **No exam claim.** The blueprint, time limit and pass threshold remain unknown; the report prints
   counts and limitations, never a score, percentage, pass probability or readiness claim.
3. **No timer or time budget.** `elapsed_ms` is wall-clock from just before the prompt to submission
   (0 ms for flag-driven demo answers). It is recorded, not enforced.
4. **Repeats are visible, not prevented.** A repeated attempt after the key was shown is labelled and
   excluded from first-attempt counts. Question id is the stable identity across content versions;
   a content rewrite that reuses an id inherits its exposure history (see Q-0001-01).
5. **Local single learner.** No account, encryption or multi-device sync. The database is `0600` in a
   `0700` directory, which protects against other OS users but not an administrator or a compromised
   account. It is gitignored.
6. **`net` is linked but not called.** The pure-Go SQLite driver chain pulls it in; strace shows zero
   network syscalls on this Linux x64 host. This is not a cross-platform network proof.
7. **Cold module cache not re-proven locally.** `go.mod`/`go.sum` pin the dependency graph and
   `go mod tidy -diff` is clean; the cold-cache build is CI's job per `docs/harness.md`. The local
   module cache was warm.
8. **Human prompt is line-oriented.** Only `A`–`D`, `skip` and `quit` are accepted; EOF before a
   valid answer records nothing. There is no mouse/UI surface until goal 0011.
9. **Archive link.** The contract moved unchanged; `docs/goals/core-design.md` is a one-line redirect
   so its `../core-design.md` link still resolves from the archive. Delete the redirect if the owner
   prefers an archive link exemption instead.

## Reproduction

```bash
export PATH="$HOME/.local/share/mise/shims:$PATH"
go test ./... -count=1
bash scripts/gate.sh
```

The full transcript above can be regenerated by following the Tier 2 commands with a fresh
`demo=$(mktemp -d)` directory.
