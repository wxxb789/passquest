# 0014 — A pushed note lands in the deck the CLI claims: evidence

- Tree: working tree on top of `6a02af0` (`docs(0014): activate the deck read-back contract`), which
  is based on `1bdbc7e`. The operator owns the commit; this run did not commit, push or open a pull
  request. The goal file moved unchanged to [`../goals/archive/0014-note-lands-in-the-declared-deck.md`](../goals/archive/0014-note-lands-in-the-declared-deck.md).
- Date: 2026-11-14. Host: Linux 7.0.0-1017-azure x64.
- Toolchain: Go 1.27.3; Node v24.16.0; Bun 1.4.4.
- Contract: [`../goals/archive/0014-note-lands-in-the-declared-deck.md`](../goals/archive/0014-note-lands-in-the-declared-deck.md).
  Plan: [`../plans/2026-11-14-0229-0014-note-lands-in-the-declared-deck.md`](../plans/2026-11-14-0229-0014-note-lands-in-the-declared-deck.md).
  Behaviour: [`../notes.md`](../notes.md). Finding and defaults: `Q-0014-01`..`Q-0014-04` in
  [`../decisions/OPEN-QUESTIONS.md`](../decisions/OPEN-QUESTIONS.md).

Every measurement below is a loopback AnkiConnect stub in this repository's harness or in a
throwaway comparison script under `/tmp`; no Anki Desktop, no real deck and no external network were
involved. Byte counts are those of the recorded run; a document carrying a `created_at`/`updated_at`
stamp can differ by one byte between runs, because RFC 3339 nanosecond fields vary in length. The
owner's real-machine observation (Anki Desktop 26.9.4 with a source-built AnkiConnect,
2026-11-13) is what the stub reproduces and stays owner-supplied evidence for the live path.

## What was built

| Package / area | Truth it owns |
|---|---|
| `internal/anki` | `cardsInfo` added to the action allowlist plus `CardsInfoParams`, `CardInfo`, `DecodeCardsInfo`; `NoteInfo.Cards` (the card ids `notesInfo` reports); `MaxDeckNameChars` (200). |
| `agent/anki-host.mjs` | `cardsInfo` allow-listed; still one versioned document, at most 50 actions, loopback only, redirects refused. |
| `internal/store` | Schema v16: `notes.anki_deck` (bounded at 200 characters) and `deck_mismatch` in both `notes.anki_state` and `anki_push_attempts.outcome`; `AnkiAttempt.Keep` (a read-only verdict that leaves the queue as the write left it), `AnkiAttempt.AnkiDeck` and `AnkiAttempt.SetDeck` (a completed deck read writes the whole claim, so a read that names no deck clears the name an earlier read recorded; no other attempt touches it). Both affected tables are rebuilt with an explicit copy and their append-only triggers recreated. |
| `internal/cli` | The deck verify pass after the write and read-back phases, `readNoteDecks`/`usableDeckName`, the `deck_mismatch` item state, `counts.deck_mismatch`, doc state `deck_mismatch`, envelope `anki_deck_mismatch`, exit 3, the human mismatch block with one stated action, and `note show`'s recorded and expected deck. |
| `harness/gates/note-smoke.ts` | The stub files a card in another deck on demand and answers `cardsInfo`; the gate asserts the read happened, the mismatch is named, the id survives, a repeat push writes nothing, the learner's move is confirmed, and `report`/`next`/`questions` stay byte-identical. 1140 assertions (806 before this increment). |
| `harness/gates/self-test.ts` | A 35th case: the product sources are copied into the fixture root and the deck comparison is removed from `internal/cli/note_anki.go`; note smoke must fail. |

Go-side coverage: `internal/anki/anki_test.go` (payload, decoder, null result, allowlist),
`internal/store/note_test.go` (v15→v16 migration with the widened vocabulary and intact triggers, and
the `anki_deck` bound asserted against `anki.MaxDeckNameChars`; `Keep` leaves the queue drained while a
non-`Keep` verdict still re-pends it; an unconfirmed check clears a recorded deck while a write and a
miss leave it alone), `internal/cli/note_test.go` (fake transport with decks and
cards, the `readNoteDecks` table with a foreign `cardsInfo` row, the full mismatch/revise/move
scenario, the unconfirmed read against a card the endpoint cannot name, and the readd after a card is
deleted between the read-back and the update),
`internal/cli/note_golden_test.go` plus `note-push-deck-mismatch.golden.json` and
`note-show-deck-mismatch.golden.json`.

Falsifiability of those two cases: with `SetDeck` removed from the check attempt, the store case fails
on the note row (`AnkiDeck: Default` beside `anki_state: deck_mismatch`) and the CLI case fails on
`note show` printing `anki=deck_mismatch deck=Default (expected Pass Quest)` for a card the endpoint
never named; with it restored both pass.

## Acceptance 1 — a conforming endpoint is unaffected

Baseline: `git archive HEAD | tar -x -C /tmp/pq-old` and both binaries built with
`go build -o … ./cmd/passquest`. `/tmp/deck-compare.ts` then ran the same sequence against one stub
per binary: `import` → `question add` → `page review` → `question review` → `practice` →
`tutor consent` → `note propose` → `note accept` → push, then a revision push (human), a second
revision push, an idle push and `note show`. Timestamps and the stub's ephemeral port are masked;
note ids are identical because each binary gets its own stub.

| Capture | exit (old → new) | bytes (old → new) | verdict |
|---|---|---|---|
| first push, `--json` | 0 → 0 | 929 → 929 | identical |
| revision push, human | 0 → 0 | 403 → 403 | identical |
| second revision push, `--json` | 0 → 0 | 939 → 939 | identical |
| idle push, `--json` | 0 → 0 | 774 → 774 | identical |
| `note show --json` | 0 → 0 | 3503 → 3537 | only the recorded `"deck": "Pass Quest"` |

Exactly two documents changed, both intended: `note show` gained the recorded deck (contract:
"visible in `note show`"), and the wire gained reads (no write changed):

```text
old: version,createDeck,modelNames,addNote,version,createDeck,modelNames,notesInfo,updateNoteFields,version,createDeck,modelNames,notesInfo,updateNoteFields
new: version,createDeck,modelNames,addNote,notesInfo,cardsInfo,version,createDeck,modelNames,notesInfo,updateNoteFields,cardsInfo,version,createDeck,modelNames,notesInfo,updateNoteFields,cardsInfo
```

`internal/cli/testdata/note-push.golden.json` and `note-show.golden.json` are untouched in this diff
and still pass byte-for-byte (`go test ./internal/cli -run TestGoldenNote`), which is the frozen-clock
form of the same claim.

## Acceptance 2, 3 — a mis-assigning endpoint is named, typed and non-zero

Same script with the stub filing added cards in `Default` while the payload asks for `Pass Quest`.
The old binary's transcript asserted `pushed/ok`; the new one:

```json
{ "state": "deck_mismatch", "counts": { "pending": 0, "pushed": 0, "deck_mismatch": 1 },
  "notes": [ { "note_id": "note-e7f54e2c40bf7326", "revision": 1, "anki_note_id": 101,
               "state": "deck_mismatch", "outcome": "deck_mismatch", "deck": "Default",
               "detail": "card landed in \"Default\", not \"Pass Quest\"" } ],
  "error": { "code": "anki_deck_mismatch",
             "message": "1 note(s) landed in a deck other than \"Pass Quest\"; their Anki note ids are kept" } }
```

Human summary of the next (revision) push: exit 0 → 3, the summary itself moving from stdout to stderr
(373 → 771 bytes written; 414 → 812 in the compare script's per-command count, which adds one
`name: exit N` line). Re-captured for this record against a stub on the fixed loopback port 47831,
human mode without `--json`, so the transcript and its length repeat exactly:

```text
Pass Quest — Anki push
Endpoint: http://127.0.0.1:47831
Deck: Pass Quest · model: PassQuest Note (created: false)
Pending 0 · pushed 0 · already present 0 · conflicts 0 · failed 0 · queued 0
  note-e7f54e2c40bf7326 rev 2  deck_mismatch/deck_mismatch  anki note 101  card landed in "Default", not "Pass Quest"
Deck mismatch: 1 note(s) are not in "Pass Quest".
  note-e7f54e2c40bf7326 rev 2 landed in "Default" (anki note 101)
Move the card into "Pass Quest" in Anki — the note id is kept, so a later revision updates the same card.
passquest: anki_deck_mismatch: 1 note(s) landed in a deck other than "Pass Quest"; their Anki note ids are kept
AnkiWeb and mobile synchronization are your own manual actions in Anki. Pass Quest never contacts AnkiWeb or a phone.
```

The last-but-one line is the house `passquest: <code>: <message>` failure line
[`../notes.md`](../notes.md) states, the sync statement follows it, and 771 bytes is this whole stderr.
The old binary's success summary for the same push was 373 bytes on stdout, with no error line.

`note show --json` for the same note: `"state": "pushed"` → `"deck_mismatch"`, plus
`"deck": "Default"` and `"deck_expected": "Pass Quest"` (2957 → 3034 bytes). The count line no longer
contains `pushed 1`, and the requested deck is never presented as achieved.

## Acceptance 4 — the note id survives, no second card

| Step | Wire actions (new binary, mis-assigned stub) |
|---|---|
| first push | `version,createDeck,modelNames,addNote,notesInfo,cardsInfo` |
| repeat push (no new revision) | *(none — the endpoint was never contacted; 776 bytes, identical to the old binary's idle document)* |
| revision push | `version,createDeck,modelNames,notesInfo,updateNoteFields,cardsInfo` |
| after the learner moves the card | `version,createDeck,modelNames,notesInfo,updateNoteFields,cardsInfo` |

`addNote` appears exactly once across four pushes, every later write is `updateNoteFields`, and the
note keeps `anki_note_id: 101`, `pushed_revision: 2` and a drained outbox row (asserted in
`internal/store/note_test.go` and `internal/cli/note_test.go`, and in the gate). One card.

## Acceptance 5 — the check is a read, not an assumption

- The gate asserts the push sent `cardsInfo` and that the deck it reports is the stub's, while the
  payload asked for `Pass Quest`; the mismatch verdict exists only because the read disagrees with the
  request.
- Falsifiability: `harness/gates/self-test.ts` copies `go.mod`, `go.sum`, `cmd`, `internal` and
  `agent` into the fixture root, replaces the one occurrence of
  `if deck != expected && !containsString(unexpected, deck) {` with `if false && …`, and runs the real
  note gate against those sources. Result: `fail`, 12 failures, the first being
  `mismatch push exit = 0` (a push that reads the deck and ignores it claims success). The self-test
  requires a failure that names the mismatch and rejects a run that broke an earlier stage.
- A conforming note is never flagged: the update push in the gate asserts the item carries no `deck`
  field and `counts.deck_mismatch` is absent; `readNoteDecks` is additionally a table test over
  matching decks, one and two foreign decks, a missing card row, a missing deck name, an unprintable
  deck name, a 201-character deck name, a 200-character multibyte deck name, a deck list that exceeds
  one recorded name, and a note with no cards.

## Acceptance 6 — nothing else moves

The gate runs `report --json`, `next --json` and `questions SRC --json` (the stored answer key) before
the mismatched push and after it, and requires byte-identical stdout. The Go mismatch test repeats the
`report`/`next` comparison with a frozen clock. `harness/gates/note-smoke.ts` also keeps its existing
attempt, revision, export, model, conflict, duplicate, queue and consent scenarios unchanged.

## Gate results

Command: `bun run gate` (the single gate definition; `bash scripts/gate.sh` execs it). The run below is
the local run on the tree that carries every code, gate, golden, doc and archive change of this
increment: the accepted simplify pass applied on the same day (the rune-count deck bound, the bounded
multi-deck aggregate, the card-id clear at a scheduled readd with its regression case, the `askNotes`
rename and the comment trims) plus the fix-stage response to the review findings F-01 and F-02
(`AnkiAttempt.SetDeck` with the store and CLI cases that pin both directions, and the re-captured human
mismatch transcript above). Two counted numbers moved: `go test` gained
`TestNotePushReaddsANoteDeletedAfterTheReadBack` in the simplify pass (783 → 784) and
`TestDeckReadWithoutADeckNameClearsTheRecordedDeck` plus `TestNotePushRecordsNoDeckWhenTheReadCannotNameIt`
here (784 → 786), and `docs links` went 177 → 178 checked because the re-captured transcript's
paragraph links `../notes.md`. No other counted number moved: `note smoke` is still 1140 assertions and
the two markdown-reading gates exit `0` (`docs links (178 checked, 0 broken)`,
`secrets/sources (434 files, 0 findings)`).

```text
$ bun run gate
== typecheck
PASS  typecheck (0 type errors)

== gofmt
PASS  gofmt (100 files)

== go vet
PASS  go vet

== go build
PASS  go build

== go test
PASS  go test (18 packages, 786 tests, 0 skipped)

== go mod tidy
PASS  go mod tidy

== docs links
PASS  docs links (178 checked, 0 broken)

== secrets/sources
PASS  secrets/sources (434 files, 0 findings)

== local-only CLI
PASS  local-only CLI (17 first-party packages, 0 network imports, 0 confidential markers)

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
PASS  note smoke (1140 assertions)

== find smoke
PASS  find smoke (113 assertions)

== assess smoke
PASS  assess smoke (275 assertions)

== portable paths
PASS  portable paths (100 Go files, no platform-bound path literal)

== provider smoke
PASS  provider smoke (92 assertions)

== golden contract
PASS  golden contract (2 golden(s) declared by 4 contract document(s) against 1bdbc7e397fa)

== harness self-test
PASS  harness self-test (35 cases (1 must pass), 21/21 gates)

22 passed, 0 failed, 0 skipped
GATE: PASS
EXIT 0
```

`internal/cli/testdata/note-push.golden.json` and `note-show.golden.json` are unchanged in this diff
and still pass byte-for-byte, which is why the golden-contract gate needed a contract document for the
two goldens this increment adds (`note-push-deck-mismatch.golden.json`,
`note-show-deck-mismatch.golden.json`) rather than for any rewrite of an existing one.

An earlier identical run completed green before the last `agent/README.md` line was added; the counts
above are from the run on the final tree, which is the one reported.

## Limits and residual risks

- A real Anki 26.x with the current AnkiConnect still files the card in the default deck; after this
  increment the learner is told so by name and moves it once. Whether a released AnkiConnect honours
  `deckName` is not settled here and is not claimed.
- The recorded deck is the last one Anki reported at a write; a manual move is not re-read until the
  next revision push, and an idle push still contacts nothing (Q-0014-03).
- An endpoint that omits `notesInfo.cards` or `cardsInfo.deckName` is reported as `deck_mismatch`
  (unconfirmed) rather than trusted (Q-0014-02). The documented AnkiConnect v6 API answers both. The
  same unconfirmed outcome covers a single deck name over 200 characters and a note whose several
  reported decks together exceed that: the mismatch is typed, and the row records no deck — a name an
  earlier read recorded is cleared, so no stale deck is shown beside the state that contradicts it.
- A deck read that fails after an acknowledged write exits 3 with the write recorded and no deck
  claim; the next push does not re-check that note until it writes to it again.
- The gate's stub models one card per note (the PassQuest note type has one template). Several cards in
  several decks are covered at the `readNoteDecks` level, not against a live endpoint.
- Windows and macOS run the same gate modules in CI; the deck check itself is platform-independent and
  has no owner-machine evidence beyond the finding recorded in `Q-0014-01`.
