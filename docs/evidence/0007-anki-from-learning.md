# 0007 — Useful Anki notes created during study: evidence

- Tree: working tree on top of `0d81987029ad7a0135aebdacc6aa20c8e5ff9018` (`docs(0007): activate the
  Anki note contract with disclosed defaults`). The operator owns the commit; this run did not
  commit, push or open a pull request.
- Date: 2026-11-11. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3; Node v24.16.0; Bun 1.4.4.
- Contract: [`../goals/archive/0007-anki-from-learning.md`](../goals/archive/0007-anki-from-learning.md).
  Plan: [`../plans/2026-11-11-0001-0007-anki-from-learning.md`](../plans/2026-11-11-0001-0007-anki-from-learning.md).
  Behaviour: [`../notes.md`](../notes.md).

The demonstration used `/tmp/passquest-0007-demo/`, a locally built binary, the real
`agent/anki-host.mjs` and a loopback AnkiConnect stub on `127.0.0.1:18765`. The stub made no external
network call and no real deck was involved. The provider refinement used the real
`agent/tutor-host.mjs` against a loopback OpenAI-compatible stub on `127.0.0.1:18767` with the
synthetic credential `test-key-not-real`. Nothing from the demonstration was added to the repository;
the databases, stub logs and exports stayed under `/tmp`.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/note` | Target identity, deterministic key-safe draft, consent withholding, exact field HTML, the note model/CSS/card template, atomic HTML/TSV export, refinement prompt parsing. No database, no network. |
| `internal/anki` | AnkiConnect action allowlist, loopback endpoint validation, host request/response validation, duplicate/not-found classifiers, payload builders, Node transport. |
| `agent/anki-host.mjs` | The only Anki egress: one versioned document, 1–50 allow-listed actions, loopback-only, redirects refused, 1 MiB response cap, no database or tool access. |
| `internal/store` | Schema v10: `note_drafts`, `notes`, append-only `note_revisions`, one-row `note_outbox`, append-only `anki_push_attempts`, append-only `note_refinements`. No table can update an attempt, source, question or answer key. |
| `internal/cli` | `note propose/preview/accept/skip/show/model/export/push`, JSON/human documents, consent checks, revision equality, outbox orchestration and conflict/queue handling. |
| `harness/gates/note-smoke.ts` | Real binary + real Anki host + loopback stub; 802 assertions across draft, consent, idempotency, revisions, export, model, add, update, duplicate, conflict, queue/drain, revoked consent, missing Node and host validation. |

## Draft and consent

```bash
passquest import --path internal/cli/testdata/import --db demo.sqlite
passquest question add SRC 1 --file internal/cli/testdata/import/question-q2-tagged.json --db demo.sqlite
passquest page review SRC 1 --reviewer demo --db demo.sqlite
passquest question review SRC q2 --answer C --reviewer demo --db demo.sqlite
passquest practice --source SRC --page 1 --question q2 --answer A --db demo.sqlite
passquest note propose --attempt 1 --db demo.sqlite --json
```

| Observation | Value |
|---|---|
| Draft id / target | 1 · `src-bb4d9a5479518365-p001` · `q2` · attempt 1 |
| Origin / restricted / consent | `deterministic` / `true` / `false` |
| Tactic | `not-except-polarity` → field `NOT/EXCEPT polarity` |
| Source field | `src-bb4d9a5479518365 · p001 · q2` |
| Withheld before consent | `front`, `back`; both field values empty |
| `note accept` before consent | exit 2, `consent_required`; no note row |
| Preview before consent | no `Westbridge Community Library` text in the document |

After `passquest tutor consent SRC --reviewer demo`:

| Field | Value |
|---|---|
| Front | `The library opens at 8 a.m. on Sundays.` |
| Back | `No Sunday opening time appears in the notice.` + blank line + `Example: The Westbridge Community Library closes at 4 p.m. on Saturdays.` |
| Tactic | `NOT/EXCEPT polarity` |
| Source | `src-bb4d9a5479518365 · p001 · q2` |
| `html_sha256` | `cdb5d55b990873e201c2254b2c4ad55908156fc5be0e87b2e56e91cf9cf3e549` |

## Accept semantics and revision history

| Step | Observed |
|---|---|
| First `note accept` | `accepted`, note `note-e7f54e2c40bf7326`, revision 1, `created:true`, `changed:true`, previous 0 |
| Repeat `note accept` | `unchanged`, same note id, revision 1, `created:false`, `changed:false`, previous 1 |
| Edited accept (`--front "closes on Sundays" --back "A closing time.\n\nExample: …"`) | `accepted`, same note id, revision 2, previous 1 |
| `note show` | one note; revisions `[1, 2]`; revision 1 origin `deterministic`, revision 2 `edited`; revision 1 hash `1b7a235ffb38f028…`, revision 2 `eea8992dcfb80633…` |
| Attempt report before/after a later note accept | `cmp` exit 0 (byte-identical); attempts 1, incorrect 1, independent 1 |

## Unanswered question

A reviewed but unattempted q1 was proposed with `--mark "closes"`:

| Observation | Value |
|---|---|
| `unanswered` | `true` |
| Front | `closes` |
| Back | `This item tests: When does the library close on Saturdays?` + blank line + `Example: When does the library close on Saturdays?` |
| Key in the preview JSON | `At 4 p.m.` absent, `at 4 p.m.` absent, `opens at 8 a.m. on Sundays` absent, `No Sunday opening time` absent |

The key is read only to reject a passage sentence that would reproduce the keyed option or the
evidence sentence; the note falls back to the visible stem.

## Export

```bash
passquest note export --db demo.sqlite --json
```

| Observation | Value |
|---|---|
| Path | `/tmp/passquest-0007-demo/passquest-notes.tsv` |
| Notes / bytes / sha256 | 1 / 299 / `f3afb50cd3e13df58ab49558907604690056404b6883b3fde9d81ab580257fc2` |
| Header | `Front\tBack\tTactic\tSource` + `#separator:tab`, `#html:true`, `#notetype:PassQuest Note`, `#deck:Pass Quest`, `#tags:passquest` |
| Row | `closes on Sundays` · `<div class="pq-meaning">A closing time.</div><div class="pq-example">Example: …</div>` · `NOT/EXCEPT polarity` · source |
| Failure path (`--out outdir`, an existing directory) | exit 1, `internal_error`; the message names the failed rename; no `.outdir.tmp-*` remained |
| Repeat export to the same path | replaced atomically; content changed |

## AnkiConnect push

### Create, add and no-op

First push (`--endpoint http://127.0.0.1:18765`):

| Observation | Value |
|---|---|
| Exit / state | 0 / `pushed` |
| Model created | `true` |
| Counts | pending 1, pushed 1, already present 0, conflicts 0, failed 0, queued 0 |
| Note | revision 2 → Anki note 101 |
| Actions | `version, createDeck, modelNames, createModel, addNote` |
| `createModel` | model `PassQuest Note`; fields `Front, Back, Tactic, Source`; CSS has the system font stack and no `overflow: hidden`/external URL; card template carries all four field placeholders |
| `addNote` | four fields only; `options.allowDuplicate:false` |

An immediate second push: `idle`, 0 endpoint actions.

### Revision updates the same id

After an edited accept:

| Observation | Value |
|---|---|
| Actions | `version, createDeck, modelNames, notesInfo, updateNoteFields` |
| Update target | note id 101; no `addNote` |
| Stub state | one note, front `closes at 4 p.m.` |
| Result | revision 3, state `pushed` |

### Conflict, then explicit force

The stub's note 101 `Back` was changed to `learner edited this in Anki`, then a new local revision was
accepted.

| Observation | Value |
|---|---|
| Plain push | exit 3; state `conflict`; conflicts 1; detail `Anki fields changed since the last push; use --force to overwrite` |
| Actions | `version, createDeck, modelNames, notesInfo`; **no** `updateNoteFields` |
| Outbox | the revision stayed pending (SQLite: status `pending`) |
| `note push --force` | exit 0; state `pushed`; actions `version, createDeck, modelNames, updateNoteFields`; Anki note 101 overwritten |
| Ledger | revision 4 recorded `check/conflict`, then `update/ok` |

### Duplicate and duplicate revision

With the stub returning `cannot create note because it is a duplicate` for `addNote`:

| Observation | Value |
|---|---|
| First add of a new note | exit 0; state `already_present`; already present 1; pushed 0; detail `already present in Anki` |
| Actions | `version, createDeck, modelNames, addNote` |
| Later revision of that duplicate-only note | exit 3; state `conflict`; detail `Anki already holds a note with this front; its note id is unknown`; no update |
| Ledger | note `note-68ce01a37fcd19d7`: revision 1 `add/duplicate`, revision 2 `add/conflict` |

### Queue and single drain

A local revision was accepted, then pushed to `http://127.0.0.1:1` (nothing listening):

| Observation | Value |
|---|---|
| Queued push | exit 0; state `queued`; queued 1; 0 actions reached the stub |
| Outbox | pending, attempts 1, last error `endpoint did not answer` |
| Next push to the stub | exit 0; state `pushed`; **one** `updateNoteFields`; 5 actions |
| Push immediately after | `idle`; 0 actions; no second write |
| Ledger | revision 5 `check/unreachable`, then `update/ok` |

### Consent revoke and missing Node

| Scenario | Observed |
|---|---|
| Accept a revision, then `tutor revoke`, then push | exit 2, `consent_required`; 0 endpoint actions; outbox unchanged |
| Re-consent, then push with `PASSQUEST_NODE=/tmp/passquest-0007-demo/missing-node` | exit 3, `anki_unavailable` (`fork/exec … no such file or directory`); pending rows kept |
| Pending rows after that failure | note `note-68ce01a37fcd19d7` revision 2 and note `note-e7f54e2c40bf7326` revision 6 |

### Host validation

The real host was invoked directly with a remote endpoint and with an unknown action. Both returned
`state:"failed"`; the remote message names loopback; neither contacted a remote host.

## Provider refinement

```bash
passquest tutor config --endpoint http://127.0.0.1:18767/v1/chat/completions --model demo-model --db refine.sqlite
PASSQUEST_TUTOR_API_KEY=test-key-not-real \
PASSQUEST_TUTOR_HOST=agent/tutor-host.mjs \
passquest note propose --attempt 1 --refine --db refine.sqlite --json
```

| Observation | Value |
|---|---|
| Exit / origin / advisory | 0 / `refined` / `true` |
| Front / back | `closes on Saturdays` / `Shut on the stated day.` + blank line + `Example: …` |
| Refinement record | id 1, model `demo-model`, request sha256 `6f1af2e65feeed40…`, 1318 bytes, 220 prompt tokens, 24 completion tokens, 81 ms |
| `tutor history --json` | `calls:0`, `daily_calls:1`, `daily_limit:20` — the shared budget counts the refinement, the coaching list does not |
| Reservation rows after finalize | 0 |
| Notes created | 0 |

## Model and template

`passquest note model --json`:

| Check | Observed |
|---|---|
| Deck / model / fields | `Pass Quest` / `PassQuest Note` / `Front, Back, Tactic, Source` |
| System font stack | `font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif` |
| Long text | `max-width: 42rem`, `overflow-wrap: anywhere`, `word-break: break-word`; no `overflow: hidden`, no fixed height |
| External reference | none of `http://`, `https://`, `@import`, `url(`, `<script`, `<link`, `<img` in CSS or templates |
| Rendered sample | contains `pq-front`, `pq-meaning`, `pq-example`, `pq-tactic`, `pq-source` |

## Store boundary

SQLite after the demonstration:

```text
attempts            1
note_revisions      note-e7f54e2c40bf7326: 1,2,3,4,5,6
                    note-68ce01a37fcd19d7: 1,2
attempts unchanged  report --json byte-identical before/after a later note accept
outbox              pending rows only for undelivered revisions; delivered revisions are `drained`
```

The v9→v10 migration test opens a v9 fixture, keeps its attempt, accepts a note, and verifies the
append-only triggers still reject an attempt update.

## Gates

`bash scripts/gate.sh` (real output; the final run is quoted here):

```text
$ bun run harness/gate.ts

== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (69 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (14 packages, 552 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (116 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (378 files, 0 findings)

== local-only CLI
PASS  local-only CLI (13 first-party packages, 0 network imports, 0 confidential markers)

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
PASS  note smoke (802 assertions)

== portable paths
PASS  portable paths (69 Go files, no platform-bound path literal)

== harness self-test
PASS  harness self-test (29 cases, 17/17 gates)

18 passed, 0 failed, 0 skipped
GATE: PASS
```

The gate runs 18 modules. `note-smoke` reports `802 assertions`; `harness self-test` reports
`29 cases, 17/17 gates`.

## Limitations

- **No real Anki Desktop or AnkiWeb verification.** This host has no Anki binary and nothing answers
  `127.0.0.1:8765`; CI cannot reach a learner's desktop on any OS. The adapter is proven against a
  loopback AnkiConnect stub. A real deck write, a real duplicate, a real read-back and AnkiWeb sync
  remain owner-supplied evidence.
- **3-OS CI not run here.** Only Linux was executed locally. The gate uses no POSIX-only assertion in
  its note path; the export failure test in Go skips on Windows because POSIX directory modes do not
  exist there. The repository workflow remains the 3-OS proof.
- **Export is not a revision channel.** Anki's text importer has no note id; re-importing an edited
  export can create a duplicate card. `push` is the revision path.
- **Duplicate without an id cannot be updated.** AnkiConnect reports no note id with a duplicate
  error, so a later revision of a duplicate-only note is a conflict, not an update.
- **The provider refinement shares the tutor daily budget.** `tutor history` shows the shared count
  but lists coaching calls only; the refinement row and its usage live in `note_refinements`.
- **Private text stays local until consent.** A note accepted after consent is not deleted by a later
  revoke; a later export or push is blocked, but the local note and its revisions remain.
- **No Anki clone.** No scheduling, `.apkg`, images, audio, AnkiWeb API or phone push exists, and no
  output claims one.

## Contract clause → evidence

| Clause | Evidence |
|---|---|
| 1. Draft from a stored attempt with front/back/tactic/source/restricted | `note propose --attempt 1` table; draft target, fields and `restricted:true`; JSON above |
| 2. Preview is the exact payload; unanswered key hidden; restricted text waits for consent | before/after consent table and field values; `accept` exit 2 `consent_required`; unanswered preview key-absence checks |
| 3. Repeated accept is a no-op; an edit bumps revision and keeps history | accept table: revisions 1, 1, 2; `note show` history `[1,2]` |
| 4. Export is importable text, written atomically | export header/row/bytes/sha256; directory-target failure exit 1 with no temp file; repeat replacement |
| 5. AnkiConnect create model/deck, add, duplicate, update-not-re-add, queue-and-drain | action sequences and counts above; `note-smoke` 802 assertions |
| 6. Model template is mobile-safe and self-contained | `note model` table; CSS/template checks; `createModel` payload |
| 7. No output claims a phone push | `note-smoke` scans every captured output for 8 forbidden phrases and asserts the manual-sync statement; human preview assertion |
