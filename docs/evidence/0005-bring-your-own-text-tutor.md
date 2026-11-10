# 0005 — Helpful AI tutor with user-supplied text model: evidence

- Tree: goal-0005 increment committed as `120812d50a258644850e2ee00ee984e01bfe8c26` on top of
  `4b68b80373410072b14b16125cf98b21888b5632`; the accepted simplify findings are applied uncommitted
  on top of it. The operator owns the commit.
- Date: 2026-11-10. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3 via mise; Node v24.16.0; Bun 1.4.4; `modernc.org/sqlite` v1.62.0 (pure Go).
  No real provider, API key or external network call was used.
- Contract: [`../goals/archive/0005-bring-your-own-text-tutor.md`](../goals/archive/0005-bring-your-own-text-tutor.md).
  Plan: [`../plans/2026-11-10-1512-0005-bring-your-own-text-tutor.md`](../plans/2026-11-10-1512-0005-bring-your-own-text-tutor.md).

Commands ran from the repository root unless a temporary demonstration directory is named. The
demonstration used `/tmp/passquest-0005-demo/`, a locally built binary and a loopback
OpenAI-compatible stub; nothing from the demonstration was added to the repository. The stub's text
is marked `Advisory marker 0005` so any leak into a report, recommendation or export is detectable.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/tutor` | Config validation (https/loopback, no userinfo/query/fragment, bounds), the `tutor-v1` system/user prompt, privacy classification, field clipping, request SHA-256, the versioned host request/response and the stdin/stdout Node transport. It imports no network package and writes no database. |
| `internal/store` | Schema v7: mutable non-secret `tutor_config`, append-only `tutor_consent_events`, append-only `tutor_calls`. Effective consent is the latest event; the daily count includes failures. Update/delete triggers reject rewriting either trail. |
| `internal/cli` | `tutor config/preview/ask/history/show/consent/revoke/consents`, attempt/content resolution, per-source consent gating, credential and budget checks, advisory rendering, failure rows and the new exit-3 path. |
| `agent/tutor-host.mjs` | The single network egress: endpoint validation, bearer auth from `PASSQUEST_TUTOR_API_KEY`, no redirects, timeout and 1 MiB body bound, response/usage validation and structured errors with the credential redacted. No database or tool access. |
| `harness` | `tutor-smoke` (126 assertions) against a loopback stub; a 21st self-test case over 14/14 gates. File-based gates now read tracked plus untracked non-ignored working-tree files so a pre-commit run scans new deliverables and survives a pending archive move. |

The model response is stored only in `tutor_calls` as advisory text. It is never read by `report`,
`next`, `practice`, an answer key or a question file. A call cannot append or update an attempt.

## Tier 1 — automated checks

| Command | Result |
|---|---|
| `go test ./... -count=1` | exit 0; 10 packages, 409 test runs, 409 passes, 0 skips, 0 failures (`go test -json` count) |
| `go vet ./...` | exit 0 |
| `go mod tidy -diff` | empty diff, exit 0 |
| `gofmt -l` (working-tree `*.go`) | empty output, exit 0 |
| `tsc --noEmit -p tsconfig.json` | exit 0, 0 type errors |
| `tutor-smoke` alone | pass, 126 assertions |
| `harness self-test` alone | pass, 21 cases, 14/14 gates |
| `bash scripts/gate.sh` | exit 0; final output below |

The tests that pin this increment's contract clauses:

| Test area | Clause it pins |
|---|---|
| `TestConfigValidation`, `TestTutorConfigRejectsCredentialInEndpoint`, `TestProviderHostOmitsPathAndQuery` | One endpoint shape; https off-loopback; loopback http; userinfo/query/fragment/relative/ftp rejected; model/timeout/token/budget bounds; only host:port is recorded in a call. |
| `TestBuildFirstPartyIsGrounded`, `TestBuildModesChangeOnlyTheTask` | The prompt carries the attempted question, verified key, evidence, authored reasoning, skill lesson and observed elapsed/assistance; explain/probe/micro-lesson have distinct bounded tasks. |
| `TestBuildPrivateWithoutConsentWithholdsEveryPrivateString`, `TestBuildPrivateWithConsentIncludesSourceText`, `TestBuildPrivateStoredTextWithoutConsentIsWithheld` | Imported page, stem, options, evidence, reasoning and stored option texts are absent without consent and present after a grant. |
| `TestBuildChangedContentUsesStoredTextOnly`, `TestBuildUnavailableContentDisclosesAndKeepsStoredLabels` | A changed or missing question is disclosed; stale current passage/evidence is not sent. |
| `TestBuildClipsLongPassageAndFields`, `TestBuildOmitsExtraLessonsWithinThePromptBound` | Passage is a bounded window around the evidence; long fields are marked truncated; extra multi-skill lessons are omitted with a local disclosure. |
| `TestBuildRequestHashIsStable`, `TestPreviewJSONShapeIsStable` | The canonical chat content has a stable SHA-256 and the host protocol is versioned. |
| `TestDecodeResponse`, `TestContainsUnsafeControl`, `TestHostTransportRejectsBadRequestBeforeSpawn` | Host responses are strictly decoded; unknown fields, wrong protocol, empty/oversized text, negative usage, trailing data and terminal-control text are rejected; an invalid request never spawns the host. |
| `TestMigrationFromVersionSixAddsTutorTables`, `TestTutorConfigRoundTripAndUpdate` | A v6 database keeps its attempts/exposures and gains the three tutor tables; config is mutable, validated and credential-free. |
| `TestTutorConsentLatestWinsAndIsAppendOnly` | Grant/revoke is append-only; the latest event wins; reviewer/source are required; UPDATE/DELETE are rejected. |
| `TestTutorCallRoundTripAndAppendOnly`, `TestTutorCallCountSinceUsesTheDayBoundary`, `TestAppendTutorCallRejectsMalformedRecords` | Call records round-trip with usage, filter by attempt, count at the UTC boundary and reject malformed rows; UPDATE/DELETE are rejected. |
| `TestTutorConfigInspectSaveAndPartialUpdate` | Configuration inspects, saves, partially updates, reports credential presence and refuses invalid values without changing stored config. |
| `TestTutorPreviewFirstParty`, `TestTutorAskSuccessPreservesAttemptsAndNeverStoresKey` | Preview is send-free and key-free; ask labels the text advisory, records usage and leaves the attempt report byte-identical. |
| `TestTutorPrivateSourceConsentRoundTrip`, `TestTutorConsentValidation` | Withheld → preview → consent → shared → ask → revoke → withheld; unknown source and missing reviewer exit 2. |
| `TestTutorProviderFailureIsRecordedAndPracticeStillWorks`, `TestTutorTransportFailureIsAgentUnavailable`, `TestTutorInvalidResponseIsRecorded` | Provider/auth/transport/invalid-response failures exit 3, record an error row, do not change the report and do not block practice. |
| `TestTutorAskRequiresCredentialAndBudget`, `TestTutorDailyCountCountsFailures` | Missing credential and exhausted budget make no provider call; failed calls consume the budget. |
| `TestTutorMissingNodeLeavesPracticeWorking` | A missing Node binary is `agent_unavailable` exit 3; local practice still records and grades. |
| `TestTutorCredentialIsRedactedFromProviderText`, `TestTutorCredentialIsRedactedFromProviderError`, `TestTutorTerminalControlResponseIsRecorded`, `tutor-smoke` host-stderr and provider-error modes, database byte scans | A credential echoed by provider text, a provider error or the host's stderr is redacted before storage/output; terminal escape sequences are refused; the SQLite bytes contain no key. |
| `TestTutorShowFailedCallHasNoSuggestion`, `TestTutorHistoryLabelsWithheldPrivateSource`, `TestTutorHistoryEmptyState` | Failures store no model text; history distinguishes first-party, shared-private and withheld-private calls; empty history is `[]`. |
| `TestTutorGolden*` | Raw config, first-party preview, withheld private preview, advisory answer, history and consent documents. |

The test count is not the evidence that the product works; the Tier 2 transcripts and the gate's
byte-level checks are.

## Tier 2 — real CLI transcripts

Binary: `/tmp/passquest-0005/passquest` built from the working tree. Stub: a local Node HTTP server
on `127.0.0.1` returning OpenAI-compatible JSON; no external network. The credential was the
non-secret string `demo-not-real-key`, supplied only through `PASSQUEST_TUTOR_API_KEY`.

### Configure and inspect

```text
$ passquest tutor config --endpoint http://127.0.0.1:38271/v1/chat/completions --model stub-model --db demo.sqlite
Pass Quest — tutor configuration
Provider endpoint: http://127.0.0.1:38271/v1/chat/completions
Model: stub-model
Timeout: 30000 ms
Max output tokens: 350
Daily call budget: 20 (UTC day, failed calls included)
Credential: PASSQUEST_TUTOR_API_KEY is set (value never shown or stored)

The agent host is the only component that contacts the provider; ordinary practice works without it.
```

### First-party attempt: preview is send-free, ask is advisory

```text
$ passquest practice --question q1 --answer B --db demo.sqlite
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

Result: CORRECT
Your answer: B — At 4 p.m.
Correct answer: B — At 4 p.m.
Evidence: "On Saturdays it will close at 4 p.m., two hours earlier than before."
Why: The notice gives the new Saturday closing time directly: 4 p.m., two hours earlier than the old time.
First attempt on this question; no earlier answer or evidence was shown.
Recorded attempt #1 (question q1, attempt 1 on this question, 0 ms, assistance independent).
Database: /tmp/passquest-0005-demo/demo.sqlite

$ passquest tutor preview --attempt 1 --db demo.sqlite
Pass Quest — tutor preview (not sent)
Attempt #1: sample-0001 / q1 — correct, 0 ms, independent
Mode: explain
Provider: stub-model @ http://127.0.0.1:38271/v1/chat/completions
Credential: PASSQUEST_TUTOR_API_KEY is set (value never printed)
Privacy: first-party material only; no private source text is involved
Provider request: 2506 bytes, sha256 1c106909d616215f76544367efd9032ff87a3ff2766c72bb6d743ab0ff5610ca
```

The provider request count was 0 after `preview`. `ask` then produced:

```text
$ passquest tutor ask --attempt 1 --db demo.sqlite
Pass Quest — advisory AI suggestion (not a verified answer)
Attempt #1: sample-0001 / q1 — correct, 0 ms, independent
Mode: explain
Model: stub-model @ 127.0.0.1:38271
Privacy: first-party material only; no private source text is involved
Usage: 812 prompt tokens, 96 completion tokens, 61 ms

--- AI suggestion (advisory) ---
Advisory marker 0005: locate the sentence that gives the Saturday closing time, then check the day word.
--- end AI suggestion ---

This is model output. It is not a verified answer, grade or exam fact. The recorded attempt and answer key are unchanged.
Advisory call #1 recorded; request sha256 1c106909d616215f76544367efd9032ff87a3ff2766c72bb6d743ab0ff5610ca.
```

The stub received one request at `/v1/chat/completions` with `Authorization: Bearer demo-not-real-key`,
`model: stub-model`, `max_tokens: 350`, two messages, and no credential in the JSON body. The
`report --json` document was byte-identical before and after the call; `report` and `next` did not
contain the advisory marker.

### Observed difficulty

A delayed interactive answer on a second attempt shows the attempt facts that reach the prompt:

```text
$ { sleep 0.45; printf 'A\n'; } | passquest practice --question q2 --db difficulty.sqlite
Result: INCORRECT
Your answer: A — The library wants to open on Sundays instead.
Recorded attempt #1 (question q2, attempt 1 on this question, 446 ms, assistance independent).

$ passquest tutor preview --attempt 1 --db difficulty.sqlite
Attempt #1: sample-0001 / q2 — incorrect, 446 ms, independent
...
Observed attempt: status=incorrect; selected=A; verified_key=B; elapsed_ms=446;
attempt_number=1; repeat=false; assistance=independent
```

The prompt includes the verified key, evidence, authored reasoning and elapsed time; the model does
not receive a pass estimate or a diagnosis as fact.

### Private imported source: withheld, authorized, revoked

The first-party import fixture was imported, question `q2` was added, page 1 and the answer `C` were
reviewed by `demo-owner`, and the question was practiced. The source id is deterministic:
`src-bb4d9a5479518365`.

```text
$ passquest tutor preview --db demo.sqlite
Attempt #2: src-bb4d9a5479518365-p001 / q2 — correct, 0 ms, independent
Privacy: private source src-bb4d9a5479518365 is NOT authorized; source text shared: no
Withheld: passage, question_stem, options, evidence, reasoning, selected_option_text, correct_option_text
Skills: not-except-polarity
Provider request: 3778 bytes, sha256 c279b6b2d2fa6e58baa68ae3c9aad55c20290870bf5fa972d553586298e7c9e2

page sentence present in prompt: false
question stem present in prompt: false
```

```text
$ passquest tutor consent src-bb4d9a5479518365 --reviewer demo-owner --db demo.sqlite
Tutor consent for src-bb4d9a5479518365 (import): authorized
Recorded by demo-owner at 2026-11-10T08:19:17.634831566Z.
`passquest tutor preview` now includes this source's page and question text in the provider payload.
The event is append-only; earlier decisions remain in the local audit trail.

$ passquest tutor preview --db demo.sqlite
Privacy: private source src-bb4d9a5479518365 is authorized; source text shared: yes
Provider request: 4258 bytes, sha256 8f4f7a07681047be2461320ec9d53bd7cb48c03b07a6ab4155976ba0120d592c

page sentence present in prompt: true
```

A subsequent `ask` sent 4258 bytes with the page sentence and question; its call record carried
`source_text_shared: true`. After `revoke --reviewer demo-owner`, the same preview contained no page
sentence and reported `consent_granted: false`. `tutor consents` showed the source with
`granted: false` and the latest reviewer/time.

### Failure and fallback

```text
$ # stub returns HTTP 500
$ passquest tutor ask --db demo.sqlite
passquest: provider_error: stub provider failure
Advisory call #3 recorded as a failure; no attempt or answer key was changed.
exit: 3

$ passquest practice --question q3 --answer B --db demo.sqlite
exit: 0  (a new attempt was recorded and graded)

$ PASSQUEST_NODE=/tmp/passquest-0005-demo/missing-node passquest tutor ask --db demo.sqlite
passquest: agent_unavailable: tutor: agent host failed: fork/exec .../missing-node: no such file or directory
Advisory call #4 recorded as a failure; no attempt or answer key was changed.
exit: 3
```

`tutor history` after these calls:

```text
Calls: 4 (ok 2, failed 2); tokens: 1624 prompt, 192 completion
Daily budget: 4/20 calls used (UTC)

  #1  attempt #1  sample-0001/q1  explain  ok  stub-model  61 ms  812/96 tokens  first-party
  #2  attempt #2  src-bb4d9a5479518365-p001/q2  explain  ok  stub-model  54 ms  812/96 tokens  source-text-shared
  #3  attempt #2  src-bb4d9a5479518365-p001/q2  explain  error provider_error (HTTP 500)  stub-model  63 ms  no tokens  source-text-withheld
  #4  attempt #3  sample-0001/q3  explain  error agent_unavailable  stub-model  0 ms  no tokens  first-party
```

The private-source failure is labelled `source-text-withheld`, not `first-party`: the call record
keeps the source id even when no text was shared. The Go test
`TestTutorHistoryLabelsWithheldPrivateSource` pins that distinction.

### Budget

A second database configured with `--daily-calls 1` accepted one call and refused the second before
contacting the stub:

```text
$ passquest tutor ask --db budget.sqlite
Pass Quest — advisory AI suggestion (not a verified answer)
Attempt #1: sample-0001 / q1 — correct, 0 ms, independent
Mode: explain
Model: stub-model @ 127.0.0.1:38271
Privacy: first-party material only; no private source text is involved
Usage: 812 prompt tokens, 96 completion tokens, 64 ms

--- AI suggestion (advisory) ---
Advisory marker 0005: locate the sentence that gives the Saturday closing time, then check the day word.
--- end AI suggestion ---

This is model output. It is not a verified answer, grade or exam fact. The recorded attempt and answer key are unchanged.
Advisory call #1 recorded; request sha256 1c106909d616215f76544367efd9032ff87a3ff2766c72bb6d743ab0ff5610ca.
Database: /tmp/passquest-0005-demo/budget.sqlite

$ passquest tutor ask --db budget.sqlite
passquest: 1 of 1 tutor calls are used today (UTC); raise the budget with passquest tutor config --daily-calls N
exit: 2
```

`history` reported `calls: 1`, `daily_limit: 1`.

### Credential and report boundary

The main and budget SQLite files did **not** contain `demo-not-real-key`; no captured stdout/stderr
did either. A Go test additionally returns the key inside provider text and asserts the stored text
contains `[redacted]` rather than the key. The final `tutor-smoke` repeats the database and output
scans with a runtime-generated `sk-` value.

## Tier 3 — boundary and limitations

- **Attempt integrity**: `tutor ask` writes only `tutor_calls`; the gate compares the entire
  `report --json` before and after successes and failures and requires byte equality. No attempt,
  answer key, question, skill snapshot or exposure is updated.
- **Network boundary**: `go list` still reports 0 first-party network imports; the Go CLI spawns the
  Node host. The gate uses a loopback stub and no external address. The host refuses redirects and
  non-loopback plain HTTP.
- **Credential boundary**: env-only, redacted from host errors/model text, scanned in SQLite bytes and
  all captured output. No key is in argv.
- **Private material**: imported page/stem/options/evidence/reasoning/option texts are absent from
  the prompt without a consent event, present after a grant, absent after revoke. The provider
  payload never includes source title/path, database path, endpoint path, consent reviewer or history.
- **Persistence**: v6→v7 migration keeps attempts; config/consent/call tables are validated and
  consent/call rows reject UPDATE/DELETE.
- **Real-provider limit**: no external provider was contacted; `tutor-smoke` uses a local stub with
  the OpenAI-compatible response shape. A live endpoint's TLS, quota, latency and content remain
  unverified until the operator configures one.
- **Credential limit**: the key lives in the environment until goal 0011's OS secret store. It must be
  exported per session; see Q-0005-01.
- **Prompt limit**: one attempt, ≤ 20 000 prompt characters, ≤ 350 output tokens by default,
  ≤ 12 000 returned characters, 30 s default timeout, 20 calls/day. Extra skill lessons are omitted
  with a disclosure. The model can still be wrong or misleading; its text is labelled advisory.
- **Consent granularity**: per source, not per excerpt. A learner can withhold the whole source or
  edit the local page/question.
- **No retry/streaming/durable state**: goal 0006. A failed call is recorded and the learner may ask
  again manually; no retry loop hides failures.

## Harness growth and reconciliation

- New `harness/gates/tutor-smoke.ts` (126 assertions) registers before `self-test`; the self-test
  gains a 21st case and reports 21 cases over 14/14 gates.
- `harness/lib/repo.ts` now lists tracked plus untracked non-ignored files that exist, so a
  pre-commit gate run scans new deliverables and does not fail on the pending goal move. `secrets`
  reports 0 findings over the expanded set; `docs-links` checks the new docs and evidence.
- `docs/harness.md` gains the tutor-smoke row, updated counts, the Node prerequisite and the
  loopback-stub statement. CI pins Node 22 with `actions/setup-node@v4`.
- `README.md`, `docs/tutor.md`, `docs/source-import.md`, `docs/adaptive.md`,
  `docs/goals/README.md`, `docs/decisions/OPEN-QUESTIONS.md` (Q-0005-01..04) and the goal archive are
  reconciled. The goal file moved unchanged: SHA-256
  `d06ac4b3c0475e678fc759b3ef2112c386865077ebdfb626a04083f5c07ad0fc`.

## Final gate

```text
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (41 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (10 packages, 409 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (89 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (328 files, 0 findings)

== local-only CLI
PASS  local-only CLI (10 first-party packages, 0 network imports, 0 confidential markers)

== baseline integrity
PASS  baseline integrity (165 pages (22 native, 143 OCR), 0 fixture references)

== import smoke
PASS  import smoke (35 assertions)

== tactic smoke
PASS  tactic smoke (70 assertions)

== adaptive smoke
PASS  adaptive smoke (120 assertions)

== tutor smoke
PASS  tutor smoke (126 assertions)

== harness self-test
PASS  harness self-test (21 cases, 14/14 gates)

15 passed, 0 failed, 0 skipped
GATE: PASS
```

Residual risks, with numbers:

- 409 Go test runs and 126 gate assertions cover the local behaviour; 0 external provider calls were
  made, so live-provider compatibility is unverified.
- The prompt contains one attempt, up to 20 000 characters and at most 350 requested output tokens;
  the stub returned fixed text, so real model usefulness and latency are unmeasured.
- Imported source consent is per source; a learner who authorizes a source shares any page/question
  selected by the attempt, up to the passage window.
- The credential is environment-only; if the environment leaks, Pass Quest cannot detect that.
